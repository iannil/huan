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

