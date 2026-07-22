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

