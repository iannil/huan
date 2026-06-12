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
