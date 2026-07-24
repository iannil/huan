# Sitemap 增强器 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Sitemap 增强器插件，在 `OnOutputWritten` 阶段自动补全 sitemap.xml 中缺失的 `<priority>` 和 `<changefreq>`

**Architecture:** 插件在 `internal/seo/sitemap/` 实现 `build.Hook` + `SchemaProvider`，核心在 `OnOutputWritten` 阶段读取 sitemap.xml，解析 XML，按页面种类推算 priority/changefreq，写回

**Tech Stack:** Go + `encoding/xml` (标准库)

## Global Constraints

- 插件名称 `"sitemap_enhancer"`，注册在 `plugins:` 命名空间
- Schema 必须实现 `plugin.SchemaProvider` 接口
- Config 所有字段都有默认值，不配也能工作
- 只实现 `OnOutputWritten`，其他 Hook 方法返回 nil
- 集合不中断语义：失败只 log warning，不中止构建
- 不覆盖已有 `<priority>`/`<changefreq>`（frontmatter 优先）
- guessKind 必须独立实现（避免跨包依赖 `internal/seo/injector/`）
- 使用 `encoding/xml` 标准库，不引入第三方 XML 库

---

### Task 1: Sitemap 增强器核心逻辑 — Config + Enhance 函数

**Files:**
- Create: `internal/seo/sitemap/plugin.go`
- Create: `internal/seo/sitemap/enhance.go`
- Test: `internal/seo/sitemap/plugin_test.go`

**Interfaces:**
- Produces: `Config` struct, `ParseConfig(raw map[string]any) (*Config, error)`, `EnhanceSitemap(xmlSrc string, opts *EnhanceOptions) (string, error)`, `GuessKindFromURL(path string) string`

- [ ] **Step 1: 创建 `internal/seo/sitemap/plugin.go` — Config 类型 + ParseConfig + SchemaProvider**

```go
package sitemap

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// Config is the sitemap enhancer configuration from huan.yaml plugins.sitemap_enhancer.*
type Config struct {
	DefaultPriority   map[string]float64 `yaml:"defaultPriority"`
	DefaultChangefreq map[string]string  `yaml:"defaultChangefreq"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultPriority: map[string]float64{
			"home":     1.0,
			"page":     0.8,
			"section":  0.6,
			"taxonomy": 0.5,
			"term":     0.4,
		},
		DefaultChangefreq: map[string]string{
			"home":     "daily",
			"page":     "weekly",
			"section":  "weekly",
			"taxonomy": "weekly",
			"term":     "monthly",
		},
	}
}

// ParseConfig parses raw yaml config into Config.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["defaultPriority"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("defaultPriority: expected map, got %T", v)
		}
		parsed := make(map[string]float64, len(m))
		for k, val := range m {
			f, err := toFloat64(val)
			if err != nil {
				return nil, fmt.Errorf("defaultPriority.%s: %w", k, err)
			}
			parsed[k] = f
		}
		cfg.DefaultPriority = parsed
	}
	if v, ok := raw["defaultChangefreq"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("defaultChangefreq: expected map, got %T", v)
		}
		parsed := make(map[string]string, len(m))
		for k, val := range m {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("defaultChangefreq.%s: expected string, got %T", k, val)
			}
			parsed[k] = s
		}
		cfg.DefaultChangefreq = parsed
	}
	return cfg, nil
}

func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "defaultPriority", Type: "map", Required: false, Description: "per-kind priority values"},
		{Key: "defaultChangefreq", Type: "map", Required: false, Description: "per-kind changefreq values"},
	}}
}
```

- [ ] **Step 2: 创建 `internal/seo/sitemap/enhance.go` — 核心增强逻辑 + guessKind**

```go
package sitemap

import (
	"encoding/xml"
	"fmt"
	"path"
	"strings"
)

// EnhanceOptions carries parameters for a single sitemap enhancement.
type EnhanceOptions struct {
	DefaultPriority   map[string]float64
	DefaultChangefreq map[string]string
}

