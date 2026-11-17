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

## 15:12 Task 5 修复（评审 round 1）：重建后广播 LiveReload reload/alert

- `hub := dev.NewHub()` 上移到重建闭包之前；闭包内对齐 `huan dev` 参考：重建成功 `hub.BroadcastReload()`、失败 `hub.BroadcastAlert("huan rebuild error: ...")`。桌面预览窗口内容编辑后可自动刷新，重建错误可推送浏览器。验证：`go test ./internal/desktop/ -run TestServeRuntime -v` 3 passed、`-race -count=3` 9 passed、全包回归 ok、`go build ./... && go vet ./...` 通过。

## 15:15 Task 6 完成：生产构建运行时（internal/desktop/build.go）

### 完成内容

- 新增 `internal/desktop/build.go`：`BuildSiteTo(sourceDir, outputDir, logf) error` —— 等价 `huan build` 默认路径：`config.Load` → `plugin.NewRegistryFromConfig` → `theme.NewManager`（`cfg.Theme != ""` 时 Activate，失败仅 logf 不阻断）→ `build.Options{Timings: NewTimings(), SourceDir, OutputDir, Logf, PluginRegistry, ThemeManager}` → `IsMultiLanguage` 分支 BuildMultiSite/BuildSite → 收尾 `image.RunProcessors`。无 dev 专用缓存（单次构建无增量收益）。`logf == nil` 时退化为 no-op。
- TDD：先写 `build_test.go`（复用 server_test.go 包级 `writeSite` 夹具，未重定义）→ RED（`undefined: BuildSiteTo`，build.go 不存在的预期编译失败）→ 实现 → GREEN。

### API 核对结果（源码 grep，实际签名 vs brief）

- 全部一致，无需适配：`plugin.NewRegistryFromConfig(cfg *config.Config, sourceDir, pluginDirOverride string)`、`image.RunProcessors(outputDir, sourceDir string, registry *plugin.Registry)`、`theme.NewManager(registry *plugin.Registry)` + `(*Manager).Activate(name string) error`、`config.Load(sourceDir string) (*Config, error)`、`(*Config).IsMultiLanguage()`、`cfg.Theme` 字段、`build.BuildSite(opts Options) (*Result, error)`、`build.BuildMultiSite(opts Options) (*MultiSiteResult, error)`、`build.NewTimings()`、`build.Options.PluginRegistry/ThemeManager` 字段、module path `github.com/iannil/huan`。
- 唯一调整：brief 中 `PluginRegistry: reg,` 的 struct 字面量对齐空格不合 gofmt，按 gofmt 对齐重排（纯空白，语义不变）。

### 验证

- RED：`go test ./internal/desktop/ -run TestBuildSiteTo -v` → FAIL：`build_test.go:12:12: undefined: BuildSiteTo`、`build_test.go:23:12: undefined: BuildSiteTo`。
- GREEN：同命令 2 passed（ProducesOutput 断言磁盘上 `index.html` 与 `posts/alpha/index.html` 真实存在；InvalidSiteFails 断言空目录构建报错）。
- 全包回归：`go test ./internal/desktop/ -v` → 14 passed, 0 failed。
- `gofmt -l internal/desktop/`：新文件无输出（仅 config_test.go 为 Task 3 遗留项，本任务未碰）。
- `go build ./... && go vet ./...`：通过。

### 备注

- 测试输出除 observability JSON 日志外无噪声；未引入任何 MyGo 内容。

## 15:21 Task 7 完成：图标资源 + 托盘/菜单（internal/desktop/menu.go、assets.go、scripts/gen-desktop-assets）

### 完成内容

