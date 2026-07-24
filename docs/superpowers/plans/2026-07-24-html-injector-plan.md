# HTML 注入器 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 HTML 注入器插件，在 `OnPageRendered` 阶段对每个页面注入自定义 HTML 片段

**Architecture:** 插件在 `internal/seo/htmlinjector/` 实现 `build.Hook` + `SchemaProvider`，核心在 `OnPageRendered` 阶段按 page.Kind 过滤后注入 Head/BodyEnd 片段

**Tech Stack:** Go + `strings` (标准库，无需 HTML 解析器)

## Global Constraints

- 插件名称 `"html_injector"`，注册在 `plugins:` 命名空间
- Schema 必须实现 `plugin.SchemaProvider` 接口
- Config 所有字段都有默认值，不配也能工作（空列表 = 无注入）
- 只实现 `OnPageRendered`，其他 Hook 方法返回 nil
- 集合不中断语义：失败只 log warning，不中止构建
- `IncludeKinds` 和 `ExcludeKinds` 互斥使用（`IncludeKinds` 优先）
- Head 片段注入到 `</head>` 之前，BodyEnd 片段注入到 `</body>` 之前
- 如果目标标签不存在，跳过注入
- 使用 `strings` 库操作，不需要 HTML 解析器

---

### Task 1: HTML 注入器核心逻辑 — Config + Inject 函数

**Files:**
- Create: `internal/seo/htmlinjector/plugin.go`
- Create: `internal/seo/htmlinjector/inject.go`
- Test: `internal/seo/htmlinjector/plugin_test.go`

**Interfaces:**
- Produces: `Config` struct, `ParseConfig(raw map[string]any) (*Config, error)`, `InjectHTML(htmlSrc string, cfg *Config, pageKind string) (string, error)`

- [ ] **Step 1: 创建 `internal/seo/htmlinjector/plugin.go` — Config 类型 + ParseConfig + SchemaProvider**

```go
package htmlinjector

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// Config is the HTML injector configuration from huan.yaml plugins.html_injector.*
type Config struct {
	Head         []string `yaml:"head"`
	BodyEnd      []string `yaml:"bodyEnd"`
	IncludeKinds []string `yaml:"includeKinds"`
	ExcludeKinds []string `yaml:"excludeKinds"`
}

// DefaultConfig returns a Config with sensible defaults (empty lists = no injection).
func DefaultConfig() *Config {
	return &Config{}
}

// ParseConfig parses raw yaml config into Config.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["head"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("head: %w", err)
		}
		cfg.Head = items
	}
	if v, ok := raw["bodyEnd"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("bodyEnd: %w", err)
		}
		cfg.BodyEnd = items
	}
	if v, ok := raw["includeKinds"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("includeKinds: %w", err)
		}
		cfg.IncludeKinds = items
	}
	if v, ok := raw["excludeKinds"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("excludeKinds: %w", err)
		}
		cfg.ExcludeKinds = items
	}
	return cfg, nil
}

func toStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", v)
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "head", Type: "string_slice", Required: false, Description: "HTML fragments to inject before </head>"},
		{Key: "bodyEnd", Type: "string_slice", Required: false, Description: "HTML fragments to inject before </body>"},
		{Key: "includeKinds", Type: "string_slice", Required: false, Description: "only inject for these page kinds"},
		{Key: "excludeKinds", Type: "string_slice", Required: false, Description: "skip injection for these page kinds"},
	}}
}
```

- [ ] **Step 2: 创建 `internal/seo/htmlinjector/inject.go` — 核心注入逻辑**

```go
package htmlinjector

import "strings"

// InjectHTML injects configured HTML fragments into the page content.
// Returns the modified HTML. If no injection is needed, returns the original.
func InjectHTML(htmlSrc string, cfg *Config, pageKind string) string {
	if cfg == nil {
		return htmlSrc
	}

	// Check kind filters
	if len(cfg.IncludeKinds) > 0 {
		if !contains(cfg.IncludeKinds, pageKind) {
			return htmlSrc
		}
	}
	if len(cfg.ExcludeKinds) > 0 {
		if contains(cfg.ExcludeKinds, pageKind) {
			return htmlSrc
		}
	}

	// Inject Head fragments (before </head>)
	if len(cfg.Head) > 0 {
		headClose := strings.Index(htmlSrc, "</head>")
		if headClose >= 0 {
			injection := "\n" + strings.Join(cfg.Head, "\n") + "\n"
			htmlSrc = htmlSrc[:headClose] + injection + htmlSrc[headClose:]
		}
	}

	// Inject BodyEnd fragments (before </body>)
	if len(cfg.BodyEnd) > 0 {
		bodyClose := strings.Index(htmlSrc, "</body>")
		if bodyClose >= 0 {
			injection := "\n" + strings.Join(cfg.BodyEnd, "\n") + "\n"
			htmlSrc = htmlSrc[:bodyClose] + injection + htmlSrc[bodyClose:]
		}
	}

	return htmlSrc
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: 创建测试文件 `internal/seo/htmlinjector/plugin_test.go`**

```go
package htmlinjector

