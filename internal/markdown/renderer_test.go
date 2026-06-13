package markdown

import (
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
)

// TestRenderer_RendersFootnotes verifies that PHP Markdown Extra footnote syntax
// ([^1]) renders as a proper footnotes section (matching Hugo's goldmark config),
// not as literal text. Hugo enables goldmark-extension footnote rendering.
func TestRenderer_RendersFootnotes(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "Hello[^1] world.\n\n[^1]: Footnote text here."
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if strings.Contains(html, "[^1]") {
		t.Errorf("Footnote syntax not rendered; output still contains literal [^1]:\n%s", html)
	}
	if !strings.Contains(html, "class=footnotes") && !strings.Contains(html, "class=\"footnotes\"") {
		t.Errorf("Footnotes section not generated; output:\n%s", html)
	}
}

// TestRenderer_CodeBlockNumericEntities verifies that " in code blocks renders
// as the numeric HTML entity &#34; (matching Hugo's chroma behavior), not the
// named entity &quot; (goldmark default). This ensures byte-level HTML output
// matches Hugo for chapters with code samples, AND that WordCount computed
// from plain text matches Hugo.
func TestRenderer_CodeBlockNumericEntities(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```python\ns = \"hello\"\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if strings.Contains(html, "&quot;") {
		t.Errorf("Code block contains named entity &quot; (expected numeric &#34;); output:\n%s", html)
	}
	if !strings.Contains(html, "&#34;") {
		t.Errorf("Code block missing numeric entity &#34;; output:\n%s", html)
	}
}

// TestRenderer_InlineCodeNumericEntities verifies that " in inline code renders
// as &#34; not &quot;.
func TestRenderer_InlineCodeNumericEntities(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "Use `\"hello\"` as input."
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if strings.Contains(html, "&quot;") {
		t.Errorf("Inline code contains named entity &quot; (expected numeric &#34;); output:\n%s", html)
	}
	if !strings.Contains(html, "&#34;") {
		t.Errorf("Inline code missing numeric entity &#34;; output:\n%s", html)
	}
}