- 新增 `scripts/gen-desktop-assets/main.go`：一次性占位图标生成器（纯图形、不引字体），产出 18x18 template 托盘图标（黑+alpha 实心圆盘）与 512x512 应用图标（深色方块 + 浅色圆盘）。
- 运行 `go run ./scripts/gen-desktop-assets` 生成 `internal/desktop/assets/tray.png`（141B）与 `internal/desktop/assets/icon.png`（7.4K），入库；`.gitkeep` 保留。
- 新增 `internal/desktop/assets.go`：`//go:embed` 暴露 `trayPNG []byte`、`iconPNG []byte`。
- 新增 `internal/desktop/menu.go`：`Controller` 接口（9 方法：ToggleServe/Rebuild/OpenSiteFolder/OpenSiteInBrowser/SwitchSite/ChooseSite/OpenLogs/ShowWindow/Quit）、`BuildTrayMenu`（固定项 + recents 子菜单或 choose-site 回退 + 退出项）、`BuildAppMenu`（RoleAppMenu + 站点子菜单 + RoleClose/RoleQuit）、`MenuSet{Tray, AppMenu, TrayMenu}`、`BuildTray`（创建 template 托盘图标 + App.SetMenu + OnClick→ShowWindow）、`SetServing`（切换 toggle-serve 标签）、`SetBuilding`（切换 rebuild 可用性）、`RefreshRecents`（重建托盘菜单并 Tray.SetMenu）。

### MyGo API 核对结果（go doc，实际签名 vs brief）

- 全部一致：`NewTray(TrayOptions) (*Tray, error)`、`TrayOptions{ID, Icon, IconIsTemplate, Title, MonospacedTitle, ToolTip, Menu}`、`NewMenu(items []*MenuItem) *Menu`、`MenuItem{ID, Label, Type, Role, Submenu []*MenuItem, Click func(*MenuItem, *Window)}`、`MenuItemSubmenu MenuItemType = "submenu"`、`RoleAppMenu/RoleClose/RoleQuit`、`Separator() *MenuItem`、`(*Menu).ItemByID(id string) *MenuItem`、`(*MenuItem).SetLabel/SetEnabled`、`var App *Application` + `(*Application).SetMenu(*Menu)`、`(*Tray).OnClick(func()) func()`（返回的 off 函数按 brief 丢弃）、`(*Tray).SetMenu(*Menu)`。
- 适配 1（TrayOptions.Icon 字段类型）：brief 写 `icon, err := mygo.NewMenuIcon(trayPNG, true)` 后 `Icon: icon`；实际 `TrayOptions.Icon` 为 `[]byte`（原始 PNG），`*MenuIcon` 仅用于 `MenuItem.Icon`。改为 `Icon: trayPNG`（去掉 BuildTray 中的 NewMenuIcon 调用），语义不变（同一嵌入 PNG 以 template 方式作托盘图标）。
- 注释修正 2 处（纯注释、语义不变）：`MenuSet` 文档由"持动态更新 item 引用"改为"按 ID 查找（见 SetServing/SetBuilding）"以贴合实现；`SetServing` 文档删去代码未实现的"disables conflicting items"从句。
- struct 字面量按 gofmt 对齐重排（纯空白，语义不变）。

### 验证

- `go build ./internal/desktop/ && go vet ./internal/desktop/`：通过。
- `go test ./internal/desktop/ -v`：14 passed, 0 failed（既有测试无回归）。
- `gofmt -l`（三个新文件）：无输出，干净。
- 原生层行为（托盘/菜单真实渲染）按计划在 Task 8 装配后手动验收，本任务无单测（brief Step 4 约定）。

## 15:32 Task 8 完成：绑定 + 应用装配（internal/desktop/bindings.go、app.go 完整版）+ 入口更新

### 完成内容

