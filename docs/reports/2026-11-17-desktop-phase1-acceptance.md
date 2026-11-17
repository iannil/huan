# huan desktop (MyGo) Phase 1 — 验收报告

- 日期：2026-11-17（UTC）
- 分支：`iannil/fde-next`（worktree scup）。打包基于代码 HEAD `2aa6ad1` + `mygo.json` main 路径修复（§2，提交 `4ee7bcb`）；发布提交为主题 `chore(release): huan desktop v0.13.0 phase 1 (.app) with acceptance report` 的 docs-only 提交（SHA 见 git log）。
- Spec：[docs/superpowers/specs/2026-11-17-huan-desktop-mygo-design.md](../superpowers/specs/2026-11-17-huan-desktop-mygo-design.md)
- 进度日志：[docs/reports/completed/2026-11-17-desktop-phase1.md](completed/2026-11-17-desktop-phase1.md)
- 状态：**自动化范围全部通过；交互式人工验收待办（见 §5，未视为通过）**

## 1. 验收标准（spec §1 摘录）

> `mygo build` 产出双击可用的 `.app`（+ DMG），窗口内完整可用 admin SPA，托盘可启停 serve、一键 build、打开站点目录、切换站点、退出。签名/公证/自动更新不在 Phase 1。

其中「窗口内完整可用 admin SPA」「托盘各项操作」属交互式验收，归人工待办（§5）；本文 §3 记录自动化可证部分。

## 2. 打包记录（VERIFIED）

- 命令：`mygo build -platform darwin/arm64`（mygo CLI v0.9.2）
- 前置：`internal/admin/dist/index.html` 存在（embed 依赖满足）
- 修复：`mygo.json` 的 `main` 由 `cmd/huan-desktop` 改为模块全路径 `github.com/iannil/huan/cmd/huan-desktop`。原因：mygo 的 `packageDir`/`goBuild` 将该字段原样传给 `go list`/`go build`，裸包路径 `cmd/*` 被 Go 按 std 包解析而报 "not in std"；此前各任务仅用 `go build` 验证，`mygo build` 系首次运行，问题在本任务暴露。
- 输出（与计划假设路径一致）：

```
build/desktop/darwin-arm64/
  huan.app                 20.8 MB
  huan-0.13.0-arm64.dmg     5.6 MB
```

### .app 结构检查（自动化可证部分）

| 检查项 | 结果 |
|--------|------|
| 二进制存在且可执行 | `Contents/MacOS/huan`，20.8 MB，Mach-O 64-bit arm64，权限 755 |
| Info.plist | CFBundleIdentifier=`io.github.iannil.huan.desktop`，版本 0.13.0，LSMinimumSystemVersion 12.0，NSHighResolutionCapable=true |
| 签名 | adhoc（Phase 1 按决策不签名公证；分发首启需 `xattr -cr` 或右键打开，见 release README） |
| 图标 | bundle `Resources/` 为空、Info.plist 无 CFBundleIconFile → 应用图标为系统默认（占位图标未进 bundle；「图标美化」在遗留清单） |

## 3. 全仓验证（VERIFIED）

| 命令 | 结果 |
|------|------|
| `go vet ./...`（全仓） | 通过，无输出，exit 0 |
| `go test ./...`（全仓） | **33 个包 ok，0 失败**，exit 0 |
| `go test -count=1 ./...`（无缓存复跑） | **33 ok / 0 fail**，exit 0 |
| `gofmt -l internal/desktop/*.go cmd/huan-desktop/*.go`（桌面范围） | 仅 `internal/desktop/config_test.go`（Task 3 已知遗留，按决议不修，列遗留） |

- Task 1 遗留验证在本任务消化：x/net、x/sys、x/text 的 MVS 升级与 go directive 1.26.2→1.27.3 已随全仓 `go test ./...` + `go vet ./...` 通过验证（33 包全绿）。
- gofmt 备注：用 go1.27.3 工具链 gofmt 对 `internal/ cmd/ scripts/` 全仓扫描会额外标出 73 个文件；经与分支变更集（68329ed..HEAD）求交，**其中仅 `internal/desktop/config_test.go` 属本分支触碰**，其余 73 个为历史遗留漂移（旧版 gofmt 的尾随注释对齐风格，例：`internal/version/version.go` VCSInfo 字段注释），非本分支引入，不在此处理。

