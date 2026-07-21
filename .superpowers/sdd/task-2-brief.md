### Task 2: EventBus 新增插件生命周期事件

**Files:**
- Modify: `internal/daemon/eventbus/types.go` — 新增 4 个事件类型
- Modify: `internal/daemon/eventbus/bus_test.go` — 测试新事件可发布

**Interfaces:**
- Produces: `EventPluginLoaded`, `EventPluginUnloaded`, `EventPluginReloaded`, `EventPluginError`

- [ ] **Step 1: 在 types.go 中添加新事件常量**

```go
// 在 EventServerShutdown 之后添加
    EventPluginLoaded   EventType = iota + 10 // 插件加载完成
    EventPluginUnloaded                       // 插件卸载完成
    EventPluginReloaded                       // 插件热重载完成
    EventPluginError                          // 插件异常
```

更新 `String()` 方法，在 `return "unknown"` 之前添加：

```go
    case EventPluginLoaded:
        return "plugin_loaded"
    case EventPluginUnloaded:
        return "plugin_unloaded"
    case EventPluginReloaded:
        return "plugin_reloaded"
    case EventPluginError:
        return "plugin_error"
```

- [ ] **Step 2: 运行现有测试确认无回归**

Run: `go test ./internal/daemon/eventbus/ -v`
Expected: ALL PASS

- [ ] **Step 3: 提交**

```bash
git add internal/daemon/eventbus/types.go
git commit -m "feat(eventbus): add plugin lifecycle event types (loaded/unloaded/reloaded/error)"
```

---

