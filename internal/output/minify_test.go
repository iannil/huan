package output

import (
	"strings"
	"sync"
	"testing"
)

// TestMinify_NilMinifierReturnsInput verifies the nil-safe contract —
// callers can pass a nil *Minifier to disable minification.
func TestMinify_NilMinifierReturnsInput(t *testing.T) {
	var mi *Minifier
	in := `<html>  <body>hi</body>  </html>`
	if got := mi.Minify("page.html", in); got != in {
		t.Errorf("nil Minifier: got %q, want input %q", got, in)
	}
	if got := mi.MinifyBytes("page.html", []byte(in)); string(got) != in {
		t.Errorf("nil Minifier bytes: got %q, want input %q", string(got), in)
	}
}

// TestMinify_HTMLRemovesWhitespace verifies HTML output has redundant
// whitespace stripped (matches Hugo's tdewolff config: keepWhitespace=false).
func TestMinify_HTMLRemovesWhitespace(t *testing.T) {
	mi := NewMinifier()
	in := `<html>
  <body>
    <p>Hello</p>
  </body>
</html>`
	got := mi.Minify("page.html", in)
	// Whitespace between tags should be removed.
	if strings.Contains(got, "> <") || strings.Contains(got, ">\n<") {
		t.Errorf("HTML minify left whitespace between tags:\n%s", got)
	}
	// Content preserved.
	if !strings.Contains(got, "Hello") {
		t.Errorf("HTML minify lost content:\n%s", got)
	}
}

// TestMinify_CSSRemovesWhitespaceAndShortensHex verifies CSS-specific minification.
func TestMinify_CSSRemovesWhitespaceAndShortensHex(t *testing.T) {
	mi := NewMinifier()
	in := `body {
  color: #ffffff;
  background: #000000;
}`
	got := mi.Minify("style.css", in)
	// Hex shortened: #ffffff → #fff.
	if strings.Contains(got, "#ffffff") {
		t.Errorf("CSS minify did not shorten hex:\n%s", got)
	}
	// Whitespace removed.
	if strings.Contains(got, "  ") || strings.Contains(got, "\n") {
		t.Errorf("CSS minify left whitespace:\n%s", got)
	}
}

// TestMinify_JSONCompactsWhitespace verifies JSON output is single-line.
func TestMinify_JSONCompactsWhitespace(t *testing.T) {
	mi := NewMinifier()
	in := `{
  "key": "value",
  "num": 42
}`
	got := mi.Minify("data.json", in)
	if strings.Contains(got, "\n") {
		t.Errorf("JSON minify left newlines:\n%s", got)
	}
	if !strings.Contains(got, `"key":"value"`) {
		t.Errorf("JSON minify lost content:\n%s", got)
	}
}

// TestMinify_XMLPreservesContent verifies RSS/sitemap XML is minified
// without losing structure.
func TestMinify_XMLPreservesContent(t *testing.T) {
	mi := NewMinifier()
	in := `<?xml version="1.0"?>
<rss>
  <channel>
    <title>Test</title>
  </channel>
</rss>`
	got := mi.Minify("feed.xml", in)
	if !strings.Contains(got, "<title>Test</title>") {
		t.Errorf("XML minify lost content:\n%s", got)
	}
}

// TestMinify_UnknownExtensionReturnsInput verifies non-minifiable file
// types (.txt, .md, .png) are returned unchanged.
func TestMinify_UnknownExtensionReturnsInput(t *testing.T) {
	mi := NewMinifier()
	for _, ext := range []string{".txt", ".md", ".png", ".woff2"} {
		t.Run(ext, func(t *testing.T) {
			in := `some  weird   content
   with   whitespace`
			got := mi.Minify("file"+ext, in)
			if got != in {
				t.Errorf("Minify(%s): got %q, want input unchanged", ext, got)
			}
		})
	}
}

// TestMinify_MinifyBytesMatchesString verifies the byte and string APIs
// produce identical output for the same input.
func TestMinify_MinifyBytesMatchesString(t *testing.T) {
	mi := NewMinifier()
	in := `<p>x</p>`
	gotStr := mi.Minify("p.html", in)
	gotBytes := string(mi.MinifyBytes("p.html", []byte(in)))
	if gotStr != gotBytes {
		t.Errorf("Minify vs MinifyBytes diverged:\n  str:   %q\n  bytes: %q", gotStr, gotBytes)
	}
}

// TestMediaTypeForExt covers the extension → media-type mapping.
func TestMediaTypeForExt(t *testing.T) {
	cases := map[string]string{
		"page.html": "text/html",
		"page.htm":  "text/html",
		"style.css": "text/css",
		"app.js":    "application/javascript",
		"app.mjs":   "application/javascript",
		"data.json": "application/json",
		"logo.svg":  "image/svg+xml",
		"feed.xml":  "application/xml",
		"index.txt": "",
		"image.png": "",
		"noext":     "",
		"weird.PNG": "", // case-insensitive ext, but png not mapped
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := mediaTypeForExt(path); got != want {
				t.Errorf("mediaTypeForExt(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

// TestMinify_JSMinifiesBasic verifies JS minification is wired up.
func TestMinify_JSMinifiesBasic(t *testing.T) {
	mi := NewMinifier()
	in := `function foo() {
  var x = 1;
  return x;
}`
	got := mi.Minify("app.js", in)
	// Whitespace stripped.
	if strings.Contains(got, "\n") {
		t.Errorf("JS minify left newlines:\n%s", got)
	}
	if !strings.Contains(got, "function") {
		t.Errorf("JS minify lost content:\n%s", got)
	}
}

func TestMinifyConcurrentFormatsMatchSequential(t *testing.T) {
	cases := []struct{ path, content string }{
		{"page.html", `<html><head><style>body { color: #ffffff; }</style></head><body><script>function x() { return 2 + 3; }</script><svg><path d="M 0 0 L 10 10" /></svg><p> hello </p></body></html>`},
		{"style.css", `body { color: #ffffff; margin: 0px 0px; }`},
		{"app.js", `function foo(value) { var sum = value + 1; return sum; }`},
		{"feed.xml", `<?xml version="1.0"?><rss> <channel><title>Test</title></channel> </rss>`},
		{"data.json", `{ "name": "test", "values": [1, 2, 3] }`},
		{"logo.svg", `<svg xmlns="http://www.w3.org/2000/svg"><style>path { fill: #ffffff; }</style><path d="M 0 0 L 10 10" /></svg>`},
	}
	sequential := NewMinifier()
	expected := make([]string, len(cases))
	for i, c := range cases {
		expected[i] = sequential.Minify(c.path, c.content)
		if expected[i] == c.content {
			t.Fatalf("fixture %s was not minified", c.path)
		}
	}
	shared := NewMinifier()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for round := 0; round < 32; round++ {
		for i, c := range cases {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if got := shared.Minify(c.path, c.content); got != expected[i] {
					t.Errorf("concurrent string %s differs", c.path)
				}
				if got := string(shared.MinifyBytes(c.path, []byte(c.content))); got != expected[i] {
					t.Errorf("concurrent bytes %s differs", c.path)
				}
			}()
		}
	}
	close(start)
	wg.Wait()
}