- 新增 `internal/desktop/bindings.go`：`DesktopBindings{app *App}`（绑定名即类型名）、`AppVersion() string`（`internal/version.String()`，嵌入 VERSION 文件）、`RevealSiteFolder() error`（TrustedOrigins 通路的验证绑定，无站点时报 "no site selected"）。SPA 未改动，generated client 留待后续。
- 重写 `internal/desktop/app.go`（替换 Task 1 spike）：`App{mu, cfg, cfgDir, rt, win, menus, log, logDir}`；`Run()` 装配日志文件（config 目录下 `logs/desktop.log`，MultiWriter 到 stderr）→ `ConfigPath`/`LoadOrDefault` → `mygo.Bind` → 注册 `OnWindowAllClosed` → `WhenReady(onReady)` → `App.Run()`；`onReady`（托盘 → 最近站点校验/目录选择 → `startAndServe` → `showWindow`）；`pickSiteDialog`（循环选择直到有效或取消）；`startAndServe`（stop 旧 rt → 新 ServeRuntime → OnServeExited/OnRebuildFailed 回调 → Start → AddRecentSite + Save 持久化 → 菜单刷新 → loadAdmin）；`loadAdmin`/`showOffline`；Controller 九方法实现（菜单回调均派发 goroutine，`ShowWindow` 无阻塞直接执行）。新增 `var _ Controller = (*App)(nil)` 编译期断言。
- `cmd/huan-desktop/main.go` 未改动（spike 版 `Run()` 签名一致）。

### MyGo API 核对结果（go doc，实际签名 vs brief）

- 全部一致：`Dialog.Open(OpenDialogOptions) ([]string, error)`、`OpenDialogOptions{Title, Directory}`、`Dialog.Error(title, content string)`（无返回值）、`NewNotification(NotificationOptions{Title, Body})` + `(*Notification).Show() error`、`ErrNotificationsDenied`、`Shell.ShowItemInFolder(path string)`（无返回值）、`Shell.OpenExternal(url string) error`（brief 按语句丢弃返回值，合法）、`Bind(services ...any)`、`PageOptions.TrustedOrigins []string`（经 `WindowOptions.Page` 嵌入字段）、`WindowOptions{Title, Width, Height, Page}`、`Page.LoadURL(rawURL string) error`、`(*Application).WhenReady(func())`、`(*Application).Run() error`、`(*Application).Quit()`、observability 全部签名（`NewLoggerWithWriter`/`Log`/`LogError`/`LogFunctionStart` 返回 spanID/`EventBranch`/`EventPoint`）。
- 适配 1（`NewWindow` 返回值）：brief 写 `newWin, err := mygo.NewWindow(...)` + 错误分支；实际 `func NewWindow(opts WindowOptions) *Window`（单返回值，无 error）。改为单赋值、删错误分支，语义不变。
- 适配 2（`Page.LoadHTML` 参数）：brief 调 `LoadHTML(html)`；实际 `LoadHTML(html, baseURL string)`。传 `""` 作为 baseURL（页面落在 about:blank），语义不变。
- 适配 3（`LogFunctionEnd` 参数）：brief 调 `LogFunctionEnd(startLog, payload)` 两参；实际 `LogFunctionEnd(spanID string, duration time.Duration, payload map[string]any)` 三参。`startAndServe` 增加 `startTime`，传 `time.Since(startTime)`，语义不变（补齐耗时记录）。
- 适配 4（窗口关闭回调）：brief 注释给的是 `OnClose(func(){...})` 形状；实际 `OnClose(fn func(e *CloseEvent))`（`e.PreventDefault()` 可阻止关闭）与 `OnClosed(fn func())`（窗口真正关闭后触发）两个回调。选 `OnClosed` 清引用：窗口真正销毁后才置 `a.win = nil`，即使未来出现阻止关闭的监听者也不会留下悬空引用；不调 `PreventDefault`，关闭行为保持默认。
- 适配 5（Quit 兜底）：brief 写 `mygo.App.Quit(); os.Exit(0)` 并注明 os.Exit 仅为 Quit 缺失时的兜底；go doc 确认 `(*Application).Quit()` 存在（关闭全部窗口后退出，可被 OnBeforeQuit/OnWillQuit/OnClose 取消——本应用注册的 OnClosed 不取消）。删除 `os.Exit(0)`：紧随 Quit 立即硬退出会跳过优雅退出流程。
- 适配 6（brief 缺 `ShowWindow`）：Controller 接口要求导出的 `ShowWindow()`，brief 只定义了小写 `showWindow`（编译错误暴露）。新增导出包装 `ShowWindow() { a.showWindow() }`（无阻塞工作，直接在菜单回调线程执行）。
- 适配 7（未用导入）：brief import 列表含 `internal/config`，app.go 实际未引用，删除。
- 适配 8（gofmt）：`ServeOptions.Logf` 一行闭包超宽，gfmt 重排为多行（纯空白，语义不变）。

