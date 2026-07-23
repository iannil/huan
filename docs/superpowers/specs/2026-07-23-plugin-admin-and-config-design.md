# 插件管理后台与配置验证体系设计

- **日期**：2026-07-23
- **状态**：Draft
- **关联 ADR**：[ADR 0003](../adr/0003-unified-plugin-system.md)

## 背景

当前插件系统骨架已搭建完毕（`internal/plugin/` 核心宿主、`.so` 动态加载、`LifecycleManager`、插件热重载），但存在两个缺口：

1. **管理后台缺少插件页面** — Admin API 的插件端点（`GET /admin/api/plugins`、`POST load/unload/reload`）已完整实现，但前端没有对应的 UI 页面，用户无法在 `/admin` 中可视化地查看和管理插件。
2. **插件配置缺少校验** — `cfg.Plugins` 是 `map[string]map[string]any`，没有任何类型约束或 schema 校验。用户配置错误（拼写错误、漏填必填字段、类型错误）只能在运行时通过 HTTP 500 暴露。

## 范围

### 包含

- 前端插件管理页面（列表、加载、卸载、重载）
- 插件配置 Schema 声明接口与校验
- 未知插件检测与警告
- 前端导航栏增加插件入口

### 不包含

- 运行时配置热更新（`huan dev` 修改 yaml 后自动重建，配置自然重新加载）
- JSON Schema / OpenAPI 级别的 schema 描述
- 跨字段校验（如 `field_a` 与 `field_b` 互斥）
- 插件开发者脚手架（Plugin SDK）—— 留待后续

## 设计

### Section 1：Admin UI 插件管理页面

#### 1.1 前端路由

在 `App.tsx` 中新增路由：

```
/admin/plugins  → Plugins.tsx
```

#### 1.2 导航入口

在 `Layout.tsx` 的 `navItems` 中增加"插件"菜单项，使用 `Puzzle` 图标（来自 `lucide-react`），放在"设置"之前。

#### 1.3 插件列表页面（Plugins.tsx）

页面结构：

```
┌─────────────────────────────────────────────────────────┐
│  插件管理          [加载新插件]                          │
│                                                         │
│  ┌─────────────────────────────────────────────────────┐│
│  │ 名称    来源    能力    状态    加载时间    操作    ││
│  │─────────────────────────────────────────────────────││
│  │ cloudflare compiled deploy  ● active  2026-07-23  - ││
│  │ foo       loaded   -       ● active  2026-07-23  ⚡×││
│  │ bar       loaded   -       ● error   2026-07-23  ⚡×││
│  │                    (error details: ...)              ││
│  └─────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────┘
```

- **表格列**：Name、Source（`compiled`/`loaded`/`grpc`）、Capability、Status（`active`→绿色圆点，`error`→红色圆点 + 错误信息）、LoadedAt、操作
- **操作按钮**：
  - `loaded` 来源：显示 Unload 按钮（× 图标）和 Reload 按钮（⚡ 图标）
  - `compiled` 来源：操作列显示"系统内置"（禁用态）
  - `error` 状态：在行下方展开错误详情
- **加载新插件**：点击后在 Dialog 中输入 .so 文件路径，提交 POST /admin/api/plugins/load
- **空状态**：显示"暂无插件"提示
- **加载中**：显示骨架屏或"加载中..."
- **错误状态**：API 调用失败时在页面顶部显示错误横幅

#### 1.4 后端依赖

API 端点已存在，无需改动后端：
- `GET /admin/api/plugins` → `pluginManager.List()`
- `POST /admin/api/plugins/load` → `pluginManager.Load()`
- `POST /admin/api/plugins/unload` → `pluginManager.Unload()`
- `POST /admin/api/plugins/reload` → `pluginManager.Reload()`

### Section 2：配置验证体系

#### 2.1 Schema 声明类型

在 `internal/plugin/schema.go` 中新增：

```go
// Schema describes the full config shape a plugin expects.
type Schema struct {
    Fields []FieldSchema
}

// FieldSchema describes a single config field.
type FieldSchema struct {
    Key         string      // 字段名，对应 yaml key
    Type        string      // "string" | "int" | "bool" | "string_slice" | "map"
    Required    bool        // true = 必填，启动时校验
    Default     any         // 默认值（Required=false 时生效）
    Description string      // 人类可读的说明
    Sensitive   bool        // true = 在 CLI info 中 mask 为 ***
    EnvVarHint  string      // 建议的环境变量名，仅用于文档提示
}
```

