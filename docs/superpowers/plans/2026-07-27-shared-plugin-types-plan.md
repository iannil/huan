# 共享插件类型包实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 创建 `pkg/plugin/` 共享类型包，解决 .so 插件跨模块类型断言不兼容的问题，删除所有自包含类型副本。

**架构:** (1) 从 `internal/plugin/` 和 `internal/theme/` 提取接口到 `pkg/plugin/`；(2) 所有 .so 插件通过 `replace` 指令引用主模块的 `pkg/plugin/`；(3) 删除 `plugins/*/plugin/` 下的自包含副本。

**Tech Stack:** Go + plugin (go build -buildmode=plugin) + replace directive

## 全局约束

- `pkg/plugin/` 导出所有插件需要的类型，无 `internal/` 限制
- 插件通过 `github.com/iannil/huan/pkg/plugin` 导入，go.mod 用 `replace` 指向本地
- 删除所有 `plugins/*/plugin/plugin.go` 自包含副本
- 保持与现有 Plugin 接口完全兼容

---

### Task 1: 创建 `pkg/plugin/` 共享类型包

**Files:**
- Create: `pkg/plugin/plugin.go`
- Create: `pkg/plugin/theme.go`
- Create: `pkg/plugin/hook.go`
- Create: `pkg/plugin/schema.go`

**Interfaces:**
- Produces: `pkg/plugin` 包，包含所有插件需要的类型

- [ ] **Step 1: 创建 `pkg/plugin/plugin.go`**

```go
// Package plugin defines the shared plugin types for huan and its .so plugins.
// .so plugins import this package via `github.com/iannil/huan/pkg/plugin`
// (resolved through a replace directive in go.mod) so that concrete types
// are the same across the module boundary — enabling type assertions like
// plugin.Find[theme.ThemePlugin](registry) to work on .so-loaded plugins.
package plugin

import (
	"context"
	"fmt"
	"sort"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

// Plugin is the base interface every plugin satisfies.
type Plugin interface {
	Name() string
}

// Registry holds plugins keyed by Name().
type Registry struct {
	plugins map[string]Plugin
	order   []string
}

func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin)}
}

func (r *Registry) Register(p Plugin) error {
	if p == nil { return fmt.Errorf("plugin: register nil") }
	name := p.Name()
	if name == "" { return fmt.Errorf("plugin: empty name") }
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin: duplicate registration %q", name)
	}
	r.plugins[name] = p
	r.order = append(r.order, name)
	return nil
}

func (r *Registry) Get(name string) (Plugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

func (r *Registry) All() []Plugin {
	out := make([]Plugin, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.plugins[name])
	}
	return out
}

func (r *Registry) Unregister(name string) bool {
	if _, exists := r.plugins[name]; !exists { return false }
	delete(r.plugins, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return true
}

func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

func Find[T any](r *Registry) []T {
	var out []T
	for _, p := range r.All() {
		if t, ok := p.(T); ok {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) SortedNames() []string {
	out := r.Names()
	sort.Strings(out)
	return out
}

// PluginMeta carries human-readable metadata for a plugin.
type PluginMeta struct {
	Version    string   `json:"version"`
	Author     string   `json:"author"`
	RepoURL    string   `json:"repoURL"`
	License    string   `json:"license"`
	Tags       []string `json:"tags"`
	IsOfficial bool     `json:"isOfficial"`
}

type MetadataProvider interface {
	PluginMetadata() PluginMeta
}

type SchemaProvider interface {
	ConfigSchema() Schema
}

type Schema struct {
	Fields []FieldSchema
}

type FieldSchema struct {
	Key         string
	Type        string
	Required    bool
	Default     any
	Description string
}

type EventSubscriber interface {
	SubscribedEvents() []eventbus.EventType
	HandleEvent(ctx context.Context, event eventbus.Event) error
}
```

- [ ] **Step 2: 创建 `pkg/plugin/theme.go`**

