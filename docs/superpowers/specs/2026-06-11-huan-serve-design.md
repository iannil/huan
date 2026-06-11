# huan serve 设计文档

**日期**: 2026-06-11
**状态**: 已批准（设计阶段），待转入实现计划
**作者**: 共同讨论（用户 + Claude）

## 目标

为 huan 静态站点生成器实现 `serve` 子命令，行为类似 `hugo serve`：在本地起 HTTP 服务、监听文件变化自动重建、通过 LiveReload 协议让浏览器自动刷新。

## 范围

### 包含
- HTTP 静态文件服务（端口默认 1313，可配置）
- 基于 fsnotify 的文件监听（递归，含新增子目录自动补加）
- debounce 后触发增量重建
- WebSocket LiveReload 服务端（vendor hugo 同款 livereload.js）
- HTML build 阶段注入 livereload `<script>` 标签
- CLI flags：`-D/--buildDrafts`、`--port`、`--bind`、`--debounce`、`--disableLiveReload`、`--disableWatch`
- 构建产物写入系统临时目录，进程退出自动清理

### 不包含（明确排除）
- `--navigateToChanged`（自动开浏览器到变更页面）
- HTTPS
- 多 server 协作（`--appendPort` 等）
- 端口冲突时自动找空闲端口（改为直接报错退出）
- 新增 CI 配置

## 现状

- huan 是 Go 写的 Hugo 替代品，阶段一目标：输出与 Hugo 字节级一致
- `cmd/huan/main.go`（858 行）已有完整 `build` 子命令，`serve` 仅 stub（打印 "Serve not yet implemented."）
- 二进制尚未安装到 PATH，通过 `./huan -s <project>` 形式调用
- 用户工作流：在 `/Users/rong.zhu/Code/zhurongshuo`（内容项目）下通过 `huan -s . serve` 调用

## 文件布局

```
huan/
├── cmd/huan/
│   ├── main.go              (从 858 行降到 ~80 行，只剩 CLI 注册)
│   └── serve.go             (runServe 的 cobra handler，调 internal/serve)
├── internal/
│   ├── build/
│   │   ├── build.go         (BuildSite 函数 + Options + Result)
│   │   ├── context.go       (从 main.go 搬过去的 build*Context 系列 helper)
│   │   └── summary.go       (truncateHTMLByWords / stripHTML / countWords)
│   └── serve/
│       ├── server.go        (HTTP file server + livereload 路由)
│       ├── watcher.go       (fsnotify 封装 + debounce)
│       ├── livereload.go    (WebSocket hub + 客户端管理)
│       └── livereload.js    (vendor livereload-js v4，//go:embed)
└── go.mod                    (新增 fsnotify + coder/websocket)
```

**搬移规则**：所有"渲染一次站点"相关逻辑进 `internal/build/`；所有"持续服务"相关进 `internal/serve/`；`cmd/huan/` 只做参数解析与组装。

## §1 — BuildSite 抽取

把 `cmd/huan/main.go` 里 runBuild 的核心（第 56-425 行）抽成可复用函数：

```go
// internal/build/build.go
type Options struct {
    SourceDir        string
    OutputDir        string  // 绝对路径；build 用 sourceDir+publishDir，serve 用临时目录
    IncludeDrafts    bool    // -D
    InjectLiveReload bool    // serve 专用；只在 true 时使用 LiveReloadURL
    LiveReloadURL    string  // 形如 "ws://localhost:1313/livereload"；空字符串时不注入
    Logf             func(format string, args ...any)  // nil 时默认 fmt.Printf
}

type Result struct {
    PagesRendered int
    FilesWritten  int
    BytesWritten  int64
    Errors        int
    Duration      time.Duration
}

func BuildSite(opts Options) (*Result, error)
```

