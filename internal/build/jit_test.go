package build

import (
	"strings"
	"testing"
)

func TestResolveSourceFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"home", "/", "_index.md"},
		{"section", "/posts/", "posts/_index.md"},
		{"simple page", "/posts/hello/", "posts/hello.md"},
		{"nested page", "/posts/2026/new-year/", "posts/2026/new-year.md"},
		{"deep page", "/books/v1/ch1/", "books/v1/ch1.md"},
		{"explicit _index", "/posts/_index/", "posts/_index.md"},
		{"no trailing slash", "/posts/hello", "posts/hello.md"},
		{"leading+trailing slash stripped", "/posts/hello/", "posts/hello.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSourceFromURL(tc.url)
			if got != tc.want {
				t.Errorf("ResolveSourceFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestRenderPageWithCache_RendersSinglePage(t *testing.T) {
	tmpDir := writeJITTestSite(t)
	cache := buildJITCacheViaFullBuild(t, tmpDir)

	html, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: true,
		Logf:          t.Logf,
		PipelineCache: cache,
	}, cache, "/posts/hello/")

	if err != nil {
		t.Fatalf("RenderPageWithCache: %v", err)
	}
	if !strings.Contains(html, "Hello World") {
		t.Errorf("rendered HTML missing page title; got:\n%s", html)
	}
	if !strings.Contains(html, "Hello body") {
		t.Errorf("rendered HTML missing page content; got:\n%s", html)
	}
}

func TestRenderPageWithCache_PageNotFound(t *testing.T) {
	tmpDir := writeJITTestSite(t)
	cache := buildJITCacheViaFullBuild(t, tmpDir)

	_, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: true,
		Logf:          t.Logf,
		PipelineCache: cache,
	}, cache, "/nonexistent/")

	if err == nil {
		t.Fatal("expected error for nonexistent page")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q, want contains 'not found'", err.Error())
	}
}

func TestRenderPageWithCache_NoCacheFallback(t *testing.T) {
	tmpDir := writeJITTestSite(t)

	// cache=nil should fall back to full pipeline setup and still render.
	html, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: true,
		Logf:          t.Logf,
	}, nil, "/posts/hello/")

	if err != nil {
		t.Fatalf("RenderPageWithCache with nil cache: %v", err)
	}
	if !strings.Contains(html, "Hello World") {
		t.Errorf("nil-cache render missing title; got:\n%s", html)
	}
}

func TestRenderPageWithCache_DraftPage(t *testing.T) {
	tmpDir := writeJITTestSite(t)
	cache := buildJITCacheViaFullBuild(t, tmpDir)

	// A draft page should render because JIT forces IncludeDrafts=true.
	html, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Logf:          t.Logf,
		PipelineCache: cache,
	}, cache, "/posts/draft-page/")

	if err != nil {
		t.Fatalf("RenderPageWithCache draft: %v", err)
	}
	if !strings.Contains(html, "Draft Content") {
		t.Errorf("draft render missing content; got:\n%s", html)
	}
}

// writeJITTestSite creates a minimal site with a regular page, a draft page,
// and templates, then returns the temp dir.
func writeJITTestSite(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	writeFile(t, tmpDir, "huan.yaml", "baseURL: \"https://example.com/\"\ntitle: \"JIT Test\"\npublishDir: \"docs\"\n")
	writeFile(t, tmpDir, "content/posts/hello.md", "---\ntitle: \"Hello World\"\ndate: \"2026-01-01\"\n---\nHello body.\n")
	writeFile(t, tmpDir, "content/posts/draft-page.md", "---\ntitle: \"Draft\"\ndate: \"2026-01-02\"\ndraft: true\n---\nDraft Content.\n")
	writeFile(t, tmpDir, "layouts/_default/single.html", "<html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>")
	writeFile(t, tmpDir, "layouts/_default/list.html", "<html><body>{{ range .Pages }}<a href=\"{{ .URL }}\">{{ .Title }}</a>{{ end }}</body></html>")

	return tmpDir
}

// buildJITCacheViaFullBuild runs a full build to populate PipelineCache.
func buildJITCacheViaFullBuild(t *testing.T, tmpDir string) *PipelineCache {
	t.Helper()
	cache := NewPipelineCache()
	_, err := BuildSite(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: false, // drafts excluded so they need JIT
		Logf:          t.Logf,
		PipelineCache: cache,
	})
	if err != nil {
		t.Fatalf("BuildSite for cache setup: %v", err)
	}
	if cache.Templates == nil {
		t.Fatal("PipelineCache not populated after full build")
	}
	return cache
}
