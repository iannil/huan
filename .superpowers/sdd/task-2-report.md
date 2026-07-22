# Task 2 Report — SubscribeBus（EventBus 桥接）

**状态：** ✅ 完成
**日期：** 2026-07-22
**提交：** `e3d2988` — `feat(sse): add SubscribeBus to bridge EventBus events`

## 做了什么

为 `internal/daemon/sse/hub.go` 中的 `SSEHub` 新增 `SubscribeBus(bus eventbus.EventBus)` 方法，将
daemon EventBus 上的关键事件转发给所有 SSE 客户端。订阅的事件类型：

- `EventBuildCompleted`
- `EventBuildFailed`
- `EventContentChanged`
- `EventPluginLoaded`
- `EventPluginUnloaded`

每个 handler 调用 `h.Broadcast(Event{Type: ev.Type.String(), Data: ev.Payload})`，把
`EventType.String()` 作为 SSE event 名称、`Payload` 作为 data 广播。

## 修改文件

- `internal/daemon/sse/hub.go` — 新增 import (`github.com/iannil/huan/internal/daemon/eventbus`) 与 `SubscribeBus` 方法（+19 行）。
- `internal/daemon/sse/hub_test.go` — 新增 import + `TestSubscribeBus_Bridges` 测试（+27 行）。

## TDD 流程

1. **RED**：先写 `TestSubscribeBus_Bridges`，运行：
   ```
   go test ./internal/daemon/sse/ -run "TestSubscribeBus" -v
   ```
   预期编译失败（`h.SubscribeBus undefined`），实际失败确认：
   ```
   internal/daemon/sse/hub_test.go:175:4: h.SubscribeBus undefined (type *SSEHub has no field or method SubscribeBus)
   FAIL	github.com/iannil/huan/internal/daemon/sse [build failed]
   ```

2. **GREEN**：实现 `SubscribeBus`，再跑同一个命令：
   ```
   --- PASS: TestSubscribeBus_Bridges (0.00s)
   PASS
   ok  	github.com/iannil/huan/internal/daemon/sse	0.487s
   ```

3. **全量回归**：
   ```
   go test ./internal/daemon/sse/ -v
   ```
   全部 11 个测试 PASS（含 `TestStartHeartbeat_Broadcasts` 耗时 15s 属正常）。

4. `go vet ./internal/daemon/sse/ ./internal/daemon/eventbus/` 与 `go build ./...` 均无输出（通过）。

## 提交

```
git add internal/daemon/sse/hub.go internal/daemon/sse/hub_test.go
git commit -m "feat(sse): add SubscribeBus to bridge EventBus events"
```

只提交了实现与测试两个文件；其它 SDD 文档编辑（`task-1-brief.md` 等）未纳入本提交，留给设计文档侧单独管理。

## 测试总结

| 测试 | 结果 | 耗时 |
|------|------|------|
| TestSubscribeBus_Bridges | PASS | 0.00s |
| TestBroadcast_DeliversToClient | PASS | 0.00s |
| TestBroadcast_MultipleClients | PASS | 0.00s |
| TestBroadcast_SlowClientDrops | PASS | 0.00s |
| TestClientCount | PASS | 0.00s |
| TestBroadcastRaw_HeartbeatComment | PASS | 0.00s |
| TestEncodeEvent_StructuredWith_Type | PASS | 0.00s |
| TestEncodeEvent_StructuredWithout_Type | PASS | 0.00s |
| TestEncodeEvent_RawVerbatim | PASS | 0.00s |
| TestStartHeartbeat_Broadcasts | PASS | 15.00s |
| TestNewSSEHub_NilLogf_NoPanic | PASS | 0.00s |

全部 PASS，无回归。

## 关键设计点

- **handler 签名**：`func(_ context.Context, ev eventbus.Event) error` 返回 `nil`，符合
  `eventbus.Handler` 契约；ChannelBus 在每个 handler 的 30s 超时内异步 goroutine 调用，
  因此 `Broadcast` 是非阻塞的，handler 立即返回。
- **闭包捕获**：在 `range` 中使用局部 `eventType`，handler 闭包只捕获 `h`，事件类型来自
  `ev.Type`，避免经典循环变量捕获问题。
- **一次性调用**：方法注释明确"Call once at daemon startup"，由调用方（后续 Task）负责时机，
  本方法不做幂等保护——重复调用会重复订阅导致事件多播。

## 关注点 / 潜在后续

1. **未做去重订阅**：若上层重复调用 `SubscribeBus`，同一事件会广播多次。后续若有 daemon
   重连/重启路径需要考虑单次保证（例如保存订阅 ID 列表 + Unsubscribe）。
2. **未覆盖 EventType 集合**：`EventBuildStarted`、`EventCacheUpdated`、`EventServerStart/Shutdown`、
   `EventPluginReloaded/Error` 未订阅。这与 brief 一致（只列了 5 个），但若客户端也需要这些信号，
   需要后续扩展。
3. **测试覆盖范围**：`TestSubscribeBus_Bridges` 只验证了 `EventBuildCompleted` 路径。其它 4 个事件
   类型走相同代码路径（同一个闭包模板），逻辑等价；如需更高保证，可加一个表驱动测试覆盖全部 5 种。
   当前 brief 只要求一个测试，按 brief 执行。
4. **未与真实 HTTP/SSE endpoint 串起来**：本任务只验证"桥接到 Broadcast"，端到端（HTTP handler →
   encodeEvent → wire）属于后续 Task 范围。
5. 无阻塞问题，可进入 Task 3。
