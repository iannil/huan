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

// ---------------------------------------------------------------------------
// Chroma syntax highlighting tests
// ---------------------------------------------------------------------------

// TestRenderer_ChromaBashBlock verifies that a fenced bash code block produces
// Hugo-compatible chroma output: <div class="highlight"><pre tabindex="0"
// class="chroma"><code class="language-bash" data-lang="bash"> with per-line
// <span class="line"><span class="cl"> wrappers and chroma token classes.
func TestRenderer_ChromaBashBlock(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```bash\n# comment\ngit clone url\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	checks := []struct {
		needle string
		desc   string
	}{
		{`class="highlight"`, "highlight wrapper div"},
		{`tabindex="0"`, "tabindex on pre"},
		{`class="chroma"`, "chroma class on pre"},
		{`class="language-bash"`, "language class on code"},
		{`data-lang="bash"`, "data-lang on code"},
		{`class="line"`, "per-line span"},
		{`class="cl"`, "per-line content span"},
		{`class="c1"`, "comment token class"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.needle) {
			t.Errorf("Bash code block missing %s; output:\n%s", c.desc, html)
		}
	}

	// Should NOT contain the old plain goldmark output
	if strings.Contains(html, "<pre><code") {
		t.Errorf("Bash code block uses plain goldmark rendering instead of chroma; output:\n%s", html)
	}
}

// TestRenderer_ChromaPythonBlock verifies Python syntax highlighting with
// string token highlighting and numeric entity encoding for quotes.
func TestRenderer_ChromaPythonBlock(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```python\ns = \"hello\"\nprint(s)\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	checks := []struct {
		needle string
		desc   string
	}{
		{`class="highlight"`, "highlight wrapper div"},
		{`class="chroma"`, "chroma class on pre"},
		{`class="language-python"`, "language class on code"},
		{`data-lang="python"`, "data-lang on code"},
		{`&#34;`, "numeric entity for double quote"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.needle) {
			t.Errorf("Python code block missing %s; output:\n%s", c.desc, html)
		}
	}
}

// TestRenderer_ChromaTextBlock verifies that a fenced code block with
// "text" language still gets the chroma wrapper structure but no syntax
// token classes.
func TestRenderer_ChromaTextBlock(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```text\nsome plain text\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	checks := []struct {
		needle string
		desc   string
	}{
		{`class="highlight"`, "highlight wrapper div"},
		{`class="chroma"`, "chroma class on pre"},
		{`class="language-text"`, "language class on code"},
		{`data-lang="text"`, "data-lang on code"},
		{`class="line"`, "per-line span"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.needle) {
			t.Errorf("Text code block missing %s; output:\n%s", c.desc, html)
		}
	}
}

// TestRenderer_ChromaNoLanguageBlock verifies that a fenced code block with
// no language specification is rendered as plain text without chroma
// highlighting (matching Hugo's behavior when no lexer is found and
// guessSyntax is true but analysis fails).
func TestRenderer_ChromaNoLanguageBlock(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```\nplain code here\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	// Without a language, the code should still be rendered (not raw markdown)
	if !strings.Contains(html, "plain code here") {
		t.Errorf("Plain code block content missing; output:\n%s", html)
	}
}

// TestRenderer_ChromaWrapperStructure verifies the exact Hugo-compatible
// wrapper structure: <div class="highlight"> wrapping <pre tabindex="0"
// class="chroma"><code class="language-..." data-lang="...">
func TestRenderer_ChromaWrapperStructure(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```bash\necho hello\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	// Verify correct nesting order
	if !strings.Contains(html, `<div class="highlight"><pre tabindex="0" class="chroma"><code class="language-bash" data-lang="bash">`) {
		t.Errorf("Wrapper structure does not match Hugo format; output:\n%s", html)
	}

	// Verify closing tags in correct order
	if !strings.Contains(html, "</code></pre></div>") {
		t.Errorf("Closing tags not in expected order; output:\n%s", html)
	}
}

// TestRenderer_ChromaNoModeClass verifies that the chroma v2 "mode" class
// (e.g. "dark") is stripped from the pre element, matching Hugo's older
// chroma version output.
func TestRenderer_ChromaNoModeClass(t *testing.T) {
	cfg := &config.MarkupConfig{Goldmark: config.GoldmarkConfig{Extensions: config.GoldmarkExtensionsConfig{Typographer: false}}}
	md := NewRenderer(cfg)
	src := "```bash\necho test\n```"
	html, err := md.Render(src)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if strings.Contains(html, `class="chroma dark"`) {
		t.Errorf("Chroma mode class 'dark' should be stripped; output:\n%s", html)
	}
	if strings.Contains(html, `class="chroma light"`) {
		t.Errorf("Chroma mode class 'light' should be stripped; output:\n%s", html)
	}
}
