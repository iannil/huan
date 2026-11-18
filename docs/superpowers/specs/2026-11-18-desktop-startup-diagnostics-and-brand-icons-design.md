# 桌面端启动诊断 + 品牌图标接入 设计

- 日期：2026-11-18
- 状态：已批准（对话内确认），待实现
- 关联：[2026-11-17 huan desktop MyGo 设计](2026-11-17-huan-desktop-mygo-design.md) · Phase 1 验收报告 `docs/reports/2026-11-17-desktop-phase1-acceptance.md`

## 背景与问题

1. **插件加载警告不可见**。`internal/plugin/loader.go` 两处直接 `fmt.Fprintf(os.Stderr, "huan: plugin load warning: ...")`；Finder 启动的 .app 无 stderr，警告石沉大海。实测（2026-11-18）：7 个插件中 4 个（diagram-renderer / html-injector / seo-injector / sitemap-enhancer）报 `plugin was built with a different version of package internal/goarch` —— 插件以旧版 Go 工具链构建，与当前 go1.27.3 宿主不匹配。
2. **启动期问题无统一入口**。serve 启动失败、serve 异常退出、重建失败等已有零散 Dialog/通知，但没有「本次启动有什么问题」的全貌视图；插件警告这类非致命问题连零散提示都没有。
3. **logo 不对**。`internal/desktop/assets/icon.png`（512 占位圆盘）与 `tray.png`（18 占位黑盘）是脚本画的占位图；`mygo.json` 未配 `macos.icon`，.app/DMG 用 MyGo 默认图标。而品牌资产已齐备：`web/admin/public/brand/`（2026-11-16 确认的几何图标 16–1024px 全尺寸 PNG + SVG，含 PRODUCT-LOGOS.md 使用指南），桌面端从未接入。

## 目标 / 非目标

**目标**
1. 启动期问题（插件警告、serve 启动失败、serve 异常退出、重建失败、站点校验失败）统一收集，桌面端可见可查：通知 + 菜单详情对话框。
2. 插件 .so 版本不匹配警告归零（运营修复：当前工具链重编部署）。
3. huan 品牌图标接入三处：.app/DMG bundle 图标、托盘 template 图标、裸二进制运行时 Dock 图标。

**非目标**
- 不改 admin SPA（Phase 1 原则「SPA 一行未改」）。
- 不做插件自动重编（诊断可见即可；修复是人工运营动作）。
- 不覆盖配置文件损坏导致的 `Run()` 早期退出（app 未就绪无法弹 UI，维持 stderr + 进程退出现状）。

## 已确认的决策（对话内裁决）

| 决策点 | 选择 |
|---|---|
| 提示范围 | 全部启动期问题（建统一收集器，未来新问题类型直接挂入） |
| 提示形式 | 系统通知 + 菜单详情对话框（不启动即弹窗） |
| logo 覆盖面 | 三处全部（bundle / 托盘 template / 运行时 Dock） |
| 架构 | 方案 A：Registry 显式暴露警告 + 桌面端集中诊断收集器（否决：就地弹窗散点方案、stderr 劫持方案） |

## 设计

### §1 internal/plugin 警告显式化

- 新类型 `plugin.LoadWarning{ Name string; Err error }`。
- `Registry` 增加导出字段 `Warnings []LoadWarning`。
- `loader.go` 两处 stderr 打印改为 append 到 `Registry.Warnings`，loader 不再直接打印。
- 打印职责上移到调用方：
  - CLI（serve / build 等所有 `NewRegistryFromConfig` 调用点）在取得 Registry 后统一打印，格式保持 `huan: plugin load warning: <name>: <err>` —— 终端行为不回归；
  - 桌面端读字段进收集器（§2）。

### §2 桌面端 Diagnostics 收集器

`internal/desktop/diagnostics.go`（新文件）：

```go
type DiagLevel string // "warn" | "error"

type DiagItem struct {
    Level     DiagLevel
    Component string // "plugin" | "serve" | "build" | "site"
    Message   string
}

type Diagnostics struct{ /* mutex + items */ }
func NewDiagnostics() *Diagnostics
func (d *Diagnostics) Add(level DiagLevel, component, msg string)
func (d *Diagnostics) Snapshot() []DiagItem
func (d *Diagnostics) Reset() // 每次启动新会话（startAndServe 入口）重置
```

挂接点（**只增登记，不动现有 Dialog/notify 分支**）：

| 场景 | 现有提示 | 新增登记 |
|---|---|---|
| 插件加载警告（serve 路径） | 无 | `Add(Warn, "plugin", …)`；`ServeOptions` 加可选 `OnPluginWarnings func([]plugin.LoadWarning)`，server.go 在 `NewRegistryFromConfig` 后回调 |
| 插件加载警告（一键 build 路径） | 无 | `BuildSiteTo` 同样回调，登记 `"build"` |
| `rt.Start()` 失败 | `Dialog.Error`（保留） | `Add(Error, "serve", …)` |
| serve 异常退出 | notify（保留） | `Add(Error, "serve", …)`（stale 回调守卫之后） |
| 重建失败 | notify（保留） | `Add(Error, "build", …)` |
| onReady 站点校验失败 | `Dialog.Error`（保留） | `Add(Error, "site", …)` |

App 结构体新增 `diag *Diagnostics` 字段；所有 Add 走结构化日志（全链路可观测模式延续）。