import (
	"strings"
	"testing"
)

func TestParseConfig_Default(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	if len(cfg.Head) != 0 {
		t.Errorf("Head = %v, want empty", cfg.Head)
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	raw := map[string]any{
		"head":    []any{"<script src='a.js'></script>", "<link rel='stylesheet' href='b.css'>"},
		"bodyEnd": []any{"<script>init()</script>"},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Head) != 2 {
		t.Errorf("Head len = %d, want 2", len(cfg.Head))
	}
	if len(cfg.BodyEnd) != 1 {
		t.Errorf("BodyEnd len = %d, want 1", len(cfg.BodyEnd))
	}
}

func TestInjectHTML_HeadInjection(t *testing.T) {
	html := `<html><head><title>Test</title></head><body><p>Content</p></body></html>`
	cfg := &Config{
		Head: []string{`<script src="https://analytics.example.com/script.js"></script>`},
	}
	result := InjectHTML(html, cfg, "page")
	if !strings.Contains(result, `src="https://analytics.example.com/script.js"`) {
		t.Errorf("expected script in output, got: %s", result)
	}
	if !strings.Contains(result, "</head>") {
		t.Error("expected </head> to remain")
	}
}

func TestInjectHTML_BodyEndInjection(t *testing.T) {
	html := `<html><head><title>Test</title></head><body><p>Content</p></body></html>`
	cfg := &Config{
		BodyEnd: []string{`<script>init()</script>`},
	}
	result := InjectHTML(html, cfg, "page")
	if !strings.Contains(result, `<script>init()</script>`) {
		t.Errorf("expected script before </body>, got: %s", result)
	}
}

func TestInjectHTML_Both(t *testing.T) {
	html := `<html><head><title>Test</title></head><body><p>Content</p></body></html>`
	cfg := &Config{
		Head:    []string{`<meta name="test" content="x">`},
		BodyEnd: []string{`<script>init()</script>`},
	}
	result := InjectHTML(html, cfg, "page")
	if !strings.Contains(result, `<meta name="test"`) {
		t.Error("expected head injection")
	}
	if !strings.Contains(result, `<script>init()</script>`) {
		t.Error("expected body end injection")
	}
}

func TestInjectHTML_NoHeadTag(t *testing.T) {
	html := `<html><body><p>Content</p></body></html>`
	cfg := &Config{
		Head: []string{`<script src="x.js"></script>`},
	}
	result := InjectHTML(html, cfg, "page")
	if result != html {
		t.Errorf("expected unchanged when no </head>, got: %s", result)
	}
}

func TestInjectHTML_NoBodyTag(t *testing.T) {
	html := `<html><head><title>Test</title></head><p>Content</p></html>`
	cfg := &Config{
		BodyEnd: []string{`<script>init()</script>`},
	}
	result := InjectHTML(html, cfg, "page")
	if result != html {
		t.Errorf("expected unchanged when no </body>, got: %s", result)
	}
}

func TestInjectHTML_IncludeKinds(t *testing.T) {
	html := `<html><head></head><body><p>Content</p></body></html>`
	cfg := &Config{
		Head:         []string{`<script src="x.js"></script>`},
		IncludeKinds: []string{"page", "home"},
	}

	// page kind should be included
	result := InjectHTML(html, cfg, "page")
	if !strings.Contains(result, `src="x.js"`) {
		t.Error("expected injection for page kind")
	}

	// taxonomy kind should be excluded
	result2 := InjectHTML(html, cfg, "taxonomy")
	if result2 == html {
		t.Error("result should equal html: taxonomy kind excluded")
	}
	if strings.Contains(result2, `src="x.js"`) {
		t.Error("expected no injection for excluded kind")
	}
}

func TestInjectHTML_ExcludeKinds(t *testing.T) {
	html := `<html><head></head><body><p>Content</p></body></html>`
	cfg := &Config{
		Head:         []string{`<script src="x.js"></script>`},
		ExcludeKinds: []string{"taxonomy", "term"},
	}

	// page kind should be included
	result := InjectHTML(html, cfg, "page")
	if !strings.Contains(result, `src="x.js"`) {
		t.Error("expected injection for page kind")
	}

	// taxonomy kind should be excluded
	result2 := InjectHTML(html, cfg, "taxonomy")
	if strings.Contains(result2, `src="x.js"`) {
		t.Error("expected no injection for excluded kind taxonomy")
	}
}

func TestInjectHTML_NilConfig(t *testing.T) {
	html := `<html><head></head><body></body></html>`
	result := InjectHTML(html, nil, "page")
	if result != html {
		t.Error("expected unchanged for nil config")
	}
}

func TestInjectHTML_EmptyConfig(t *testing.T) {
	html := `<html><head></head><body></body></html>`
	cfg := &Config{}
	result := InjectHTML(html, cfg, "page")
	if result != html {
		t.Error("expected unchanged for empty config")
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd /Users/rong.zhu/Code/zhurong/huan && go test ./internal/seo/htmlinjector/ -v`
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/seo/htmlinjector/
git commit -m "feat(seo): add HTML injector core logic (Config + InjectHTML)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: HTML 注入器插件集成 — Plugin + 注册

**Files:**
- Modify: `internal/seo/htmlinjector/plugin.go` — 添加 HTMLInjector struct 和 Hook 实现
- Modify: `cmd/huan/plugins.go` — 添加 `case "html_injector"` 注册
- Test: `internal/seo/htmlinjector/plugin_test.go` — 添加插件级测试

**Interfaces:**
- Consumes: `build.Hook` interface (OnPageRendered), `plugin.SchemaProvider`
- Produces: `HTMLInjector` plugin struct implementing `build.Hook` + `plugin.SchemaProvider`

- [ ] **Step 1: 在 `internal/seo/htmlinjector/plugin.go` 末尾添加 HTMLInjector struct 和 Hook 实现**

```go
package htmlinjector

import (
	"context"
	"fmt"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/plugin"
)

// HTMLInjector is the build.Hook plugin that injects custom HTML fragments.
type HTMLInjector struct {
	cfg  *Config
}

// New creates a new HTMLInjector plugin.
func New(cfg *Config) *HTMLInjector {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &HTMLInjector{cfg: cfg}
}

// Name returns the plugin name.
func (p *HTMLInjector) Name() string { return "html_injector" }

// ConfigSchema returns the plugin.Schema for config validation.
func (p *HTMLInjector) ConfigSchema() plugin.Schema {
	return p.cfg.ConfigSchema()
}

// OnContentLoaded is a no-op for this plugin.
func (p *HTMLInjector) OnContentLoaded(_ context.Context, pages []*content.Page) ([]*content.Page, error) {
	return nil, nil
}

// OnPageRendered injects configured HTML fragments into the page.
func (p *HTMLInjector) OnPageRendered(_ context.Context, page *content.Page) error {
	modified := InjectHTML(string(page.Content), p.cfg, page.Kind)
	if modified != string(page.Content) {
		page.Content = content.HTML(modified)
	}
	return nil
}

// OnOutputWritten is a no-op for this plugin.
func (p *HTMLInjector) OnOutputWritten(_ context.Context, outputDir string) error {
	return nil
}

// Ensure compile-time interface satisfaction.
var _ plugin.Plugin = (*HTMLInjector)(nil)
var _ plugin.SchemaProvider = (*HTMLInjector)(nil)
```

Note: `content.HTML` is `template.HTML` — need to import `"html/template"` or use `template.HTML(modified)`.

- [ ] **Step 2: 在测试文件中添加插件级测试**

```go
func TestNewHTMLInjector(t *testing.T) {
	p := New(nil)
	if p.Name() != "html_injector" {
		t.Errorf("Name() = %q, want html_injector", p.Name())
	}
}

func TestHTMLInjector_OnPageRendered(t *testing.T) {
	cfg := &Config{
		Head: []string{`<script src="test.js"></script>`},
	}
	p := New(cfg)
	page := &content.Page{
		Content: template.HTML(`<html><head><title>Test</title></head><body><p>Content</p></body></html>`),
		Kind:    "page",
	}
	err := p.OnPageRendered(context.Background(), page)
	if err != nil {
		t.Fatalf("OnPageRendered: %v", err)
	}
	if !strings.Contains(string(page.Content), `src="test.js"`) {
		t.Errorf("expected script tag in page content, got: %s", string(page.Content))
	}
}

func TestHTMLInjector_HooksReturnNil(t *testing.T) {
	p := New(nil)
	pages, err := p.OnContentLoaded(context.Background(), nil)
	if err != nil || pages != nil {
		t.Errorf("OnContentLoaded: err=%v pages=%v", err, pages)
	}
	err = p.OnOutputWritten(context.Background(), "")
	if err != nil {
		t.Errorf("OnOutputWritten: %v", err)
	}
}
```

- [ ] **Step 3: 修改 `cmd/huan/plugins.go`，添加 html_injector 编译型插件注册**

在 switch 的 `// ### Compiled-in plugins ###` 区域中添加：

```go
case "html_injector":
    raw := cfg.Plugins[name]
    pluginCfg, err := htmlinjector.ParseConfig(raw)
    if err != nil {
        return nil, fmt.Errorf("plugin %s: %w", name, err)
    }
    if err := r.Register(htmlinjector.New(pluginCfg)); err != nil {
        return nil, fmt.Errorf("plugin %s: %w", name, err)
    }
```

在 import 块中添加：
```go
"github.com/iannil/huan/internal/seo/htmlinjector"
```

- [ ] **Step 4: 运行所有测试**

```bash
go test ./internal/seo/htmlinjector/ ./cmd/huan/ -v
```
Expected: ALL PASS

- [ ] **Step 5: 构建验证**

```bash
go build ./cmd/huan/
```
Expected: Build succeeds

- [ ] **Step 6: 提交**

```bash
git add internal/seo/htmlinjector/ cmd/huan/plugins.go
git commit -m "feat(seo): integrate HTML injector as compiled-in plugin

Co-Authored-By: Claude <noreply@anthropic.com>"
```