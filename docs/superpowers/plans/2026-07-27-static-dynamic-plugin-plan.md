# 静态/动态插件分类实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将插件分为 static/dynamic/mixed 三类，3 个 SEO 插件从 internal 迁移到 plugins/ 目录，支持静态构建时加载 .so 插件。

**架构:** (1) config 新增 PluginConfig 类型支持 category 字段；(2) Loader 扩展按 category 过滤加载；(3) 3 个 SEO 插件迁移到 plugins/ 目录，建独立 go.mod；(4) cmd/huan/plugins.go 重构为通用加载逻辑；(5) 集成到 build/dev/daemon 流程。

**Tech Stack:** Go + plugin (go build -buildmode=plugin) + cobra

## 全局约束

- category: static — build/dev 时加载，daemon 时不加载
- category: dynamic — daemon 时加载，build/dev 时不加载
- category: mixed — 都加载
- 未指定 category 的插件默认 dynamic（向后兼容）
- 插件名使用下划线或连字符，需与 yaml key 一致

---

### Task 1: 修改 Config 支持 PluginConfig

**Files:**
- Modify: `internal/config/config.go` — 新增 `PluginConfig` 类型，修改 `Plugins` 字段类型

**Interfaces:**
- Produces: `config.PluginConfig` 结构体

- [ ] **Step 1: 新增 PluginConfig 类型**

```go
// internal/config/config.go — 新增

// PluginConfig 是单个插件的配置。
type PluginConfig struct {
    Category string         `yaml:"category"`  // "static" | "dynamic" | "mixed"，空 = dynamic
    Config   map[string]any `yaml:",inline"`
}
```

修改 `Config` 结构体中的 `Plugins` 字段：

```go
// 修改前
Plugins map[string]map[string]any `yaml:"plugins"`

// 修改后
Plugins map[string]PluginConfig `yaml:"plugins"`
```

- [ ] **Step 2: 添加获取 raw config 的兼容方法**

```go
// internal/config/config.go — 新增

// PluginRawConfig 返回插件的原始配置（不含 category 字段）。
// 旧代码中 `cfg.Plugins["cloudflare"]` 拿到的是 map[string]any，
// 现在需要改为调用此方法。
func (p PluginConfig) RawConfig() map[string]any {
    return p.Config
}
```

- [ ] **Step 3: 修复所有引用 `cfg.Plugins` 的代码**

`cfg.Plugins[name]` 从 `map[string]any` 变为 `PluginConfig`，需要修改：
- `cmd/huan/plugins.go` 中 `cfg.Plugins[name]` → `cfg.Plugins[name].Config`（或 `RawConfig()`）
- `cmd/huan/plugin_cmd.go` 中 `cfg.Plugins[args[0]]` 同理
- `internal/admin/api.go` 中 `cfg.Plugins` 的引用
- `internal/plugin/validate.go` 中 `ValidateRawConfigs` 的签名需要适配

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: 运行全量编译**

Run: `go build ./...`
Expected: 所有引用处已修复

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add PluginConfig type with category field"
```

---

### Task 2: 扩展 Loader 支持按 Category 加载

**Files:**
- Modify: `internal/plugin/loader.go` — 新增 Category 常量和按 category 加载方法
- Test: `internal/plugin/loader_test.go`

**Interfaces:**
- Consumes: `config.PluginConfig`（来自 Task 1）
- Produces: `CategoryStatic`, `CategoryDynamic`, `CategoryMixed` 常量

- [ ] **Step 1: 添加 Category 常量**

```go
// internal/plugin/loader.go — 新增

const (
    CategoryStatic  = "static"
    CategoryDynamic = "dynamic"
    CategoryMixed   = "mixed"
)
```

- [ ] **Step 2: 添加 ShouldLoadInCategory 辅助函数**

```go
// internal/plugin/loader.go — 新增

// ShouldLoadInCategory 判断给定 category 的插件是否应在当前 mode 下加载。
// mode 为 "build" 或 "daemon"。
func ShouldLoadInCategory(pluginCategory, mode string) bool {
    if pluginCategory == "" {
        pluginCategory = CategoryDynamic // 默认 dynamic
    }
    switch pluginCategory {
    case CategoryStatic:
        return mode == "build"
    case CategoryDynamic:
        return mode == "daemon"
    case CategoryMixed:
        return true // 都加载
    default:
        return mode == "daemon" // 未知 category 默认 dynamic
    }
}
```

- [ ] **Step 3: 添加测试**

```go
// internal/plugin/loader_test.go — 新增

