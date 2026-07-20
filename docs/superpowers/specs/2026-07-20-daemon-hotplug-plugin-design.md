# 设计文档：Daemon 热插拔插件系统（Hot-Pluggable Plugin System）

- **日期**：2026-07-20
- **状态**：Draft
- **关联 ADR**：[ADR 0003 — 统一插件系统](docs/adr/0003-unified-plugin-system.md)
- **实现阶段**：v0.7.0

## 1. 背景

huan daemon 已具备持久化运行的服务端能力（HTTP 服务、构建管线、文件监听、Admin API），但插件的加载方式仅支持**编译期注册**（composition root 模式，见 ADR 0003 §7）。这限制了 daemon 运行时扩展的能力：

- 新增插件需要重新编译整个二进制
- 无法在运行时热加载/卸载插件
- 无法通过 Admin API 管理插件生命周期

本设计在现有统一插件系统基础上，增加**运行时热插拔能力**，采用 Go plugin + gRPC 预留的混合分层方案。

## 2. 设计目标

1. **热插拔**：daemon 运行期间可以加载/卸载/重载插件，无需重启
2. **兼容现有系统**：不改动已有 Plugin 接口、Registry、capability 查询（Find[T]）
3. **安全可控**：插件操作需要鉴权 + 审计
4. **预留扩展**：gRPC 接口定义骨架，不强制实现，等跨语言需求时再落地
5. **渐进演进**：先做 Go plugin 层，后续增量添加 gRPC 层

## 3. 架构总览

```
┌─────────────────────────────────────────────────────────────────┐
│                    huan daemon (runtime)                         │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │               Plugin Lifecycle Manager (新增)             │     │
│  │  ┌───────────┐  ┌──────────┐  ┌─────────────────────┐  │     │
│  │  │ Loader    │  │ Watcher  │  │ Lifecycle Hooks      │  │     │
│  │  │ (.so scan)│  │ (fsnotify│  │ (onLoad/onUnload/   │  │     │
│  │  │           │  │ 热重载)   │  │  onReload)           │  │     │
│  │  └─────┬─────┘  └────┬─────┘  └──────────┬──────────┘  │     │
│  │        └──────────────┴───────────────────┘              │     │
│  │                       │                                  │     │
│  │                       ▼                                  │     │
│  │  ┌─────────────────────────────────────────────────┐     │     │
│  │  │           Plugin Registry (增强已有)              │     │     │
│  │  │  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │     │     │
│  │  │  │ compiled │  │ loaded   │  │ grpc proxy    │  │     │     │
│  │  │  │ (原有)    │  │ (新增.so)│  │ (预留stub)     │  │     │     │
│  │  │  └──────────┘  └──────────┘  └───────────────┘  │     │     │
│  │  └─────────────────────────────────────────────────┘     │     │
│  └─────────────────────────────────────────────────────────┘     │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐ │
│  │  EventBus    │  │  Admin API   │  │  HTTP/Serving (已有)    │ │
│  │  (已有)       │  │  (已有+扩展)  │  │  + gRPC stub (新增)     │ │
│  └──────────────┘  └──────────────┘  └────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 3.1 分层说明

| 层 | 机制 | 用途 | 隔离性 |
|----|------|------|--------|
| Layer 1: Go plugin | `plugin.Open` 加载 `.so` | 进程内扩展（deploy/翻译/内容处理） | 同一进程 |
| Layer 2: gRPC | Protobuf 协议（预留骨架） | 跨语言扩展（付费/会员/外部服务） | 独立进程 |

## 4. 新增组件

### 4.1 Loader（`internal/plugin/loader.go`）

扫描并加载 `.so` 插件文件。

```go
type Loader struct {
    pluginDir string
}

func NewLoader(pluginDir string) *Loader

// LoadPlugin 加载单个 .so 文件，返回 plugin.Plugin 实例
// 步骤：plugin.Open → Lookup("InitPlugin") → 类型断言 → 调用 → 返回
func (l *Loader) LoadPlugin(path string) (Plugin, error)

// ScanAndLoad 扫描 pluginDir 下所有 .so 文件并批量加载
// 部分失败不中断整体流程，跳过无效文件
func (l *Loader) ScanAndLoad() ([]Plugin, error)

// PluginInitFunc 是 .so 插件必须导出的初始化函数签名
type PluginInitFunc func(cfg map[string]any) (Plugin, error)
```

### 4.2 LifecycleManager（`internal/plugin/lifecycle.go`）

管理插件的完整生命周期。

```go
type LifecycleManager struct {
    registry *Registry
    loader   *Loader
    watcher  *PluginWatcher
    bus      eventbus.EventBus
}

