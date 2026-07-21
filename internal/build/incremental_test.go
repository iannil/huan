package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSite creates a minimal but complete site for incremental render tests:
// huan.yaml + a single.html layout + list.html layout + two content pages.
// Returns the source and output directory paths.
func writeSite(t *testing.T) (srcDir, outDir string) {
	t.Helper()
	srcDir = t.TempDir()
	outDir = filepath.Join(srcDir, "public")

	writeFile(t, srcDir, "huan.yaml", `
baseURL: https://example.com/
title: Incremental Test
languageCode: en
`)

	// Flat single.html — no template inheritance, just render the page body.
	writeFile(t, srcDir, filepath.Join("layouts", "_default", "single.html"),
		`<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>
<body><article><h1>{{ .Title }}</h1><div class="body">{{ .Content }}</div></article></body></html>
`)

	// Flat list.html for section/home pages.
	writeFile(t, srcDir, filepath.Join("layouts", "_default", "list.html"),
		`<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>
<body><section><h1>{{ .Title }}</h1>{{ range .Data.Pages }}<a href="{{ .Permalink }}">{{ .Title }}</a>{{ end }}</section></body></html>
`)

	writeFile(t, srcDir, filepath.Join("content", "posts", "alpha.md"),
		"---\ntitle: \"Alpha\"\ndate: 2026-07-01T00:00:00Z\n---\nOriginal alpha body.\n")
	writeFile(t, srcDir, filepath.Join("content", "posts", "beta.md"),
		"---\ntitle: \"Beta\"\ndate: 2026-07-02T00:00:00Z\n---\nOriginal beta body.\n")
	return srcDir, outDir
}

// TestIncrementalRender_EmptyAffectedURLsIsNoop verifies that an empty
// affectedURLs slice short-circuits without touching the output directory.
func TestIncrementalRender_EmptyAffectedURLsIsNoop(t *testing.T) {
	srcDir, outDir := writeSite(t)
	opts := Options{SourceDir: srcDir, OutputDir: outDir}
	if err := IncrementalRender(opts, nil, nil); err != nil {
		t.Fatalf("IncrementalRender with nil affectedURLs: %v", err)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Errorf("expected output dir to not exist after empty affectedURLs, got err=%v", err)
	}
}

// TestIncrementalRender_NilCacheBuildsInfrastructure verifies that calling
// IncrementalRender with cache=nil falls back to setupTemplatesAndWriter and
// still renders the affected page correctly.
func TestIncrementalRender_NilCacheBuildsInfrastructure(t *testing.T) {
	srcDir, outDir := writeSite(t)
	opts := Options{SourceDir: srcDir, OutputDir: outDir}

	// Affected URL: the alpha page only.
	affected := []string{"/posts/alpha/"}
	if err := IncrementalRender(opts, nil, affected); err != nil {
		t.Fatalf("IncrementalRender nil cache: %v", err)
	}

	alphaHTML, err := os.ReadFile(filepath.Join(outDir, "posts", "alpha", "index.html"))
	if err != nil {
		t.Fatalf("alpha index.html not written: %v", err)
	}
	if !strings.Contains(string(alphaHTML), "Original alpha body.") {
		t.Errorf("alpha output missing body content:\n%s", alphaHTML)
	}

	// Beta was not in affectedURLs — must not be written by this incremental pass.
	if _, err := os.Stat(filepath.Join(outDir, "posts", "beta", "index.html")); err == nil {
		t.Error("beta index.html was written but was not in affectedURLs")
	}
}

// TestIncrementalRender_ReusesCacheFromFullBuild verifies the happy path:
// a full BuildSite populates a PipelineCache, then IncrementalRender uses
// that cache to re-render only the affected page after its content changes.
func TestIncrementalRender_ReusesCacheFromFullBuild(t *testing.T) {
	srcDir, outDir := writeSite(t)

	cache := NewPipelineCache()
	opts := Options{
		SourceDir:    srcDir,
		OutputDir:    outDir,
		PipelineCache: cache,
	}
	if _, err := BuildSite(opts); err != nil {
		t.Fatalf("BuildSite initial: %v", err)
	}

	// Sanity: full build wrote both pages + home.
	alphaPath := filepath.Join(outDir, "posts", "alpha", "index.html")
	betaPath := filepath.Join(outDir, "posts", "beta", "index.html")
	if _, err := os.Stat(alphaPath); err != nil {
		t.Fatalf("full build did not write alpha: %v", err)
	}
	if _, err := os.Stat(betaPath); err != nil {
		t.Fatalf("full build did not write beta: %v", err)
	}

	// Cache must be populated after BuildSite.
	if cache.Templates == nil {
		t.Fatal("cache.Templates nil after BuildSite")
	}
	if cache.Writer == nil {
		t.Fatal("cache.Writer nil after BuildSite")
	}
	if cache.SiteCfg == nil {
		t.Fatal("cache.SiteCfg nil after BuildSite")
	}

	// Mutate alpha content on disk.
	writeFile(t, srcDir, filepath.Join("content", "posts", "alpha.md"),
		"---\ntitle: \"Alpha\"\ndate: 2026-07-01T00:00:00Z\n---\nUpdated alpha body.\n")

	// Re-run incremental render using the cache. Only alpha affected.
	incrOpts := Options{SourceDir: srcDir, OutputDir: outDir}
	if err := IncrementalRender(incrOpts, cache, []string{"/posts/alpha/"}); err != nil {
		t.Fatalf("IncrementalRender: %v", err)
	}

	// Alpha output must reflect the new content.
	alphaHTML, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatalf("read alpha after incremental: %v", err)
	}
	if !strings.Contains(string(alphaHTML), "Updated alpha body.") {
		t.Errorf("alpha not updated by incremental render:\n%s", alphaHTML)
	}
	if strings.Contains(string(alphaHTML), "Original alpha body.") {
		t.Errorf("alpha still shows stale content:\n%s", alphaHTML)
	}
}
