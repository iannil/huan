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

