# 桌面端启动诊断 + 品牌图标接入 — 验收报告

- 日期：2026-11-18（UTC）
- 分支：`feat/desktop-startup-diag-brand`（worktree desktop-startup-diag-brand，base master@81cfd7c）
- Spec：[docs/superpowers/specs/2026-11-18-desktop-startup-diagnostics-and-brand-icons-design.md](../superpowers/specs/2026-11-18-desktop-startup-diagnostics-and-brand-icons-design.md)
- 计划：[docs/superpowers/plans/2026-11-18-desktop-startup-diagnostics-and-brand-icons.md](../superpowers/plans/2026-11-18-desktop-startup-diagnostics-and-brand-icons.md)（Task 1–11 + 用户裁决新增 Task 12）
- 执行方式：superpowers:subagent-driven-development（每任务独立实现者 + 审查者 + 修复环），台账 `.superpowers/sdd/2026-11-18-desktop-startup-diagnostics-and-brand-icons/progress.md`
- 状态：**自动化 + 运行时验证范围全部通过；交互式人工验收待办见 §6（未视为通过）**
- 最终全分支评审（2026-11-18）：1 Important（托盘徽标重启丢失，已修）+ 2 Minor（1 修复 1 留档）+ /build/ gitignore 已补

## 1. 交付范围（spec §目标 ↔ 实现）

| Spec 目标 | 实现任务 | 关键提交 |
|---|---|---|
| §1 插件加载警告显式化（loader → Registry.Warnings → CLI 单点打印） | Task 1/2 | `3aafe10`、`b59bbb0` |
| §2 启动期问题收集器（Diagnostics：serve 失败/异常退出/重建失败） | Task 3/4 | `0e7910e`、`4e9007f` |
| §3 UI 三件套（菜单徽标计数 / 详情对话框 / 系统通知 `startup-diag`） | Task 5/6 | `40f5473`、`bab3256` |
| §4 图标三处（bundle icns / 托盘 template / 运行时 Dock） | Task 6/7/8 | `0c40078`、`e916561`、`e1806f5` |
| §5 插件运营修复（go1.27.3 重编 7 个 .so，警告归零） | Task 10 | 运营操作（无产品代码） |
| §6 版本与发布（0.13.1，.app/DMG + SHA256SUMS + README） | Task 9 | `86e461a` |
| **新增**：config 级插件警告（yaml 已声明但 .so 缺失）进入启动诊断 | Task 12（用户裁决 2026-11-18「现在补齐」） | `92a3b2b` |

Task 12 补齐了 spec 验收项 1 的缺口：「huan.yaml 已声明但 .so 缺失」原为 `registry_config.go` 内联 stderr 打印，桌面端不可见；现收集进 `Registry.ConfigWarnings`，经 `ServeOptions.OnConfigWarnings` / `BuildSiteTo` 第 5 参进入诊断收集器，CLI stderr 行格式逐字节不变（config 行先、load 行后为 spec 不变量；改前内联顺序为 load 先）。

## 2. 自动化验收（spec「测试与验收」§自动化）

| 检查项 | 结果 | 证据 |
|---|---|---|
| `go vet ./...` | ✅ 无输出 | 2026-11-18 全量跑 |
| `go test ./...` | ✅ 33 pkgs ok | 同上；`TestServeRuntime_StartServeStop` 在高并行负载下 5s Stop 超时为**既有 flake**（路径未被本分支触碰），隔离运行 `-count=1` 通过 |
| gofmt 交集 | ✅ 无新增漂移 | 仓库级既有漂移（工具链格式化版本差）；本分支触碰的 3 个漂移文件（`cmd/huan/plugins_test.go`、`internal/daemon/daemon.go`、`internal/plugin/lifecycle.go`）在 merge-base 与 HEAD 的 gofmt diff 行数逐一相等（12/50/50）——漂移区未被改动 |
| CLI stderr 行格式不变 | ✅ | `cmd/huan` 测试断言 `huan: plugin config warning: …\n` 与 `huan: plugin load warning: …\n` 行格式与改前内联打印逐字节一致；config → load 的**顺序**为 spec 规定的不变量（分支前内联顺序为 load 先，本分支有意改为 config 先） |
| 单测覆盖（spec 列表） | ✅ | Registry.Warnings/ConfigWarnings 收集（成功/失败混合目录，`HUAN_HOME` 隔离）；Diagnostics 并发 Add/Snapshot/Reset；菜单标题分支 0/n>0；通知触发条件；OnPluginWarnings/OnConfigWarnings 接线 |

## 3. 运行时验收（自动化可证部分）

环境说明：mygo v0.9.2 bundle 以 `CGO_ENABLED=0` 构建 → Go plugin 包不可用（"plugin: not implemented"）→ **bundle 内插件功能天然缺失**（v0.13.0 同源限制，非本分支回归）；因此警告场景验证全部在裸二进制/CLI 完成（cgo 生效），通知/点击路由在 bundle 验证。

