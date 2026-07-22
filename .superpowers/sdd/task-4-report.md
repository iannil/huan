# Task 4 Report — SSE daemon 集成（serving + daemon.go）

**Date:** 2026-07-22
**Commit:** `056ac41` — `feat(daemon): wire SSEHub and /api/v1/events into daemon`
**Status:** COMPLETE — build green, all daemon + sse tests pass

## Summary

将 Task 1–3 实现的 `sse.SSEHub` 接入 daemon 运行时，启用
`/api/v1/events` 实时推送端点。所有 build/content/plugin 事件通过 EventBus
桥接到 SSEHub，再以 SSE 协议广播给所有已连接的浏览器客户端。

实现严格遵循 `.superpowers/sdd/task-4-brief.md` 的 verbatim 代码块。

## Changes

### `internal/daemon/serving.go`
- 新增 `github.com/iannil/huan/internal/daemon/sse` import。
- `ServingOptions` 新增 `SSEHub *sse.SSEHub` 字段（带文档注释）。
- `Start()` 中，在 `ContentAPI` 注册之后、`/` catch-all 之前注册：

  ```go
  // SSE real-time push — /api/v1/events
  if s.opts.SSEHub != nil {
      mux.HandleFunc("/api/v1/events", s.opts.SSEHub.HandleSubscribe)
  }
  ```

  走 `mux.HandleFunc` 精确路径（按 brief），不会与 `/api/v1/` 前缀冲突
  —— Go ServeMux 在精确路径 vs 前缀匹配时优先选择前者。

### `internal/daemon/daemon.go`
- 新增 `github.com/iannil/huan/internal/daemon/sse` import。
- `Run()` 中创建 admin handler 之后、`NewServing` 之前：

  ```go
  // Init SSEHub (real-time push via /api/v1/events)
  sseHub := sse.NewSSEHub(log.Printf)
  sseHub.SubscribeBus(d.bus)
  sseWatchCtx, sseWatchCancel := context.WithCancel(context.Background())
  defer sseWatchCancel()
  sseHub.Start(sseWatchCtx)
  log.Println("daemon: SSE push enabled (/api/v1/events)")
  ```

- 将 `SSEHub: sseHub` 注入 `ServingOptions`。
- 采用独立的 `sseWatchCtx`（而非 HTTP server 的 ctx）控制心跳生命周期，
  使 SSE 心跳的 cancel 与 HTTP server 的 graceful shutdown 解耦。

### `internal/daemon/daemon_test.go`（TDD 新增集成测试）
- 新增 `sse` import。
- `TestServing_SSEHub_RoutesEvents` —— wiring contract：
  启动 `Serving.Start()` 在 OS-assigned port，`GET /api/v1/events`：
  - 返回 `200`，
  - `Content-Type` 以 `text/event-stream` 开头，
  - `hub.ClientCount() == 1`（handler 完成了 register）。
- `TestServing_SSEHub_NilSkipsRegistration` —— nil-safe contract：
  当 `SSEHub == nil` 时该路由不会派发到 SSE handler（`Content-Type`
  不应是 `text/event-stream`）。
- 测试范式沿用既有的 `TestServing_ContentAPI_RoutesV1`：OS-assigned port
  + 等 `srv.httpSrv` 就绪后再发请求。

## Verification

```
$ go build ./...
（无输出 — BUILD SUCCESS）

$ go test ./internal/daemon/...
ok  github.com/iannil/huan/internal/daemon          0.775s
ok  github.com/iannil/huan/internal/daemon/cache    (cached)
ok  github.com/iannil/huan/internal/daemon/contentindex (cached)
ok  github.com/iannil/huan/internal/daemon/dag      (cached)
ok  github.com/iannil/huan/internal/daemon/eventbus (cached)
ok  github.com/iannil/huan/internal/daemon/sse      (cached)

$ go test ./internal/daemon/ -run 'TestServing_SSEHub' -v
=== RUN   TestServing_SSEHub_RoutesEvents
--- PASS: TestServing_SSEHub_RoutesEvents (0.01s)
=== RUN   TestServing_SSEHub_NilSkipsRegistration
--- PASS: TestServing_SSEHub_NilSkipsRegistration (0.01s)
PASS

$ go test ./...
（全部包通过 — cmd/huan, internal/admin, internal/build, internal/release 等）
```

