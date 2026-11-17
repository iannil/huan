# huan desktop (MyGo) Phase 1 — 进度日志

日期：2026-11-17（时间均为 UTC）

- Spec：[docs/superpowers/specs/2026-11-17-huan-desktop-mygo-design.md](../superpowers/specs/2026-11-17-huan-desktop-mygo-design.md)
- 计划：[docs/superpowers/plans/2026-11-17-huan-desktop-mygo.md](../superpowers/plans/2026-11-17-huan-desktop-mygo.md)

## 14:14 Task 1 完成：MyGo spike 通过（窗口 + LoadURL 验证）

### 完成内容

- 环境准备：`web/admin` npm ci + build 产出 `internal/admin/dist/index.html`（embed 硬依赖）；`go get github.com/egoist/mygo@v0.9.2` 并 `go mod tidy`（go directive 随之 1.26.2 → 1.27.3，x/net、x/sys、x/text 顺带升级）；mygo CLI 0.9.2 已在 PATH。
- 新增 `mygo.json`（slug `huan`、identifier `io.github.iannil.huan.desktop`、main `cmd/huan-desktop`、out `build/desktop`）。
- 新增 spike 版 `internal/desktop/app.go`：`Run() error`，WhenReady → NewWindow(1280x800, "huan") → Page().LoadURL("https://example.com") → App.Run() 阻塞。
- 新增入口 `cmd/huan-desktop/main.go`；占位 `internal/desktop/assets/.gitkeep`。

### MyGo API 核对结果（go doc，v0.9.2 实际签名）

- `func (a *Application) WhenReady(fn func())` — 与 brief 一致。
- `func NewWindow(opts WindowOptions) *Window` — **偏差**：只返回 `*Window`，无 error。brief 中 `win, err := mygo.NewWindow(...)` 据此改为 `win := mygo.NewWindow(...)`，删除对应的错误分支（`WindowOptions.Title/Width/Height` 字段与 brief 一致）。
- `func (w *Window) Page() *Page`、`func (p *Page) LoadURL(rawURL string) error` — 与 brief 一致。
- `func (a *Application) Run() error`、`func (d DialogModule) Error(title, content string)` — 与 brief 一致。
- brief 中未使用的 `ready := make(chan struct{})` / `defer close(ready)`（无人等待）按"语义不变"原则移除；`NewWindow` 自身在未 ready 时会等待 ready，语义不受影响。

### 验证

- `go build ./cmd/huan-desktop`、`CGO_ENABLED=0 go build ./cmd/huan-desktop`、`go vet ./internal/desktop/ ./cmd/huan-desktop/` 全部通过；`go build ./... && go vet ./...` 全模块通过。
- Spike 自动化代理验证（无法点击 GUI）：后台启动 `./huan-desktop`，6 秒后进程仍存活（pid 56072），stderr 无任何输出，SIGTERM 干净终止。进程存活且无错误输出 = 窗口生命周期通路正常。**视觉确认（1280x800 窗口显示 example.com）留待人工验收。**

### 提交

- `feat(desktop): scaffold MyGo desktop shell with webview spike`（含 mygo.json、cmd/huan-desktop、internal/desktop、进度文档、go.mod/go.sum）

## 14:53 Task 5 完成：进程内 serve 运行时（internal/desktop/server.go）

### 完成内容

- 新增 `internal/desktop/server.go`：`ServeRuntime` 类型封装进程内 huan dev 服务器——config/plugin/theme 装配 → 初次构建（BuildSite/BuildMultiSite + image pipeline）→ watcher + RebuildQueue 增量重建（Markdown 缓存按变更类型失效、BuildDirSwapper 原子换目录）→ dev.Server（站点 + /admin SPA + LiveReload）绑定 127.0.0.1 随机端口。
- API：`ServeOptions{SourceDir, Logf}`、`NewServeRuntime`、`Start() (adminURL, error)`（阻塞到监听就绪，最多 10s）、`Stop(timeout) error`（幂等、优雅停 HTTP + watcher + queue + 清理 tmpDir）、`Running/AdminURL/SiteURL/SourceDir/Config`、回调 `OnServeExited`（含 panic recover）/`OnRebuildFailed`。
- TDD：先写 `server_test.go`（writeSite 夹具照搬 incremental_test.go 模式，包级函数供 Task 6 复用）→ RED（undefined: NewServeRuntime）→ 实现 → GREEN。

### API 核对结果（go doc / 源码，实际签名 vs brief）

- `observability.Logger.LogFunctionEnd(spanID, duration time.Duration, payload)` — **偏差**：brief 少传 duration；按实际签名补 `time.Since(...)`（Start/Stop 各记录起点）。
- 其余全部一致：`build.Options` 全部字段、`build.NewTimings/NewMarkdownCache/BuildSite/BuildMultiSite/BuildDirSwapper.Swap/Wait`、`dev.New/NewHub/NewWatcher/NewRebuildQueue/MarkdownOnlyChanges/WatcherOptions/ServerOptions`、`admin.ResolveToken/GenerateToken/CheckBindSafety/NewHandler/HandlerOptions`、`content.NewParseCache`、`theme.NewManager/Activate`、`eventbus.NewChannelBus`、`plugin.NewRegistryFromConfig/DirFromSource/NewLoader/NewLifecycleManager`、`image.RunProcessors`、`config.Load/IsMultiLanguage/Theme/PublishDir`。
- 语义级修正（对齐 `huan dev` 参考实现）：multi-language 分支 brief 提前 return 会跳过 image pipeline；改为 BuildSite/BuildMultiSite 之后统一跑 `image.RunProcessors`。
- 竞态修正：Start 末尾 brief 解锁后重读 `r.adminURL`（与并发 Stop 清空竞态）；改为锁内捕获局部变量 `adminURL` 后使用。
- `MarkdownCache.Close` 挂在 Start 的 defer（Start 返回即触发）；核对源码确认内存型缓存（NewMarkdownCache，disk==nil）Close 为幂等 no-op，语义无损。

### 验证

- `go test ./internal/desktop/ -run TestServeRuntime -v`：3 passed（StartServeStop 含站点 GET 含 "Alpha"、admin SPA 200、Stop 后拒连、OnServeExited 2s 内触发 nil；StopWithoutStartIsNoop；StartRejectsInvalidSite）。
- `go test ./internal/desktop/ -v`：12 passed 全包回归；`-race` 3 passed 无告警。
- `gofmt -l internal/desktop/`：仅剩 config_test.go（Task 3 已记录的遗留项，本任务不碰）。
- `go build ./... && go vet ./...`：通过。测试输出除 observability 强制 JSON 结构化日志（stderr）外无噪声。

### 备注

- 本文档此前仅含 Task 1 条目（Task 2–4 代码已提交但未追加进度记录）；本条目按指示仅追加 Task 5，未代补 Task 2–4。
