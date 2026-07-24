# SEO 注入器插件 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现第一个 `build.Hook` 插件：SEO 注入器，在构建后自动补全 HTML 文件中缺失的 SEO meta 标签

**Architecture:** 插件在 `internal/seo/injector/` 实现 `build.Hook` 和 `SchemaProvider`，核心在 `OnOutputWritten` 阶段扫描输出目录的 HTML 文件；在 `cmd/huan/plugins.go` 注册为 compiled-in plugin

**Tech Stack:** Go + `golang.org/x/net/html` (已有依赖)

## Global Constraints

- 插件名称 `"seo_injector"`，注册在 `plugins:` 命名空间
- Schema 必须实现 `plugin.SchemaProvider` 接口
- Config 所有字段都有默认值，不配也能工作
- 只实现 `OnOutputWritten`，其他 Hook 方法返回 nil
- 集合不中断语义：失败只 log warning，不中止构建
- 不覆盖已有标签（frontmatter 优先）
- Tag 注入顺序：description → og:title → og:description → og:url → og:type → og:image → twitter:card → twitter:title → twitter:description
- Og:type 判定：page kind 为 "page" 时用 "article"，否则用 "website"
- `golang.org/x/net/html` 已存在为依赖（`internal/equiv/seo.go` 使用）

---

### Task 1: SEO 注入器核心逻辑 — Config + Inject 函数

**Files:**
- Create: `internal/seo/injector/plugin.go`
- Create: `internal/seo/injector/inject.go`
- Test: `internal/seo/injector/plugin_test.go`

**Interfaces:**
- Produces: `Config` struct, `ParseConfig(raw map[string]any) (*Config, error)`, `InjectHTML(src string, cfg *InjectOptions) (string, error)`, `ExtractPlainText(htmlSrc string) string`, `TruncateToWordBoundary(text string, maxLen int) string`

- [ ] **Step 1: 创建 `internal/seo/injector/plugin.go` — Config 类型 + ParseConfig + SchemaProvider**

```go
package injector

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// Config is the SEO injector configuration from huan.yaml plugins.seo_injector.*
type Config struct {
	DescriptionMaxLength int    `yaml:"descriptionMaxLength"`
	DefaultOGImage       string `yaml:"defaultOGImage"`
	InjectOG             bool   `yaml:"injectOG"`
	InjectTwitter        bool   `yaml:"injectTwitter"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DescriptionMaxLength: 160,
		InjectOG:             true,
		InjectTwitter:        true,
	}
}

// ParseConfig parses raw yaml config into Config.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["descriptionMaxLength"]; ok {
		if f, ok := toFloat64(v); ok {
			cfg.DescriptionMaxLength = int(f)
		} else {
			return nil, fmt.Errorf("descriptionMaxLength: expected number, got %T", v)
		}
	}
	if v, ok := raw["defaultOGImage"]; ok {
		if s, ok := v.(string); ok {
			cfg.DefaultOGImage = s
		} else {
			return nil, fmt.Errorf("defaultOGImage: expected string, got %T", v)
		}
	}
	if v, ok := raw["injectOG"]; ok {
		if b, ok := v.(bool); ok {
			cfg.InjectOG = b
		} else {
			return nil, fmt.Errorf("injectOG: expected bool, got %T", v)
		}
	}
	if v, ok := raw["injectTwitter"]; ok {
		if b, ok := v.(bool); ok {
			cfg.InjectTwitter = b
		} else {
			return nil, fmt.Errorf("injectTwitter: expected bool, got %T", v)
		}
	}
	return cfg, nil
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "descriptionMaxLength", Type: "int", Required: false, Default: 160, Description: "meta description 最大长度"},
		{Key: "defaultOGImage", Type: "string", Required: false, Description: "默认 OG image 路径"},
		{Key: "injectOG", Type: "bool", Required: false, Default: true, Description: "是否注入 OG 标签"},
		{Key: "injectTwitter", Type: "bool", Required: false, Default: true, Description: "是否注入 Twitter Card 标签"},
	}}
}
```

- [ ] **Step 2: 验证 ParseConfig 测试**

Run: `go test ./internal/seo/injector/ -run TestParseConfig -v`
Expected: Package compiles, tests defined below pass

- [ ] **Step 3: 创建 `internal/seo/injector/inject.go` — 核心注入逻辑**

```go
package injector

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// InjectOptions carries parameters for a single HTML injection.
type InjectOptions struct {
	DescriptionMaxLength int
	DefaultOGImage       string
	InjectOG             bool
	InjectTwitter        bool
	PageURL              string // absolute URL of this page
	PageKind             string // "page" | "section" | "home" | "taxonomy" | "term"
	PageTitle            string // page title (already known)
}

