# SSE Real-Time Push Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 daemon 增加 SSE 实时推送（`/api/v1/events`），把 daemon 内部 EventBus 事件实时广播给所有连接的浏览器客户端。

**Architecture:** SSEHub 订阅 daemon EventBus 全量事件，每条事件通过带缓冲通道非阻塞广播给所有 SSE 客户端。HTTP handler 服务 `GET /api/v1/events`（text/event-stream），流式写入 + flush。心跳保活 + 慢客户端丢弃 + 连接数上限。

**Tech Stack:** Go 1.26.2, net/http (SSE), encoding/json, daemon EventBus

## Global Constraints

- 协议：SSE（text/event-stream），浏览器 EventSource 原生兼容
- 端点：GET /api/v1/events（与查询 API 同前缀，公开只读无 token）
- 全量广播（所有客户端收到所有事件，客户端自行过滤）
- 慢客户端保护：每客户端带缓冲通道（容量 16），满则丢弃不阻塞其他
- 心跳：每 15 秒 SSE 注释行（`:heartbeat\n\n`）
- 连接数上限：maxClients=1000，超限 503
- 复用 daemon EventBus（事件源已有，不重新造）
- 所有现有测试必须通过

---

### Task 1: SSEHub 核心 + Broadcast + 心跳

**Files:**
- Create: `internal/daemon/sse/hub.go` — Event, SSEHub, NewSSEHub, Broadcast, broadcastRaw, startHeartbeat, register/unregister, ClientCount
- Create: `internal/daemon/sse/hub_test.go` — 单元测试

**Interfaces:**
- Produces: `Event`, `SSEHub`, `NewSSEHub(logf)`, `Broadcast(Event)`, `broadcastRaw(string)`, `registerClient/unregisterClient`, `ClientCount() int`

- [ ] **Step 1: 编写测试**

`internal/daemon/sse/hub_test.go`：

```go
package sse

import (
	"testing"
	"time"
)

func TestBroadcast_DeliversToClient(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()

	h.Broadcast(Event{Type: "build_completed", Data: map[string]int{"pages": 10}})

	select {
	case ev := <-ch:
		if ev.Type != "build_completed" {
			t.Errorf("Type = %q, want build_completed", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not receive event")
	}
}

func TestBroadcast_MultipleClients(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch1 := h.registerClient()
	ch2 := h.registerClient()

	h.Broadcast(Event{Type: "content_changed", Data: nil})

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("client %d did not receive event", i)
		}
	}
}

func TestBroadcast_SlowClientDrops(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()
	// Fill the buffer (clientBufferSize events).
	for i := 0; i < clientBufferSize; i++ {
		h.Broadcast(Event{Type: "build_completed"})
	}
	// One more should be dropped (non-blocking) — client never reads.
	h.Broadcast(Event{Type: "build_completed"})
	// Drain to verify buffer not exceeded.
	count := 0
	draining := true
	for draining {
		select {
		case <-ch:
			count++
		default:
			draining = false
		}
	}
	if count != clientBufferSize {
		t.Errorf("received %d, want %d (overflow dropped)", count, clientBufferSize)
	}
}

func TestClientCount(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	if h.ClientCount() != 0 {
		t.Fatalf("initial count = %d, want 0", h.ClientCount())
	}
	ch := h.registerClient()
	if h.ClientCount() != 1 {
		t.Errorf("after register = %d, want 1", h.ClientCount())
	}
	h.unregisterClient(ch)
	if h.ClientCount() != 0 {
		t.Errorf("after unregister = %d, want 0", h.ClientCount())
	}
}

func TestBroadcastRaw_HeartbeatComment(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()
	h.broadcastRaw(":heartbeat\n\n")
	select {
	case ev := <-ch:
		// Raw broadcast is delivered as an Event with empty Type and the raw line in Data.
		if ev.Type != "" {
			t.Errorf("raw event Type = %q, want empty", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not receive raw broadcast")
	}
}

func testLogf(t *testing.T) func(string, ...any) {
	t.Helper()
	return func(format string, args ...any) { t.Logf(format, args...) }
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/sse/ -run "TestBroadcast|TestClientCount" -v
```
Expected: COMPILATION ERROR (no hub.go)