```go
package plugin

import (
	"context"
	"html/template"
	"io/fs"
)

// ThemePlugin is the core capability interface for theme plugins.
type ThemePlugin interface {
	Plugin
	Info() ThemeInfo
	Templates() []TemplateEntry
	FuncMap() template.FuncMap
	Assets() fs.FS
}

type ThemeInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	Screenshot  string   `json:"screenshot,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	MinHuanVer  string   `json:"minHuanVer,omitempty"`
}

type TemplateEntry struct {
	Path    string
	Content string
}

type ThemeHooks interface {
	BeforeRender(ctx context.Context) error
	AfterRender(ctx context.Context) error
}

type ShortcodeHandler func(ctx ShortcodeContext) (string, error)

type ShortcodeContext struct {
	Params map[string]string
	Inner  string
	Page   interface{}
	Site   interface{}
}

type ShortcodeProvider interface {
	Shortcodes() map[string]ShortcodeHandler
}
```

- [ ] **Step 3: 创建 `pkg/plugin/hook.go`**

```go
package plugin

import "context"

// Hook is the capability interface for build pipeline hooks.
type Hook interface {
	Plugin
	OnContentLoaded(ctx context.Context, pages []interface{}) ([]interface{}, error)
	OnPageRendered(ctx context.Context, page interface{}) error
	OnOutputWritten(ctx context.Context, outputDir string) error
}
```

- [ ] **Step 4: 运行编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go build ./pkg/plugin/
```

- [ ] **Step 5: Commit**

```bash
git add pkg/plugin/
git commit -m "feat(pkg/plugin): add shared plugin types for .so cross-module compatibility"
```

---

### Task 2: 更新 `internal/plugin/` 引用 `pkg/plugin/`

**Files:**
- Modify: `internal/plugin/plugin.go` — 改为封装 `pkg/plugin/`
- Modify: `internal/plugin/loader.go` — 引用 `pkg/plugin.Plugin`

**思路:** `internal/plugin/` 对 `pkg/plugin/` 做类型别名或直接复用。

- [ ] **Step 1: 更新 internal/plugin/plugin.go**

```go
// Package plugin provides internal convenience re-exports of pkg/plugin types.
// Most internal code should import github.com/iannil/huan/internal/plugin
// which delegates to pkg/plugin for the actual types.
package plugin

import pkgplugin "github.com/iannil/huan/pkg/plugin"

// Re-export all types from pkg/plugin so existing internal imports don't break.
type Plugin = pkgplugin.Plugin
type Registry = pkgplugin.Registry
type PluginMeta = pkgplugin.PluginMeta
type MetadataProvider = pkgplugin.MetadataProvider
type SchemaProvider = pkgplugin.SchemaProvider
type Schema = pkgplugin.Schema
type FieldSchema = pkgplugin.FieldSchema
type EventSubscriber = pkgplugin.EventSubscriber

// Re-export functions
var NewRegistry = pkgplugin.NewRegistry
var Find = pkgplugin.Find[any] // can't alias generics directly; use wrapper
```

Actually, Go type aliases (`type A = B`) work for structs, interfaces, and functions. But `Find[T any](r *Registry) []T` is a generic function — type aliases of generics are tricky.

**Better approach:** Have `internal/plugin/` directly re-import or just import `pkg/plugin` itself. Since all the internal code references `github.com/iannil/huan/internal/plugin`, we need backward compatibility.

Actually, the simplest approach: make the package import `pkg/plugin` and use type aliases:

```go
package plugin

import "github.com/iannil/huan/pkg/plugin"

// Alias all exported types
type Plugin = pkgplugin.Plugin
type Registry = pkgplugin.Registry
type PluginMeta = pkgplugin.PluginMeta
type MetadataProvider = pkgplugin.MetadataProvider
type SchemaProvider = pkgplugin.SchemaProvider
type Schema = pkgplugin.Schema
type FieldSchema = pkgplugin.FieldSchema
// EventSubscriber references eventbus which is internal, keep in internal/plugin

// Find is a generic function — use direct forwarding
func Find[T any](r *Registry) []T {
	return pkgplugin.Find[T](r)
}
```

