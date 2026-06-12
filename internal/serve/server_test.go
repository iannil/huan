package serve

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func writeStaticFixture(t *testing.T, dir string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mustWrite("index.html", "<html><body>hello</body></html>")
	mustWrite("posts/foo.html", "<html><body>post foo</body></html>")
}

func TestServerServesStaticFiles(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	writeStaticFixture(t, tmp)

	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0",
	})

	addrCh := make(chan string, 1)
	srv.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx) //nolint:errcheck

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start within 2s")
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello") {
		t.Errorf("body = %q, want contains 'hello'", string(body))
	}

	resp2, err := http.Get("http://" + addr + "/posts/foo.html")
	if err != nil {
		t.Fatalf("get /posts/foo.html: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), "post foo") {
		t.Errorf("body = %q, want contains 'post foo'", string(body2))
	}
}

func TestServerServesLivereloadJS(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0",
	})
	addrCh := make(chan string, 1)
	srv.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx) //nolint:errcheck

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start within 2s")
	}

	resp, err := http.Get("http://" + addr + "/livereload.js")
	if err != nil {
		t.Fatalf("get /livereload.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "LiveReload") {
		t.Errorf("body does not look like livereload.js (no 'LiveReload' substring, len=%d)", len(body))
	}
}

func TestServerRoutesLivereloadWS(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	hub := NewHub()
	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0",
		Hub:       hub,
	})
	addrCh := make(chan string, 1)
	srv.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx) //nolint:errcheck

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start within 2s")
	}

	// Try to connect via WebSocket
	c, _, err := websocket.Dial(ctx, "ws://"+addr+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial /livereload: %v", err)
	}
	defer c.CloseNow()

	c.SetReadLimit(1 << 16)
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !strings.Contains(string(msg), `"hello"`) {
		t.Errorf("first msg = %s, want hello", string(msg))
	}
}

// TestServerServesCustom404 verifies that requests for missing files
// serve the project's 404.html (if present) with status 404.
func TestServerServesCustom404(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Write index.html and a custom 404.html
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html><body>home</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "404.html"), []byte("<html><body>custom 404 page</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0",
	})
	addrCh := make(chan string, 1)
	srv.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx) //nolint:errcheck

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start within 2s")
	}

	// Missing path → should serve 404.html with 404 status
	resp, err := http.Get("http://" + addr + "/does-not-exist")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "custom 404 page") {
		t.Errorf("body = %q, want custom 404 content", string(body))
	}

	// Existing path → 200
	resp2, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp2.StatusCode)
	}
}

// TestServer404FallbackWithout404HTML verifies that when there's no 404.html,
// the server falls back to Go's default 404 behavior.
func TestServer404FallbackWithout404HTML(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte("<html><body>home</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Note: no 404.html

	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0",
	})
	addrCh := make(chan string, 1)
	srv.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx) //nolint:errcheck

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start within 2s")
	}

	resp, err := http.Get("http://" + addr + "/missing")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "404 page not found") {
		t.Errorf("body = %q, want Go default 404 text", string(body))
	}
}
