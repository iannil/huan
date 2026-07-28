# 主题插件系统实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 实现 ThemePlugin 能力接口，支持主题作为 .so 插件加载，激活后控制模板渲染、提供模板函数和静态资源，支持构建 Hook。

**架构：** 新增 `internal/theme/` 包定义 ThemePlugin 接口 + 主题管理器；改造 `internal/template/loader.go` 优先加载主题模板；改造 `internal/build/pipeline.go` 插入构建 Hook；新增 `cmd/huan/theme_cmd.go` CLI 子命令；新增 `plugins/zhurongshuo/` 首个官方主题。

**技术栈：** Go + html/template + io/fs embed + cobra + sync.RWMutex

## 全局约束

- 主题插件名 = `zhurongshuo`（不是 `zhurong`）
- 模板嵌入 .so 中（`//go:embed`），单文件部署
- 主题全局唯一激活，任何时候最多一个激活主题
- FuncMap 优先级：主题函数覆盖内置函数
- 构建 Hook 为可选接口（ThemeHooks），BeforeRender 失败中止构建，AfterRender 失败记录日志
- 切换主题时自动清除模板缓存

---

### Task 1: 定义 ThemePlugin 接口和类型（`internal/theme/types.go`）

**Files:**
- Create: `internal/theme/types.go`
- Test: `internal/theme/types_test.go`

**Interfaces:**
- Consumes: `internal/plugin/plugin.go`（Plugin 接口、Registry）
- Produces: `ThemePlugin` 接口、`ThemeInfo` 结构体、`TemplateEntry` 结构体、`ThemeHooks` 可选接口

- [ ] **Step 1: Write the failing test**

```go
// internal/theme/types_test.go
package theme_test

import (
    "context"
    "html/template"
    "io/fs"
    "testing"
    "github.com/iannil/huan/internal/plugin"
    "github.com/iannil/huan/internal/theme"
)

// mockThemePlugin implements theme.ThemePlugin for testing.
type mockThemePlugin struct {
    plugin.Plugin
    info     theme.ThemeInfo
    templates []theme.TemplateEntry
    funcMap  template.FuncMap
    assets   fs.FS
}

func (m *mockThemePlugin) Name() string { return m.info.Name }
func (m *mockThemePlugin) Info() theme.ThemeInfo { return m.info }
func (m *mockThemePlugin) Templates() []theme.TemplateEntry { return m.templates }
func (m *mockThemePlugin) FuncMap() template.FuncMap { return m.funcMap }
func (m *mockThemePlugin) Assets() fs.FS { return m.assets }

// mockThemePluginWithHooks implements both ThemePlugin and ThemeHooks.
type mockThemePluginWithHooks struct {
    mockThemePlugin
    beforeCalled bool
    afterCalled  bool
}

func (m *mockThemePluginWithHooks) BeforeRender(ctx context.Context) error {
    m.beforeCalled = true
    return nil
}
func (m *mockThemePluginWithHooks) AfterRender(ctx context.Context) error {
    m.afterCalled = true
    return nil
}

func TestThemePluginInterface(t *testing.T) {
    // Verify that a mockThemePlugin satisfies the ThemePlugin interface
    var tp theme.ThemePlugin = &mockThemePlugin{
        info: theme.ThemeInfo{
            Name:    "test_theme",
            Version: "1.0.0",
            Author:  "test",
        },
    }
    if tp.Name() != "test_theme" {
        t.Errorf("expected test_theme, got %s", tp.Name())
    }
}

func TestThemeHooksInterface(t *testing.T) {
    // Verify that a mockThemePluginWithHooks satisfies both interfaces
    var tp theme.ThemePlugin = &mockThemePluginWithHooks{
        mockThemePlugin: mockThemePlugin{
            info: theme.ThemeInfo{Name: "hook_test"},
        },
    }
    hooks, ok := tp.(theme.ThemeHooks)
    if !ok {
        t.Fatal("expected ThemePlugin to implement ThemeHooks")
    }
    if err := hooks.BeforeRender(context.Background()); err != nil {
        t.Errorf("BeforeRender: %v", err)
    }
    if err := hooks.AfterRender(context.Background()); err != nil {
        t.Errorf("AfterRender: %v", err)
    }
}

func TestThemeInfoFields(t *testing.T) {
    info := theme.ThemeInfo{
        Name:        "zhurongshuo",
        Version:     "0.1.0",
        Author:      "iannil",
        Description: "祝融说官方主题",
        Tags:        []string{"blog", "chinese"},
        MinHuanVer:  "v0.7.0",
    }
    if info.Name != "zhurongshuo" {
        t.Errorf("unexpected name: %s", info.Name)
    }
}

func TestTemplateEntry(t *testing.T) {
    entry := theme.TemplateEntry{
        Path:    "index.html",
        Content: "<html>{{ .Title }}</html>",
    }
    if entry.Path != "index.html" {
        t.Errorf("unexpected path: %s", entry.Path)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/theme/ -v`