| 场景 | 结果 | 证据（结构化日志 trace） |
|---|---|---|
| 版本不匹配（3 个 stale .so）+ 声明缺失（html_injector） | ✅ 4 条诊断（3 loader + 1 config）→ 通知计数 `{errors:0, warnings:4}` | trace `2aa5cd1c`：4x `component:plugin` diag 事件 + `startup-diag` Branch；裸二进制 |
| 干净启动（7 个全新契约 .so） | ✅ 零诊断事件、零通知、无 `startup-diag` | trace `0db367cc`：全程无 diag 事件 |
| CLI stderr config 警告行 | ✅ `huan: plugin config warning: plugin "html_injector": declared in yaml but no .so found in $HUAN_HOME or plugins/` | trace `68050f0a` 输出第 3 行逐字节一致 |
| bundle 启动（Task 12 后最终重建版） | ✅ 进程存活、admin 窗口 + 认证对话框渲染 | 截图 `huan-final-bundle.png`（2026-11-18 04:2x UTC），osascript 退出干净 |
| bundle icns = 仓库资产 | ✅ SHA256 `d9de5b18…76be3c65` 双侧一致（157620 B）；`CFBundleShortVersionString=0.13.1` | Task 9 第 3 次重建验证（92a3b2b 二进制） |
| 发布物完整性 | ✅ zip 7.9M / dmg 6.1M，`shasum -a 256 -c SHA256SUMS` 全 OK | `release/desktop/v0.13.1/` |
| 图标尺寸规范（用户裁决「图标尺寸不对 太大了」） | ✅ 内容占比 80.4%（Apple 网格 824/1024），10 个 iconset 条目 + icon.png；box/area-average 缩放核（premultiplied-alpha 正确），stdlib-only | 修复环 2 轮：`e916561`（margin 合成）→ `e1806f5`（去 x/image 依赖防插件子模块泄漏） |
| 托盘 template | ✅ 18×18 黑+alpha 三道几何条，占比 77.7%（小尺寸豁免 margin），防锯齿 | `scripts/gen-desktop-assets` 程序化绘制，逐像素核验 |

## 4. 人工验收清单核销（spec §测试与验收 1–5）

| # | 项 | 结论 |
|---|---|---|
| 1 | 缺失 .so → 通知 + 菜单计数 + 详情 | ✅ 机制验证（Task 12 后 config 级缺失同样可见）；**菜单/对话框视觉目检待人工** |
| 2 | 版本不匹配 → 同上 | ✅ 运行时验证（§3 行 1）；复现源：`~/.huan/plugins-stale-backup/` |
| 3 | 三处图标目检（Finder/Dock icns、托盘深浅色、裸二进制 Dock） | ⏳ 程序化证据齐（§3），**深浅色目检待人工** |
| 4 | bundle 图标 | ✅ Task 9 Step 3 首次验证（权限弹窗内品牌图标可见）+ 本次哈希复核 |
| 5 | 干净启动 → 无通知、菜单「启动诊断…」 | ✅ trace `0db367cc` 零事件；**菜单文案目检待人工** |
| — | 通知点击路由（计划 Step 3） | ⏳ bundle 内 `OnNotificationClick` 已注册；**macOS 通知权限首次弹窗待用户应答后才能端到端点击验证**（弹窗可能仍留在屏幕上） |

## 5. 遗留（记录为后续项，不阻塞本分支）

1. **bundle 插件缺失（上游约束）**：mygo bundle `CGO_ENABLED=0` → Go plugin 包不可用，bundle 内 .so 永远警告 not implemented；桌面端插件功能需 mygo 提供 cgo bundle 或宿主侧替代机制。
2. **registry_config.go:39/:43 两处 error 级 stderr 打印未上移**（name-conflict / register-error）：罕见、非警告级，Task 12 裁决范围外。
3. **托盘 @2x 图标**：18×18 单尺寸在 Retina 下可接受但非最优，spec 范围外留待后续。
4. **`TestServeRuntime_StartServeStop` 5s Stop 超时 flake**：既有，高并行下复现、隔离必过；建议后续放宽超时或注入假时钟。
5. ~~**`.gitignore` 缺 `/build/` 规则**~~：已于最终评审修复环补上（commit c7339b8，根锚定 `/build/`，不影响 `internal/build` 源码包）；此条仅留追溯。
6. **裸二进制 dev 运行通知噪声**：`notifyID` 每次记录 `ErrUnsupported`（3 行/次）；可用 `mygo.NotificationsSupported()` 门控，属锦上添花。
7. **dev/serve 动态路径 OnWarnings 休眠**（Task 2 已裁决 park）：静态路径事件类已被 Task 1/5/12 覆盖。
8. **macOS 通知权限**：首次弹窗待用户应答；拒绝后通知静默（菜单徽标仍工作）。
9. **`NewRegistryFromConfig` 错误路径丢弃已收集警告（最终评审 F2，裁决 STAY-PARKED）**：当其返回 error（plugins 目录不可读 / 致命配置错误）时返回 nil，已收集的 `Warnings`/`ConfigWarnings` 一并丢弃，调用方在致命错误前不打印任何警告；改前内联打印会在致命错误前输出这些警告。维持现状理由：场景罕见、致命错误本就阻断启动，为此改 API 不值得。

## 6. 待人工验收（不视为通过）

- 托盘/Dock 图标深浅色目检（系统外观切换各一次）。
- bundle 内通知点击 → 启动诊断对话框（需先应答权限弹窗）。
- 菜单「启动诊断（n 个问题）…」徽标文案与对话框条目视觉确认（可用 `~/.huan/plugins-stale-backup/` 复现）。

## 7. 提交清单（81cfd7c..HEAD）

`3aafe10` 警告显式化 · `b59bbb0` daemon/dev 接线 · `0e7910e` Diagnostics 收集器 · `4e9007f` 菜单徽标 · `40f5473` 桌面端警告接线 · `bab3256` 通知/对话框/Dock/托盘 · `0c40078` 托盘程序化绘制 · `e916561` 图标 margin 合成 · `e1806f5` stdlib 缩放核 · `dfd4c91` iconset 脚本 · `86e461a` v0.13.1 发布配置 · `c392564` 插件子模块工具链对齐 · `92a3b2b` config 级警告进诊断 · `0a7ef2b` 验收报告 · 修复环：`5d268ee` 托盘徽标重启保持 · `90fcc4a` stderr 行序文档纠正 + F2 留档 · `c7339b8` /build/ 忽略规则