### 重构纪律
- **行为保持不变** —— 阶段一目标是与 Hugo 字节级一致，现有 diff 测试（`scripts/diff-build.sh`）重构后必须仍通过
- **不修 bug、不加优化**：搬移过程中发现的"看起来不对"的代码先记下来，不顺手改 —— 改了会让 diff 测试结果难归因
- **现有 helper 函数**（`resolveTemplateName` / `buildTaxonomyContext` / `truncateHTMLByWords` 等）整组搬到 `internal/build/context.go` 和 `summary.go`，签名不变
- **新增参数** 走 Options struct，避免函数签名变长
- **runBuild cobra handler** 变成 5-10 行：解析 flags → 构造 Options → 调 BuildSite → 打印结果

## §2 — HTTP 服务器（internal/serve/server.go）

```go
type Server struct {
    sourceDir string
    outputDir string
    bind      string
    port      string  // 保持 string 类型与 CLI flag 一致（兼容 ":1313" 写法）；监听前转 int
    hub       *LiveReloadHub
    logf      func(format string, args ...any)
}

func New(opts ServerOptions) *Server
func (s *Server) Run(ctx context.Context) error  // 阻塞，监听 SIGINT/SIGTERM
```

### 路由
| 路径 | 行为 |
|---|---|
| `GET /livereload.js` | 返回 embed 的 JS（缓存 1 小时） |
| `GET /livereload` | WebSocket 升级，挂到 hub |
| 其他所有 | 从 `outputDir` 起静态文件服务 |

静态文件服务用 `http.FileServer(http.Dir(outputDir))`，无中间件改写（livereload 脚本由 build 阶段注入到 HTML 文件中，详见 §5），依赖标准库默认的路径清理防穿越。

### 生命周期
```
1. 创建临时目录（os.MkdirTemp("", "huan-serve-*")）
2. 第一次 BuildSite（InjectLiveReload=true）
3. 启动 watcher
4. 启动 HTTP server
5. (阻塞) 等 ctx done 或 SIGINT/SIGTERM
6. Shutdown HTTP → 关闭 watcher → os.RemoveAll(tmpDir)
```

### Livereload 脚本注入
在 **build 阶段**注入到 HTML 文件中（而非 HTTP 响应时改写）。理由：
- 省事，不用读 body 改写流式 HTML
- 避免改写 HTML 容易出 bug
- 临时目录多脚本标签不影响真实部署（不污染 docs/）

注入位置：`</head>` 前。

## §3 — 文件监听（internal/serve/watcher.go）

```go
type Watcher struct {
    sourceDir string
    fsw       *fsnotify.Watcher
    debounce  time.Duration  // 默认 400ms
    onChange  func()
    logf      func(format string, args ...any)
}

func New(opts WatcherOptions) (*Watcher, error)
func (w *Watcher) Run(ctx context.Context) error
```

### 监听路径
基于 build 代码里所有 `filepath.Join(sourceDir, ...)` 反推：
- `content/`
- `data/`
- `i18n/`
- `static/`
- `layouts/`（如果存在）
- `themes/<theme>/layouts/`
- `themes/<theme>/static/`
- `themes/<theme>/i18n/`
- `huan.yaml`（单文件，根目录）

### 递归监听
fsnotify 不递归，需 `filepath.Walk` 添加子目录。新增子目录（收到 `Create` 事件判定为目录）补加监听 —— 否则用户新建 content/posts/new-post/ 时监听不到。

### Debounce
```
事件 1 ──┐
事件 2 ──┼─► 重置 400ms 定时器 ──► 时间到，触发 onChange
事件 3 ──┘                         (期间有新事件再次重置)
```
默认 400ms，可通过 `--debounce` flag 配置。

### 忽略规则
- `outputDir` 本身不能监听（否则 build 写入会循环触发）
- 隐藏文件（`.DS_Store`、`.swp` 等）：以 `.` 开头的路径忽略
- sourceDir 之外的路径不监听

## §4 — LiveReload（internal/serve/livereload.go）

### 协议
LiveReload 标准 1.4 协议（official-7），与 hugo/livereload-js v4 配套。