## 4. 产物清单与 SHA256（VERIFIED）

归档目录：`release/desktop/v0.13.0/`（本地归档；`/release/` 按仓库约定 gitignore，不入库）

| 文件 | 大小 | SHA256 |
|------|------|--------|
| `huan-v0.13.0-darwin-arm64.app.zip` | 7.5 MB | `c028906d21cec89067c43bbcb92474c1bdaf27d35e2f42632f7034af37e2f37d` |
| `huan-v0.13.0-darwin-arm64.dmg` | 5.6 MB | `8ae8293d5a2de8e4b941e91342bbdf12db5cc5a616907b78c12ac52ecb0317e1` |
| `SHA256SUMS` | — | `shasum -a 256 -c SHA256SUMS` → 两项均 **OK** |
| `README.md` | — | 中文发布说明（系统要求 / 首启 xattr 或右键打开 / 功能清单 / 配置与日志位置 / 已知限制引用 spec §9） |

## 5. 人工验收待办（PENDING-HUMAN-ACCEPTANCE — 未通过，不视为验收完成）

以下项需真人在 GUI 操作；自动化冒烟（Task 8 的进程存活/日志 JSON/服务可达/端口释放代理证据）不能替代。

### 5.1 首启

- [ ] 解压 zip（或挂载 DMG）→ 首启。如被 Gatekeeper 拦截：`xattr -cr huan.app` 或右键 → 打开。

### 5.2 六项清单（Task 8 Step 5 / spec §7）

- [ ] 1. 双击启动 → 弹目录选择框 → 选 huan 站点（如 `../zhurongshuo`）→ 窗口加载 admin SPA
- [ ] 2. 托盘图标出现；「一键构建站点」→ 构建期间菜单项禁用 → 完成系统通知
- [ ] 3. 「停止预览服务」→ 窗口显示离线页；「启动预览服务」→ 恢复 admin
- [ ] 4. 「打开站点目录」→ Finder reveal；「在浏览器打开站点」→ 浏览器打开
- [ ] 5. 「切换站点」→ 选另一目录 → 窗口 reload 到新 admin
- [ ] 6. 「退出 huan」→ 进程退出、端口释放（`lsof -i :<port>` 确认）
- [ ] 7. 「复制管理 Token」→ 粘贴到 SPA 认证提示 → 数据操作可用（修复波次新增，见 §7.1；托盘菜单与「站点」应用菜单均有该项）

### 5.3 绑定功能性验证（必须 — Task 8 评审遗留交付物）

- [ ] 在 .app 窗口内触发**一次真实 SPA→Go 绑定调用**（如在 admin SPA / 开发者控制台调用 `AppVersion` 并核对返回版本号）。代码级证据（TrustedOrigins 从运行端口精确推导 + `TestAdminOrigin` 单测）已具备，但 dev/smoke 因 mygo `IsDev` 无条件信任回环而无法功能性验证绑定——生产 .app 是唯一有效验证环境。

### 5.4 验收注意事项

- **托盘 OnClick**：点击托盘图标弹出菜单而非直接开窗口——MyGo 设计（挂菜单后 OnClick 不触发），非缺陷。
- **LiveReload 热刷新**：webview 内编辑站点文件触发热刷新窗口，等同浏览器体验，属预期（spec §9）；若干扰可在验收时决定关闭注入开关。

## 6. 遗留事项

### 6.1 发布能力（spec 明确的 Phase 1 非目标）

1. 签名 + 公证（消除首启 Gatekeeper 拦截）
2. 自动更新器
3. Windows / Linux 打包（universal 留待同族）
4. 图标美化（当前为系统默认图，bundle 未嵌入图标）

### 6.2 Phase 2 加固族（Task 8 评审 deferred，代码已记录）

