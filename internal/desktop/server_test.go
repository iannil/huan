package desktop

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iannil/huan/internal/plugin"
)

// writeSite creates a minimal but complete site: huan.yaml + two layouts +
// two content pages (same shape as internal/build/incremental_test.go).
func writeSite(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("huan.yaml", "baseURL: https://example.com/\ntitle: Desktop Test\nlanguageCode: en\n")
	write(filepath.Join("layouts", "_default", "single.html"),
		"<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>\n<body><article><h1>{{ .Title }}</h1><div class=\"body\">{{ .Content }}</div></article></body></html>\n")
	write(filepath.Join("layouts", "_default", "list.html"),
		"<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>\n<body><section><h1>{{ .Title }}</h1>{{ range .Data.Pages }}<a href=\"{{ .Permalink }}\">{{ .Title }}</a>{{ end }}</section></body></html>\n")
	write(filepath.Join("content", "posts", "alpha.md"),
		"---\ntitle: \"Alpha\"\ndate: 2026-07-01T00:00:00Z\n---\nOriginal alpha body.\n")
	write(filepath.Join("content", "posts", "beta.md"),
		"---\ntitle: \"Beta\"\ndate: 2026-07-02T00:00:00Z\n---\nOriginal beta body.\n")
	return src
}

func TestServeRuntime_StartServeStop(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "admin", "dist", "index.html")); err != nil {
		t.Skip("internal/admin/dist missing — build admin SPA first (cd web/admin && npm ci && npm run build)")
	}
	src := writeSite(t)
	exited := make(chan error, 1)
	rt := NewServeRuntime(ServeOptions{SourceDir: src})
	rt.OnServeExited = func(err error) { exited <- err }

	adminURL, err := rt.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rt.Stop(5 * time.Second)
	if !rt.Running() {
		t.Fatal("runtime should be running after Start")
	}
	if !strings.HasPrefix(adminURL, "http://127.0.0.1:") || !strings.HasSuffix(adminURL, "/admin/") {
		t.Fatalf("adminURL malformed: %q", adminURL)
	}
	if rt.SiteURL() == "" || rt.Config() == nil {
		t.Fatal("SiteURL/Config must be populated after Start")
	}

	// Site home renders the list page with content.
	resp, err := http.Get(rt.SiteURL())
	if err != nil {
		t.Fatalf("GET site: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Alpha") {
		t.Errorf("site home: status=%d contains Alpha=%v", resp.StatusCode, strings.Contains(string(body), "Alpha"))
	}

	// Admin SPA is served at the admin URL.
	resp2, err := http.Get(adminURL)
	if err != nil {
		t.Fatalf("GET admin: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("admin status = %d, want 200", resp2.StatusCode)
	}

	if err := rt.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if rt.Running() {
		t.Fatal("runtime should not be running after Stop")
	}
	if _, err := http.Get(rt.SiteURL()); err == nil {
		t.Error("site should refuse connections after Stop")
	}
	select {
	case err := <-exited:
		if err != nil && err != context.Canceled {
			t.Errorf("serve exited with unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("OnServeExited not fired within 2s of Stop")
	}
}

func TestServeRuntime_StopWithoutStartIsNoop(t *testing.T) {
	rt := NewServeRuntime(ServeOptions{SourceDir: t.TempDir()})
	if err := rt.Stop(time.Second); err != nil {
		t.Fatalf("Stop without Start: %v", err)
	}
}

func TestServeRuntime_StartRejectsInvalidSite(t *testing.T) {
	rt := NewServeRuntime(ServeOptions{SourceDir: t.TempDir()})
	if _, err := rt.Start(); err == nil {
		t.Fatal("Start on non-site dir should fail")
	}
	if rt.Running() {
		t.Fatal("runtime must not be running after failed Start")
	}
}

// writeSiteWithBrokenPlugin extends writeSite's site with a declared static
// plugin whose .so is garbage — the canonical startup-warning fixture.
func writeSiteWithBrokenPlugin(t *testing.T) string {
	t.Helper()
	src := writeSite(t)
	pluginsDir := filepath.Join(src, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "broken.so"), []byte("garbage"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(src, "huan.yaml")
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, append(body, []byte("plugins:\n  broken:\n    category: static\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestServeRuntimeReportsPluginWarnings(t *testing.T) {
	src := writeSiteWithBrokenPlugin(t)
	var mu sync.Mutex
	var seen []plugin.LoadWarning
	rt := NewServeRuntime(ServeOptions{
		SourceDir: src,
		OnPluginWarnings: func(ws []plugin.LoadWarning) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, ws...)
		},
	})
	url, err := rt.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rt.Stop(5 * time.Second)
	if url == "" {
		t.Fatal("empty admin url")
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, w := range seen {
		if w.Name == "broken.so" {
			found = true
		}
	}
	if !found {
		t.Fatalf("broken.so not among warnings: %+v", seen)
	}
}