// urlEntry represents a single <url> element in sitemap.xml.
type urlEntry struct {
	Loc        string  `xml:"loc"`
	Lastmod    string  `xml:"lastmod,omitempty"`
	Changefreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// urlSet is the root element of sitemap.xml.
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

// EnhanceSitemap reads sitemap XML, fills in missing priority/changefreq, and returns the enhanced XML.
// If the XML cannot be parsed, returns the original src unchanged.
func EnhanceSitemap(src string, opts *EnhanceOptions) string {
	var us urlSet
	if err := xml.Unmarshal([]byte(src), &us); err != nil {
		return src
	}
	if len(us.URLs) == 0 {
		return src
	}

	changed := false
	for i, u := range us.URLs {
		kind := GuessKindFromURL(u.Loc)

		// Fill priority if missing
		if u.Priority == 0 && opts != nil && opts.DefaultPriority != nil {
			if pri, ok := opts.DefaultPriority[kind]; ok {
				us.URLs[i].Priority = pri
				changed = true
			}
		}

		// Fill changefreq if missing
		if u.Changefreq == "" && opts != nil && opts.DefaultChangefreq != nil {
			if cf, ok := opts.DefaultChangefreq[kind]; ok {
				us.URLs[i].Changefreq = cf
				changed = true
			}
		}
	}

	if !changed {
		return src
	}

	// Re-marshal with nice formatting
	output, err := xml.MarshalIndent(us, "", "  ")
	if err != nil {
		return src
	}

	return xml.Header + string(output) + "\n"
}

// GuessKindFromURL infers the page kind from a sitemap <loc> URL.
// The URL may be absolute (https://example.com/posts/) or relative (/posts/).
func GuessKindFromURL(loc string) string {
	// Extract path from absolute URL if needed
	pathPart := loc
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		// Find the path after the host
		if idx := strings.Index(loc, "/", 8); idx >= 0 {
			pathPart = loc[idx:]
		} else {
			pathPart = "/"
		}
	}

	// Normalize: remove trailing slash, remove index.html
	clean := strings.TrimSuffix(pathPart, "/")
	clean = strings.TrimSuffix(clean, "/index.html")

	// Root path → home
	if clean == "" || clean == "/" {
		return "home"
	}

	// Split into segments
	clean = strings.TrimPrefix(clean, "/")
	segments := strings.Split(clean, "/")

	// /tags/ → taxonomy, /tags/something/ → term
	if len(segments) >= 1 && segments[0] == "tags" {
		if len(segments) == 1 {
			return "taxonomy"
		}
		return "term"
	}

	// /categories/ → taxonomy, /categories/something/ → term
	if len(segments) >= 1 && segments[0] == "categories" {
		if len(segments) == 1 {
			return "taxonomy"
		}
		return "term"
	}

	// /page/N/ → page (paginated home)
	if len(segments) >= 1 && segments[0] == "page" {
		return "page"
	}

	// Single segment → section (e.g. /posts/, /about/)
	if len(segments) == 1 {
		return "section"
	}

	// Multiple segments → page (e.g. /posts/my-post/, /2024/01/post/)
	return "page"
}
```

- [ ] **Step 3: 创建测试文件 `internal/seo/sitemap/plugin_test.go`**

```go
package sitemap

import (
	"strings"
	"testing"
)

