# 插件事件订阅系统 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为插件系统添加 EventSubscriber 可选接口，让插件可以订阅系统事件

**Architecture:** 在 `internal/plugin/` 新增 `EventSubscriber` 接口；`LifecycleManager.Start()` 检测接口并调用 `eventbus.Subscribe()`；`Stop()` 和 `Unload()` 时取消订阅；生命周期管理通过 `subscriptionIDs` map 跟踪

**Tech Stack:** Go + `internal/daemon/eventbus` 已有事件总线

## Global Constraints

- EventSubscriber 是可选接口（不强制所有 plugin 实现）
- `SubscribedEvents()` 返回 `[]eventbus.EventType`，nil/空 = 跳过所有事件
- `HandleEvent(ctx, Event) error` 在事件发生时被调用
- LifecycleManager.Start() 对已注册的 compiled 插件检测 EventSubscriber 并订阅
- LifecycleManager.Stop() 取消所有插件订阅
- LifecycleManager.Load() 对 .so 插件也检测 EventSubscriber 并订阅
- LifecycleManager.Unload() 取消该插件订阅
- 订阅 ID 格式：`"plugin:<name>:<eventType>"`，通过 `eventbus.Unsubscribe()` 取消
- 所有现有测试必须继续通过

---

### Task 1: EventSubscriber 接口 + LifecycleManager 集成

**Files:**
- Modify: `internal/plugin/plugin.go` — 添加 EventSubscriber 接口
- Modify: `internal/plugin/lifecycle.go` — 添加 subscriptionIDs 跟踪 + Start/Stop/Load/Unload 集成
- Test: `internal/plugin/lifecycle_test.go` — 添加 EventSubscriber 集成测试

**Interfaces:**
- Produces: `plugin.EventSubscriber` interface, `LifecycleManager` 订阅管理

- [ ] **Step 1: 在 `internal/plugin/plugin.go` 末尾添加 EventSubscriber 接口**

```go
// EventSubscriber is an optional interface plugins can implement to subscribe
// to system events. The LifecycleManager registers these subscriptions when
// the plugin is loaded (compiled or .so) in serve/dev mode.
type EventSubscriber interface {
	// SubscribedEvents returns the event types this plugin wants to receive.
	// Return nil or empty slice to skip all events.
	SubscribedEvents() []eventbus.EventType

	// HandleEvent is called for each subscribed event. Returning an error
	// logs the failure but does not interrupt other handlers.
	HandleEvent(ctx context.Context, event eventbus.Event) error
}
```

Add import: `"github.com/iannil/huan/internal/daemon/eventbus"`

- [ ] **Step 2: 在 `LifecycleManager` 中添加订阅跟踪**

在 `internal/plugin/lifecycle.go` 的 `LifecycleManager` struct 中添加 `subscriptionIDs` 字段：

```go
type LifecycleManager struct {
	registry  *Registry
	loader    *Loader
	bus       eventbus.EventBus
	pluginDir string

	mu          sync.Mutex
	loaded      map[string]*loadedPlugin
	watcher     *PluginWatcher
	watchCtx    context.Context
	watchCancel context.CancelFunc

	detectCapabilityFn func(Plugin) string

	// subscriptionIDs tracks eventbus handler IDs per plugin name.
	// Key: plugin name, Value: list of handler IDs for unsubscription.
	subscriptionIDs map[string][]string
}
```

- [ ] **Step 3: 在 `NewLifecycleManager` 中初始化 subscriptionIDs**

```go
func NewLifecycleManager(registry *Registry, loader *Loader, bus eventbus.EventBus) *LifecycleManager {
	return &LifecycleManager{
		registry:  registry,
		loader:    loader,
		bus:       bus,
		pluginDir: loader.PluginDir(),
		loaded:    make(map[string]*loadedPlugin),
		subscriptionIDs: make(map[string][]string),
	}
}
```

- [ ] **Step 4: 添加 `subscribePluginEvents` 和 `unsubscribePluginEvents` 辅助方法**