### §3 UI 层：菜单 + 对话框 + 通知

- **菜单**（`menu.go`）：托盘与应用菜单各加一项「启动诊断…」；`MenuSet` 加 `SetDiagCount(n int)`，n>0 时标题为 `启动诊断（N 个问题）…`。每次 Add / Reset 后刷新。
- **对话框**：点击菜单项 → `mygo.Dialog.Message` 逐行列出 `[错误] serve: …` / `[警告] plugin: …`；空列表显示「本次启动未发现问题」。
- **通知**（触发条件修正：**有任何条目即通知**，含警告级 —— 插件不匹配正是警告级，是本需求源头；仅错误级会漏掉它）：
  - `onReady` 启动完成后 `len(Snapshot())>0` → `notify`，标题按级别分档：含错误 `huan 启动存在问题` / 仅警告 `huan 已启动，有 N 个警告`，body 为首条摘要；
  - 通知 ID 固定 `"startup-diag"`（mygo 同 ID 替换，防堆积）；
  - `mygo.App.OnNotificationClick` 匹配该 ID → 打开同一对话框。

### §4 品牌图标三处接入

**新脚本 `scripts/gen-brand-icons.sh`**，从 `web/admin/public/brand/` 现成资产生成：

1. `internal/desktop/assets/app.icns` —— iconset 暂存目录按 Apple 尺寸表映射：16←16、16@2x←32、32←32、32@2x←64、128←128、128@2x←256、256←256、256@2x←512、512←512、512@2x←huan-primary-1024；`iconutil -c icns` 生成。全尺寸 PNG 现成，零光栅化。
2. `internal/desktop/assets/icon.png` ← `huan-icon-512.png`（替换占位盘）。
3. `internal/desktop/assets/tray.png` —— 扩展 `scripts/gen-desktop-assets` Go 程序：按 `huan-primary.svg` 三平行四边形几何参数（`M 240 295 L 570 205 L 760 315 L 430 405 Z` 等三路径）线性映射绘制 18×18 黑色（黑+alpha）template 图，保持与现占位同画布尺寸（`template=true` 自适应深浅色；retina 2x 留待后续细化，无回归）。

**接线**：
- `mygo.json` 增加 `"macos": { "icon": "internal/desktop/assets/app.icns" }`（config 校验只认 .icns/.icon/.car）→ 重新 `mygo build` 的 .app/DMG 自带品牌图标；
- `app.go onReady` 增加 `mygo.App.Dock.SetIcon(iconPNG)` —— 裸二进制开发态 Dock 图标；.app 下与 bundle 图标相同，覆盖无害（icon.png 自此不再是死资源）。

**前置**：`web/admin/public/`（含 brand/）当前未纳入 git 跟踪 —— 实现时确认 .gitignore 意图后随本变更提交品牌资产（设计源文件，PRODUCT-LOGOS.md 已声明为归位资产）。

### §5 插件运营修复（本次执行）

- 当前 go1.27.3 重跑 `scripts/build-plugins.sh release/plugins`（含 macOS segcount 修复 + adhoc 重签），拷贝 7 个 .so 至 `~/.huan/plugins/`。
- 桌面端重启验证警告归零。
- 陷阱留档：每次 Go 工具链升级后必须重编插件；loader 错误消息本身含 "built with a different version" 字样，诊断对话框用户可见。

## 测试与验收

**单测**
- `plugin`：Registry.Warnings 收集正确（成功/失败混合目录）；CLI 调用点打印上移不回归（输出格式断言）。
- `desktop`：Diagnostics 并发 Add/Snapshot/Reset；菜单标题分支（0 / n>0）；通知触发条件（有/无条目）；OnPluginWarnings 接线。

**人工验收清单**（1、2 需在 §5 部署新插件**之前**执行，或保留一份旧 .so 备用）
1. 移走 `~/.huan/plugins/` 任一 .so 复现缺失警告 → 系统通知弹出 + 菜单详情可见 + 菜单标题带计数；
2. 故意放一个旧工具链编的 .so 复现版本不匹配 → 同上（警告级文案）；
3. 三处图标目检：.app 在 Finder/Dock 的 icns、菜单栏托盘 template（深浅色各看一次）、裸二进制 Dock；
4. 重打包 .app/DMG 验证 icns 进 bundle；
5. 正常站点（无插件问题）启动 → 无通知，菜单显示「启动诊断…」且详情为「未发现问题」。

**自动化**：全仓 `go test ./...` + `go vet`。

## 版本与发布

- 完成后 `mygo.json` version `0.13.0 → 0.13.1`（引擎续计，不开新版本线）。
- `mygo build -platform darwin/arm64` 重出 .app/DMG + ditto 归档 + SHA256SUMS 至 `release/desktop/v0.13.1/`，README 更新（去掉「应用图标为 MyGo 默认图」已知限制，新增启动诊断功能条目）。

## 范围外

- admin SPA 不改；插件自动重编不做；Run() 早期配置错误的 UI 提示不做（见非目标）；托盘 @2x 图标细化留待后续。

## 关联文档

- 上游设计：`docs/superpowers/specs/2026-11-17-huan-desktop-mygo-design.md`
- 品牌资产：`web/admin/public/brand/PRODUCT-LOGOS.md`
- 发布约定：`release/` 不入库；发布物以生产环境为标准（CLAUDE.md）
