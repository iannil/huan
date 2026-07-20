# huan daemon 设计文档

> 版本: v0.6
> 日期: 2026-07-20
> 状态: 设计阶段

## 1. 定位变更

huan 从一个静态站点生成器（SSG）扩展为**一体化内容引擎 + 生产级后端服务**。

| 命令 | 定位 | 说明 |
|------|------|------|
| `huan build` | 保持现有 | SSG 能力，输出静态 HTML |
| `huan dev` | 重命名（原 serve） | 开发服务器，LiveReload + watcher + 开发 Admin |
| `huan daemon` | 新增 | 生产级常驻服务，混合渲染 + API + Admin |
| `huan serve` | 废弃 | 标记 deprecated，提示使用 `huan dev`，下一版本移除 |

版本号从 v0.5.0 提升到 v0.6.0。

## 2. 整体架构

### daemon 内部架构

```
┌─────────────────────────────────────────────────────────────┐
│                    huan daemon 内部                            │
│                                                             │
│  ┌─────────────────────┐     ┌───────────────────────────┐  │
│  │      Builder         │     │        Serving            │  │
│  │                      │     │                           │  │
│  │  ┌─────────────────┐ │     │  ┌─────────────────────┐  │  │
│  │  │ Build Pipeline  │ │     │  │ Static File Server  │  │  │
│  │  │ (8 阶段管线)    │ │     │  │ (预渲染 HTML)       │  │  │
│  │  └────────┬────────┘ │     │  └─────────────────────┘  │  │
│  │           │          │     │                           │  │
│  │  ┌────────▼────────┐ │     │  ┌─────────────────────┐  │  │
│  │  │ Dependency      │ │     │  │ JIT Renderer        │  │  │
│  │  │ Graph (DAG)     │ │     │  │ (按需回退渲染)      │  │  │
│  │  └────────┬────────┘ │     │  └─────────────────────┘  │  │
│  │           │          │     │                           │  │
│  │  ┌────────▼────────┐ │     │  ┌─────────────────────┐  │  │
│  │  │ Warm Cache      │ │     │  │ Admin API           │  │  │
│  │  │ (预热写入缓存)  │ │     │  │ (内容管理)          │  │  │
│  │  └────────┬────────┘ │     │  └─────────────────────┘  │  │
│  └───────────┼──────────┘     └───────────┬───────────────┘  │
│              │                            │                  │
│              └──────────┬─────────────────┘                  │
│                         ▼                                    │
│              ┌──────────────────────┐                        │
│              │     EventBus         │                        │
│              │  (接口抽象)          │                        │
│              └──────────────────────┘                        │
│                                                             │
│  ┌────────────────────────────────────────────────────┐     │
│  │          Infrastructure Layer                       │     │
│  │  ┌──────────┐ ┌──────────┐ ┌────────┐ ┌────────┐  │     │
│  │  │  Health   │ │ Metrics  │ │  TLS   │ │ Grace- │  │     │
│  │  │ (/health) │ │(/metrics)│ │        │ │ ful    │  │     │
│  │  └──────────┘ └──────────┘ └────────┘ └────────┘  │     │
│  └────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

### 设计原则

1. **Builder 与 Serving 独立演化** — 通过 EventBus 解耦，Builder 专注于内容渲染管线，Serving 专注于 HTTP 服务
2. **混合缓存策略** — 全量预渲染为主，增量更新为常态，JIT 按需渲染为回退
3. **依赖图精确重建** — 内容变更时只重建受影响页面，减少不必要的全量 rebuild
4. **单二进制部署** — daemon 与 build/dev 共享同一二进制，无外部依赖

## 3. 渲染缓存设计

### 三层缓存结构

```go
// FullCache: 全量缓存 — 启动时全量 build 输出的文件系统目录
// Serving 的 Static File Server 直接 http.FileServer(http.Dir(tmpDir))，零拷贝
type FullCache struct {
    Root    string    // tmpDir 路径
    BuiltAt time.Time
    Version uint64
}

// IncrCache: 增量更新 — 内容变更时只重建受影响页面
// 重建结果直接覆盖 tmpDir 中对应文件

// JITCache: 按需回退缓存 — 请求路径不存在于 tmpDir 时
type JITCache struct {
    mu       sync.RWMutex
    entries  map[string]*JITEntry
    maxSize  int64     // 最大条目数
}

