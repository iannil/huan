package htmlinjector

import (
	"context"
	"html/template"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/content"
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
	if result2 != html {
		t.Errorf("expected unchanged for excluded kind taxonomy, got: %s", result2)
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