### cfgDir 修复（评审 pre-flight 发现，已应用）

- brief 的 `Run()` 未给 `a.cfgDir` 赋值，`startAndServe` 的 `Save(filepath.Join(a.cfgDir, "config.json"), ...)` 会写到 CWD。已在 `ConfigPath()` 成功后增加 `app.cfgDir = filepath.Dir(cfgPath)`。冒烟验证：运行后 `~/Library/Application Support/huan-desktop/config.json` 被应用改写（AddRecentSite + Save 持久化生效），CWD 无 config.json 产生。

### 窗口关闭行为（决议）

- go doc：`OnWindowAllClosed` "Without listeners the application quits; registering one keeps it running (the common choice on macOS)"。选择 brief 注释中的第一方案（保持托盘常驻）：`Run()` 中注册空 `OnWindowAllClosed` 监听（阻止默认退出），每窗口 `OnClosed` 清 `a.win`；`ShowWindow` 按需重建窗口。Cmd+W 关窗后应用留在托盘，退出仅经托盘「退出 huan」/`Quit()`。

### dev.Server SIGINT/SIGTERM 行为记录（Task 5 评审遗留交付物，仅记录不扩scope）

- 位置：`internal/dev/server.go:104-135`。每次 `Server.Run` 都执行 `signal.Notify(SIGINT, SIGTERM)`：首个信号 → 进入 5s 超时的 `httpSrv.Shutdown` 优雅停止并返回（serve goroutine 退出，OnServeExited 以 nil/Shutdown 错误触发，桌面窗口走离线页）；5s 窗口内第二个信号 → `os.Exit(1)` 整进程强退；`Stop()`（ctx 取消）不中和已注册的信号处理器；默认信号处置仅在 Run 返回时经 `signal.Stop` 恢复。
- Phase 1 含义：从终端 `go run ./cmd/huan-desktop` 时 Ctrl+C 表现为「优雅停服（窗口转离线页）」而非退出应用，再按一次才整进程退出；Finder/正常桌面启动收不到终端信号，不受影响。按任务指示未新增应用级信号管理。

### 托盘 OnClick 记录（Task 7 评审发现）

- `tray.OnClick(...)` 在挂了菜单后不触发（go doc：OnClick 仅在无菜单时生效）。代码按 brief 保留；Phase 1「点击托盘图标显示窗口」预期不生效，点击应弹出菜单，属人工验收注意事项。

### 冒烟验收（自动化代理 + 人工清单移交）

- 自动化代理（无法点击对话框/菜单）：预置有效站点到 config.json 的 recentSites（替代「选择站点」交互）→ `go build -o /tmp/huan-desktop-smoke ./cmd/huan-desktop && /tmp/huan-desktop-smoke` 后台启动 → 8s 后核验：(a) 进程存活（PID 72544）；(b) `~/Library/Application Support/huan-desktop/logs/desktop.log` 生成，15 行全部为合法 JSON（`Function_Start start-serve`、build Point 事件，trace_id 一致）；(c) stderr 无 panic（grep 0 命中）；(d) 站点 GET 200、`/admin/` GET 200（127.0.0.1:61932）；(e) config.json 被应用持久化（cfgDir 修复生效）；(f) `kill -9` 后进程消失、`lsof -iTCP:61932` 0 监听（端口释放）。无配置首启的目录选择弹窗属预期行为，未等待。
- spec §7 人工六项清单（构建通知、停止/恢复离线页、Finder reveal、浏览器打开、切换站点 reload、优雅退出）无法自动化点击，移交人工验收；自动化代理已覆盖其可编程子集（启动流、服务可达、端口释放）。

### 验证

- `go build ./... && go vet ./internal/desktop/ ./cmd/huan-desktop/`：通过。
- `go test ./internal/desktop/ -v`：14 passed, 0 failed。
- `gofmt -l internal/desktop/*.go cmd/huan-desktop/main.go`：仅 config_test.go（Task 3 遗留项，未碰）。

