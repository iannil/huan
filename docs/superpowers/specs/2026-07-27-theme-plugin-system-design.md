# 主题插件系统设计

- **日期**：2026-07-27
- **状态**：Draft
- **设计者**：用户 + Claude

## 一、背景与目标

huan 已完成插件化架构（ADR 0003），现有三种能力类型：
- `deploy.Deployer` — cloudflare 插件
- `translate.Translator` — qwen3 插件
- `image.ImageProcessor` — image-pipeline 插件

下一步引入第四种能力：**主题插件**。目标：

1. 主题作为 .so 插件加载，提供模板、模板函数、静态资源
2. 支持构建 Hook（BeforeRender / AfterRender）
3. 全局唯一激活（任何时候最多一个主题）
4. 首个官方主题为 zhurongshuo 站点定制

## 二、设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 主题形态 | .so 插件（ThemePlugin 接口） | 与现有 plugin 系统一致，可热插拔 |
| 模板存储 | `//go:embed` 嵌入 .so | 单文件部署，无外部依赖 |
| 激活模式 | 全局唯一 | 简化渲染管线，避免模板冲突 |
| 构建 Hook | 可选接口（ThemeHooks） | 不强制所有主题实现 |
| FuncMap 优先级 | 主题函数覆盖内置函数 | 主题可定制渲染行为 |

## 三、新增包

### 3.1 `internal/theme/types.go` — 接口定义

```go
package theme

import (
    "context"
    "html/template"
    "io/fs"
    "github.com/iannil/huan/internal/plugin"
)

// ThemePlugin 是主题插件的核心能力接口。
type ThemePlugin interface {
    plugin.Plugin

    // Info 返回主题元数据。
    Info() ThemeInfo

    // Templates 返回主题提供的模板列表。
    Templates() []TemplateEntry

    // FuncMap 返回主题自定义的模板函数。
    FuncMap() template.FuncMap

    // Assets 返回主题的静态资源文件系统。
    Assets() fs.FS
}

// ThemeInfo 携带主题元数据。
type ThemeInfo struct {
    Name        string   `json:"name"`
    Version     string   `json:"version"`
    Author      string   `json:"author"`
    Description string   `json:"description"`
    Screenshot  string   `json:"screenshot,omitempty"`
    Tags        []string `json:"tags,omitempty"`
    MinHuanVer  string   `json:"minHuanVer,omitempty"`
}

// TemplateEntry 描述一个模板文件。
type TemplateEntry struct {
    Path    string // 逻辑路径，如 "index.html"
    Content string // 模板内容
}

// ThemeHooks 是可选接口，主题可实现它来注入构建 Hook。
type ThemeHooks interface {
    BeforeRender(ctx context.Context) error
    AfterRender(ctx context.Context) error
}
```

### 3.2 `internal/theme/manager.go` — 主题管理器

```go
package theme

import (
    "fmt"
    "sync"
    "github.com/iannil/huan/internal/plugin"
)

var ErrNoActiveTheme = fmt.Errorf("theme: no active theme")

// Manager 管理主题插件的生命周期和状态。
// 全局单例，任何时候最多一个激活主题。
type Manager struct {
    mu         sync.RWMutex
    registry   *plugin.Registry
    active     ThemePlugin
    activeName string
}

func NewManager(registry *plugin.Registry) *Manager

// Activate 激活指定名称的主题插件。
func (m *Manager) Activate(name string) error

// Deactivate 停用当前激活的主题。
func (m *Manager) Deactivate()

// Active 返回当前激活的主题。
func (m *Manager) Active() ThemePlugin

// ActiveName 返回当前激活主题的名称。
func (m *Manager) ActiveName() string

// ListAvailable 列出所有注册了 ThemePlugin 能力的插件。
func (m *Manager) ListAvailable() []ThemePlugin
```

### 3.3 CLI 子命令

新增 `huan theme`：

```
huan theme activate <name>    # 激活主题
huan theme deactivate         # 停用主题
huan theme list               # 列出所有已注册的主题插件
huan theme info <name>        # 查看主题元数据
```

## 四、改造的包

### 4.1 `internal/template/loader.go`

- `Loader` 新增 `themeManager *theme.Manager` 字段
- `LoadTemplate` 优先查激活主题的 `Templates()`
- `FuncMap` 合并主题自定义函数（同名时主题优先）

### 4.2 `internal/build/pipeline.go`

- 构建管线在渲染前后调用 `ThemeHooks.BeforeRender` / `AfterRender`
- BeforeRender 失败 → 中止构建
- AfterRender 失败 → 记录日志

### 4.3 `internal/config/config.go`

- 新增 `Theme string` 字段，对应 yaml 顶层 `theme:`

### 4.4 `cmd/huan/plugins.go`

- 配置加载后调用 `themeManager.Activate(cfg.Theme)`

### 4.5 `internal/admin/handler.go`

- 增加主题管理 API 端点：`GET /admin/api/theme`、`POST /admin/api/theme/activate`、`POST /admin/api/theme/deactivate`

## 五、首个官方主题：zhurongshuo

### 目录结构

```
plugins/zhurongshuo/
├── plugin_main.go          # InitPlugin 入口
├── plugin.go               # ThemePlugin 实现
├── plugin/plugin.go        # 自包含 plugin.Plugin 接口副本
├── theme.go                # 主题逻辑（模板加载、函数注册）
├── funcs.go                # 自定义模板函数
├── templates/              //go:embed
│   ├── index.html
│   ├── _default/single.html
│   ├── _default/list.html
│   ├── partials/header.html
│   ├── partials/footer.html
│   └── ...
└── assets/                 //go:embed
    ├── css/main.css
    ├── js/theme.js
    └── img/
```

### 提供的模板函数

| 函数 | 说明 |
|------|------|
| `readingTime` | 估算文章阅读时间 |
| `relatedPosts` | 根据标签获取相关文章 |
| `toc` | 从正文生成目录树 |
| `darkModeToggle` | 深色模式切换按钮 |

## 六、配置示例

```yaml
theme: "zhurongshuo"

plugins:
  zhurongshuo:
    # 主题特定配置（如配色方案、布局选项等，预留）
  cloudflare:
    accountId: ${CLOUDFLARE_ACCOUNT_ID}
    apiToken: ${CLOUDFLARE_API_TOKEN}
  qwen3:
    model: qwen3-next:80b
```

## 七、风险与缓解

| 风险 | 缓解 |
|------|------|
| 主题插件加载失败导致站点不可用 | 停用主题自动回退 layouts/ 默认模板 |
| 主题模板函数与内置函数命名冲突 | 主题优先覆盖，但记录警告日志 |
| .so 体积过大（嵌入大量模板/资源） | 资源按需嵌入，CSS/JS 可 minify 后嵌入 |
| 主题切换时模板缓存不一致 | 切换主题自动清除模板缓存 |
| 构建 Hook 内 panic 导致 daemon 崩溃 | 用 recover 包装 Hook 调用 |

## 八、验收标准

1. 主题插件可正常加载（`huan plugin load`）
2. 激活主题后站点渲染使用主题模板（`huan theme activate zhurongshuo`）
3. 主题模板函数在模板中可用
4. 主题静态资源可通过 `/theme/zhurongshuo/` 访问
5. 停用主题后回退到 layouts/ 默认模板
6. BeforeRender 失败时构建中止
7. AfterRender 失败时构建继续但记录日志
8. 切换主题自动清除模板缓存
9. 单元测试覆盖 Manager 核心逻辑