```go
type LiveReloadHub struct {
    clients map[*websocket.Conn]struct{}
    mu      sync.RWMutex
    logf    func(format string, args ...any)
}

func NewHub() *LiveReloadHub
func (h *LiveReloadHub) HandleConn(ctx context.Context, c *websocket.Conn)
func (h *LiveReloadHub) BroadcastReload()
func (h *LiveReloadHub) BroadcastAlert(message string)
```

### 握手流程
```
1. 浏览器加载 HTML，遇到 <script src="/livereload.js?mindelay=10&v=2"></script>
2. livereload.js 连接 ws://<host>/livereload
3. 服务端先发 hello:
   {"command":"hello","protocols":["http://livereload.com/protocols/official-7"],
    "serverName":"huan"}
4. 客户端回 hello
5. 文件重建完成后，服务端广播:
   {"command":"reload","path":"/","liveCSS":true}
6. 客户端收到 reload → 刷新页面
```

### 实现要点
- 用 `github.com/coder/websocket` 的 `Accept` + `Read` / `Write`，每连接一个 goroutine
- 客户端异常断开从 hub 移除，不影响其他
- `BroadcastReload()` 非阻塞写（100ms 超时），防慢客户端拖住重建路径
- Hub 不持有任何文件/状态，纯转发器