### 备注

- 冒烟后清理了预置 config.json 与 /tmp 站点/二进制；`logs/desktop.log` 为应用自身产物保留。自审发现的低风险项（菜单回调 goroutine 中 `a.rt`/`a.menus` 无锁读的理论竞态窗口、`Rebuild` 中 `rt.Config()` 在 Running 检查后可能为 nil 的理论窗口）按 brief 设计保留，记入 Task 8 报告，Phase 2 加固。

## 15:50 Task 8 修复（评审 round 1）：TrustedOrigins 从运行中的 admin origin 派生

### 问题（评审 Important）

- 原实现 `TrustedOrigins: []string{"http://127.0.0.1"}` 不含端口；mygo v0.9.2 `Window.isTrusted`（window.go:1618-1638）以 `scheme://` + `u.Host`（含端口）构造页面 origin 并 `EqualFold` 精确比较。dev server 每次 Start 绑定随机端口（如 `http://127.0.0.1:55327`），故每个 SPA→Go 绑定调用（AppVersion/RevealSiteFolder）都会被拒绝——TrustedOrigins + bindings 的 Produces 目标落空。
- 源码另证：`IsDev()`（dev.go:12-17，非 production 环境为 true）会无条件信任 loopback http/https，开发期掩盖此 bug；Task 9 打包 .app（production）后即死路。评审结论属实，按用户裁决修复。

### 修复内容（internal/desktop/app.go、新增 app_test.go）

- 新增纯函数 `adminOrigin(adminURL string) []string`：解析 admin URL，取 `scheme://host`（host 含端口、去 path）作为 TrustedOrigins 项；无可用 URL（离线窗口）时返回 nil（无可信来源）。
- `createWindow()`（自 `showWindow` 拆出）：窗口创建时从当前运行时 `rt.AdminURL()` 派生 origins；无运行时则空 origins（离线页无需绑定）。
- **窗口重建机制（评审给定两案中的「startAndServe 关闭+重建」案）**：`startAndServe` 成功后调 `recreateWindow()`——关旧窗（`old.Close()`）、以新 origin 建新窗并加载 admin URL。理由：`WindowOptions.Page.TrustedOrigins` 为创建期固定属性（mygo 无 setter），且每次 Start 都绑定新端口，origin 必变，逐次重建比「比较窗口 origin 是否匹配」更简单且恒正确。`ShowWindow` 在无运行时调用时创建空 origins 窗口（离线页），下一次 `startAndServe` 必然重建之。
- **OnClosed 身份校验**：重建场景下旧窗 OnClosed 异步触发可能晚于新窗赋值 `a.win`；回调改为 `if a.win == newWin { a.win = nil }`（按窗口指针判定），陈旧回调不再误清新引用。单窗守卫语义保持（`a.win` 恒在 `a.mu` 下；OnClosed 清引用）。
- 删除死代码 `loadAdmin`（唯一调用点已由 `recreateWindow` 取代）；`showOffline` 保留（OnServeExited/停服路径不变）。
- 新增 `app_test.go::TestAdminOrigin`（4 子用例：带 path 的 admin URL、无 path、空 URL、无 host）。

### 冒烟复验（自动化代理，同协议）

- 预置站点启动 → 8s 核验：进程存活（PID 4968）、desktop.log 15 行全合法 JSON、stderr 无 panic；站点与 `/admin/` GET 均 200（127.0.0.1:55327）；python 复算 mygo origin 构造式确认「页面 origin == 我们传入的 TrustedOrigins 项」精确相等；`kill -9` 后进程消失、端口 0 监听。清理了预置 config 与 /tmp 产物。
- webview 内 SPA→Go 绑定调用仍无法自动点击验证，功能性绑定证明归人工验收；本次以「派生 origin 与 isTrusted 构造式精确匹配」+ 单测作为代码级证据。

### 验证