- [ ] **Step 2: 运行编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/plugin/plugin.go
git commit -m "refactor(plugin): alias pkg/plugin types in internal/plugin"
```

---

### Task 3: 更新 `internal/theme/` 引用 `pkg/plugin/`

**Files:**
- Modify: `internal/theme/types.go` — 使用 `pkgplugin.ThemePlugin` 别名或直接迁移类型

**思路:** 将 `ThemePlugin`、`ThemeInfo`、`TemplateEntry`、`ThemeHooks` 类型定义留在 `internal/theme/` 中，但使用 `pkg/plugin.ThemePlugin` 作为实际类型。

或者更简单：`internal/theme/types.go` 的类型定义移到 `pkg/plugin/theme.go`，`internal/theme/` 做别名。

- [ ] **Step 1: 将 ThemePlugin 等类型改为别名**

```go
package theme

import pkgplugin "github.com/iannil/huan/pkg/plugin"

type ThemePlugin = pkgplugin.ThemePlugin
type ThemeInfo = pkgplugin.ThemeInfo
type TemplateEntry = pkgplugin.TemplateEntry
type ThemeHooks = pkgplugin.ThemeHooks
type ShortcodeProvider = pkgplugin.ShortcodeProvider
type ShortcodeHandler = pkgplugin.ShortcodeHandler
```

Manager 类使用 `ThemePlugin` 接口，不需要改变。

- [ ] **Step 2: 运行编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/theme/types.go
git commit -m "refactor(theme): alias pkg/plugin theme types"
```

---

### Task 4: 更新所有 .so 插件

**Files:**
- Modify: 7 个 `plugins/*/go.mod` — 添加 `replace` 指令
- Modify: 7 个 `plugins/*/plugin_main.go` — 改用 `pkg/plugin.Plugin`
- Delete: 7 个 `plugins/*/plugin/plugin.go` — 删除自包含副本
- Modify: 7 个 `plugins/*/plugin.go` — 改用 `pkg/plugin` 类型

**模式：**

`go.mod` 添加：
```
require github.com/iannil/huan v0.0.0
replace github.com/iannil/huan => ../../
```

`plugin_main.go` 改为：
```go
package main

import "github.com/iannil/huan/pkg/plugin"

func InitPlugin(cfg map[string]any) (interface{}, error) {
	// ...
}

func main() {}
```

注意：`InitPlugin` 返回 `interface{}`，因为 Go 插件符号的类型必须精确匹配。huan 的 Loader 会将其类型断言为 `pkg/plugin.Plugin`。

- [ ] **Step 1: 逐插件修改（cloudflare、qwen3、image-pipeline、seo-injector、html-injector、sitemap-enhancer、zhurongshuo）**

每个插件：
1. 删除 `plugin/plugin.go`（自包含副本）
2. 更新 `plugin.go`（实现代码）中的 import 路径
3. 更新 `plugin_main.go` 的 import
4. 更新 `go.mod`
5. 构建验证

- [ ] **Step 2: 全量构建验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go build ./...

for dir in plugins/*/; do
  (cd "$dir" && go build -buildmode=plugin -o /dev/null .) && echo "OK $dir" || echo "FAIL $dir"
done
```

- [ ] **Step 3: Commit**

```bash
git add plugins/
git commit -m "refactor(plugins): use pkg/plugin shared types, remove self-contained copies"
```

---

### Task 5: 端到端验证

- [ ] **Step 1: 构建所有的 .so 文件**

```bash
for dir in plugins/*/; do
  name=$(basename "$dir")
  (cd "$dir" && go build -buildmode=plugin -o "../../${name}.so" .)
done
```

- [ ] **Step 2: 复制到 zhurongshuo 站点并构建**

```bash
cp *.so /Users/rong.zhu/Code/zhurong/zhurongshuo/plugins/
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
rm -rf docs
/Users/rong.zhu/Code/zhurong/huan/huan build
```

- [ ] **Step 3: 验证主题插件加载**

```bash
./huan theme list
```

应该显示 zhurongshuo 主题可用且可激活。

- [ ] **Step 4: 全量测试**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go test ./...
```