#### 2.2 SchemaProvider 可选接口

```go
// SchemaProvider is an optional interface plugins can implement to declare
// their config schema.
type SchemaProvider interface {
    ConfigSchema() Schema
}
```

不强制所有 plugin 实现——跟随 YAGNI 原则。只有需要做配置校验的插件才实现此接口。

#### 2.3 校验逻辑

新增 `internal/plugin/validate.go`：

```go
// ValidateConfig checks raw config against the schema. Returns a list of
// validation errors (empty = valid).
func ValidateConfig(name string, schema Schema, raw map[string]any) []string
```

校验规则：
- **必填字段**：`Required=true` 的字段在 `raw` 中必须存在且非零值
- **类型检查**：`raw[key]` 的实际类型必须匹配 `FieldSchema.Type`
- **未知字段**：`raw` 中出现不在 schema 中的字段 → warning
- **默认值**：`Required=false` 且 `raw` 中缺少该字段时，注入默认值到一个拷贝中

#### 2.4 集成到 `newPluginRegistry`

在 `cmd/huan/plugins.go` 的 `newPluginRegistry` 中，对每个已注册的 compiled-in plugin 自动检测 `SchemaProvider`，调用 `ValidateConfig`：

```go
func validatePluginConfig(name string, raw map[string]any, p Plugin) []error {
    sp, ok := p.(SchemaProvider)
    if !ok {
        return nil
    }
    schema := sp.ConfigSchema()
    return ValidateConfig(name, schema, raw)
}
```

校验结果：
- error 级别（必填缺失、类型错误）→ fail-fast，启动时直接报错退出
- warning 级别（未知字段）→ 输出到 stderr，不阻塞启动

#### 2.5 未知插件检测

在 `newPluginRegistry()` 末尾，遍历 `cfg.Plugins` 中未在 registry 中注册的 key：

```go
for name := range cfg.Plugins {
    if _, exists := registry.Get(name); !exists {
        // warning: plugin "xxx" declared in yaml but not compiled-in
        // (will be loaded from .so at runtime if available)
    }
}
```

只输出 warning，不阻塞启动——因为 .so 插件可能在运行时才加载。

## 数据流

### Admin UI 插件管理

```
用户点击"插件"导航 → Plugins.tsx 挂载
  → GET /admin/api/plugins → 渲染表格
  → 用户点击"加载新插件"
    → Dialog 输入 .so 路径
    → POST /admin/api/plugins/load
    → 刷新列表
  → 用户点击 Unload
    → POST /admin/api/plugins/unload {name: "foo"}
    → 刷新列表
  → 用户点击 Reload
    → 弹出 Dialog 输入新 .so 路径（预填当前路径）
    → POST /admin/api/plugins/reload {name: "foo", path: "..."}
    → 刷新列表
```

### 配置验证

```
huan build / dev / deploy
  → config.Load() → yaml 解析 → raw map
  → newPluginRegistry(cfg)
    → 对每个 compiled-in plugin:
      → 检测 SchemaProvider
      → ValidateConfig → error → fail-fast, warning → stderr
    → 检测未知插件 → warning → stderr
  → 注册成功，继续执行
```

## 测试策略

### 前端

- Plugins 组件：Mock API 响应，测试渲染状态（空列表、loading、错误、数据表格）
- 操作按钮：Mock 点击事件，验证 API 调用参数
- Dialog：Mock 输入和提交，验证表单提交

### 后端

- `ValidateConfig` 单元测试：必填缺失、类型错误、未知字段、默认值注入
- `SchemaProvider` 接口检测测试
- 未知插件 warning 测试

## 文件清单

### 新增

```
web/admin/src/pages/Plugins.tsx          # 插件管理页面
internal/plugin/schema.go                # Schema 类型定义
internal/plugin/schema_test.go           # Schema 类型测试
internal/plugin/validate.go              # ValidateConfig 实现
internal/plugin/validate_test.go         # 校验逻辑测试
```

### 修改

```
web/admin/src/App.tsx                    # 添加 /admin/plugins 路由
web/admin/src/components/Layout.tsx      # 添加"插件"导航菜单项
cmd/huan/plugins.go                      # 集成 SchemaProvider 校验 + 未知插件警告
```