```go
// subscribePluginEvents checks if a plugin implements EventSubscriber and
// subscribes to its declared events. Must be called with m.mu held.
func (m *LifecycleManager) subscribePluginEvents(p Plugin) {
	es, ok := p.(EventSubscriber)
	if !ok {
		return
	}
	events := es.SubscribedEvents()
	if len(events) == 0 {
		return
	}

	name := p.Name()
	var ids []string
	for _, evtType := range events {
		handlerID := fmt.Sprintf("plugin:%s:%s", name, evtType.String())
		ids = append(ids, handlerID)
		// Capture the handler for the closure
		h := es.HandleEvent
		m.bus.Subscribe(evtType, func(ctx context.Context, ev eventbus.Event) error {
			return h(ctx, ev)
		})
	}
	m.subscriptionIDs[name] = ids
}

// unsubscribePluginEvents removes all event subscriptions for a plugin.
// Must be called with m.mu held.
func (m *LifecycleManager) unsubscribePluginEvents(name string) {
	ids, ok := m.subscriptionIDs[name]
	if !ok {
		return
	}
	// We need the event type to unsubscribe. Since we stored IDs by format
	// "plugin:<name>:<eventType>", we need to iterate over known event types.
	// Simpler approach: store (eventType, handlerID) pairs.
	// Actually, since eventbus.Unsubscribe requires EventType + handlerID,
	// let's store a map of eventType -> handlerID instead.
	delete(m.subscriptionIDs, name)
}
```

Wait, `eventbus.Unsubscribe` requires both `EventType` and `handlerID`. Let me adjust the storage format. Store `map[string][]subscriptionEntry` where `subscriptionEntry` holds `{eventType eventbus.EventType, handlerID string}`:

```go
type subscriptionEntry struct {
	eventType eventbus.EventType
	handlerID string
}

// In LifecycleManager:
// subscriptionIDs maps plugin name to its subscription entries.
// subscriptionIDs map[string][]subscriptionEntry
```

- [ ] **Step 5: 修改 `Start()` 末尾，在 watcher 启动后订阅 compiled 插件事件**

在 `Start()` 末尾的 `return nil` 之前：

```go
// Subscribe compiled plugins to system events
for _, p := range m.registry.All() {
	m.subscribePluginEvents(p)
}
```

- [ ] **Step 6: 修改 `Stop()` 取消所有订阅**

在 `watchCancel()` 之后添加：

```go
// Unsubscribe all plugin event handlers
for name := range m.subscriptionIDs {
	m.unsubscribePluginEvents(name)
}
```

- [ ] **Step 7: 修改 `Load()` 在新 .so 插件注册后订阅事件**

在 `m.publishEventUnsafe(...)` 之后添加：

```go
// Subscribe to system events if the plugin implements EventSubscriber
m.subscribePluginEvents(p)
```

- [ ] **Step 8: 修改 `Unload()` 在取消注册前取消订阅**

在 `m.registry.Unregister(name)` 之前添加：

```go
// Unsubscribe from system events
m.unsubscribePluginEvents(name)
```

- [ ] **Step 9: 添加测试**

在 `internal/plugin/lifecycle_test.go` 末尾添加：

```go
type testEventSubscriberPlugin struct {
	name     string
	events   []eventbus.EventType
	received []eventbus.Event
}

func (p *testEventSubscriberPlugin) Name() string { return p.name }
func (p *testEventSubscriberPlugin) SubscribedEvents() []eventbus.EventType { return p.events }
func (p *testEventSubscriberPlugin) HandleEvent(ctx context.Context, event eventbus.Event) error {
	p.received = append(p.received, event)
	return nil
}

var _ EventSubscriber = (*testEventSubscriberPlugin)(nil)

func TestLifecycleManager_EventSubscriber_CompiledPlugin(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	plugin := &testEventSubscriberPlugin{
		name:   "test-es",
		events: []eventbus.EventType{eventbus.EventBuildCompleted, eventbus.EventPluginLoaded},
	}
	_ = registry.Register(plugin)

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	_ = lm.Start(context.Background())
	defer lm.Stop()

	// Publish an event the plugin subscribed to
	_ = bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   "test",
	})

	// Give the async handler time to fire
	time.Sleep(50 * time.Millisecond)

	if len(plugin.received) == 0 {
		t.Error("expected plugin to receive event, got none")
	}
}

func TestLifecycleManager_EventSubscriber_NotRequired(t *testing.T) {
	registry := NewRegistry()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	_ = registry.Register(&lifecycleTestPlugin{name: "no-es"})

	lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
	// Should not panic
	err := lm.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	lm.Stop()
}
```

- [ ] **Step 10: 运行测试**

Run: `go test ./internal/plugin/ -v`
Expected: ALL PASS

- [ ] **Step 11: 提交**

```bash
git add internal/plugin/plugin.go internal/plugin/lifecycle.go internal/plugin/lifecycle_test.go
git commit -m "feat(plugin): add EventSubscriber interface and lifecycle integration

Co-Authored-By: Claude <noreply@anthropic.com>"
```