### vendor livereload.js
- 来源：[livereload/livereload-js](https://github.com/livereload/livereload-js) `dist/livereload.js`
- 版本：v4 稳定线最新版（具体版本号 + dist 文件 SHA256 在拉取时锁定，写入文件头部注释）
- 文件：`internal/serve/livereload.js`
- 嵌入：`//go:embed livereload.js` 导出为 `[]byte`

### 注入 HTML
```html
<script src="/livereload.js?mindelay=10&v=2"
        data-livereload-port="1313"
        data-livereload-host="localhost"></script>
```
插入位置：`</head>` 前（由 build 阶段处理）。

## §5 — CLI 表面

```bash
# 默认用法
huan serve -s /path/to/zhurongshuo

# 带草稿
huan serve -s /path/to/zhurongshuo -D

# 自定义端口
huan serve -s /path/to/zhurongshuo --port 8080

# 暴露到局域网
huan serve -s /path/to/zhurongshuo --bind 0.0.0.0
```

### serve 子命令 flags

| flag | 类型 | 默认 | 说明 |
|---|---|---|---|
| `-s, --source` | string | `.` | 项目根（与 build 共用 PersistentFlag） |
| `-D, --buildDrafts` | bool | false | 包含 draft 内容 |
| `--port` | string | `"1313"` | HTTP 端口 |
| `--bind` | string | `"127.0.0.1"` | 监听地址 |
| `--debounce` | duration | `400ms` | 文件变化 debounce 时长 |
| `--disableLiveReload` | bool | false | 关闭浏览器自动刷新（仍监听+重建） |
| `--disableWatch` | bool | false | 完全不监听文件，等同 build + 一次性 HTTP 服务 |

### 与 hugo 的差异
- ❌ `--navigateToChanged`：先不做
- ❌ `--renderToMemory`：当前已走临时目录，行为等价
- ❌ HTTPS
- ❌ 端口冲突自动 +1：改为直接报错退出

### 启动日志
```
Building site: 祝融说。
  Source:      /Users/rong.zhu/Code/zhurongshuo
  Output:      /var/folders/xx/T/huan-serve-xxxx
  Pages:       247
  Templates:   38
  Rendered:    247 pages

Watching for changes in: content/, data/, i18n/, static/, themes/zozo/, huan.yaml
Serving at:   http://localhost:1313/
LiveReload:   ws://localhost:1313/livereload

Press Ctrl+C to stop
```

### 重建日志
```
[12:34:56] Change detected: content/posts/new.md
[12:34:56] Rebuilding... (12 files changed)
[12:34:57] Done in 380ms. LiveReload broadcasted to 2 clients.
```

## §6 — 错误处理与 UX

### build 失败时（重建路径）
- **不退出进程** —— serve 保持运行，否则用户每次保存手抖都得起服务
- 终端打印错误：`[12:34:56] ERROR: render content/posts/new.md with _default/single.html: template: ...`
- LiveReload 广播 `{"command":"alert","message":"..."}`，浏览器右上角弹错误提示（livereload.js 原生支持）
- 已构建的旧版本继续可访问 —— 文件已写入临时目录，不会因为单次失败而失效

### 首次启动 build 失败
真错误（配置错、模板错），直接退出非 0，完整错误打到 stderr。

### WebSocket 异常
- 单个客户端断开：从 hub 移除，记一行 debug 日志
- hub 广播超时（100ms 内某客户端没接）：丢弃该客户端这条消息，不阻塞主路径

### 临时目录清理
- `defer os.RemoveAll(tmpDir)` + 信号 handler 双保险
- 进程被 SIGKILL 时无法清理 —— 文件在 `$TMPDIR` 下，OS 定期清理
- 启动时发现旧 `huan-serve-*` 目录（很久前的 mtime），打印 warning，不自动删（避免误删别人的运行实例）

### 端口冲突
直接报错退出：`ERROR: port 1313 already in use. Try --port 1314.`

### 监听失败（fsnotify 在 SMB/网络盘不工作）
启动时 warning：`WARNING: file watcher not available on this filesystem. Use --disableWatch to suppress this warning.`
仍然起 HTTP 服务，用户改文件后手动重启。

## §7 — 依赖与测试

### 新增依赖

| 包 | 版本 | 用途 | 备注 |
|---|---|---|---|
| `github.com/fsnotify/fsnotify` | `v1.7.0` | 文件监听 | hugo 也用，事实标准；纯 Go |
| `github.com/coder/websocket` | `v1.8.x` | WebSocket | 原 `nhooyr.io/websocket`；活跃维护；纯 Go |

均走 `go get`，无 cgo，跨平台（含 macOS Darwin）。

### 测试策略

| 测试类型 | 范围 | 工具 |
|---|---|---|
| **build 重构不回归** | BuildSite 输出与重构前字节级一致 | 现有 `scripts/diff-build.sh` |
| **build 单元** | `truncateHTMLByWords` / `countWordsInPlain` / `urlEscape` 等 helper | 现有测试搬到新包后跑通 |
| **serve HTTP** | 启 server → httptest 发请求 → 验证 livereload.js 路由、静态文件、HTML 注入 | `net/http/httptest` |
| **serve LiveReload** | 启 hub → 连一个 ws 客户端 → 广播 → 验证收到 reload 消息 | `coder/websocket` 客户端 + 短超时 |
| **serve watcher** | 创建 tmpdir → 启 watcher → 写文件 → 等 callback 触发 | 真实 fsnotify（无 mock） |
| **serve debounce** | 连发 5 个事件 → 验证只触发一次 onChange | 时间断言带 buffer |
| **端到端** | 起 serve → ws 客户端模拟浏览器 → 改文件 → 验证 reload 到达 | ws 客户端模拟，不引入 chromedp |

### 不测试的
- 跨平台文件监听（CI Linux，本地 mac，网络盘手动验证）
- 真实浏览器 LiveReload 行为（vendor livereload.js 假定它对）

### CI 影响
现有 huan 项目无 CI（`scripts/` 都是手动 shell），本次不新增。测试代码先行，本地 `go test ./...` 跑通即可。

## 验收标准

1. `go build -o huan ./cmd/huan` 成功
2. `./huan -s /Users/rong.zhu/Code/zhurongshuo serve` 起服务，浏览器访问 http://localhost:1313/ 看到站点
3. 修改 `content/` 下任一 markdown 文件保存，浏览器自动刷新看到新内容
4. 重构后 `./scripts/diff-build.sh` 与 Hugo 输出仍字节级一致
5. `go test ./...` 全绿
6. SIGINT 退出后临时目录被清理

## 后续可能（不在本次范围）

- `--navigateToChanged`：自动打开浏览器到变更页面
- 增量构建（仅重建受影响页面）
- HTTPS 支持
- CI 集成
