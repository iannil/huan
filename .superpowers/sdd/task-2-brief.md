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
