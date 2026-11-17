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
