package build

import (
	"strings"
	"testing"
)

func TestInjectLiveReloadInsertsScriptTag(t *testing.T) {
	html := `<html><head><title>x</title></head><body></body></html>`
	got := InjectLiveReload(html, "ws://localhost:1313/livereload")
	if !strings.Contains(got, `src="http://localhost:1313/livereload.js?mindelay=10&v=2"`) {
		t.Errorf("missing script tag with absolute URL in:\n%s", got)
	}
	if !strings.Contains(got, `data-livereload-port="1313"`) {
		t.Errorf("missing data-livereload-port in:\n%s", got)
	}
	if !strings.Contains(got, `data-livereload-host="localhost"`) {
		t.Errorf("missing data-livereload-host in:\n%s", got)
	}
	// Must be inserted BEFORE </head>
	headIdx := strings.Index(got, "</head>")
	scriptIdx := strings.Index(got, "<script src=\"http://")
	if scriptIdx >= headIdx {
		t.Errorf("script not before </head>: script=%d head=%d", scriptIdx, headIdx)
	}
}

func TestInjectLiveReloadFallsBackToBody(t *testing.T) {
	// No </head> — should fall back to before </body>
	html := `<html><body>x</body></html>`
	got := InjectLiveReload(html, "ws://localhost:1313/livereload")
	bodyIdx := strings.Index(got, "</body>")
	scriptIdx := strings.Index(got, "<script src=\"http://")
	if scriptIdx >= bodyIdx {
		t.Errorf("script not before </body>: script=%d body=%d", scriptIdx, bodyIdx)
	}
}

func TestInjectLiveReloadNoHeadNoBodyAppends(t *testing.T) {
	html := `<p>just a fragment</p>`
	got := InjectLiveReload(html, "ws://localhost:1313/livereload")
	if !strings.HasSuffix(got, "</script>") {
		t.Errorf("expected script appended at end, got:\n%s", got)
	}
}

func TestPortFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ws://localhost:1313/livereload", "1313"},
		{"ws://127.0.0.1:8080/livereload", "8080"},
		{"ws://example.com/livereload", "1313"}, // no port → default
		{"wss://secure.example.com:443/lr", "443"},
		{"ws://[::1]:1313/livereload", "1313"},        // IPv6 loopback
		{"ws://[2001:db8::1]:8080/lr", "8080"},        // IPv6 with port
		{"ws://[::1]/livereload", "1313"},             // IPv6 no port → default
	}
	for _, c := range cases {
		got := portFromURL(c.in)
		if got != c.want {
			t.Errorf("portFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHostFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ws://localhost:1313/livereload", "localhost"},
		{"ws://127.0.0.1:8080/livereload", "127.0.0.1"},
		{"ws://example.com/livereload", "example.com"},
		{"wss://secure.example.com:443/lr", "secure.example.com"},
		{"ws://[::1]:1313/livereload", "::1"},                // IPv6 loopback, brackets stripped
		{"ws://[2001:db8::1]:8080/lr", "2001:db8::1"},        // IPv6 with port
		{"ws://[::1]/livereload", "::1"},                     // IPv6 no port
	}
	for _, c := range cases {
		got := hostFromURL(c.in)
		if got != c.want {
			t.Errorf("hostFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
