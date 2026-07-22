package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/dag"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

// TestResolveSourceFile_URLFallback verifies that when the DAG has no entry
// for a URL, resolveSourceFile falls back to URL-based derivation.
func TestResolveSourceFile_URLFallback(t *testing.T) {
	dg := dag.NewDependencyGraph() // empty DAG

	cases := []struct {
		name    string
		url     string
		want    string
		wantOk  bool
	}{
		{"home", "/", "_index.md", true},
		{"section", "/posts/", "posts/_index.md", true},
		{"page", "/posts/hello/", "posts/hello.md", true},
		{"nested", "/posts/2026/new-year/", "posts/2026/new-year.md", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveSourceFile(dg, tc.url)
			if ok != tc.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOk)
			}
			if got != tc.want {
				t.Errorf("resolveSourceFile(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// TestResolveSourceFile_NilDAG verifies nil DAG gracefully falls back to URL
// derivation (no panic).
func TestResolveSourceFile_NilDAG(t *testing.T) {
	got, ok := resolveSourceFile(nil, "/posts/hello/")
	if !ok {
		t.Fatal("expected ok=true for nil DAG fallback")
	}
	if got != "posts/hello.md" {
		t.Errorf("got %q, want posts/hello.md", got)
	}
}

// TestResolveSourceFile_DAGHit verifies the DAG path takes precedence over
// URL derivation when the DAG has the entry.
func TestResolveSourceFile_DAGHit(t *testing.T) {
	dg := dag.NewDependencyGraph()
	// SourceFromPagePath uses the internal pageToSource map, populated via
	// BuildFromSite. We use the public SourceFromPagePath contract directly:
	// a miss returns ("", false); for a hit test we rely on a real build in
	// TestRenderPageJIT_RendersBuiltPage below.
	if _, ok := dg.SourceFromPagePath("/missing/"); ok {
		t.Fatal("expected empty DAG to have no /missing/ entry")
	}
	// Fallback should still produce a value for /missing/.
	got, ok := resolveSourceFile(dg, "/missing/")
	if !ok {
		t.Fatal("expected ok=true via URL fallback")
	}
	if got != "missing/_index.md" {
		t.Errorf("got %q, want missing/_index.md", got)
	}
}

// writeJITBuilderSite creates a minimal site for RenderPageJIT integration
// tests: one published page and one draft page.
func writeJITBuilderSite(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "huan-jit-builder-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	mustWrite := func(rel, content string) {
		full := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("huan.yaml", "baseURL: \"https://example.com/\"\ntitle: \"JIT Builder Test\"\npublishDir: \"docs\"\n")
	mustWrite("content/posts/hello.md", "---\ntitle: \"Hello World\"\ndate: \"2026-01-01\"\n---\nHello body.\n")
	mustWrite("content/posts/draft-page.md", "---\ntitle: \"Draft\"\ndate: \"2026-01-02\"\ndraft: true\n---\nDraft Content.\n")
	mustWrite("layouts/_default/single.html", "<html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>")
	mustWrite("layouts/_default/list.html", "<html><body>{{ range .Pages }}<a href=\"{{ .URL }}\">{{ .Title }}</a>{{ end }}</body></html>")

	return tmpDir
}

// TestRenderPageJIT_RendersBuiltPage verifies the full RenderPageJIT path:
// after a full build with PipelineCache, RenderPageJIT reuses the cache to
// render an existing page on demand without writing to disk.
func TestRenderPageJIT_RendersBuiltPage(t *testing.T) {
	tmpDir := writeJITBuilderSite(t)

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	pipelineCache := build.NewPipelineCache()
	dg := dag.NewDependencyGraph()

	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dg,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   false, // exclude drafts so JIT path is exercised for the draft below
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}
	if pipelineCache.Templates == nil {
		t.Fatal("PipelineCache not populated after FullBuild")
	}

	// JIT-render the published page — should succeed and include content.
	html, err := builder.RenderPageJIT(context.Background(), "/posts/hello/")
	if err != nil {
		t.Fatalf("RenderPageJIT published page: %v", err)
	}
	if !strings.Contains(html, "Hello World") {
		t.Errorf("missing title; got:\n%s", html)
	}
	if !strings.Contains(html, "Hello body") {
		t.Errorf("missing body; got:\n%s", html)
	}
}

// TestRenderPageJIT_RendersDraftPage verifies that JIT rendering forces
// IncludeDrafts=true so that draft pages (skipped at full build time) are
// still served.
func TestRenderPageJIT_RendersDraftPage(t *testing.T) {
	tmpDir := writeJITBuilderSite(t)

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	pipelineCache := build.NewPipelineCache()
	dg := dag.NewDependencyGraph()

	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dg,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   false,
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Draft was excluded from the full build, but the source file exists on
	// disk — JIT should still render it.
	html, err := builder.RenderPageJIT(context.Background(), "/posts/draft-page/")
	if err != nil {
		t.Fatalf("RenderPageJIT draft page: %v", err)
	}
	if !strings.Contains(html, "Draft Content") {
		t.Errorf("draft render missing content; got:\n%s", html)
	}
}

// TestRenderPageJIT_SourceNotFound verifies that when the resolved source
// file does not exist on disk, RenderPageJIT returns an error (which the
// serving layer maps to 404).
func TestRenderPageJIT_SourceNotFound(t *testing.T) {
	tmpDir := writeJITBuilderSite(t)

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	pipelineCache := build.NewPipelineCache()
	dg := dag.NewDependencyGraph()

	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dg,
		JITCache:      cache.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		Logf:          t.Logf,
		PipelineCache: pipelineCache,
	})

	// Source file /nonexistent/ → nonexistent/_index.md does not exist.
	_, err := builder.RenderPageJIT(context.Background(), "/nonexistent/")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
	if !strings.Contains(err.Error(), "source not found") {
		t.Errorf("err = %q, want contains 'source not found'", err.Error())
	}
}
