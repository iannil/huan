package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/markdown"
)

// renderPage renders markdown and wraps it in a minimal HTML document.
func renderPage(t *testing.T, md string) string {
	t.Helper()
	r := markdown.NewRenderer(&config.MarkupConfig{})
	body, err := r.Render(md)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return "<html><head><title>t</title></head><body>" + body + "</body></html>"
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestOnOutputWrittenRendersSVG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`))
	}))
	defer srv.Close()

	out := t.TempDir()
	writeFile(t, out, "index.html", renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n"))

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.KrokiURL = srv.URL
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	p := New(cfg)

	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatalf("OnOutputWritten: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "index.html"))
	s := string(got)
	if !strings.Contains(s, `<figure class="diagram diagram-mermaid"`) {
		t.Errorf("no figure wrap: %s", s)
	}
	if strings.Contains(s, `class="highlight"`) {
		t.Errorf("highlight block not replaced: %s", s)
	}

	// Idempotency: second run makes no changes.
	before, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if string(before) != string(after) {
		t.Errorf("not idempotent")
	}
}

func TestOnOutputWrittenFallbackOnKrokiDown(t *testing.T) {
	out := t.TempDir()
	writeFile(t, out, "index.html", renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n"))

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.KrokiURL = "http://127.0.0.1:1" // connection refused
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	p := New(cfg)

	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatalf("fallback path must not error: %v", err)
	}
	s, _ := os.ReadFile(filepath.Join(out, "index.html"))
	str := string(s)
	if !strings.Contains(str, `<pre class="mermaid">`) {
		t.Errorf("no client fallback: %s", str)
	}
	if strings.Count(str, "mermaid.initialize") != 1 {
		t.Errorf("mermaid.js not injected once: %s", str)
	}
}

func TestOnOutputWrittenDisabledNoop(t *testing.T) {
	out := t.TempDir()
	page := renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n")
	writeFile(t, out, "index.html", page)
	p := New(DefaultConfig()) // Enabled=false
	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if string(got) != page {
		t.Errorf("disabled plugin modified output")
	}
}

func TestExcludeKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`))
	}))
	defer srv.Close()
	out := t.TempDir()
	writeFile(t, out, "index.html", renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n"))
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.KrokiURL = srv.URL
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.ExcludeKinds = []string{"home"} // index.html => home
	p := New(cfg)
	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if strings.Contains(string(got), "<figure") {
		t.Errorf("excluded kind should be skipped")
	}
}