// InjectHTML scans HTML <head>, checks existing tags, and injects missing ones.
// Returns modified HTML. If src has no <head>, returns src unchanged.
func InjectHTML(src string, opts *InjectOptions) (string, error) {
	if opts == nil {
		return src, nil
	}

	// Parse existing tags
	existing := ExtractExistingTags(src)

	// Build missing tags
	var tags []string

	// description
	if _, has := existing["description"]; !has {
		desc := extractDescriptionFromHTML(src, opts.DescriptionMaxLength)
		if desc != "" {
			tags = append(tags, fmt.Sprintf(`<meta name="description" content="%s">`, html.EscapeString(desc)))
		}
	}

	if opts.InjectOG {
		// og:title
		if _, has := existing["og:title"]; !has && opts.PageTitle != "" {
			tags = append(tags, fmt.Sprintf(`<meta property="og:title" content="%s">`, html.EscapeString(opts.PageTitle)))
		}

		// og:description
		if _, has := existing["og:description"]; !has {
			desc := extractDescriptionFromHTML(src, opts.DescriptionMaxLength)
			if desc != "" {
				tags = append(tags, fmt.Sprintf(`<meta property="og:description" content="%s">`, html.EscapeString(desc)))
			}
		}

		// og:url
		if _, has := existing["og:url"]; !has && opts.PageURL != "" {
			tags = append(tags, fmt.Sprintf(`<meta property="og:url" content="%s">`, html.EscapeString(opts.PageURL)))
		}

		// og:type
		if _, has := existing["og:type"]; !has {
			ogType := "website"
			if opts.PageKind == "page" {
				ogType = "article"
			}
			tags = append(tags, fmt.Sprintf(`<meta property="og:type" content="%s">`, ogType))
		}

		// og:image
		if _, has := existing["og:image"]; !has && opts.DefaultOGImage != "" {
			tags = append(tags, fmt.Sprintf(`<meta property="og:image" content="%s">`, html.EscapeString(opts.DefaultOGImage)))
		}
	}

	if opts.InjectTwitter {
		// twitter:card
		if _, has := existing["twitter:card"]; !has {
			tags = append(tags, `<meta name="twitter:card" content="summary_large_image">`)
		}

		// twitter:title
		if _, has := existing["twitter:title"]; !has && opts.PageTitle != "" {
			tags = append(tags, fmt.Sprintf(`<meta name="twitter:title" content="%s">`, html.EscapeString(opts.PageTitle)))
		}

		// twitter:description
		if _, has := existing["twitter:description"]; !has {
			desc := extractDescriptionFromHTML(src, opts.DescriptionMaxLength)
			if desc != "" {
				tags = append(tags, fmt.Sprintf(`<meta name="twitter:description" content="%s">`, html.EscapeString(desc)))
			}
		}
	}

	if len(tags) == 0 {
		return src, nil
	}

	// Inject before </head>
	headClose := strings.Index(src, "</head>")
	if headClose < 0 {
		return src, nil
	}

	comment := "\n<!-- huan seo-injector -->\n"
	injection := comment + strings.Join(tags, "\n") + "\n"
	return src[:headClose] + injection + src[headClose:], nil
}

// ExtractExistingTags returns a set of already-present meta tag identifiers.
// Key for name-based: name attribute value. Key for property-based: property attribute value.
func ExtractExistingTags(htmlSrc string) map[string]bool {
	result := make(map[string]bool)
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return result
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			name := getAttr(n, "name")
			prop := getAttr(n, "property")
			if name != "" {
				result[name] = true
			}
			if prop != "" {
				result[prop] = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return result
}

// extractDescriptionFromHTML extracts plain text from <body> and truncates it.
func extractDescriptionFromHTML(htmlSrc string, maxLen int) string {
	bodyText := ExtractPlainText(htmlSrc)
	if bodyText == "" {
		return ""
	}
	return TruncateToWordBoundary(bodyText, maxLen)
}

// ExtractPlainText extracts all text content from <body> of an HTML document.
func ExtractPlainText(htmlSrc string) string {
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return ""
	}
	var buf strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n == nil {
			return
		}
		// Skip <style>, <script>, <nav>, <header>, <footer> content
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "style" || tag == "script" || tag == "nav" || tag == "header" || tag == "footer" {
				return
			}
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if buf.Len() > 0 {
					buf.WriteString(" ")
				}
				buf.WriteString(text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}

	// Find <body>
	var findBody func(*html.Node) *html.Node
	findBody = func(n *html.Node) *html.Node {
		if n == nil {
			return nil
		}
		if n.Type == html.ElementNode && n.Data == "body" {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := findBody(c); found != nil {
				return found
			}
		}
		return nil
	}

	body := findBody(doc)
	if body == nil {
		return ""
	}
	extract(body)
	return strings.TrimSpace(buf.String())
}

