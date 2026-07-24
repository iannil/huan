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