func TestShouldLoadInCategory(t *testing.T) {
    tests := []struct {
        category string
        mode     string
        expected bool
    }{
        {"static", "build", true},
        {"static", "daemon", false},
        {"dynamic", "build", false},
        {"dynamic", "daemon", true},
        {"mixed", "build", true},
        {"mixed", "daemon", true},
        {"", "build", false},  // 默认 dynamic
        {"", "daemon", true},  // 默认 dynamic
        {"unknown", "build", false},
        {"unknown", "daemon", true},
    }
    for _, tt := range tests {
        got := ShouldLoadInCategory(tt.category, tt.mode)
        if got != tt.expected {
            t.Errorf("ShouldLoadInCategory(%q, %q) = %v, want %v", tt.category, tt.mode, got, tt.expected)
        }
    }
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/plugin/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plugin/loader.go internal/plugin/loader_test.go
git commit -m "feat(plugin): add category constants and ShouldLoadInCategory"
```

---

### Task 3: 迁移 seo-injector 插件到 plugins/seo-injector

**Files:**
- Create: `plugins/seo-injector/plugin_main.go`
- Create: `plugins/seo-injector/plugin.go`
- Create: `plugins/seo-injector/plugin/plugin.go`
- Create: `plugins/seo-injector/go.mod`
- Files from `internal/seo/injector/` (copy logic, NOT import)

**参考:** `plugins/cloudflare/` 和 `plugins/zhurongshuo/plugin/plugin.go` 的自包含类型模式。

- [ ] **Step 1: 创建 plugin/plugin.go（自包含类型副本）**

```go
// plugins/seo-injector/plugin/plugin.go
package plugin

type Plugin interface {
    Name() string
}

// 自包含的 build.Hook 接口副本
type Hook interface {
    Plugin
    OnContentLoaded(ctx context.Context, pages []any) ([]any, error)
    OnPageRendered(ctx context.Context, page any) error
    OnOutputWritten(ctx context.Context, outputDir string) error
}
```

- [ ] **Step 2: 创建 plugin.go（实现逻辑）**

从 `internal/seo/injector/plugin.go` 复制逻辑，将 `github.com/iannil/huan/internal/plugin` 的引用替换为自包含 `plugin` 包。

```go
// plugins/seo-injector/plugin.go
package main

import (
    "context"
    "github.com/iannil/huan-plugin-seo-injector/plugin"
)

type SEOInjector struct {
    cfg Config
}

func (p *SEOInjector) Name() string { return "seo_injector" }
func (p *SEOInjector) OnContentLoaded(ctx context.Context, pages []any) ([]any, error) { return nil, nil }
func (p *SEOInjector) OnPageRendered(ctx context.Context, page any) error { return nil }
func (p *SEOInjector) OnOutputWritten(ctx context.Context, outputDir string) error {
    // 原有逻辑：扫描 outputDir 中 HTML 文件，注入 SEO meta 标签
}
```

- [ ] **Step 3: 创建 plugin_main.go**

```go
// plugins/seo-injector/plugin_main.go
package main

import "github.com/iannil/huan-plugin-seo-injector/plugin"

func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    return &SEOInjector{cfg: *parsedCfg}, nil
}

func main() {}
```

- [ ] **Step 4: 创建 go.mod**

```go
module github.com/iannil/huan-plugin-seo-injector

go 1.26.2

require golang.org/x/net v0.56.0
```

- [ ] **Step 5: 构建验证**

```bash
cd plugins/seo-injector && go build -buildmode=plugin -o ../../seo-injector.so .
```

- [ ] **Step 6: Commit**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
git add plugins/seo-injector/
git commit -m "feat(plugin): migrate seo-injector to plugins/seo-injector"
```

---

### Task 4: 迁移 html-injector 插件到 plugins/html-injector

**Files:**
- Create: `plugins/html-injector/plugin_main.go`
- Create: `plugins/html-injector/plugin.go`
- Create: `plugins/html-injector/plugin/plugin.go`
- Create: `plugins/html-injector/go.mod`

**步骤**与 Task 3 一致，从 `internal/seo/htmlinjector/` 复制逻辑。

---

### Task 5: 迁移 sitemap-enhancer 插件到 plugins/sitemap-enhancer

**Files:**
- Create: `plugins/sitemap-enhancer/plugin_main.go`
- Create: `plugins/sitemap-enhancer/plugin.go`
- Create: `plugins/sitemap-enhancer/plugin/plugin.go`
- Create: `plugins/sitemap-enhancer/go.mod`

**步骤**与 Task 3 一致，从 `internal/seo/sitemap/` 复制逻辑。

---

### Task 6: 重构 plugins.go — 统一静态/动态加载