// TruncateToWordBoundary truncates text to maxLen characters at the last word boundary.
func TruncateToWordBoundary(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	// Find last space before maxLen
	trimmed := text[:maxLen]
	if idx := strings.LastIndex(trimmed, " "); idx > 0 {
		return text[:idx]
	}
	return trimmed
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
```

- [ ] **Step 4: 编写测试文件 `internal/seo/injector/plugin_test.go`**

```go
package injector

import (
	"testing"
)

func TestParseConfig_Default(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	if cfg.DescriptionMaxLength != 160 {
		t.Errorf("DescriptionMaxLength = %d, want 160", cfg.DescriptionMaxLength)
	}
	if !cfg.InjectOG {
		t.Error("InjectOG = false, want true")
	}
	if !cfg.InjectTwitter {
		t.Error("InjectTwitter = false, want true")
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	raw := map[string]any{
		"descriptionMaxLength": 200,
		"defaultOGImage":       "/images/default.png",
		"injectOG":             false,
		"injectTwitter":        false,
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.DescriptionMaxLength != 200 {
		t.Errorf("DescriptionMaxLength = %d, want 200", cfg.DescriptionMaxLength)
	}
	if cfg.DefaultOGImage != "/images/default.png" {
		t.Errorf("DefaultOGImage = %q", cfg.DefaultOGImage)
	}
	if cfg.InjectOG {
		t.Error("InjectOG = true, want false")
	}
	if cfg.InjectTwitter {
		t.Error("InjectTwitter = true, want false")
	}
}

func TestInjectHTML_NoHead(t *testing.T) {
	src := "<html><body>no head</body></html>"
	result, err := InjectHTML(src, &InjectOptions{PageTitle: "test"})
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}
	if result != src {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestInjectHTML_AddsMissingTags(t *testing.T) {
	src := `<html><head><title>My Page</title></head><body><p>Hello world. This is a test description for SEO purposes.</p></body></html>`
	opts := &InjectOptions{
		PageTitle: "My Page",
		PageURL:   "https://example.com/page/",
		PageKind:  "page",
	}
	result, err := InjectHTML(src, opts)
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}

	// Verify key tags were injected
	checks := []string{
		`<meta name="description" content="`,
		`<meta property="og:title" content="My Page">`,
		`<meta property="og:url" content="https://example.com/page/">`,
		`<meta property="og:type" content="article">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<!-- huan seo-injector -->`,
	}
	for _, check := range checks {
		if !contains(result, check) {
			t.Errorf("missing injected tag: %s", check)
		}
	}
}

func TestInjectHTML_SkipsExistingTags(t *testing.T) {
	src := `<html><head><meta name="description" content="existing desc"></head><body><p>Content</p></body></html>`
	opts := &InjectOptions{
		PageTitle: "My Page",
	}
	result, err := InjectHTML(src, opts)
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}
	// Should keep only one description
	count := strings.Count(result, `name="description"`)
	if count != 1 {
		t.Errorf("expected 1 description, got %d: %s", count, result)
	}
}

func TestInjectHTML_OgTypeWebsite(t *testing.T) {
	src := `<html><head><title>Section</title></head><body><p>Content</p></body></html>`
	opts := &InjectOptions{
		PageTitle: "Section",
		PageKind:  "section",
	}
	result, err := InjectHTML(src, opts)
	if err != nil {
		t.Fatalf("InjectHTML: %v", err)
	}
	if !strings.Contains(result, `content="website"`) {
		t.Errorf("expected og:type website for section, got: %s", result)
	}
}

func TestExtractPlainText(t *testing.T) {
	html := `<html><head><title>Test</title></head><body><p>Hello world.</p><p>Second paragraph.</p></body></html>`
	text := ExtractPlainText(html)
	expected := "Hello world. Second paragraph."
	if text != expected {
		t.Errorf("got %q, want %q", text, expected)
	}
}

func TestExtractPlainText_SkipsNavStyles(t *testing.T) {
	html := `<html><body><nav>Nav links</nav><style>.foo{}</style><p>Real content</p><footer>Footer</footer></body></html>`
	text := ExtractPlainText(html)
	expected := "Real content"
	if text != expected {
		t.Errorf("got %q, want %q", text, expected)
	}
}

func TestTruncateToWordBoundary(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"Short text", 100, "Short text"},
		{"Hello world this is a test", 15, "Hello world"},
		{"Oneword", 3, "Oneword"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := TruncateToWordBoundary(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("TruncateToWordBoundary(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestExtractExistingTags(t *testing.T) {
	html := `<html><head>
		<meta name="description" content="x">
		<meta property="og:title" content="y">
		<meta name="twitter:card" content="summary">
	</head></html>`
	tags := ExtractExistingTags(html)
	checks := []string{"description", "og:title", "twitter:card"}
	for _, c := range checks {
		if !tags[c] {
			t.Errorf("missing tag %q in extracted set", c)
		}
	}
}

// helpers
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
```

- [ ] **Step 5: 运行测试**

Run: `cd /Users/rong.zhu/Code/zhurong/huan && go test ./internal/seo/injector/ -v`
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/seo/injector/
git commit -m "feat(seo): add SEO injector core logic (Config + InjectHTML)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: SEO 注入器插件集成 — Plugin + 注册

**Files:**
- Modify: `internal/seo/injector/plugin.go` — 添加 SEOInjector struct 和 Hook 实现
- Modify: `cmd/huan/plugins.go` — 添加 `case "seo_injector"` 注册
- Test: `internal/seo/injector/plugin_test.go` — 添加插件级测试

**Interfaces:**
- Consumes: `build.Hook` interface (OnOutputWritten), `config.Config` struct, `plugin.SchemaProvider`
- Produces: `SEOInjector` plugin struct implementing `build.Hook` + `plugin.SchemaProvider`

- [ ] **Step 1: 在 `internal/seo/injector/plugin.go` 末尾添加 SEOInjector struct 和 Hook 实现**

```go
// SEOInjector is the build.Hook plugin that injects SEO meta tags.
type SEOInjector struct {
	cfg    *Config
	logf   func(string, ...any)
}

// New creates a new SEOInjector plugin.
func New(cfg *Config) *SEOInjector {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &SEOInjector{cfg: cfg, logf: func(format string, args ...any) {}}
}

// SetLogf sets the logger function.
func (p *SEOInjector) SetLogf(fn func(string, ...any)) {
	p.logf = fn
}

// Name returns the plugin name.
func (p *SEOInjector) Name() string { return "seo_injector" }

// OnContentLoaded is a no-op for this plugin.
func (p *SEOInjector) OnContentLoaded(_ context.Context, pages []*content.Page) ([]*content.Page, error) {
	return nil, nil
}

// OnPageRendered is a no-op for this plugin.
func (p *SEOInjector) OnPageRendered(_ context.Context, page *content.Page) error {
	return nil
}

// OnOutputWritten scans the output directory for HTML files and injects missing SEO meta tags.
func (p *SEOInjector) OnOutputWritten(ctx context.Context, outputDir string) error {
	entries, err := filepath.Glob(filepath.Join(outputDir, "**/*.html"))
	if err != nil {
		p.logf("seo-injector: glob %s: %v\n", outputDir, err)
		return nil // collection-not-interruption: log warning, don't abort
	}

	for _, filePath := range entries {
		select {
		case <-ctx.Done():
			p.logf("seo-injector: cancelled\n")
			return nil
		default:
		}

		if err := p.processFile(filePath, outputDir); err != nil {
			p.logf("seo-injector: %s: %v\n", filePath, err)
		}
	}
	return nil
}

// processFile reads, injects, and writes a single HTML file.
func (p *SEOInjector) processFile(filePath, outputDir string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Compute relative URL
	rel, err := filepath.Rel(outputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel path: %w", err)
	}
	// Convert filesystem path to URL path. On Windows, use ToSlash.
	urlPath := "/" + strings.ReplaceAll(rel, string(filepath.Separator), "/")

	opts := &InjectOptions{
		DescriptionMaxLength: p.cfg.DescriptionMaxLength,
		DefaultOGImage:       p.cfg.DefaultOGImage,
		InjectOG:             p.cfg.InjectOG,
		InjectTwitter:        p.cfg.InjectTwitter,
		PageURL:              urlPath, // relative to site root; caller should prepend baseURL if needed
		PageKind:             p.guessKind(rel),
		PageTitle:            p.extractTitle(string(data)),
	}

	result, err := InjectHTML(string(data), opts)
	if err != nil {
		return fmt.Errorf("inject: %w", err)
	}

	if result == string(data) {
		return nil // no changes
	}

	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// guessKind tries to infer the page kind from the file path.
func (p *SEOInjector) guessKind(relPath string) string {
	// Files in the root (index.html) are "home"
	if relPath == "index.html" || relPath == "/index.html" {
		return "home"
	}
	// Section index files
	if strings.HasSuffix(relPath, "/index.html") {
		return "section"
	}
	// Everything else is a page
	return "page"
}

// extractTitle extracts the <title> from HTML.
func (p *SEOInjector) extractTitle(htmlSrc string) string {
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return ""
	}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil || title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = strings.TrimSpace(n.FirstChild.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title
}
```

Needs additional imports in `plugin.go`:
```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/internal/content"
	"golang.org/x/net/html"
)
```

- [ ] **Step 2: 在测试文件末尾添加插件级测试**

```go
func TestNewSEOInjector(t *testing.T) {
	p := New(nil)
	if p.Name() != "seo_injector" {
		t.Errorf("Name() = %q, want seo_injector", p.Name())
	}
}

func TestSEOInjector_HooksReturnNil(t *testing.T) {
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

func TestGuessKind(t *testing.T) {
	p := New(nil)
	tests := []struct{ path, kind string }{
		{"index.html", "home"},
		{"/index.html", "home"},
		{"posts/index.html", "section"},
		{"posts/2024/01/post/index.html", "section"},
		{"posts/2024/01/post/index.html", "section"},
		{"posts/page/2/index.html", "page"},
		{"404.html", "page"},
	}
	for _, tt := range tests {
		got := p.guessKind(tt.path)
		if got != tt.kind {
			t.Errorf("guessKind(%q) = %q, want %q", tt.path, got, tt.kind)
		}
	}
}

func TestExtractTitle(t *testing.T) {
	p := New(nil)
	html := `<html><head><title>My Awesome Page</title></head><body></body></html>`
	title := p.extractTitle(html)
	if title != "My Awesome Page" {
		t.Errorf("extractTitle = %q, want %q", title, "My Awesome Page")
	}
}

func TestProcessFile(t *testing.T) {
	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, "index.html")
	content := `<html><head><title>Home</title></head><body><p>Welcome to my site.</p></body></html>`
	if err := os.WriteFile(htmlFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := New(&Config{
		DescriptionMaxLength: 160,
		InjectOG:             true,
		InjectTwitter:        true,
		DefaultOGImage:       "/images/default.png",
	})
	p.SetLogf(t.Logf)

	err := p.processFile(htmlFile, tmpDir)
	if err != nil {
		t.Fatalf("processFile: %v", err)
	}

	data, err := os.ReadFile(htmlFile)
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)
	if !strings.Contains(result, `<!-- huan seo-injector -->`) {
		t.Error("expected injection marker in output")
	}
	if !strings.Contains(result, `content="Home"`) {
		t.Error("expected og:title in output")
	}
}
```

- [ ] **Step 3: 修改 `cmd/huan/plugins.go`，添加 seo_injector 编译型插件注册**

在 switch 中，`// ### Compiled-in plugins ###` 注释后添加：

```go
case "seo_injector":
    raw := cfg.Plugins[name]
    pluginCfg, err := injector.ParseConfig(raw)
    if err != nil {
        return nil, fmt.Errorf("plugin %s: %w", name, err)
    }
    if err := r.Register(injector.New(pluginCfg)); err != nil {
        return nil, fmt.Errorf("plugin %s: %w", name, err)
    }
```

在 import 块中添加：
```go
"github.com/iannil/huan/internal/seo/injector"
```

- [ ] **Step 4: 运行所有测试**

```bash
go test ./internal/seo/injector/ ./cmd/huan/ -v
```
Expected: ALL PASS

- [ ] **Step 5: 构建验证**

```bash
go build ./cmd/huan/
```
Expected: Build succeeds

- [ ] **Step 6: 提交**

```bash
git add internal/seo/injector/ cmd/huan/plugins.go
git commit -m "feat(seo): integrate SEO injector as compiled-in plugin

Co-Authored-By: Claude <noreply@anthropic.com>"
```
