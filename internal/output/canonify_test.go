package output

import (
	"strings"
	"testing"
)

// TestCanonify_EmptyBaseURLReturnsInput verifies the early-return path:
// without a baseURL, no rewriting happens. This is the default for sites
// that don't enable canonifyURLs.
func TestCanonify_EmptyBaseURLReturnsInput(t *testing.T) {
	in := `<a href="/posts/foo/">link</a>`
	got := Canonify(in, CanonifyOptions{BaseURL: ""})
	if got != in {
		t.Errorf("empty baseURL: got %q, want %q", got, in)
	}
}

// TestCanonify_QuotedAttributes verifies the main use case: quoted href/src
// attributes get rewritten to absolute URLs.
func TestCanonify_QuotedAttributes(t *testing.T) {
	in := `<a href="/posts/foo/">link</a><img src="/images/bar.png">`
	want := `<a href="https://example.com/posts/foo/">link</a><img src="https://example.com/images/bar.png">`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/"})
	if got != want {
		t.Errorf("quoted attrs:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestCanonify_BareAttributes verifies post-minify bare attributes (no quotes).
// This is the common case in production output.
func TestCanonify_BareAttributes(t *testing.T) {
	in := `<a href=/posts/foo/>link</a><img src=/images/bar.png>`
	want := `<a href=https://example.com/posts/foo/>link</a><img src=https://example.com/images/bar.png>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/"})
	if got != want {
		t.Errorf("bare attrs:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestCanonify_BareRootAttribute verifies the special case `href=/` and
// `src=/` (bare root path, no quotes). Without dedicated handling, the
// general bare pattern would miss this — the regex needs a trailing
// whitespace or `>`.
func TestCanonify_BareRootAttribute(t *testing.T) {
	in := `<a href=/>home</a>`
	want := `<a href=https://example.com/>home</a>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/"})
	if got != want {
		t.Errorf("bare root:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestCanonify_SkipsCodeRegions verifies URLs inside <code>/<pre> blocks
// are NOT rewritten. This is critical for code samples showing source
// (e.g., a tutorial showing `<link href=/api/foo>`). The inside-code text
// arrives HTML-escaped (`&lt;a href=/foo&gt;`), and canonify must preserve
// it verbatim — both the entity-encoded bracket AND the bare path.
func TestCanonify_SkipsCodeRegions(t *testing.T) {
	in := `<p>See <a href="/guide/">guide</a>.</p><pre><code>&lt;a href=/foo&gt;</code></pre>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/"})
	// Outside code: rewritten.
	if !strings.Contains(got, `href="https://example.com/guide/"`) {
		t.Errorf("outside code not rewritten: %s", got)
	}
	// Inside code: unchanged — entity-encoded brackets AND bare path preserved.
	if !strings.Contains(got, `&lt;a href=/foo&gt;`) {
		t.Errorf("inside code mangled: %s", got)
	}
	// Negative check: must NOT have rewritten the inside-code path.
	if strings.Contains(got, `&lt;a href=https://example.com/foo`) {
		t.Errorf("inside code incorrectly rewritten: %s", got)
	}
}

// TestCanonify_SkipsAbsoluteURLs verifies that absolute http(s) URLs and
// protocol-relative URLs are not double-prefixed.
func TestCanonify_SkipsAbsoluteURLs(t *testing.T) {
	in := `<a href="https://other.com/path">x</a><a href="//cdn.example.com/file.js">y</a>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/"})
	if strings.Contains(got, "example.com/https://other.com") {
		t.Errorf("absolute URL double-prefixed: %s", got)
	}
	if strings.Contains(got, "example.com///cdn.example.com") {
		t.Errorf("protocol-relative URL mangled: %s", got)
	}
}

// TestCanonify_IsHomeInjectsGenerator verifies the Hugo generator meta tag
// is injected immediately after <head> on the home page only.
func TestCanonify_IsHomeInjectsGenerator(t *testing.T) {
	in := `<html><head><title>Home</title></head><body></body></html>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/", IsHome: true})
	if !strings.Contains(got, `<meta name=generator content="Hugo `) {
		t.Errorf("generator meta not injected: %s", got)
	}
	// Verify it's right after <head>.
	headIdx := strings.Index(got, "<head>")
	genIdx := strings.Index(got, `<meta name=generator`)
	if headIdx < 0 || genIdx < 0 || genIdx < headIdx || genIdx-headIdx > 10 {
		t.Errorf("generator not immediately after <head>: headIdx=%d genIdx=%d", headIdx, genIdx)
	}
}

// TestCanonify_NonHomeDoesNotInjectGenerator verifies the generator meta
// only appears on home pages.
func TestCanonify_NonHomeDoesNotInjectGenerator(t *testing.T) {
	in := `<html><head><title>Post</title></head><body></body></html>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/", IsHome: false})
	if strings.Contains(got, "generator") {
		t.Errorf("generator injected on non-home: %s", got)
	}
}

// TestCanonify_MinifiesJSONLD verifies JSON-LD script contents get minified.
func TestCanonify_MinifiesJSONLD(t *testing.T) {
	in := `<script type="application/ld+json">{
  "@context": "https://schema.org",
  "@type": "BlogPosting"
}</script>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/"})
	if strings.Contains(got, "\n") && strings.Contains(got, "BlogPosting") {
		// OK if it has newlines elsewhere, but JSON-LD body should be compact.
		jsonStart := strings.Index(got, "{")
		jsonEnd := strings.Index(got, "}<")
		if jsonStart >= 0 && jsonEnd > jsonStart {
			body := got[jsonStart : jsonEnd+1]
			if strings.Contains(body, "\n") {
				t.Errorf("JSON-LD not minified:\n%s", body)
			}
		}
	}
}