**Files:**
- Modify: `cmd/huan/plugins.go` — 移除 hardcoded case，改为通用加载
- Modify: `cmd/huan/main.go` — runBuild 中加载静态插件
- Modify: `cmd/huan/dev.go` — runBuild 加载静态插件
- Modify: `cmd/huan/daemon.go` — 传递静态插件并让 LifecycleManager 加载动态插件
- Modify: `cmd/huan/plugin_cmd.go` — 适配新注册表
- Modify: `cmd/huan/theme_cmd.go` — 适配新注册表
- Modify: `cmd/huan/deploy.go` — 适配新注册表

- [ ] **Step 1: 重写 plugins.go**

```go
// cmd/huan/plugins.go
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/iannil/huan/internal/config"
    "github.com/iannil/huan/internal/deploy"
    "github.com/iannil/huan/internal/image"
    "github.com/iannil/huan/internal/plugin"
    "github.com/iannil/huan/internal/theme"
    "github.com/iannil/huan/internal/translate"
)

// newPluginRegistry 创建插件注册表，加载所有 category: static 和 category: mixed 的插件。
func newPluginRegistry(cfg *config.Config, sourceDir string) (*plugin.Registry, error) {
    r := plugin.NewRegistry()
    pluginDir := filepath.Join(sourceDir, "plugins")
    loader := plugin.NewLoader(pluginDir)

    // 加载所有 .so 插件，按 category 过滤
    results, err := loader.ScanAndLoadByCategory(cfg.Plugins, plugin.CategoryStatic, plugin.CategoryMixed)
    if err != nil {
        return nil, fmt.Errorf("plugin: scan static plugins: %w", err)
    }
    for _, result := range results {
        name := result.Plugin.Name()
        if _, exists := r.Get(name); exists {
            fmt.Fprintf(os.Stderr, "huan: plugin %q: name conflict, skipping\n", name)
            continue
        }
        if err := r.Register(result.Plugin); err != nil {
            fmt.Fprintf(os.Stderr, "huan: plugin %q: register error: %v\n", name, err)
        }
    }

    // 验证配置
    // ...
    return r, nil
}
```

- [ ] **Step 2: 更新所有调用 `newPluginRegistry` 的地方**

`newPluginRegistry(cfg)` → `newPluginRegistry(cfg, sourceDir)`（需要 sourceDir 参数）

- [ ] **Step 3: 更新 runBuild 传递静态插件**

```go
// cmd/huan/main.go — runBuild
reg, _ := newPluginRegistry(cfg, sourceDir)
buildOpts := build.Options{
    // ...
    PluginRegistry: reg,
}

// 创建 ThemeManager 并激活
themeMgr := theme.NewManager(reg)
if cfg.Theme != "" {
    themeMgr.Activate(cfg.Theme)
}
buildOpts.ThemeManager = themeMgr
```

- [ ] **Step 4: 更新 capabilitiesLabels**

```go
// cmd/huan/plugins.go — capabilityLabels 保持不变
// 仍然是类型断言检查，不需要修改
```

- [ ] **Step 5: 编译验证**

Run: `go build ./...`
Expected: 成功

- [ ] **Step 6: Commit**

```bash
git add cmd/huan/
git commit -m "feat(plugin): refactor plugins.go for static/dynamic plugin loading"
```

---

### Task 7: 删除 internal/seo/ 目录

**Files:**
- Delete: `internal/seo/injector/`
- Delete: `internal/seo/htmlinjector/`
- Delete: `internal/seo/sitemap/`
- Delete: `internal/seo/`（空目录）
- Modify: 任何引用 internal/seo 的导入

- [ ] **Step 1: 确认没有其他代码引用 internal/seo**

```bash
grep -rn "internal/seo" --include="*.go" .
```

- [ ] **Step 2: 删除目录**

```bash
git rm -r internal/seo/
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git commit -m "chore: remove internal/seo/ (migrated to plugins/)"
```

---

### Task 8: 端到端验证

**Files:**
- Run: zhurongshuo 站点构建验证
- Run: 全量测试

- [ ] **Step 1: 全量测试**

```bash
go test ./...
```

- [ ] **Step 2: 构建插件**

```bash
cd plugins/seo-injector && go build -buildmode=plugin -o ../../seo-injector.so .
cd plugins/html-injector && go build -buildmode=plugin -o ../../html-injector.so .
cd plugins/sitemap-enhancer && go build -buildmode=plugin -o ../../sitemap-enhancer.so .
cd plugins/zhurongshuo && go build -buildmode=plugin -o ../../zhurongshuo.so .
```

- [ ] **Step 3: 构建 zhurongshuo 站点**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
rm -rf docs
/Users/rong.zhu/Code/zhurong/huan/huan build
```

- [ ] **Step 4: 验证输出**

```bash
find docs/en/books -name "index.html" | wc -l
find docs/en/practices -name "index.html" | wc -l
```