- [ ] **Step 3: 实现 hub.go**

`internal/daemon/sse/hub.go`：

```go
// Package sse provides Server-Sent Events (SSE) push for the daemon.
// SSEHub subscribes to daemon EventBus events and broadcasts them to all
// connected browser clients via the /api/v1/events endpoint.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const (
	// clientBufferSize is the per-client event buffer. A slow client that
	// falls behind this many events has new events dropped (non-blocking).
	clientBufferSize = 16
	// maxClients caps concurrent SSE connections to prevent resource exhaustion.
	maxClients = 1000
	// heartbeatInterval is how often a keep-alive comment line is sent.
	heartbeatInterval = 15 * time.Second
)

// Event is a single SSE message sent to clients.
type Event struct {
	// Type is the SSE event name (e.g. "build_completed"). Empty for raw
	// comment lines (heartbeats) — see broadcastRaw.
	Type string
	// Data is the event payload (JSON-marshaled on the wire). For raw
	// broadcasts this holds the literal wire text.
	Data any
	// raw, when true, means Data is already-formatted wire text written
	// verbatim (used by heartbeats).
	raw bool
}

// SSEHub manages SSE client connections and broadcasts daemon events.
// Thread-safe. Slow clients do not block fast clients.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
	logf    func(format string, args ...any)
}

// NewSSEHub creates an empty hub.
func NewSSEHub(logf func(string, ...any)) *SSEHub {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &SSEHub{
		clients: map[chan Event]struct{}{},
		logf:    logf,
	}
}

// registerClient adds a new buffered client channel and returns it.
// The caller must eventually call unregisterClient to avoid leaks.
func (h *SSEHub) registerClient() chan Event {
	ch := make(chan Event, clientBufferSize)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// unregisterClient removes a client channel and drains it.
func (h *SSEHub) unregisterClient(ch chan Event) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// ClientCount returns the number of connected clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends an event to all clients. A client whose buffer is full
// (slow consumer) has the event dropped — it never blocks other clients.
func (h *SSEHub) Broadcast(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
			h.logf("sse: client buffer full, dropping event %q", event.Type)
		}
	}
}

// broadcastRaw sends already-formatted wire text (e.g. a heartbeat comment
// line) to all clients as a raw Event.
func (h *SSEHub) broadcastRaw(line string) {
	h.Broadcast(Event{Data: line, raw: true})
}

// startHeartbeat periodically broadcasts an SSE comment line to keep
// connections alive through proxies. Runs until ctx is cancelled.
func (h *SSEHub) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.broadcastRaw(":heartbeat\n\n")
		}
	}
}

// encodeEvent formats an Event as SSE wire text. Structured events become
// "event: <type>\ndata: <json>\n\n"; raw events are written verbatim.
func encodeEvent(ev Event) (string, error) {
	if ev.raw {
		if s, ok := ev.Data.(string); ok {
			return s, nil
		}
	}
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return "", fmt.Errorf("sse: marshal event data: %w", err)
	}
	if ev.Type == "" {
		return fmt.Sprintf("data: %s\n\n", data), nil
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Type, data), nil
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/sse/ -run "TestBroadcast|TestClientCount" -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/sse/hub.go internal/daemon/sse/hub_test.go
git commit -m "feat(sse): add SSEHub with broadcast and heartbeat"
```

---

### Task 2: SubscribeBus — EventBus 桥接

**Files:**
- Modify: `internal/daemon/sse/hub.go` — 新增 SubscribeBus 方法
- Modify: `internal/daemon/sse/hub_test.go` — 桥接测试

**Interfaces:**
- Consumes: `eventbus.EventBus`, `eventbus.EventType`, `eventbus.Event`（`internal/daemon/eventbus/`）
- Produces: `SSEHub.SubscribeBus(bus eventbus.EventBus)`

- [ ] **Step 1: 编写测试**

在 `internal/daemon/sse/hub_test.go` 末尾添加：

```go
import (
	// ... existing
	"context"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

func TestSubscribeBus_Bridges(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	h.SubscribeBus(bus)

	// Publish a build_completed event.
	_ = bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   map[string]int{"pages": 5},
	})

	select {
	case ev := <-ch:
		if ev.Type != "build_completed" {
			t.Errorf("Type = %q, want build_completed", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive bridged event")
	}
}
```