func NewLifecycleManager(registry *Registry, loader *Loader, bus eventbus.EventBus) *LifecycleManager

func (m *LifecycleManager) Start(ctx context.Context) error
func (m *LifecycleManager) Stop() error
func (m *LifecycleManager) Load(path string) (Plugin, error)
func (m *LifecycleManager) Unload(name string) error
func (m *LifecycleManager) Reload(name string) (Plugin, error)
func (m *LifecycleManager) List() []PluginInfo
```

**`Start()` 流程：**
1. 扫描 `pluginDir` 下所有 `.so` 文件
2. 逐个加载到 Registry
3. 对每个成功加载的插件发布 `EventPluginLoaded`
4. 启动文件监听（`PluginWatcher`），检测新增/变更的 `.so` 自动热重载

**`Stop()` 流程：**
1. 停止文件监听
2. 遍历所有已加载插件，逐个从 Registry 移除
3. 对每个卸载发布 `EventPluginUnloaded`

**`Reload(name)` 流程：**
1. 从 Registry 获取当前插件实例
2. 记录其 source path
3. 从 Registry 移除
4. 加载新版本
5. 成功 → 注册并发布 `EventPluginReloaded`
6. 失败 → 重新注册旧版本（回滚）

### 4.3 PluginInfo（插件元数据）

```go
type PluginInfo struct {
    Name       string `json:"name"`
    Version    string `json:"version"`
    Source     string `json:"source"`     // "compiled" | "loaded" | "grpc"
    Capability string `json:"capability"`
    Status     string `json:"status"`     // "active" | "inactive" | "error"
    LoadedAt   string `json:"loadedAt,omitempty"`
    Error      string `json:"error,omitempty"`
}
```

### 4.4 GRPCStub（`internal/plugin/grpc_stub.go`）

gRPC 通信的预留骨架。首期只定义接口和空实现，**不引入 gRPC 依赖**。

```go
var ErrGRPCNotImplemented = errors.New("plugin: gRPC not implemented yet")

type GRPCPlugin interface {
    Plugin
    Capability() string
    Call(ctx context.Context, method string, payload []byte) ([]byte, error)
    Health(ctx context.Context) error
}

type GRPCStub struct {
    name       string
    capability string
    address    string
}
```

## 5. 与现有系统的集成

### 5.1 Registry 增强

现有 `Registry` 新增 `Unregister` 方法以支持运行时卸载：

```go
// Unregister 移除指定名称的插件。如果插件不存在，返回 nil。
func (r *Registry) Unregister(name string) bool
```

### 5.2 EventBus 新事件

```go
// eventbus/types.go 新增
const (
    EventPluginLoaded   EventType = iota + 10
    EventPluginUnloaded
    EventPluginReloaded
    EventPluginError
)
```

### 5.3 Daemon 启动流程集成

`internal/daemon/daemon.go` 在 Init Builder 之后、Init Serving 之前插入：

```
Run() 原有流程:
  1. Load config
  2. Init EventBus
  3. Init Cache
  4. Init Health + Metrics
  5. Create temp dir
  6. Init DAG
  7. Init Builder
  8. Init Plugin Lifecycle Manager      ← 新增
  9. Init Serving (注入 LifecycleManager)
  10. Subscribe event handlers
  11. Initial full build
  ...
```

```go
// Daemon 结构体新增
type Daemon struct {
    // ... 原有字段
    pluginManager *plugin.LifecycleManager
}