- `go build ./... && go vet ./internal/desktop/ ./cmd/huan-desktop/`：通过。
- `go test ./internal/desktop/ -v`：15 passed（既有 14 + 新增 TestAdminOrigin）。
- `gofmt -l`（app.go、app_test.go、bindings.go）：干净。

## 16:05 Task 9 完成：打包 + 本地归档 + 验收文档（Phase 1 自动化范围收尾）

### 完成内容

- `mygo build -platform darwin/arm64` 产出 `build/desktop/darwin-arm64/`：`huan.app`（20.8 MB）+ `huan-0.13.0-arm64.dmg`（5.6 MB），与计划假设路径一致。
- **修复 `mygo.json`**：`main` 由 `cmd/huan-desktop` 改为 `github.com/iannil/huan/cmd/huan-desktop`。mygo 的 `packageDir`/`goBuild` 把该字段原样传给 `go list`/`go build`，裸 `cmd/*` 路径被按 std 包解析报 "not in std"；此前从未运行过 `mygo build`（各任务用 `go build` 验证），本任务首次运行暴露。
- .app 结构检查（自动化）：二进制 Mach-O arm64 755、Info.plist（identifier `io.github.iannil.huan.desktop`、版本 0.13.0、min macOS 12.0）、adhoc 签名；bundle 未嵌图标（图标美化留待办）。
- 本地归档（按用户裁决 `/release/` 保持 gitignore，不入库）：`release/desktop/v0.13.0/` 下 app.zip（7.5 MB）+ dmg（5.6 MB）+ SHA256SUMS（`shasum -a 256 -c` 两项 OK）+ 中文 README（首启 xattr/右键说明、功能清单、日志位置、已知限制引用 spec §9）。
- 验收文档：`docs/reports/2026-11-17-desktop-phase1-acceptance.md`（VERIFIED / PENDING-HUMAN-ACCEPTANCE 分离）。
- 全仓门禁：`go vet ./...` 干净；`go test ./...` 33 包 ok / 0 失败（`-count=1` 复跑同）——Task 1 deferred 的 x/net+x/sys+x/text MVS 升级验证在此消化。gofmt：桌面范围仅 `config_test.go`（Task 3 残留，不修）；全仓另有 73 个文件为历史漂移（与分支变更集求交确认仅 config_test.go 属本分支）。

### 人工验收待办（不视为通过）

- 六项清单（启动选站/托盘构建/停止启动预览/打开目录浏览器/切换站点/退出释放端口）、双击首启（Gatekeeper 提示见 release README）。
- **必须含一次真实 SPA→Go 绑定调用**（AppVersion/RevealSiteFolder）——dev/smoke 因 mygo IsDev 信任回环无法功能性验证绑定，生产 .app 唯一有效。
- 托盘 OnClick：点击弹菜单不开窗口，MyGo 设计，非缺陷。

### 验证

- `go test ./...` / `go vet ./...`：全绿（见上）。
- SHA256SUMS 校验：`shasum -a 256 -c SHA256SUMS` → app.zip OK、dmg OK。
- git：本提交仅含文档与 `mygo.json` 修复；`release/`（gitignore）与 `build/`（未跟踪）均不入库。

### 状态

Phase 1 九个任务自动化范围全部完成；人工验收完成后本阶段方可视为最终关闭。进度文档按计划 `git mv` 至 `docs/reports/completed/2026-11-17-desktop-phase1.md`。

## 2026-11-17 16:41 UTC 终审修复波次（final fix wave）：Token 浮出 + 状态机接线 + v0.13.0 对齐

整分支终审结论「Ready after fixes」；用户对三项裁决拍板后一次性落地，代码提交 `ecf73d4`，验收报告新增 §7（含状态映射表与重打包证据），发布物已按新代码重打包（新 SHA 见验收报告 §7.4）。逐项记录：

### Fix 1（Critical，用户裁决）：托盘菜单浮出管理 Token