（合并 import：把新 import 加入文件顶部已有的 import 块。）

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/sse/ -run "TestSubscribeBus" -v
```
Expected: COMPILATION ERROR (no SubscribeBus)

- [ ] **Step 3: 实现 SubscribeBus**

在 `internal/daemon/sse/hub.go` 末尾添加。先在 import 块加入：

```go
	"github.com/iannil/huan/internal/daemon/eventbus"
```

然后在文件末尾添加：

```go
// SubscribeBus wires the hub to the daemon EventBus: every build/content/
// plugin event is forwarded to all SSE clients. Call once at daemon startup.
func (h *SSEHub) SubscribeBus(bus eventbus.EventBus) {
	for _, eventType := range []eventbus.EventType{
		eventbus.EventBuildCompleted,
		eventbus.EventBuildFailed,
		eventbus.EventContentChanged,
		eventbus.EventPluginLoaded,
		eventbus.EventPluginUnloaded,
	} {
		bus.Subscribe(eventType, func(_ context.Context, ev eventbus.Event) error {
			h.Broadcast(Event{Type: ev.Type.String(), Data: ev.Payload})
			return nil
		})
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/sse/ -run "TestSubscribeBus" -v
```
Expected: PASS

- [ ] **Step 5: 运行 sse 全部测试**

```bash
go test ./internal/daemon/sse/ -v
```
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/daemon/sse/hub.go internal/daemon/sse/hub_test.go
git commit -m "feat(sse): add SubscribeBus to bridge EventBus events"
```

---

### Task 3: HandleSubscribe — SSE HTTP handler

**Files:**
- Create: `internal/daemon/sse/handler.go` — HandleSubscribe HTTP handler
- Create: `internal/daemon/sse/handler_test.go` — handler 测试

**Interfaces:**
- Consumes: SSEHub（Task 1-2）, encodeEvent
- Produces: `SSEHub.HandleSubscribe(w, r)`, `SSEHub.Start(ctx)`（启动心跳）

**说明：** SSE 流式 handler：设置 headers → 注册客户端 → 循环读通道写响应 + flush → 断开清理。maxClients 检查。心跳在 ServeHTTP 入口启动（或由 daemon 启动）。为简化，提供 Start(ctx) 启动心跳，daemon 调用一次。

- [ ] **Step 1: 编写测试**

`internal/daemon/sse/handler_test.go`：

```go
package sse

import (
	"bufio"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleSubscribe_SSEHeaders(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	// Run handler in background; it blocks until client disconnect.
	done := make(chan struct{})
	go func() {
		h.HandleSubscribe(rec, req)
		close(done)
	}()

	// Give it a moment to set headers, then cancel the request to end the handler.
	time.Sleep(50 * time.Millisecond)
	// httptest.ResponseRecorder doesn't support context cancellation the way
	// a real server does; we simulate disconnect by closing via the hub.
	// Instead, verify headers by reading after a short delay.
	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}

	// Force handler exit by unregistering all clients (simulates shutdown).
	h.mu.Lock()
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
	h.mu.Unlock()
	<-done
}

func TestHandleSubscribe_EventFormat(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	go h.HandleSubscribe(rec, req)

	// Wait for registration, then broadcast.
	time.Sleep(50 * time.Millisecond)
	h.Broadcast(Event{Type: "build_completed", Data: map[string]int{"pages": 3}})

	time.Sleep(50 * time.Millisecond)
	body := rec.Body.String()
	if !strings.Contains(body, "event: build_completed") {
		t.Errorf("body missing event line:\n%s", body)
	}
	if !strings.Contains(body, `"pages":3`) {
		t.Errorf("body missing data:\n%s", body)
	}

	h.mu.Lock()
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
	h.mu.Unlock()
}

func TestHandleSubscribe_MaxClientsRejected(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	// Artificially fill to maxClients.
	for i := 0; i < maxClients; i++ {
		h.registerClient()
	}

	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)

	if rec.Code != 503 {
		t.Errorf("Code = %d, want 503 (max clients)", rec.Code)
	}
}

func TestHandleSubscribe_ReadsStream(t *testing.T) {
	// End-to-end: real httptest.Server, read SSE stream via bufio.Scanner.
	h := NewSSEHub(testLogf(t))
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Start(ctx)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Broadcast an event.
	time.Sleep(50 * time.Millisecond) // let client register
	h.Broadcast(Event{Type: "content_changed", Data: "hello"})

	scanner := bufio.NewScanner(resp.Body)
	got := false
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for SSE event")
		default:
		}
		if !scanner.Scan() {
			break
		}
		if strings.Contains(scanner.Text(), "content_changed") {
			got = true
		}
		if got && scanner.Text() == "" {
			break // end of event block (blank line)
		}
	}
	if !got {
		t.Error("did not see content_changed in stream")
	}
}
```

需要 `net/http` import（http.Get）。在 handler_test.go 顶部 import 块加 `"net/http"`。

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/sse/ -run "TestHandleSubscribe" -v
```
Expected: COMPILATION ERROR (no HandleSubscribe/Start)

- [ ] **Step 3: 实现 handler.go**

`internal/daemon/sse/handler.go`：

```go
package sse

import (
	"context"
	"fmt"
	"net/http"
)

// Start launches the heartbeat goroutine. Call once at daemon startup
// (or per request — it's safe but redundant). Cancelling ctx stops it.
func (h *SSEHub) Start(ctx context.Context) {
	go h.startHeartbeat(ctx)
}

// HandleSubscribe is the HTTP handler for GET /api/v1/events.
// It registers the client, streams events until disconnect, then cleans up.
func (h *SSEHub) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.ClientCount() >= maxClients {
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := h.registerClient()
	defer h.unregisterClient(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			return
		case ev, ok := <-ch:
			if !ok {
				// Channel closed (e.g. shutdown).
				return
			}
			line, err := encodeEvent(ev)
			if err != nil {
				h.logf("sse: encode error: %v", err)
				continue
			}
			if _, err := fmt.Fprint(w, line); err != nil {
				// Write failed (client gone).
				return
			}
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/sse/ -v
```
Expected: ALL PASS

Note: 测试中模拟断开用 close(ch) + unregister。如果 TestHandleSubscribe_SSEHeaders/EventFormat 因时序 flaky，调整 sleep 或用 channel 同步。若 httptest.ResponseRecorder 不支持 Flusher，TestHandleSubscribe_ReadsStream 用真实 httptest.NewServer（支持 Flusher）验证流。

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/sse/handler.go internal/daemon/sse/handler_test.go
git commit -m "feat(sse): add HandleSubscribe handler with stream + heartbeat start"
```

---

### Task 4: daemon 集成（serving + daemon.go）

**Files:**
- Modify: `internal/daemon/serving.go` — ServingOptions 新增 SSEHub，注册 /api/v1/events
- Modify: `internal/daemon/daemon.go` — 创建 SSEHub + SubscribeBus + Start + 注入

**Interfaces:**
- Consumes: SSEHub, SubscribeBus, HandleSubscribe, Start（Task 1-3）

- [ ] **Step 1: ServingOptions 新增 SSEHub 字段**

在 `internal/daemon/serving.go` 的 `ServingOptions` 结构体添加：

```go
	// SSEHub, if non-nil, enables the /api/v1/events real-time push endpoint.
	SSEHub *sse.SSEHub
```

并在 serving.go 顶部 import 块加入：

```go
	"github.com/iannil/huan/internal/daemon/sse"
```

- [ ] **Step 2: serving.go 注册 /api/v1/events**

在 `Start()` 中，ContentAPI handler 注册之后，添加：

```go
	// SSE real-time push — /api/v1/events
	if s.opts.SSEHub != nil {
		mux.HandleFunc("/api/v1/events", s.opts.SSEHub.HandleSubscribe)
	}
```

- [ ] **Step 3: daemon.go 创建 SSEHub + 订阅 + 启动心跳**

在 `internal/daemon/daemon.go` 的 `Run()` 中，创建 Serving 之前（EventBus d.bus 已就绪），添加：

```go
	// Init SSEHub (real-time push via /api/v1/events)
	sseHub := sse.NewSSEHub(log.Printf)
	sseHub.SubscribeBus(d.bus)
	sseWatchCtx, sseWatchCancel := context.WithCancel(context.Background())
	defer sseWatchCancel()
	sseHub.Start(sseWatchCtx)
	log.Println("daemon: SSE push enabled (/api/v1/events)")
```

在 Serving 创建时注入：

```go
	ServingOptions{
		// ... 原有字段
		SSEHub: sseHub,
	}
```

在 daemon.go 顶部 import 块加入：

```go
	"github.com/iannil/huan/internal/daemon/sse"
```

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行 daemon 测试确保无回归**

```bash
go test ./internal/daemon/... -v
```
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/daemon/serving.go internal/daemon/daemon.go
git commit -m "feat(daemon): wire SSEHub and /api/v1/events into daemon"
```

---

### Task 5: 集成测试 + 全量验证

**Files:**
- Modify: `internal/daemon/daemon_test.go` — SSE 集成测试
- Modify: `docs/superpowers/specs/2026-07-22-sse-push-design.md` — 标记实现状态

- [ ] **Step 1: 编写集成测试**

在 `internal/daemon/daemon_test.go` 末尾添加：

```go
// TestDaemon_SSE_BuildEventPushed verifies a build triggers a build_completed
// SSE event broadcast to a connected client.
func TestDaemon_SSE_BuildEventPushed(t *testing.T) {
	tmpDir := setupContentAPISite(t) // reuse the site fixture (needs content + templates)
	cache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	contentIdx := contentindex.NewContentIndex("https://example.com/")

	// SSEHub subscribed to the bus.
	sseHub := sse.NewSSEHub(t.Logf)
	sseHub.SubscribeBus(bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseHub.Start(ctx)
	ch := sseHub.registerClient() // simulate a client

	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dag.NewDependencyGraph(),
		JITCache:      cache_pkg.NewJITCache(100, 5*time.Minute),
		Logf:          t.Logf,
		PipelineCache: cache,
		ContentIndex:  contentIdx,
	})

	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Type != "build_completed" {
			t.Errorf("event Type = %q, want build_completed", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive build_completed SSE event")
	}
}

// TestDaemon_SSE_ContentChangedPushed verifies content change events broadcast.
func TestDaemon_SSE_ContentChangedPushed(t *testing.T) {
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	sseHub := sse.NewSSEHub(t.Logf)
	sseHub.SubscribeBus(bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseHub.Start(ctx)
	ch := sseHub.registerClient()

	// Publish a content_changed event directly.
	_ = bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventContentChanged,
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"changed_files": []string{"content/posts/x.md"}},
	})

	select {
	case ev := <-ch:
		if ev.Type != "content_changed" {
			t.Errorf("Type = %q, want content_changed", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive content_changed SSE event")
	}
}
```

注意 import：daemon_test.go 需 import `github.com/iannil/huan/internal/daemon/sse`。`cache_pkg` 若 daemon_test.go 已用 `cache` 别名则改为 `cache.NewJITCache`（与之前 ContentAPI 测试一致，用已有的 cache import，不要 cache_pkg）。

- [ ] **Step 2: 运行集成测试**

```bash
go test ./internal/daemon/ -run "TestDaemon_SSE" -v
```
Expected: ALL PASS

- [ ] **Step 3: 全量编译 + vet**

```bash
go build ./... && go vet ./...
```
Expected: SUCCESS

- [ ] **Step 4: 全量测试**

```bash
go test ./... -count=1
```
Expected: ALL PASS

- [ ] **Step 5: 更新设计文档状态**

修改 `docs/superpowers/specs/2026-07-22-sse-push-design.md`：`- **状态**：Draft` → `- **状态**：Implemented`

- [ ] **Step 6: 最终提交**

```bash
git add -A
git commit -m "feat: implement SSE real-time push (/api/v1/events)

- SSEHub broadcasts daemon EventBus events to all connected clients
- GET /api/v1/events (text/event-stream, public read-only)
- Full broadcast model, slow-client drop, 15s heartbeat, maxClients cap
- SubscribeBus bridges build/content/plugin events
- daemon wires SSEHub + heartbeat at startup

Co-Authored-By: Claude <noreply@anthropic.com>"
```