Expected: FAIL with "package theme is not in std"

- [ ] **Step 3: Write minimal implementation**

```go
// internal/theme/types.go
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/theme/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/theme/types.go internal/theme/types_test.go
git commit -m "feat(theme): add ThemePlugin interface and types"
```

---

### Task 2: 实现主题管理器（`internal/theme/manager.go`）

**Files:**
- Create: `internal/theme/manager.go`
- Test: `internal/theme/manager_test.go`

**Interfaces:**
- Consumes: `plugin.Registry`、`ThemePlugin`（来自 Task 1）
- Produces: `Manager` 结构体（`NewManager`、`Activate`、`Deactivate`、`Active`、`ActiveName`、`ListAvailable`）

- [ ] **Step 1: Write the failing test**

```go
// internal/theme/manager_test.go
package theme_test

import (
    "html/template"
    "io/fs"
    "testing"

    "github.com/iannil/huan/internal/plugin"
    "github.com/iannil/huan/internal/theme"
)

type simpleTheme struct {
    name string
}

func (s *simpleTheme) Name() string                 { return s.name }
func (s *simpleTheme) Info() theme.ThemeInfo          { return theme.ThemeInfo{Name: s.name} }
func (s *simpleTheme) Templates() []theme.TemplateEntry { return nil }
func (s *simpleTheme) FuncMap() template.FuncMap      { return nil }
func (s *simpleTheme) Assets() fs.FS                  { return nil }

func TestManagerActivateDeactivate(t *testing.T) {
    reg := plugin.NewRegistry()
    _ = reg.Register(&simpleTheme{name: "test_theme"})

    mgr := theme.NewManager(reg)

    // Initially no active theme
    if mgr.Active() != nil {
        t.Error("expected no active theme initially")
    }
    if mgr.ActiveName() != "" {
        t.Errorf("expected empty active name, got %s", mgr.ActiveName())
    }

    // Activate
    if err := mgr.Activate("test_theme"); err != nil {
        t.Fatalf("Activate: %v", err)
    }
    if mgr.ActiveName() != "test_theme" {
        t.Errorf("expected test_theme, got %s", mgr.ActiveName())
    }
    if mgr.Active() == nil {
        t.Fatal("expected non-nil active theme")
    }

    // Deactivate
    mgr.Deactivate()
    if mgr.Active() != nil {
        t.Error("expected no active theme after deactivation")
    }
}

func TestManagerActivateNotFound(t *testing.T) {
    reg := plugin.NewRegistry()
    mgr := theme.NewManager(reg)

    err := mgr.Activate("nonexistent")
    if err == nil {
        t.Fatal("expected error for nonexistent plugin")
    }
}

func TestManagerActivateNonThemePlugin(t *testing.T) {
    reg := plugin.NewRegistry()
    // Register a non-theme plugin (just Name, no ThemePlugin)
    _ = reg.Register(&simplePlugin{name: "cloudflare"})

    mgr := theme.NewManager(reg)
    err := mgr.Activate("cloudflare")
    if err == nil {
        t.Fatal("expected error for non-theme plugin")
    }
}

type simplePlugin struct{ name string }
func (s *simplePlugin) Name() string { return s.name }

func TestManagerListAvailable(t *testing.T) {
    reg := plugin.NewRegistry()
    _ = reg.Register(&simpleTheme{name: "theme_a"})
    _ = reg.Register(&simpleTheme{name: "theme_b"})
    _ = reg.Register(&simplePlugin{name: "not_a_theme"})

    mgr := theme.NewManager(reg)
    available := mgr.ListAvailable()
    if len(available) != 2 {
        t.Errorf("expected 2 available themes, got %d", len(available))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/theme/ -v`
Expected: FAIL with compilation errors (manager.go not found)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/theme/manager.go
package theme

import (
    "fmt"
    "sync"

    "github.com/iannil/huan/internal/plugin"
)

var ErrNoActiveTheme = fmt.Errorf("theme: no active theme")

// Manager 管理主题插件的生命周期和状态。
type Manager struct {
    mu         sync.RWMutex
    registry   *plugin.Registry
    active     ThemePlugin
    activeName string
}

// NewManager 创建主题管理器。
func NewManager(registry *plugin.Registry) *Manager {
    return &Manager{registry: registry}
}

// Activate 激活指定名称的主题插件。
func (m *Manager) Activate(name string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    p, ok := m.registry.Get(name)
    if !ok {
        return fmt.Errorf("theme: plugin %q not found", name)
    }
    tp, ok := p.(ThemePlugin)
    if !ok {
        return fmt.Errorf("theme: plugin %q does not implement ThemePlugin", name)
    }

    m.active = tp
    m.activeName = name
    return nil
}

// Deactivate 停用当前激活的主题。
func (m *Manager) Deactivate() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.active = nil
    m.activeName = ""
}