- 问题：ServeRuntime 静默生成 admin token（每 Start 再生成、不持久化），SPA 认证提示所需的 Token 在 Finder 启动的 .app 中无任何获取途径（HUAN_ADMIN_TOKEN/stderr 均不可达），全部 /admin/api/* 永久 401。
- 修复：ServeRuntime 在 mu 下存 token 并新增 `Token()`（Start 前空、Stop 与 adminURL 等同批清空）；`Controller` 增 `CopyAdminToken()`；托盘菜单与「站点」应用菜单均加「复制管理 Token」（ID `copy-token`，紧随 serve/build 项）。无 Token 时 Dialog「预览服务未运行」。
- MyGo 适配：`go doc` 核实 `ClipboardModule.WriteText(text string)` 存在（void，无错误面）→ 直接剪贴板方案，无需 Dialog.Message 回退；复制后系统通知确认。

### Fix 2（Important，用户裁决）：状态机接线 + startAndServe 串行化

- 问题：Task 4 的 `Machine` 导出且有测试但纯死代码，spec §5 状态模型从未驱动应用；startAndServe 无串行化（并发菜单动作可产生孤儿 ServeRuntime）。
- 修复 + **关键适配**：4 态无法区分「稳定 serving」与「serving=启动中」——字面入口门 `Transition(StateIdle, StateServing)` 会令切换站点（对运行会话的重启）恒失败。故 App 增私有 `startMu` 串行化 startAndServe（串行化本就是裁决目标之一），使入口时 Serving 必指稳定会话；入口按「暂停穿越 idle」声明（idle→serving / serving→idle→serving / building→idle→serving / error→idle→serving），丢 CAS 记日志合并返回。其余迁移点：ToggleServe 停止与 OnServeExited 走 serving→idle CAS（丢则跳过 UI 降级）；OnServeExited 先做 a.rt 指针身份校验（startAndServe 停旧前先 detach → 陈旧回调 `stale-exit-skipped`）；Rebuild 入口 serving→building（保留 Running() 守卫，堵启动窗口 rt 未就绪即构建的洞），完成 building→serving 丢失则保持新持有者状态。完整映射表见验收报告 §7.2。
- SetBuilding/SetServing 仍由实际结果驱动；state.go 测试全绿；无新增导出 API。

### Fix 3（机械批次）

- `internal/version/VERSION` 0.12.7 → 0.13.0（对齐 mygo.json/发布目录；引擎下一发布从 v0.13.0 续计——已记 daily note）。
- `config_test.go` gofmt 已清（Task 3 遗留关闭）；`server.go` 死字段 `tmpDir` 移除；`assets/.gitkeep` git rm（目录已有真实 PNG）；就绪轮询改 `&http.Client{Timeout: 2s}`；`ServeOptions` 增可选 `Logger`（nil 回退 stderr-only），App 注入文件 logger，serve-start/stop span 落 desktop.log。
- 全仓 gofmt 历史漂移（75 文件）与分支变更集（23 个 .go）求交为空，证明零扩散、未处理项均系历史遗留。

### Fix 5（验证门禁，全部通过）

- `go build ./...` + `go vet ./...` 干净；`go test ./internal/desktop/ -v` 15 用例全过；`go test ./...` 33 包全绿；`go test -race -count=2 ./internal/desktop/` 通过；分支范围 gofmt 干净。
- MyGo 引用仍仅 `internal/desktop/{app,menu,bindings}.go`；cmd/huan-desktop 仅 import internal/desktop；SPA 与 cmd/huan 未触碰。

### Fix 6（重新打包，本地归档不入库）

- `mygo build -platform darwin/arm64` 基于修复后代码重打；二进制 `grep -ac` 命中 `复制管理 Token`×3 / `copy-token`×1 / `预览服务未运行`×2（BSD strings 不识别多字节 UTF-8，grep -a 为有效证据）；新 SHA256SUMS 两项校验 OK，归档 zip 解包复验含 Token 菜单串。release/ 与 build/ 均不入 git。

### 状态

Phase 1 自动化范围在修复波次后重新闭环；人工验收（原六项 + 绑定调用 + 新增第 7 项复制 Token）须在新归档（验收报告 §7.4 SHA）上执行后方可最终关闭。