func TestParseConfig_Default(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	if cfg.DefaultPriority["home"] != 1.0 {
		t.Errorf("home priority = %f, want 1.0", cfg.DefaultPriority["home"])
	}
	if cfg.DefaultChangefreq["page"] != "weekly" {
		t.Errorf("page changefreq = %q, want weekly", cfg.DefaultChangefreq["page"])
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	raw := map[string]any{
		"defaultPriority": map[string]any{
			"home": 0.9,
			"page": 0.7,
		},
		"defaultChangefreq": map[string]any{
			"home": "hourly",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.DefaultPriority["home"] != 0.9 {
		t.Errorf("home priority = %f, want 0.9", cfg.DefaultPriority["home"])
	}
	if cfg.DefaultPriority["page"] != 0.7 {
		t.Errorf("page priority = %f, want 0.7", cfg.DefaultPriority["page"])
	}
	if cfg.DefaultChangefreq["home"] != "hourly" {
		t.Errorf("home changefreq = %q, want hourly", cfg.DefaultChangefreq["home"])
	}
	// Unoverridden defaults should still be present
	if cfg.DefaultChangefreq["page"] != "weekly" {
		t.Errorf("page changefreq = %q, want weekly", cfg.DefaultChangefreq["page"])
	}
}

func TestGuessKindFromURL(t *testing.T) {
	tests := []struct {
		loc  string
		kind string
	}{
		{"https://example.com/", "home"},
		{"https://example.com/index.html", "home"},
		{"/", "home"},
		{"https://example.com/posts/", "section"},
		{"https://example.com/posts/my-post/", "page"},
		{"https://example.com/about/", "section"},
		{"https://example.com/2024/01/post/", "page"},
		{"https://example.com/tags/", "taxonomy"},
		{"https://example.com/tags/golang/", "term"},
		{"https://example.com/categories/", "taxonomy"},
		{"https://example.com/categories/dev/", "term"},
		{"https://example.com/page/2/", "page"},
		{"/page/2/", "page"},
	}
	for _, tt := range tests {
		got := GuessKindFromURL(tt.loc)
		if got != tt.kind {
			t.Errorf("GuessKindFromURL(%q) = %q, want %q", tt.loc, got, tt.kind)
		}
	}
}

func TestEnhanceSitemap_AddsMissingPriority(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
    <lastmod>2026-01-01T00:00:00-07:00</lastmod>
  </url>
  <url>
    <loc>https://example.com/posts/</loc>
  </url>
  <url>
    <loc>https://example.com/posts/my-post/</loc>
    <priority>0.9</priority>
  </url>
</urlset>`

	opts := &EnhanceOptions{
		DefaultPriority: map[string]float64{
			"home":    1.0,
			"section": 0.6,
			"page":    0.8,
		},
	}
	result := EnhanceSitemap(input, opts)

	// Home gets priority 1.0
	if !strings.Contains(result, "<priority>1</priority>") {
		t.Errorf("expected home priority 1, got: %s", result)
	}
	// Section gets priority 0.6
	if !strings.Contains(result, "<priority>0.6</priority>") {
		t.Errorf("expected section priority 0.6, got: %s", result)
	}
	// Existing priority 0.9 should NOT be overwritten
	if !strings.Contains(result, "<priority>0.9</priority>") {
		t.Errorf("expected existing priority 0.9 preserved, got: %s", result)
	}
}

func TestEnhanceSitemap_AddsMissingChangefreq(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
  </url>
  <url>
    <loc>https://example.com/posts/my-post/</loc>
    <changefreq>hourly</changefreq>
  </url>
</urlset>`

	opts := &EnhanceOptions{
		DefaultChangefreq: map[string]string{
			"home": "daily",
			"page": "weekly",
		},
	}
	result := EnhanceSitemap(input, opts)

	if !strings.Contains(result, "<changefreq>daily</changefreq>") {
		t.Errorf("expected home changefreq daily, got: %s", result)
	}
	// Existing changefreq should NOT be overwritten
	if !strings.Contains(result, "<changefreq>hourly</changefreq>") {
		t.Errorf("expected existing changefreq hourly preserved, got: %s", result)
	}
}

func TestEnhanceSitemap_InvalidXML(t *testing.T) {
	input := "not valid xml"
	result := EnhanceSitemap(input, &EnhanceOptions{})
	if result != input {
		t.Errorf("expected unchanged for invalid XML")
	}
}

func TestEnhanceSitemap_NoChanges(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
    <priority>1.0</priority>
    <changefreq>daily</changefreq>
  </url>
</urlset>`
	// All fields already present, no options
	result := EnhanceSitemap(input, nil)
	if result != input {
		t.Errorf("expected unchanged when all fields present and no options")
	}
}

func TestEnhanceSitemap_NilOptions(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
  </url>
</urlset>`
	result := EnhanceSitemap(input, nil)
	if result != input {
		t.Errorf("expected unchanged when opts is nil")
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd /Users/rong.zhu/Code/zhurong/huan && go test ./internal/seo/sitemap/ -v`
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/seo/sitemap/
git commit -m "feat(seo): add sitemap enhancer core logic (Config + EnhanceSitemap + GuessKind)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: Sitemap 增强器插件集成 — Plugin + 注册

**Files:**
- Modify: `internal/seo/sitemap/plugin.go` — 添加 SitemapEnhancer struct 和 Hook 实现
- Modify: `cmd/huan/plugins.go` — 添加 `case "sitemap_enhancer"` 注册
- Test: `internal/seo/sitemap/plugin_test.go` — 添加插件级测试

**Interfaces:**
- Consumes: `build.Hook` interface, `plugin.SchemaProvider`
- Produces: `SitemapEnhancer` plugin struct implementing `build.Hook` + `plugin.SchemaProvider`

- [ ] **Step 1: 在 `internal/seo/sitemap/plugin.go` 末尾添加 SitemapEnhancer struct 和 Hook 实现**

```go
package sitemap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/plugin"
)

// SitemapEnhancer is the build.Hook plugin that enhances sitemap.xml with
// priority and changefreq values based on page kind.
type SitemapEnhancer struct {
	cfg  *Config
	logf func(string, ...any)
}

// New creates a new SitemapEnhancer plugin.
func New(cfg *Config) *SitemapEnhancer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &SitemapEnhancer{cfg: cfg, logf: func(format string, args ...any) {}}
}

// SetLogf sets the logger function.
func (p *SitemapEnhancer) SetLogf(fn func(string, ...any)) {
	p.logf = fn
}

// Name returns the plugin name.
func (p *SitemapEnhancer) Name() string { return "sitemap_enhancer" }

// ConfigSchema returns the plugin.Schema for config validation.
func (p *SitemapEnhancer) ConfigSchema() plugin.Schema {
	return p.cfg.ConfigSchema()
}

// OnContentLoaded is a no-op for this plugin.
func (p *SitemapEnhancer) OnContentLoaded(_ context.Context, pages []*content.Page) ([]*content.Page, error) {
	return nil, nil
}

// OnPageRendered is a no-op for this plugin.
func (p *SitemapEnhancer) OnPageRendered(_ context.Context, page *content.Page) error {
	return nil
}

// OnOutputWritten enhances the sitemap.xml in the output directory.
func (p *SitemapEnhancer) OnOutputWritten(ctx context.Context, outputDir string) error {
	sitemapPath := filepath.Join(outputDir, "sitemap.xml")

	data, err := os.ReadFile(sitemapPath)
	if err != nil {
		p.logf("sitemap-enhancer: read %s: %v\n", sitemapPath, err)
		return nil // collection-not-interruption
	}

	opts := &EnhanceOptions{
		DefaultPriority:   p.cfg.DefaultPriority,
		DefaultChangefreq: p.cfg.DefaultChangefreq,
	}

	result := EnhanceSitemap(string(data), opts)
	if result == string(data) {
		return nil // no changes
	}

	if err := os.WriteFile(sitemapPath, []byte(result), 0644); err != nil {
		p.logf("sitemap-enhancer: write %s: %v\n", sitemapPath, err)
		return nil
	}

	return nil
}
```

- [ ] **Step 2: 在测试文件中添加插件级测试**

```go
func TestNewSitemapEnhancer(t *testing.T) {
	p := New(nil)
	if p.Name() != "sitemap_enhancer" {
		t.Errorf("Name() = %q, want sitemap_enhancer", p.Name())
	}
}

func TestSitemapEnhancer_HooksReturnNil(t *testing.T) {
	p := New(nil)
	pages, err := p.OnContentLoaded(context.Background(), nil)
	if err != nil || pages != nil {
		t.Errorf("OnContentLoaded: err=%v pages=%v", err, pages)
	}
	err = p.OnPageRendered(context.Background(), nil)
	if err != nil {
		t.Errorf("OnPageRendered: %v", err)
	}
}

func TestOnOutputWritten_EnhancesSitemap(t *testing.T) {
	tmpDir := t.TempDir()
	sitemapPath := filepath.Join(tmpDir, "sitemap.xml")
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
  </url>
  <url>
    <loc>https://example.com/posts/</loc>
  </url>
</urlset>`
	if err := os.WriteFile(sitemapPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	p := New(nil)
	p.SetLogf(t.Logf)
	err := p.OnOutputWritten(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("OnOutputWritten: %v", err)
	}

	data, err := os.ReadFile(sitemapPath)
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)
	// Home should get priority 1.0
	if !strings.Contains(result, "<priority>1</priority>") {
		t.Errorf("expected home priority 1, got: %s", result)
	}
	// Section should get priority 0.6
	if !strings.Contains(result, "<priority>0.6</priority>") {
		t.Errorf("expected section priority 0.6, got: %s", result)
	}
}

func TestOnOutputWritten_NoSitemap(t *testing.T) {
	tmpDir := t.TempDir()
	p := New(nil)
	p.SetLogf(t.Logf)
	err := p.OnOutputWritten(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("OnOutputWritten on empty dir: %v", err)
	}
}
```

- [ ] **Step 3: 修改 `cmd/huan/plugins.go`，添加 sitemap_enhancer 编译型插件注册**

在 switch 的 `// ### Compiled-in plugins ###` 区域中添加：

```go
case "sitemap_enhancer":
    raw := cfg.Plugins[name]
    pluginCfg, err := sitemap.ParseConfig(raw)
    if err != nil {
        return nil, fmt.Errorf("plugin %s: %w", name, err)
    }
    if err := r.Register(sitemap.New(pluginCfg)); err != nil {
        return nil, fmt.Errorf("plugin %s: %w", name, err)
    }
```

在 import 块中添加：
```go
"github.com/iannil/huan/internal/seo/sitemap"
```

- [ ] **Step 4: 运行所有测试**

```bash
go test ./internal/seo/sitemap/ ./cmd/huan/ -v
```
Expected: ALL PASS

- [ ] **Step 5: 构建验证**

```bash
go build ./cmd/huan/
```
Expected: Build succeeds

- [ ] **Step 6: 提交**

```bash
git add internal/seo/sitemap/ cmd/huan/plugins.go
git commit -m "feat(seo): integrate sitemap enhancer as compiled-in plugin

Co-Authored-By: Claude <noreply@anthropic.com>"
```