// Options 新增
type Options struct {
    // ... 原有字段
    PluginDir     string
    DisablePlugin bool
}
```

### 5.4 Admin API 新增端点

```
GET    /admin/api/plugins          → 列出所有插件（含状态/版本/能力）
POST   /admin/api/plugins/load     → 加载一个 .so 插件文件
POST   /admin/api/plugins/unload   → 卸载一个已加载的插件
POST   /admin/api/plugins/reload   → 热重载指定插件
```

所有端点受现有 `TokenMiddleware`（L2 鉴权）保护。

### 5.5 CLI 命令扩展

```
huan plugin list --all             # 列出所有插件（编译期 + 已加载 .so）
huan plugin load <path>            # 运行时加载一个 .so 插件
huan plugin unload <name>          # 运行时卸载一个插件
huan plugin reload <name>          # 热重载一个插件
```

CLI 子命令优先通过 HTTP 调用 daemon 的 Admin API；daemon 未运行时回退到直接加载（开发模式）。

## 6. Error Handling

### 6.1 Loader 层

| 错误场景 | 处理方式 |
|---------|---------|
| `.so` 文件无法打开 | 跳过，log warn，不中断批量扫描 |
| 缺少 `InitPlugin` 符号 | 返回 `ErrMissingInitSymbol` |
| `InitPlugin` 返回 error | 记录详细错误，跳过 |
| 插件 Name() 与已有冲突 | 返回 `ErrPluginNameConflict` |
| 编译版本不兼容 | Go plugin 原生 error，包装传递 |

### 6.2 LifecycleManager 层

| 操作 | 错误处理 |
|------|---------|
| Load | Loader → Registry → 发布事件；失败则回滚 |
| Unload | 先从 Registry 移除 → 发布事件；不存在返回 `ErrPluginNotFound` |
| Reload | 保存旧实例 → Unload → Load → 失败则回滚到旧实例 |

### 6.3 Admin API 层

| 错误 | HTTP Status |
|------|------------|
| `ErrPluginNotFound` | 404 |
| `ErrPluginNameConflict` | 409 |
| `ErrMissingInitSymbol` | 400 |
| 其他 | 500 |

## 7. 安全边界

1. **Admin API 鉴权（L2）**：复用现有 `TokenMiddleware`
2. **操作审计（L4）**：load/unload/reload 记录到 audit log
3. **路径安全**：只加载 `--plugin-dir` 指定目录下的 `.so`，清理路径穿越
4. **禁用开关**：`--disable-plugin` 参数可完全禁用插件加载
5. **进程隔离**：Go plugin 在同一进程（弱隔离），gRPC 插件可实现强隔离

## 8. 测试策略

### 8.1 单元测试

- Loader：成功加载 / 缺少符号 / Init 失败 / 目录不存在 / 部分失败
- LifecycleManager：Load/Unload/Reload/事件发布/重名冲突/Reload回滚

### 8.2 集成测试

- Daemon 启动带 pluginDir
- Admin API 调用 load/unload/reload 端点
- 验证插件出现在 Registry 中
- 验证 EventBus 事件被发布

### 8.3 Test Fixture

`internal/plugin/testdata/` 目录下放置测试用 .so 插件源码，通过 `go build -buildmode=plugin` 预编译。

## 9. 文件变更清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/plugin/loader.go` | 新增 | .so 文件加载器 |
| `internal/plugin/loader_test.go` | 新增 | 加载器测试 |
| `internal/plugin/lifecycle.go` | 新增 | 生命周期管理器 |
| `internal/plugin/lifecycle_test.go` | 新增 | 生命周期测试 |
| `internal/plugin/grpc_stub.go` | 新增 | gRPC 预留骨架 |
| `internal/plugin/grpc_stub_test.go` | 新增 | gRPC 骨架测试 |
| `internal/plugin/plugin.go` | 修改 | Registry 增加 Unregister |
| `internal/plugin/plugin_test.go` | 修改 | 补充测试 |
| `internal/daemon/daemon.go` | 修改 | 集成 LifecycleManager |
| `internal/daemon/daemon_test.go` | 修改 | 插件集成测试 |
| `internal/daemon/eventbus/types.go` | 修改 | 新增 4 个事件类型 |
| `internal/admin/api.go` | 修改 | 新增 /plugins 路由 |
| `internal/admin/types.go` | 修改 | 新增插件管理类型 |
| `internal/admin/audit.go` | 修改 | 新增插件操作审计 |
| `cmd/huan/daemon.go` | 修改 | 新增 --plugin-dir / --disable-plugin |
| `cmd/huan/plugin_load.go` | 新增 | load 子命令 |
| `cmd/huan/plugin_unload.go` | 新增 | unload 子命令 |
| `cmd/huan/plugin_reload.go` | 新增 | reload 子命令 |
| `internal/plugin/testdata/` | 新增 | 测试用 .so 插件源码 |
| `docs/adr/0012-hot-pluggable-plugins.md` | 新增 | 本设计对应的 ADR（可选） |

## 10. 未来扩展路径

- **gRPC 插件**：实现 `internal/plugin/grpc/` 包，引入 protobuf 定义
- **插件市场**：`huan plugin install <name>` 从远程仓库下载 .so
- **插件沙箱**：通过 gRPC 子进程实现内存隔离
- **多站点管理**：同一 daemon 进程托管多个站点