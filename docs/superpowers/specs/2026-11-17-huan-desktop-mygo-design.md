# huan 桌面端（MyGo）设计文档

- 日期：2026-11-17
- 状态：已确认（brainstorming 产出）
- 框架：[MyGo](https://mygo.run/zh) v0.9.x（`github.com/egoist/mygo`，纯 Go 无 cgo）
- 平台：macOS 优先（arm64；后续可扩 universal / Win / Linux）
- 阶段：Phase 1 —— 管理后台壳

## 1. 目标与定位

把现有 `huan serve` + 内嵌 `/admin` SPA 装进桌面窗口（系统 webview），加托盘/菜单快捷操作。Phase 1 是「壳」：复用最大化，admin SPA 一行不改。

**验收标准：** `mygo build` 产出双击可用的 `.app`（+ DMG），窗口内完整可用 admin SPA，托盘可启停 serve、一键 build、打开站点目录、切换站点、退出。签名/公证/自动更新不在 Phase 1。

**非目标（Phase 1）：** 内容编辑器、多站点管理面板、建站向导、自动更新器、Windows/Linux 打包、SPA 功能改动。

## 2. 已确认的决策

| 决策点 | 结论 |
|--------|------|
| 核心定位 | 管理后台壳（webview 承载 admin SPA + 原生托盘/菜单） |
| 集成方案 | 方案 A：同仓同模块 + 进程内嵌入（B 子进程、C 独立模块已否决） |
| 平台 | macOS 优先 |
| 站点来源 | 原生目录选择 Dialog + 最近站点列表（存本地配置） |
| 验收 | 可打包 `.app`，不签名不公证 |

**方案 A 否决 B/C 的理由：**

- B（子进程模型）：.app 内需打包两个二进制，端口/生命周期/日志跨进程管理，Phase 1 复杂度不值。
- C（独立模块）：`internal/` 包不能跨模块导入，需先把桌面所需能力全部提升为 `pkg/` 公开 API，属大重构，YAGNI。

**MyGo 关键事实（已核实）：**

- 模块路径 `github.com/egoist/mygo`；CLI `github.com/egoist/mygo/cmd/mygo`
- `win.Page().LoadURL("http://127.0.0.1:<port>/admin/")` 可加载运行中 HTTP 服务；SPA→Go 绑定需 `TrustedOrigins` 信任 localhost 源
- Tray / Menu / Dialog / `mygo.App.WhenReady` 生命周期钩子齐备
- `mygo build` 产出 `.app` + DMG，无需 cgo，可交叉构建
- 配置文件支持 `devUrl`/`devCommand`/`buildCommand`/`frontendDist`（本设计不用前端嵌入，走 LoadURL）

## 3. 架构总览

```
huan-desktop.app（MyGo 应用，进程内）
│
├── MyGo 原生层
│   ├── Window(webview) → Page().LoadURL("http://127.0.0.1:<port>/admin/")
│   │                      + TrustedOrigins 信任 localhost（打通 SPA→Go 绑定）
│   ├── Tray + Menu（原生：启停 serve / 一键 build / 打开站点目录 / 切换站点 / 退出）
│   └── Dialog（目录选择、错误弹窗）
│
├── huan server（goroutine，进程内）
│   ├── internal/dev  — HTTP + LiveReload + 内嵌 /admin SPA
│   └── internal/daemon — watch + 增量构建 + SSE 事件
│   （监听 127.0.0.1:0 随机端口，零外部暴露）
│
└── 桌面状态层（internal/desktop）
    ├── 当前站点目录 + 最近站点列表（上限 10 条）
    └── 配置持久化 JSON（os.UserConfigDir()/huan-desktop/）
```

进程内嵌入复用现有 daemon 增量构建、SSE 事件总线、admin SPA 与 LiveReload；托盘/菜单直接调 Go 函数，无跨进程协议。

## 4. 目录结构

桌面代码全部收在 `internal/desktop` 一个包树里，MyGo 框架依赖不外溢：

```
cmd/huan-desktop/main.go      — 入口：装配并 desktop.Run()
internal/desktop/
  ├── app.go        — MyGo 应用生命周期（WhenReady、窗口创建）
  ├── server.go     — 进程内 huan server 启停封装
  ├── menu.go       — 托盘 + 菜单构建与事件处理
  ├── state.go      — 站点状态机（idle/serving/building/error）、最近列表
  ├── config.go     — 配置读写（JSON，UserConfigDir）
  └── bindings.go   — 暴露给 SPA 的类型安全绑定（最小集）
```

**前置小重构（唯一动现有代码的点）：** `huan serve` 的装配逻辑（daemon + dev server 接线）目前长在 `cmd/huan`；提取为 `internal` 包内可调用函数，CLI 与桌面两个入口共用。

## 5. 交互流与数据流

### 启动流

```
双击 .app → main → desktop.Run()
  1. 读配置 → 有最近站点？
     ├─ 有 → 取最近一条为当前站点
     └─ 无 → 原生 Dialog 选目录 → 校验含 huan.yaml → 记入最近列表
  2. 启动 server goroutine（daemon watch + dev server，127.0.0.1:0 随机端口）
  3. WhenReady → 创建窗口 → LoadURL(admin URL)
  4. Tray 就绪，图标进入 serving 状态
```

已核实：`internal/dev` 原生支持 `Port: ":0"`（OS 选空闲端口）并通过 `addrCh` 回报实际地址；LiveReload 同源注入自动跟随实际端口。

### 托盘/菜单操作（直接调 Go，无 IPC）

| 菜单项 | 行为 |
|--------|------|
| 预览窗口 | 已开则前置，未开则创建窗口并 LoadURL |
| 启停预览服务 | stop：关 server goroutine、窗口显示离线页；start：重新装配启动 |
| 一键 build | 生产构建到 PublishDir，期间菜单项禁用，完成/失败走系统通知 |
| 打开站点目录 | Finder reveal 站点根目录 |
| 切换站点 | 子菜单：最近列表逐条 + 「选择其他目录…」→ 停 server → 换目录 → 重启 → 窗口 reload |
| 退出 | 先优雅停 server（watcher 停止、HTTP shutdown）再退出 |

### 状态机

状态：`idle` / `serving` / `building` / `error`。迁移函数：`StartServe` / `StopServe` / `SwitchSite` / `RebuildSite`——每个迁移单函数实现，带超时与幂等保护（重复点击合并）。

切换站点：`serving --StopServe--> idle --StartServe(新目录)--> serving`，窗口 reload 到新端口。

### SPA→Go 绑定（Phase 1 最小集）

目的是打通 TrustedOrigins + 类型安全 IPC 通路（评估换框架成本的关键面），SPA 代码一行不改：

```go
type Desktop struct{}
func (Desktop) AppVersion() string        // 版本号
func (Desktop) RevealSiteFolder() error   // Finder reveal
```

绑定生成客户端文件备用，不主动接 UI。

## 6. 错误处理

| 场景 | 处理 |
|------|------|
| 选的目录无 `huan.yaml` / 校验失败 | Dialog 报错，停在选目录环节，不进主窗口 |
| server 启动失败（端口/装配错误） | 系统通知 + 托盘状态置 `error`，菜单保留「重试」入口 |
| server goroutine panic | `recover` → 状态置 `error`，托盘图标变化 + 通知，菜单可重启 |
| build 失败 | 通知携带错误摘要，完整日志写 `UserConfigDir()/huan-desktop/logs/`，菜单「打开日志目录」 |
| 配置 JSON 损坏 | 备份旧文件后重置为默认，不阻塞启动 |
| 退出时 server 停不掉 | shutdown 带 5s 超时，超时强制退出（进程结束兜底） |

**可观测性**（遵循 CLAUDE.md 全链路可观测约定）：`internal/desktop` 各迁移函数进出打 JSON 结构化日志（timestamp / trace_id / span_id / event_type / payload），复用 huan 现有 observability 包；日志落 `UserConfigDir()/huan-desktop/logs/`。

## 7. 测试策略

| 层 | 方式 |
|----|------|
| `state.go` / `config.go` 纯逻辑 | 单测：状态机迁移、最近列表上限 10 条、配置损坏恢复 |
| `server.go` 装配 | 集成测试：`t.TempDir()` 造假站点 → 起 server → GET `/admin/` 与首页 200 → 停止无泄漏 |
| menu / tray / dialog / webview | MyGo 原生层不自动化，人工验收清单（写入验收文档） |
| `.app` 产物 | `mygo build` 后手工验收：双击启动、选目录、serve、build、切换站点、退出 |

## 8. 发布

- `mygo build -platform darwin/arm64` → `.app` + `.dmg` 归档到 `/release`（遵循项目发布约定，含版本号）
- Phase 1 无签名公证——首次打开需右键打开；发布文档写明
- go.mod 锁定 mygo v0.9.x（pre-1.0，不自动跟随升级）
- CLI 二进制（`cmd/huan`）不受影响，无 mygo 依赖进入其构建产物

## 9. 风险

| 风险 | 缓解 |
|------|------|
| MyGo v0.9.2 pre-1.0，API 可能变动 | 框架调用全部收在 `internal/desktop`，升级只动一个包；go.mod 锁 minor 版本 |
| admin SPA 同源假设 | SPA 的 API 调用走同源 fetch，加载 `127.0.0.1` URL 天然满足；实现时排查 SPA 是否有硬编码 origin |
| 随机端口与 LiveReload | 已核实支持（`Port: ":0"` + `addrCh`），实现时验证注入脚本端口跟随 |
| webview 内 LiveReload 会触发热刷新窗口 | 属预期行为（等同浏览器开发体验）；若干扰使用可在桌面端关掉注入开关（留待验收时判断） |