// Active 返回当前激活的主题。
func (m *Manager) Active() ThemePlugin {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.active
}

// ActiveName 返回当前激活主题的名称。
func (m *Manager) ActiveName() string {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.activeName
}

// ListAvailable 列出所有注册了 ThemePlugin 能力的插件。
func (m *Manager) ListAvailable() []ThemePlugin {
    return plugin.Find[ThemePlugin](m.registry)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/theme/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/theme/manager.go internal/theme/manager_test.go
git commit -m "feat(theme): add theme Manager with activate/deactivate"
```

---

### Task 3: Config 新增 Theme 字段

**Files:**
- Modify: `internal/config/config.go`

**Interfaces:**
- Consumes: `Config` 结构体
- Produces: `Config.Theme` 字段

- [ ] **Step 1: Add `Theme` field to Config**

```go
// internal/config/config.go — 在 Config 结构体中添加字段
type Config struct {
    // ... 现有字段
    Plugins      map[string]map[string]any `yaml:"plugins"`
    Theme        string                    `yaml:"theme"` // 新增：激活的主题插件名称
    // ... 其余字段
}
```

- [ ] **Step 2: Run existing tests to verify no regression**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add Theme field for active theme plugin"
```

---

### Task 4: 改造模板加载器支持主题插件模板

**Files:**
- Modify: `internal/template/loader.go`
- Modify: `internal/template/context.go`（`LoadAllTemplates`）
- Test: `internal/template/loader_test.go`

**Interfaces:**
- Consumes: `theme.Manager`（来自 Task 2）
- Produces: 修改后的 `Loader`（支持主题插件模板优先加载）

- [ ] **Step 1: Write the failing test**

```go
// internal/template/loader_test.go 追加
func TestLoaderWithThemePlugin(t *testing.T) {
    reg := plugin.NewRegistry()
    themeReg := plugin.NewRegistry()
    themeMgr := theme.NewManager(themeReg)

    // Register a mock theme plugin
    mockTheme := &mockThemePlugin{
        info: theme.ThemeInfo{Name: "test_theme"},
        templates: []theme.TemplateEntry{
            {Path: "index.html", Content: "theme index: {{ .Title }}"},
            {Path: "_default/single.html", Content: "theme single: {{ .Title }}"},
        },
        funcMap: template.FuncMap{
            "themeFunc": func() string { return "from_theme" },
        },
    }
    _ = themeReg.Register(mockTheme)
    _ = themeMgr.Activate("test_theme")

    fm := template.FuncMap{}
    loader := template.NewLoaderWithTheme("/tmp/test", "/tmp/test/layouts", fm, themeMgr)

    // The loader should prefer theme templates over layouts
    // (We can't fully test without creating a real layouts dir, but we can
    //  verify the loader accepts the themeManager parameter)
    if loader == nil {
        t.Fatal("expected non-nil loader")
    }
}
```

- [ ] **Step 2: Modify Loader to accept theme.Manager**

```go
// internal/template/loader.go — 修改 Loader 结构体和构造函数

type Loader struct {
    themeDir     string
    layoutsDir   string
    funcMap      template.FuncMap
    themeManager *theme.Manager // 新增
}

// NewLoader 保持原有签名（兼容旧调用者）
func NewLoader(sourceDir, themeName string, funcMap template.FuncMap) *Loader {
    return &Loader{
        themeDir:   filepath.Join(sourceDir, "themes", themeName, "layouts"),
        layoutsDir: filepath.Join(sourceDir, "layouts"),
        funcMap:    funcMap,
    }
}

// NewLoaderWithTheme 新增：支持主题插件的加载器
func NewLoaderWithTheme(sourceDir, layoutsDir string, funcMap template.FuncMap, themeMgr *theme.Manager) *Loader {
    return &Loader{
        themeDir:     filepath.Join(sourceDir, "themes", "", "layouts"),
        layoutsDir:   layoutsDir,
        funcMap:      funcMap,
        themeManager: themeMgr,
    }
}
```

- [ ] **Step 3: Modify `LoadAll` 方法优先加载主题插件模板**

```go
// internal/template/loader.go — 修改 LoadAll 方法

func (l *Loader) LoadAll() (*template.Template, error) {
    templates := map[string]string{}

    // 1. 优先加载激活主题插件的模板
    if l.themeManager != nil {
        if tp := l.themeManager.Active(); tp != nil {
            for _, entry := range tp.Templates() {
                templates[entry.Path] = entry.Content
            }
        }
    }

    // 2. 加载主题目录（旧方式，兼容）
    if _, err := os.Stat(l.themeDir); err == nil {
        if err := l.walkDir(l.themeDir, templates); err != nil {
            return nil, fmt.Errorf("load theme: %w", err)
        }
    }

    // 3. 加载 layouts/ 目录（覆盖优先级高于主题插件）
    if _, err := os.Stat(l.layoutsDir); err == nil {
        if err := l.walkDir(l.layoutsDir, templates); err != nil {
            return nil, fmt.Errorf("load layouts: %w", err)
        }
    }

    // ... 后续代码不变
}
```

- [ ] **Step 4: Modify `LoadAllTemplates` 接受 theme.Manager 参数**

```go
// internal/template/context.go — 修改 LoadAllTemplates

func LoadAllTemplates(sourceDir, baseURL string, themeMgr ...*theme.Manager) (*template.Template, error) {
    themeName := ""
    themesDir := filepath.Join(sourceDir, "themes")
    if entries, err := os.ReadDir(themesDir); err == nil {
        for _, e := range entries {
            if e.IsDir() {
                themeName = e.Name()
                break
            }
        }
    }

    funcMap := FuncMap(baseURL)
    var loader *Loader
    if len(themeMgr) > 0 && themeMgr[0] != nil {
        layoutsDir := filepath.Join(sourceDir, "layouts")
        loader = NewLoaderWithTheme(sourceDir, layoutsDir, funcMap, themeMgr[0])
    } else {
        loader = NewLoader(sourceDir, themeName, funcMap)
    }
    return loader.LoadAll()
}
```

- [ ] **Step 5: 在 FuncMap 中合并主题函数**

```go
// internal/template/loader.go — 修改 LoadAll 中创建 tmpl 的部分

// 在创建 tmpl 之后，合并主题函数
tmpl := template.New("").Funcs(l.funcMap)

// 合并激活主题的 FuncMap（同名时主题优先）
if l.themeManager != nil {
    if tp := l.themeManager.Active(); tp != nil {
        for k, v := range tp.FuncMap() {
            tmpl.Funcs(template.FuncMap{k: v})
        }
    }
}
```

- [ ] **Step 6: Run tests to verify**

Run: `go test ./internal/template/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/template/loader.go internal/template/context.go
git commit -m "feat(template): support theme plugin templates and FuncMap"
```

---

### Task 5: 改造构建管线支持 ThemeHooks

**Files:**
- Modify: `internal/build/pipeline.go`
- Modify: `internal/build/build.go`（Options + BuildSite）
- Test: `internal/build/build_test.go` 或新增 `internal/build/theme_hooks_test.go`

**Interfaces:**
- Consumes: `theme.Manager`、`ThemeHooks`
- Produces: 构建管线中的 BeforeRender / AfterRender 调用点

- [ ] **Step 1: 在 Options 中添加 ThemeManager 字段**

```go
// internal/build/build.go — 在 Options 结构体中添加

type Options struct {
    // ... 现有字段
    PluginRegistry *plugin.Registry

    // ThemeManager, if non-nil, is used to discover theme hooks
    // that participate in the build pipeline.
    ThemeManager *theme.Manager // 新增
}
```

- [ ] **Step 2: 在 pipeline 中添加 themeManager 字段**

```go
// internal/build/pipeline.go — 在 pipeline 结构体中添加

type pipeline struct {
    // ... 现有字段
    themeManager *theme.Manager // 新增
}
```

- [ ] **Step 3: 在 newPipeline 中初始化**

```go
// internal/build/pipeline.go — 修改 newPipeline

func newPipeline(opts Options) *pipeline {
    return &pipeline{
        opts:         opts,
        logf:         opts.logf(),
        result:       &Result{},
        now:          time.Now(),
        themeManager: opts.ThemeManager, // 新增
    }
}
```

- [ ] **Step 4: 在 BuildSite 中插入 BeforeRender / AfterRender 调用**

```go
// internal/build/build.go — 修改 BuildSite

func BuildSite(opts Options) (*Result, error) {
    start := time.Now()
    p := newPipeline(opts)

    // BeforeRender Hook
    if p.themeManager != nil {
        if tp := p.themeManager.Active(); tp != nil {
            if hooks, ok := tp.(theme.ThemeHooks); ok {
                if err := hooks.BeforeRender(context.Background()); err != nil {
                    return nil, fmt.Errorf("theme before render: %w", err)
                }
            }
        }
    }

    stages := []struct {
        name string
        fn   func() error
    }{
        {"load config", p.loadConfig},
        // ... 不变
    }

    // ... 渲染流程 ...

    p.result.Duration = time.Since(start)

    // AfterRender Hook
    if p.themeManager != nil {
        if tp := p.themeManager.Active(); tp != nil {
            if hooks, ok := tp.(theme.ThemeHooks); ok {
                if err := hooks.AfterRender(context.Background()); err != nil {
                    p.logf("WARN: theme after render: %v\n", err)
                }
            }
        }
    }

    // ... 后续代码不变
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/build/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/build/pipeline.go internal/build/build.go
git commit -m "feat(build): add ThemeHooks (BeforeRender/AfterRender) to build pipeline"
```

---

### Task 6: 新增 `huan theme` CLI 子命令

**Files:**
- Create: `cmd/huan/theme_cmd.go`
- Modify: `cmd/huan/main.go`（注册子命令）
- Modify: `cmd/huan/plugins.go`（`capabilityLabels` 添加 theme 检测）

**Interfaces:**
- Consumes: `theme.Manager`、`config.Config`
- Produces: CLI 子命令 `huan theme activate <name>`、`huan theme deactivate`、`huan theme list`、`huan theme info <name>`

- [ ] **Step 1: Write the CLI command**

```go
// cmd/huan/theme_cmd.go
package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/iannil/huan/internal/config"
    "github.com/iannil/huan/internal/theme"
    "github.com/spf13/cobra"
)

func newThemeCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "theme",
        Short: "Manage themes",
        Long:  "Activate, deactivate, and list theme plugins.",
    }
    cmd.AddCommand(newThemeActivateCmd())
    cmd.AddCommand(newThemeDeactivateCmd())
    cmd.AddCommand(newThemeListCmd())
    cmd.AddCommand(newThemeInfoCmd())
    return cmd
}

func newThemeActivateCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "activate <name>",
        Short: "Activate a theme plugin",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(sourceDir)
            if err != nil {
                return fmt.Errorf("load config: %w", err)
            }
            registry, err := newPluginRegistry(cfg)
            if err != nil {
                return fmt.Errorf("plugin registry: %w", err)
            }
            mgr := theme.NewManager(registry)
            if err := mgr.Activate(args[0]); err != nil {
                return err
            }
            fmt.Printf("Theme %q activated\n", args[0])
            return nil
        },
    }
}

func newThemeDeactivateCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "deactivate",
        Short: "Deactivate the current theme",
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(sourceDir)
            if err != nil {
                return fmt.Errorf("load config: %w", err)
            }
            registry, err := newPluginRegistry(cfg)
            if err != nil {
                return fmt.Errorf("plugin registry: %w", err)
            }
            mgr := theme.NewManager(registry)
            mgr.Deactivate()
            fmt.Println("Theme deactivated")
            return nil
        },
    }
}

func newThemeListCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "list",
        Short: "List all available theme plugins",
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(sourceDir)
            if err != nil {
                return fmt.Errorf("load config: %w", err)
            }
            registry, err := newPluginRegistry(cfg)
            if err != nil {
                return fmt.Errorf("plugin registry: %w", err)
            }
            mgr := theme.NewManager(registry)
            available := mgr.ListAvailable()
            if len(available) == 0 {
                fmt.Println("No theme plugins available.")
                return nil
            }
            fmt.Printf("%-20s %-10s %-15s %s\n", "NAME", "VERSION", "AUTHOR", "STATUS")
            for _, tp := range available {
                info := tp.Info()
                status := "AVAILABLE"
                if mgr.ActiveName() == info.Name {
                    status = "ACTIVE"
                }
                fmt.Printf("%-20s %-10s %-15s %s\n", info.Name, info.Version, info.Author, status)
            }
            return nil
        },
    }
}

func newThemeInfoCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "info <name>",
        Short: "Show detailed info for a theme plugin",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(sourceDir)
            if err != nil {
                return fmt.Errorf("load config: %w", err)
            }
            registry, err := newPluginRegistry(cfg)
            if err != nil {
                return fmt.Errorf("plugin registry: %w", err)
            }
            p, ok := registry.Get(args[0])
            if !ok {
                return fmt.Errorf("theme %q not found", args[0])
            }
            tp, ok := p.(theme.ThemePlugin)
            if !ok {
                return fmt.Errorf("plugin %q is not a theme", args[0])
            }
            info := tp.Info()
            fmt.Printf("Name:        %s\n", info.Name)
            fmt.Printf("Version:     %s\n", info.Version)
            fmt.Printf("Author:      %s\n", info.Author)
            fmt.Printf("Description: %s\n", info.Description)
            fmt.Printf("Min Huan:    %s\n", info.MinHuanVer)
            fmt.Printf("Templates:   %d\n", len(tp.Templates()))
            fmt.Printf("Funcs:       %d\n", len(tp.FuncMap()))
            return nil
        },
    }
}
```

- [ ] **Step 2: 在 main.go 中注册 theme 子命令**

```go
// cmd/huan/main.go — 在 rootCmd.AddCommand 中新增
rootCmd.AddCommand(newThemeCmd())
```

- [ ] **Step 3: 在 plugins.go 的 capabilityLabels 中添加 theme 检测**

```go
// cmd/huan/plugins.go — 修改 capabilityLabels 函数

func capabilityLabels(p plugin.Plugin) []string {
    var labels []string
    if _, ok := p.(deploy.Deployer); ok {
        labels = append(labels, "deploy")
    }
    if _, ok := p.(translate.Translator); ok {
        labels = append(labels, "translate")
    }
    if _, ok := p.(image.ImageProcessor); ok {
        labels = append(labels, "image")
    }
    if _, ok := p.(theme.ThemePlugin); ok {
        labels = append(labels, "theme")
    }
    return labels
}
```

- [ ] **Step 4: Build and test**

Run: `go build -o huan ./cmd/huan && ./huan theme list`
Expected: Empty list, no errors

- [ ] **Step 5: Commit**

```bash
git add cmd/huan/theme_cmd.go cmd/huan/main.go cmd/huan/plugins.go
git commit -m "feat(cli): add huan theme subcommand"
```

---

### Task 7: 创建首个官方主题插件 `zhurongshuo`

**Files:**
- Create: `plugins/zhurongshuo/plugin_main.go`
- Create: `plugins/zhurongshuo/plugin.go`
- Create: `plugins/zhurongshuo/plugin/plugin.go`
- Create: `plugins/zhurongshuo/theme.go`
- Create: `plugins/zhurongshuo/funcs.go`
- Create: `plugins/zhurongshuo/templates/index.html`
- Create: `plugins/zhurongshuo/templates/_default/single.html`
- Create: `plugins/zhurongshuo/templates/_default/list.html`
- Create: `plugins/zhurongshuo/templates/partials/header.html`
- Create: `plugins/zhurongshuo/templates/partials/footer.html`
- Create: `plugins/zhurongshuo/assets/css/main.css`
- Create: `plugins/zhurongshuo/assets/js/theme.js`

**Interfaces:**
- Consumes: `plugin.Plugin`（自包含副本）、`theme.ThemePlugin`（从 huan 主包导入）
- Produces: `.so` 文件 `zhurongshuo.so`

- [ ] **Step 1: Create plugin/plugin.go（自包含接口副本）**

```go
// plugins/zhurongshuo/plugin/plugin.go
package plugin

type Plugin interface {
    Name() string
}
```

- [ ] **Step 2: Create plugin.go（ThemePlugin 实现）**

```go
// plugins/zhurongshuo/plugin.go
package main

import (
    "html/template"
    "io/fs"
    "embed"
    "sort"

    "github.com/iannil/huan-plugin-zhurongshuo/plugin"
    "github.com/iannil/huan/internal/theme" // 编译期依赖
)

//go:embed templates/*
var templateFS embed.FS

//go:embed assets/*
var assetFS embed.FS

type ZhurongshuoTheme struct {
    plugin.Plugin
    cfg Config
}

type Config struct {
    // 预留：主题配置项
}

func (t *ZhurongshuoTheme) Name() string { return "zhurongshuo" }

func (t *ZhurongshuoTheme) Info() theme.ThemeInfo {
    return theme.ThemeInfo{
        Name:        "zhurongshuo",
        Version:     "0.1.0",
        Author:      "iannil",
        Description: "祝融说官方主题 — 中文内容排版优化",
        Tags:        []string{"blog", "chinese", "philosophy"},
        MinHuanVer:  "v0.7.0",
    }
}

func (t *ZhurongshuoTheme) Templates() []theme.TemplateEntry {
    entries := []theme.TemplateEntry{}
    // Walk embedded templates
    fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        if d.IsDir() { return nil }
        data, _ := templateFS.ReadFile(path)
        // Strip "templates/" prefix
        relPath := path[len("templates/"):]
        entries = append(entries, theme.TemplateEntry{
            Path:    relPath,
            Content: string(data),
        })
        return nil
    })
    sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
    return entries
}

func (t *ZhurongshuoTheme) FuncMap() template.FuncMap {
    fm := template.FuncMap{
        "readingTime":  readingTime,
        "relatedPosts": relatedPosts,
        "toc":          toc,
        "darkModeToggle": darkModeToggle,
    }
    return fm
}

func (t *ZhurongshuoTheme) Assets() fs.FS {
    sub, _ := fs.Sub(assetFS, "assets")
    return sub
}
```

- [ ] **Step 3: Create funcs.go**

```go
// plugins/zhurongshuo/funcs.go
package main

import (
    "html/template"
    "math"
    "strings"
)

// readingTime 估算阅读时间，中文按 300 字/分钟，英文按 200 词/分钟
func readingTime(content string) string {
    // 简化实现：统计中文字符和英文单词
    cjkCount := 0
    wordCount := 0
    inWord := false
    for _, r := range content {
        if r >= 0x4E00 && r <= 0x9FFF {
            cjkCount++
            inWord = false
        } else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
            if !inWord {
                wordCount++
                inWord = true
            }
        } else {
            inWord = false
        }
    }
    minutes := math.Ceil(float64(cjkCount)/300 + float64(wordCount)/200)
    if minutes < 1 {
        minutes = 1
    }
    return template.HTML(fmt.Sprintf("%d 分钟", int(minutes)))
}

// relatedPosts 根据标签获取相关文章（占位，实际由 huan 内置逻辑处理）
func relatedPosts() string { return "" }

// toc 从正文生成目录树（占位，实际由 huan 内置逻辑处理）
func toc() string { return "" }

// darkModeToggle 返回深色模式切换按钮的 HTML
func darkModeToggle() template.HTML {
    return template.HTML(`<button id="dark-mode-toggle" aria-label="切换深色模式">🌓</button>`)
}
```

- [ ] **Step 4: Create plugin_main.go**

```go
// plugins/zhurongshuo/plugin_main.go
package main

import "github.com/iannil/huan-plugin-zhurongshuo/plugin"

func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    return &ZhurongshuoTheme{}, nil
}

func main() {}
```

- [ ] **Step 5: Create template files（最小化版本）**

```html
<!-- templates/index.html -->
<!DOCTYPE html>
<html lang="{{ site.Language.LanguageCode }}">
<head>
    <meta charset="utf-8">
    <title>{{ .Title }} | {{ .Site.Title }}</title>
    <link rel="stylesheet" href="/theme/zhurongshuo/css/main.css">
</head>
<body>
    {{ partial "header" . }}
    <main>
        <h1>{{ .Title }}</h1>
        {{ .Content }}
    </main>
    {{ partial "footer" . }}
    <script src="/theme/zhurongshuo/js/theme.js"></script>
</body>
</html>
```

```html
<!-- templates/_default/single.html -->
<!DOCTYPE html>
<html lang="{{ site.Language.LanguageCode }}">
<head>
    <meta charset="utf-8">
    <title>{{ .Title }} | {{ .Site.Title }}</title>
    <link rel="stylesheet" href="/theme/zhurongshuo/css/main.css">
</head>
<body>
    {{ partial "header" . }}
    <article>
        <h1>{{ .Title }}</h1>
        <div class="meta">{{ readingTime .Content }} · {{ .Date.Format "2006-01-02" }}</div>
        {{ .Content }}
    </article>
    {{ partial "footer" . }}
    <script src="/theme/zhurongshuo/js/theme.js"></script>
</body>
</html>
```

```html
<!-- templates/_default/list.html -->
<!DOCTYPE html>
<html lang="{{ site.Language.LanguageCode }}">
<head>
    <meta charset="utf-8">
    <title>{{ .Title }} | {{ .Site.Title }}</title>
    <link rel="stylesheet" href="/theme/zhurongshuo/css/main.css">
</head>
<body>
    {{ partial "header" . }}
    <main>
        <h1>{{ .Title }}</h1>
        <ul>
        {{ range .Pages }}
            <li><a href="{{ .URL }}">{{ .Title }}</a></li>
        {{ end }}
        </ul>
    </main>
    {{ partial "footer" . }}
    <script src="/theme/zhurongshuo/js/theme.js"></script>
</body>
</html>
```

```html
<!-- templates/partials/header.html -->
<header>
    <nav>
        <a href="/">{{ site.Title }}</a>
    </nav>
</header>
```

```html
<!-- templates/partials/footer.html -->
<footer>
    <p>&copy; {{ now.Year }} {{ site.Title }}</p>
    {{ darkModeToggle }}
</footer>
```

- [ ] **Step 6: Create asset files**

```css
/* assets/css/main.css */
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; line-height: 1.6; max-width: 800px; margin: 0 auto; padding: 1rem; }
```

```js
// assets/js/theme.js
// 占位：深色模式切换等功能
```

- [ ] **Step 7: Build the .so plugin**

```bash
cd plugins/zhurongshuo
go build -buildmode=plugin -o ../../zhurongshuo.so .
```

- [ ] **Step 8: Commit**

```bash
git add plugins/zhurongshuo/
git commit -m "feat(theme): add zhurongshuo official theme plugin"
```

---

### Task 8: 集成主题管理器到 daemon 启动流程

**Files:**
- Modify: `cmd/huan/daemon.go`（创建 ThemeManager 并传递给构建管线）
- Modify: `cmd/huan/dev.go`（同上）
- Modify: `internal/daemon/daemon.go`（持有 ThemeManager 引用）

**Interfaces:**
- Consumes: `theme.Manager`、`build.Options`
- Produces: daemon 启动时自动激活配置的主题

- [ ] **Step 1: 在 daemon 中创建 ThemeManager 并自动激活**

```go
// cmd/huan/daemon.go — 在 daemon 启动逻辑中

// 创建 plugin registry
registry, err := newPluginRegistry(cfg)
if err != nil { /* 处理错误 */ }

// 创建 ThemeManager 并自动激活
themeMgr := theme.NewManager(registry)
if cfg.Theme != "" {
    if err := themeMgr.Activate(cfg.Theme); err != nil {
        fmt.Fprintf(os.Stderr, "huan: theme activate %q: %v\n", cfg.Theme, err)
    }
}

// 将 ThemeManager 传递给 daemon
daemonOpts := daemon.Options{
    // ... 现有选项
    ThemeManager: themeMgr,
}
```

- [ ] **Step 2: 在 daemon 的构建调用中传递 ThemeManager**

```go
// internal/daemon/daemon.go — 构建时传递 ThemeManager

opts := build.Options{
    SourceDir:     d.opts.SourceDir,
    OutputDir:     d.opts.OutputDir,
    // ... 其他选项
    ThemeManager:  d.opts.ThemeManager,
}
```

- [ ] **Step 3: 在 daemon 的静态文件服务中挂载主题资源**

```go
// internal/daemon/daemon.go — HTTP handler 中

if d.opts.ThemeManager != nil {
    if tp := d.opts.ThemeManager.Active(); tp != nil {
        mux.Handle("/theme/" + tp.Name() + "/", http.FileServer(http.FS(tp.Assets())))
    }
}
```

- [ ] **Step 4: Build and test**

Run: `go build -o huan ./cmd/huan && ./huan daemon &`
Expected: Daemon starts, theme activated

- [ ] **Step 5: Commit**

```bash
git add cmd/huan/daemon.go cmd/huan/dev.go internal/daemon/
git commit -m "feat(daemon): integrate theme manager into daemon startup"
```

---

### Task 9: Admin API 主题管理端点

**Files:**
- Modify: `internal/admin/handler.go`
- Modify: `internal/admin/api.go`
- Modify: `internal/admin/types.go`
- Web: `web/admin/src/`（React 前端主题管理页面）

**Interfaces:**
- Consumes: `theme.Manager`
- Produces: Admin API 端点 `/admin/api/theme`、`/admin/api/theme/activate`、`/admin/api/theme/deactivate`

- [ ] **Step 1: 在 Admin API 中添加主题管理端点**

```go
// internal/admin/api.go — 新增 themeHandler

type themeAPIHandler struct {
    themeManager *theme.Manager
}

func (h *themeAPIHandler) list(w http.ResponseWriter, r *http.Request) {
    available := h.themeManager.ListAvailable()
    active := h.themeManager.ActiveName()
    result := make([]map[string]any, 0, len(available))
    for _, tp := range available {
        info := tp.Info()
        entry := map[string]any{
            "name":        info.Name,
            "version":     info.Version,
            "author":      info.Author,
            "description": info.Description,
            "active":      info.Name == active,
            "templates":   len(tp.Templates()),
            "funcs":       len(tp.FuncMap()),
        }
        result = append(result, entry)
    }
    json.NewEncoder(w).Encode(map[string]any{"themes": result})
}

func (h *themeAPIHandler) activate(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name string `json:"name"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }
    if err := h.themeManager.Activate(req.Name); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    json.NewEncoder(w).Encode(map[string]string{"status": "ok", "theme": req.Name})
}