type JITEntry struct {
    Path    string
    HTML    []byte
    Size    int64
    RenderedAt time.Time
    TTL     time.Duration
}
```

### 缓存失效策略

| 触发条件 | 动作 | 影响范围 |
|---------|------|---------|
| 启动全量 build | 完全初始化 tmpDir，清空 JITCache | 全部 |
| 新增/删除文件 | 全量 rebuild（staging → swap） | 全部 |
| 修改文件 | 增量更新（DAG 推导受影响页面） | 受影响的页面 + JITCache 中对应条目 |
| JIT 条目 TTL 过期 | 重新渲染单个页面 | 单条目 |
| JITCache 满 | LRU 淘汰 | 最久未命中条目 |

## 4. 依赖图设计

### 节点定义

```go
type Node struct {
    PagePath   string   // 页面路径，如 /posts/hello/
    SourceFile string   // 源文件路径，如 content/posts/hello.md
    Kind       string   // page / section / home / term
    DependsOn  []string // 依赖的页面路径
    DependedBy []string // 被哪些页面依赖
}

type DependencyGraph struct {
    mu      sync.RWMutex
    nodes   map[string]*Node   // 页面路径 → 节点
    sources map[string]string  // 源文件路径 → 页面路径
}
```

### 依赖推导规则

| 页面类型 | 依赖哪些页面 |
|---------|------------|
| 文章页面 | 所属 section 列表页，首页，标签页，RSS |
| Section 列表页 | 其下所有文章 |
| 首页 | 所有文章（通过 paginate 截断） |
| 标签页 | 所有带该标签的文章 |
| RSS | 对应 section 下的所有文章 |
| sitemap.xml | 所有页面 |
| taxonomy 列表页 | 所有文章 |

### 增量更新算法

```
输入: 变更文件列表 []string

1. 找到变更文件对应的页面节点
2. BFS 从变更节点出发，沿 DependedBy 反向遍历，收集所有需要重建的页面
3. 按依赖排序（被依赖的优先重建）
4. 逐个渲染受影响页面（复用 build 管线中的单页渲染能力）
5. 更新 DAG 中受影响节点的信息
6. 持久化到 tmpDir/.dag.json
7. 发布 EventCacheUpdated 事件
```

## 5. 事件总线设计

### 接口

```go
type EventType int

const (
    EventContentChanged  EventType = iota // 内容变更
    EventCacheUpdated                     // 缓存已更新
    EventBuildStarted                     // 构建开始
    EventBuildCompleted                   // 构建完成
    EventBuildFailed                      // 构建失败
    EventServerStart                      // 服务启动
    EventServerShutdown                   // 服务关闭中
)

type Event struct {
    Type      EventType
    Timestamp time.Time
    Payload   any
}

type Handler func(ctx context.Context, event Event) error