`gofmt -w` 和 `go vet ./internal/daemon/...` 均无问题。

## 步骤完成情况（对照 brief）

| Brief Step | 状态 | 备注 |
| --- | --- | --- |
| Step 1: ServingOptions 加 SSEHub 字段 + import | ✅ | verbatim 来自 brief |
| Step 2: serving.go 注册 `/api/v1/events` | ✅ | verbatim 来自 brief，放在 ContentAPI 之后 |
| Step 3: daemon.go 创建 SSEHub + SubscribeBus + Start + 注入 | ✅ | verbatim 来自 brief，包括独立 `sseWatchCtx` |
| Step 4: `go build ./...` | ✅ | BUILD SUCCESS |
| Step 5: `go test ./internal/daemon/... -v` | ✅ | ALL PASS |
| Step 6: 提交 | ✅ | `056ac41` |
| TDD：新增集成测试覆盖 wiring contract | ✅（超出 brief） | 两个 Serving 层集成测试 |

## Commits

- `056ac41` — feat(daemon): wire SSEHub and /api/v1/events into daemon
  - Files: `internal/daemon/serving.go`, `internal/daemon/daemon.go`,
    `internal/daemon/daemon_test.go`
  - Stat: 3 files changed, 184 insertions(+), 32 deletions(-)

## 设计决策与说明

- **路由派发方式**：brief 指定 `mux.HandleFunc("/api/v1/events", s.opts.SSEHub.HandleSubscribe)`。
  虽然 Task 3 也提供了 `ServeHTTP` adapter 可走 `mux.Handle("/api/v1/events", sseHub)`，
  两者功能等价 —— 按 brief 采用第一种。
- **心跳生命周期**：用独立的 `sseWatchCtx`，避免与 HTTP server ctx 耦合。
  `defer sseWatchCancel()` 在 `Run()` 返回（shutdown）时触发，心跳 goroutine 退出。
- **gofmt 顺带对齐**：`Options` 与 `BuilderOptions` 结构体字段被 gofmt 重新对齐
  （`PluginRegistry` 字段比其它字段长，原代码对齐错位）。这是 gofmt 的自动副作用，
  纯空白调整、无语义变化。

## 关切事项 / 后续

- **端到端验证**：本任务只验证路由 wiring 与 SSE 协议头。完整「EventBus publish → SSE
  客户端收到事件」链路由 Task 2 (`TestSubscribeBus_Bridges`) 与 Task 3
  (`TestHandleSubscribe_ReadsStream`) 分层覆盖，未在本任务再做端到端测试。
- **shutdown 顺序**：当前 SSE 客户端在 HTTP 连接断开后由 `HandleSubscribe` 中的
  `<-r.Context().Done()` 退出。若后续需要「先拒绝新 SSE 连接、再 drain 老连接」的
  显式语义，可在 `SSEHub` 增加 `Shutdown(ctx)` 方法 —— 本任务范围不涉及。
- **未随本提交带入的改动**：`.superpowers/sdd/task-{1,2,3,4}-{brief,report}.md`
  的既有 CRLF/空白差异与 `docs/superpowers/plans/2026-07-22-sse-push-plan.md`
  均非本任务产物，保持未暂存。

## 文件清单

- `internal/daemon/serving.go` —— SSEHub 字段 + `/api/v1/events` 注册
- `internal/daemon/daemon.go` —— SSEHub 创建 / SubscribeBus / Start / 注入
- `internal/daemon/daemon_test.go` —— 两个集成测试

未触及：`internal/daemon/sse/*`（Task 1–3 已实现且测试全绿）。