func (h *themeAPIHandler) deactivate(w http.ResponseWriter, r *http.Request) {
    h.themeManager.Deactivate()
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 2: 注册路由**

```go
// internal/admin/handler.go — 在 NewHandler 中注册

// Theme API
if opts.ThemeManager != nil {
    th := &themeAPIHandler{themeManager: opts.ThemeManager}
    mux.Handle("/admin/api/theme", apiWithAuth)
    mux.Handle("/admin/api/theme/activate", apiWithAuth)
    mux.Handle("/admin/api/theme/deactivate", apiWithAuth)
}
```

- [ ] **Step 3: Build**

Run: `go build -o huan ./cmd/huan`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add internal/admin/
git commit -m "feat(admin): add theme management API endpoints"
```

---

### Task 10: 端到端测试和文档

**Files:**
- Modify: `docs/adr/0003-unified-plugin-system.md`（更新能力矩阵）
- Create: `docs/adr/0012-theme-plugin-system.md`
- Run: 集成测试

- [ ] **Step 1: 编写 ADR 文档**

```markdown
# ADR 0012: 主题插件系统

- **状态**: Accepted
- **日期**: 2026-07-27
- **决策者**: 用户 + Claude

## 背景

在完成 Deployer / Translator / ImageProcessor 三种能力后，引入第四种能力：主题插件。

## 决策

1. 主题作为 .so 插件（ThemePlugin 接口），与现有 plugin 系统一致
2. 模板嵌入 .so（//go:embed），单文件部署
3. 全局唯一激活，任何时候最多一个主题
4. 构建 Hook 为可选接口（ThemeHooks）
5. 首个官方主题名称为 zhurongshuo
```

- [ ] **Step 2: 运行全量测试**

Run: `go test ./...`
Expected: ALL PASS

- [ ] **Step 3: 构建并验证**

```bash
go build -o huan ./cmd/huan
./huan plugin list
./huan theme list
```

- [ ] **Step 4: Commit**

```bash
git add docs/adr/0012-theme-plugin-system.md
git commit -m "docs: add ADR 0012 for theme plugin system"
```