type EventBus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventType EventType, handler Handler)
    Unsubscribe(eventType EventType, handlerID string)
    Close() error
}
```

### 默认实现：ChannelBus

- 异步扇出：每个 handler 独立 goroutine
- handler 超时：默认 30s
- 错误处理：handler 返回 error 仅记录日志，不中断其他 handler

### 订阅关系

| 订阅者 | 事件 | 响应 |
|-------|------|------|
| Builder | EventContentChanged | 触发增量更新或全量 rebuild |
| Serving | EventCacheUpdated | 通知 LiveReload（dev 模式）或刷新缓存状态 |
| Metrics | EventBuildStarted / Completed / Failed | 更新 Prometheus 指标 |
| Health | EventServerShutdown | 标记健康检查为 not ready |

## 6. Serving 路由设计

```
/                      → 页面请求：StaticFileServer → JIT 回退 → 404
/admin/*               → Admin Handler（现有，增强安全边界）
/health                → 健康检查（启动中/正常/关闭中）
/metrics               → Prometheus 指标
```

### Middleware 链

```
Inbound Request → RequestLogging → Recovery → Prometheus → Auth(admin only) → Router → Handler
```

### JIT 渲染流程

1. 检查 tmpDir 中是否存在对应文件 → 存在则直接返回
2. 不存在 → 检查 JITCache 是否有缓存
3. 有且未过期 → 返回缓存
4. 无/已过期 → 实时渲染：
   - 解析请求路径 → 找到对应内容文件
   - 调用 `build.RenderPage()`
   - 写入 tmpDir + JITCache
   - 返回渲染结果
5. 渲染失败 → 404

## 7. 启动流程

```
1. 解析 CLI 参数
2. 加载 huan.yaml + daemon.yaml 配置
3. 初始化 EventBus
4. 初始化 RenderCache
5. 启动 Builder（goroutine）：
   a. 执行全量 build → 写入 tmpDir
   b. 构建 DAG → 持久化到 tmpDir/.dag.json
   c. 订阅 EventContentChanged
6. 启动 Serving（HTTP）：
   a. Static File Server
   b. JIT Renderer
   c. Admin API
   d. Health / Metrics
7. 启动基础设施层：TLS / systemd / graceful shutdown / fsnotify
8. 阻塞等待退出信号
```

## 8. 包结构

```
internal/
├── build/          ← 现有，8 阶段管线，新增 RenderPage() 单页渲染
├── admin/          ← 现有，增强安全边界
├── dev/            ← 从 serve/ 重命名（原 huan serve → huan dev）
├── daemon/         ← NEW
│   ├── daemon.go           — 启动/关闭流程编排
│   ├── builder.go          — Builder: 全量 build + 增量更新
│   ├── serving.go          — Serving: HTTP 服务组合
│   ├── cache/
│   │   ├── cache.go        — RenderCache: FullCache + IncrCache + JITCache
│   │   └── jit.go          — JITCache: LRU + TTL
│   ├── dag/
│   │   ├── graph.go        — DependencyGraph: 构建 + BFS + 序列化
│   │   └── rules.go        — 依赖规则推导
│   ├── eventbus/
│   │   ├── bus.go          — EventBus 接口 + ChannelBus 实现
│   │   └── types.go        — EventType, Event, Handler 定义
│   ├── metrics.go          — Prometheus metrics
│   ├── health.go           — Health check handler
│   └── lifecycle.go        — systemd notify, graceful shutdown

cmd/huan/
├── main.go         ← 新增 daemonCmd + devCmd
├── dev.go          ← 原 serve.go 重写为 huan dev
├── daemon.go       ← NEW: huan daemon 命令定义
└── (其余不变)
```

## 9. CLI 与配置

### CLI

```bash
# daemon 命令
huan daemon --port 8080 --bind 0.0.0.0
huan daemon --systemd
huan daemon --tls-cert /etc/huan/cert.pem --tls-key /etc/huan/key.pem

# dev 命令（原 serve）
huan dev --port 1313 --bind 127.0.0.1

# serve 命令（废弃，提示使用 dev）
huan serve → "use 'huan dev' instead"
```

### daemon.yaml

```yaml
daemon:
  bind: "0.0.0.0"
  port: 8080
  tls:
    cert: ""
    key: ""
  health:
    enabled: true
    path: "/health"
  metrics:
    enabled: false
    path: "/metrics"
  cache:
    jit:
      max_entries: 1000
      ttl: "5m"
  build:
    interval: "0"            # 0 = 仅事件触发
    max_concurrent: 1
    include_drafts: false
  admin:
    enabled: true
    bind: ""
    token: ""                 # 空 = 从环境变量读取
```

## 10. 测试策略

### 单元测试

- DAG 构建/BFS/序列化
- JITCache LRU/TTL
- EventBus 发布/订阅/超时
- 依赖规则推导

### 集成测试

- Builder 全量 build → Serving 访问页面
- 内容变更 → Builder 增量更新 → 页面更新可访问
- 内容新增/删除 → 全量 rebuild
- JIT 渲染回退
- daemon 启动 → 健康检查 → 优雅关闭

### E2E 测试

- 完整 daemon 生命周期：启动 → Admin 创建内容 → 页面可访问 → 修改内容 → 页面更新
- 多轮增量更新 + 全量 rebuild 混合场景

## 11. 实施路线

### Phase 1（v0.6.0）

- [ ] `huan dev` 重命名（原 serve 改名）
- [ ] `huan serve` 标记 deprecated
- [ ] `huan daemon` 基础骨架：EventBus + daemon 启动流程
- [ ] Serving: Static File Server + Admin API + Health + Metrics
- [ ] Builder: 全量 build 管线集成（启动时全量构建）
- [ ] 基础测试覆盖

### Phase 2（v0.6.x）

- [ ] DAG：依赖图构建 + 序列化/反序列化
- [ ] 增量更新：内容变更 → DAG 推导 → 仅重建受影响页面
- [ ] JIT 渲染回退 + JITCache
- [ ] TLS / systemd / graceful shutdown
- [ ] 性能测试与优化

### Phase 3（v0.7+）

- [ ] 增量更新支持新增/删除文件（当前回退全量 rebuild）
- [ ] 实时构建队列优化
- [ ] 定时全量重建