- 并发 `startAndServe` 未串行化（含 RecentSites 解锁读）；`Rebuild` 的 `rt.Config()` 理论 nil 窗口；Stop 超时误标窗口；App 菜单不随 serving/building 状态同步；tray OnClick 死代码注记。
- Task 5 deferred（择要）：EADDRINUSE 误诊、TestServeRuntime 偶发 flake（-count=6 复测过）等，详见进度日志。
- Task 3 parked：`LoadOrDefault` 对任何读取错误重置 + 静默 .bak——留最终评审 triage。
- Task 3/4 残留：`internal/desktop/config_test.go` 未过 gofmt（本报告按决议未修）。
- 观察：全仓 73 文件的 gofmt 历史漂移（旧版尾随注释对齐风格），建议后续统一 `gofmt` 波次处理。

### 6.3 本任务修复

- `mygo.json` `main` 改为模块全路径（§2）；`build/desktop/` 不在 .gitignore，构建产物保持未跟踪、未入库、未改 .gitignore。

## 7. 修复波次（2026-11-18，终审裁决落地 — 代码提交 `ecf73d4`）

整分支终审结论「Ready after fixes」的三项裁决 + 机械项全部落地后，重新打包发布物。本节为增量记录，前文保留原状。

### 7.1 Fix 1（Critical，用户裁决）：托盘菜单浮出管理 Token

- `ServeRuntime` 将每次 Start 生成的 admin token 存入 mu 保护区，新增 `Token()` 访问器（Start 前为空、Stop 清空——与 adminURL 等字段同批清理；无持久化，每 Start 重新生成保持不变）。
- `Controller` 接口新增 `CopyAdminToken()`；托盘菜单「复制管理 Token」（ID `copy-token`，紧跟 serve/build 项）；应用菜单「站点」子菜单同步加入。
- 剪贴板机制：核实 `go doc github.com/egoist/mygo ClipboardModule` 存在 `WriteText(text string)`（void，无错误面），直接采用，无需 Dialog 回退；复制成功后以系统通知确认。无 Token/未运行时 `Dialog.Error("预览服务未运行", ...)`。
- SPA 认证提示（HUAN_ADMIN_TOKEN / stderr 均不可达的 Finder 启动场景）自此可完整走通——§5.2 清单第 7 项（PENDING-HUMAN）。

### 7.2 Fix 2（Important，用户裁决）：状态机接线 + startAndServe 串行化

Task 4 导出但死代码的 `Machine`（state.go）自此驱动应用（spec §5 状态模型落地）。**接线适配说明**：4 态模型无法区分「稳定 serving」与「serving=启动中」，字面套用入口门 `Transition(StateIdle, StateServing)` 会使切换站点/换目录（对运行中会话的重启）恒被拒——故以 App 私有 `startMu` 串行化 startAndServe（串行化本身即裁决要求的另一目标），使入口时的 Serving 必指稳定会话；入口改为「暂停穿越 idle」式声明（idle→serving 新启 / serving→idle→serving 重启 / building→idle→serving 构建期重启 / error→idle 后新启）。状态映射与 CAS 丢失行为：

| 迁移点 | 迁移 | CAS 丢失时 |
|--------|------|-----------|
| startAndServe 入口 | idle→serving（新启）；serving/building→idle→serving（重启/构建期重启）；error→idle→serving | 记 `state-gate-lost` 分支日志。**语义澄清（re-review 修正）**：入口 CAS 丢失仅在持锁期间他方迁移机器的纳秒级窗口可达；启动窗口内的重复点击实际在 startMu 上**排队**，等待后按当期机器状态执行——Serving 则完整停止+重启（非合并、非幂等） |
| startAndServe 失败 | serving→idle | —（必胜，持 startMu）；§6 error→idle 恢复塌缩为一步，重试入口同为 idle→serving |
| ToggleServe 停止路径 | serving→idle 先于 Stop | 跳过离线页/SetServing(false) UI 降级（重启/构建持锁中），记 `state-gate-lost` |
| OnServeExited | 先身份校验（a.rt 指针等同，重启时旧 runtime 已 detach → `stale-exit-skipped`），后 serving→idle；building→idle（构建中服务消亡，仍降级 UI）；余者 `unexpected-state-skip` 跳过 | — |
| Rebuild 入口 | serving→building（取代裸 rt.Running() 判定；Running() 保留为启动窗口守卫——启动窗口机器亦为 serving，此时 rt 未就绪不可构建） | 原对话框「预览服务未运行，请先选择站点」 |
| Rebuild 完成/失败 | building→serving | `build-return-transition-lost`：机器已由新持有者（服务消亡→idle / 重启→serving）接管，保持其值；UI 仍按实际结果 SetBuilding(false)+通知 |

