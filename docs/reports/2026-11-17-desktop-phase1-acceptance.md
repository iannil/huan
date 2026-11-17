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
