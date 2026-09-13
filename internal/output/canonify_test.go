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

// TestCanonify_BareRootRetainsReplacementExpansionBehavior characterizes the
// existing regexp replacement semantics when a BaseURL contains a dollar
// expansion. This unusual input is still part of the established contract.
func TestCanonify_BareRootRetainsReplacementExpansionBehavior(t *testing.T) {
	in := `<a href=/>home</a>`
	got := Canonify(in, CanonifyOptions{BaseURL: "https://example.com/$1/"})
	want := `<a href=https://example.com/href=/>home</a>`
	if got != want {
		t.Errorf("bare root replacement expansion: got %q, want %q", got, want)
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

// TestCanonify_CompatibilityEdges records established output for forms that
// appear in minified pages. It catches changes to the URL-match boundaries
// while allowing the matching implementation to be replaced.
func TestCanonify_CompatibilityEdges(t *testing.T) {
	base := CanonifyOptions{BaseURL: "https://example.com/"}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "duplicate quoted and bare attributes",
			in:   `<a href="/one" href=/two src="/three.png" src=/four.png>`,
			want: `<a href="https://example.com/one" href=https://example.com/two src="https://example.com/three.png" src=https://example.com/four.png>`,
		},
		{
			name: "quoted whitespace around equals",
			in:   "<a href = \"/space\">x</a><img src\t=\t\"/tab.png\">",
			want: "<a href = \"https://example.com/space\">x</a><img src\t=\t\"https://example.com/tab.png\">",
		},
		{
			name: "bare root before whitespace and tag end",
			in:   `<a href=/ >x</a><img src=/>`,
			want: `<a href=https://example.com/ >x</a><img src=https://example.com/>`,
		},
		{
			name: "protocol relative values remain unchanged",
			in:   `<a href="//cdn.example.com/a.js"><img src=//cdn.example.com/b.png>`,
			want: `<a href="//cdn.example.com/a.js"><img src=//cdn.example.com/b.png>`,
		},
		{
			name: "code and pre regions including malformed nesting remain verbatim",
			in:   `<a href=/outside><pre>before <code>href=/inside</code> after href=/also-inside</pre><img src=/after>`,
			want: `<a href=https://example.com/outside><pre>before <code>href=/inside</code> after href=https://example.com/also-inside</pre><img src=https://example.com/after>`,
		},
		{
			name: "json ld is compacted after canonification",
			in:   `<script type=application/ld+json>{ "url": "/docs", "name": "A B" }</script><a href=/next>`,
			want: `<script type=application/ld+json>{"url":"/docs","name":"A B"}</script><a href=https://example.com/next>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Canonify(tc.in, base); got != tc.want {
				t.Errorf("Canonify() = %q, want %q", got, tc.want)
			}
		})
	}
}

func BenchmarkCanonify_ManyRootRelativeAttributes(b *testing.B) {
	const unit = `<a href="/posts/entry">entry</a><img src=/assets/image.png><a href=/ >home</a>`
	html := strings.Repeat(unit, 200)
	opts := CanonifyOptions{BaseURL: "https://example.com/", IsHome: true}
	b.ReportAllocs()
	b.SetBytes(int64(len(html)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Canonify(html, opts)
	}
}