- SetBuilding/SetServing UI 更新仍由实际结果驱动（不变）；Machine 是并发门 + 迁移真相。state.go 既有测试全绿；无新增导出 API。

### 7.6 Fix 波 re-review 残留项（已裁决搁置，2026-11-18）

re-review 判定全部 findings ADDRESSED、状态机接线 SUBSTANTIVE、无新 Critical/Important。以下残留均为 Minor/观察级，裁决搁置：

- **剪贴板 void 吞错**（app.go `CopyAdminToken`）：`Clipboard.WriteText` 无错误返回（mygo modules.go 内部 `_ = c.Write(...)`），写失败仍会弹「已复制」通知。概率低、观感级；Phase 2 若 mygo 暴露错误接口再接。
- **切换点击捕获时序**：`ToggleServe` 启动路径在阻塞 startMu 之前捕获 `RecentSites[0]`（app.go:408），理论上 ChooseSite 排队期间的切换点击会以旧站点完成排队重启（终态仍是有效服务会话）。可选微改进：捕获移至 startMu 等待之后。
- **范围外观察**：`OpenSiteInBrowser`/`Quit` 处 `a.rt` 解锁读（相对 startAndServe 的锁定写属技术性 data race，良性指针读）；`StateError` 从未被置入（入口预清除，失败→idle 塌缩为一步——模型保真度注记，非裁决偏离）；构建期间机器处于 Building，用户无法停止服务（设计如此，见 §7.2 映射表）——人工验收时的 UX 注记；App 控制器状态流无直接单测（经静态分析 + state.go 自身 4 测试验证）。


### 7.3 Fix 3（机械批次）

- `internal/version/VERSION`：0.12.7 → **0.13.0**（与 mygo.json/发布目录对齐；引擎下一发布从 v0.13.0 续计，记于当日 daily note）。
- `config_test.go` gofmt 已清（§3 表中「仅 config_test.go」遗留就此关闭；§6.2 对应条目关闭）。全仓 75 文件 gofmt 历史漂移与分支变更集（68329ed..HEAD，23 个 .go）求交为**空**——漂移全部为历史遗留，未扩散。
- `server.go` 死字段 `tmpDir` 移除（Start 用局部变量）；`assets/.gitkeep` 移除（目录已有真实 PNG）。
- Start 就绪轮询改 `&http.Client{Timeout: 2s}`（裸 http.Get 无超时）。
- `ServeOptions` 增 `Logger *observability.Logger`（nil 回退 stderr-only），App 注入文件落盘 logger——serve-start/serve-stop span 落 desktop.log。

### 7.4 重新打包（VERIFIED）

- `mygo build -platform darwin/arm64`（mygo v0.9.2，基于修复后工作树）；二进制结构检查：`LC_ALL=C grep -ac` 命中 `复制管理 Token`×3、`copy-token`×1、`预览服务未运行`×2（BSD `strings` 不识别多字节 UTF-8，grep -a 为有效证据）；Info.plist 版本 0.13.0。
- 新 SHA256（`release/desktop/v0.13.0/SHA256SUMS`，`shasum -a 256 -c` 两项 OK；归档 zip 解包复验二进制含 Token 菜单串）：
  - app.zip `3395b30a66e343f724ba1382f5f9dcfe477963aecf90ecd27ed7c5d74a30c236`
  - dmg `f0f4b23fa23c6104f1937a60650dec52766545bb0a501c898a39308f344914ac`
- 验证门禁：`go build ./...`、`go vet ./...` 干净；`go test ./...` 33 包全绿；`go test -race -count=2 ./internal/desktop/` 通过（15 用例）；分支范围 gofmt 干净。MyGo 引用仍仅在 `internal/desktop/{app,menu,bindings}.go`（cmd/huan-desktop 仅 import internal/desktop）；SPA 与 cmd/huan 未触碰。

### 7.5 人工验收待办增量

- §5.2 第 7 项（复制管理 Token → SPA 认证 → 数据操作可用）为 PENDING-HUMAN；第 1–6 项与 §5.3 绑定调用维持待办，均应在新归档（§7.4 SHA）上执行。
