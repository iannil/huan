# 插件事件订阅系统设计

- **日期**：2026-07-25
- **状态**：Draft
- **关联**：[ADR 0003](../adr/0003-unified-plugin-system.md)、`internal/daemon/eventbus/`、`internal/plugin/lifecycle.go`

## 背景

当前事件总线（eventbus）已有 10 种事件类型和 3 个订阅者（Builder、Serving、SSE Hub），但没有任何插件能订阅这些事件。插件在 `huan serve/dev` 模式下运行时，对内容变更、构建完成、缓存更新等系统事件一无所知。

需要通过一个可选接口，让插件可以声明自己感兴趣的事件类型，并在事件发生时被回调。

## 设计

### 1. EventSubscriber 可选接口

在 `internal/plugin/` 中新增：

```go
// EventSubscriber is an optional interface plugins can implement to subscribe
// to system events. The LifecycleManager registers these subscriptions when
// the plugin is loaded in serve/dev mode.
type EventSubscriber interface {
    // SubscribedEvents returns the event types this plugin wants to receive.
    // Return nil or empty slice to skip all events.
    SubscribedEvents() []eventbus.EventType

    // HandleEvent is called for each subscribed event. Returning an error
    // logs the failure but does not interrupt other handlers.
    HandleEvent(ctx context.Context, event eventbus.Event) error
}
```

### 2. 集成到 LifecycleManager

在 `LifecycleManager.Start()` 末尾，对已注册的插件检测 `EventSubscriber` 接口，自动订阅事件：

```go
// Register event subscribers for compiled plugins
for _, p := range m.registry.All() {
    if es, ok := p.(EventSubscriber); ok {
        events := es.SubscribedEvents()
        for _, evtType := range events {
            handlerID := es.Name() + ":" + evtType.String()
            m.bus.Subscribe(evtType, func(ctx context.Context, ev eventbus.Event) error {
                return es.HandleEvent(ctx, ev)
            })
            // Store handler ID for clean unsubscription on Stop()
        }
    }
}
```

### 3. 生命周期管理

- `SubscribeEvents()` 在 `LifecycleManager.Start()` 中调用
- `UnsubscribeEvents()` 在 `LifecycleManager.Stop()` 和 `Unload()` 中调用
- 新增 `.so` 插件在 `Load()` 时也触发订阅

### 4. 示例使用场景

插件通过实现 `EventSubscriber` 可以监听：
- `EventBuildCompleted` → 构建完成后自动触发 webhook 或部署
- `EventContentChanged` → 内容变更时触发自定义逻辑
- `EventPluginLoaded` → 插件加载后初始化依赖服务

### 5. 不在此范围

- 插件发布自定义事件（事件源是系统，插件只是消费者）
- 事件过滤/条件订阅（插件收到所有订阅事件，自己在回调中过滤）
- 事件持久化/重放

## 数据流

```
插件实现 EventSubscriber { SubscribedEvents, HandleEvent }
  → LifecycleManager.Start() 检测接口
    → 为每个订阅事件类型调用 bus.Subscribe()
      → 系统事件发生时，eventbus 调用 HandleEvent
        → 插件处理事件，返回 nil 或 error
```