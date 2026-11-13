package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildFailsWhenReferencedAssetIsMissing is the regression guard for the
// silent-loss failure mode: a page referenced a stylesheet the build never
// wrote, and the published English pages shipped unstyled.
func TestBuildFailsWhenReferencedAssetIsMissing(t *testing.T) {
	dir := t.TempDir()
	writeAssetGateSite(t, dir)

	_, err := BuildSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "docs")})
	if err == nil {
		t.Fatal("expected the asset gate to fail the build")
	}
	msg := err.Error()
	if !strings.Contains(msg, "asset check failed") {
		t.Errorf("error should name the asset gate, got: %v", err)
	}
	if !strings.Contains(msg, "/css/site.css") {
		t.Errorf("error should list the missing reference, got: %v", err)
	}
}

// TestBuildPassesWhenReferencedAssetExists checks the same site once the asset
// is present: the gate must not fire on a healthy build.
func TestBuildPassesWhenReferencedAssetExists(t *testing.T) {
	dir := t.TempDir()
	writeAssetGateSite(t, dir)
	writeFile(t, dir, filepath.Join("static", "css", "site.css"), "body{}")

	if _, err := BuildSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "docs")}); err != nil {
		t.Fatalf("healthy site should build, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "css", "site.css")); err != nil {
		t.Fatalf("static asset should be copied: %v", err)
	}
}

// TestAssetGateCanBeDowngraded verifies the documented escape hatch.
func TestAssetGateCanBeDowngraded(t *testing.T) {
	t.Setenv("HUAN_ASSET_CHECK", "off")
	p := &pipeline{
		opts:      Options{OutputDir: t.TempDir()},
		result:    &Result{},
		logf:      func(string, ...any) {},
		assetRefs: newAssetRefs(),
	}
	p.assetRefs.record("/css/missing.css")

	if err := p.checkReferencedAssets(); err != nil {
		t.Fatalf("HUAN_ASSET_CHECK=off should downgrade to a warning, got: %v", err)
	}
	if p.result.MissingAssets != 1 {
		t.Errorf("Result.MissingAssets = %d, want 1", p.result.MissingAssets)
	}
}

// TestCheckStaticParityReportsMissingSharedFiles covers the language-parity
// gate: a language output lacking part of the shared static tree is a failure,
// because the language versions would no longer correspond.
func TestCheckStaticParityReportsMissingSharedFiles(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, filepath.Join("static", "css", "site.css"), "body{}")
	writeFile(t, src, filepath.Join("static", "js", "app.js"), "console.log(1)")

	out := t.TempDir()
	writeFile(t, out, filepath.Join("css", "site.css"), "body{}") // js/app.js missing

	p := &pipeline{
		opts:   Options{SourceDir: src, OutputDir: out},
		result: &Result{},
		logf:   func(string, ...any) {},
	}
	err := p.checkStaticParity()
	if err == nil {
		t.Fatal("expected the parity gate to fail")
	}
	if !strings.Contains(err.Error(), "js/app.js") {
		t.Errorf("error should name the missing file, got: %v", err)
	}
	if p.result.MissingStatic != 1 {
		t.Errorf("Result.MissingStatic = %d, want 1", p.result.MissingStatic)
	}
}

// TestExpectedStaticFilesHonorsStaticRootAndExcludes pins the copy rules the
// parity gate mirrors.
func TestExpectedStaticFilesHonorsStaticRootAndExcludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, filepath.Join("static", "css", "site.css"), "body{}")
	writeFile(t, dir, filepath.Join("static", "en", "llms.txt"), "en")
	writeFile(t, dir, filepath.Join("static", "private", "secret.txt"), "s")

	got, err := expectedStaticFiles(dir, "en", []string{"private/"})
	if err != nil {
		t.Fatalf("expectedStaticFiles: %v", err)
	}
	want := []string{"css/site.css", "llms.txt"}
	if len(got) != len(want) {
		t.Fatalf("expectedStaticFiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expectedStaticFiles = %v, want %v", got, want)
		}
	}
}

// TestLooksLikeAsset pins which references the gate judges: static files, not
// page routes (those may be served by _redirects).
func TestLooksLikeAsset(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/css/site.css", true},
		{"/js/app.js?v=2", true},
		{"/images/a.svg#frag", true},
		{"/fonts/x.woff2", true},
		{"/papers/delval/", false},
		{"/posts", false},
		{"//cdn.example.com/a.css", false},
		{"https://cdn.example.com/a.css", false},
		{"/CNAME", false},
	}
	for _, c := range cases {
		if got := looksLikeAsset(c.path); got != c.want {
			t.Errorf("looksLikeAsset(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// writeAssetGateSite writes a one-page site whose layout references a
// stylesheet that may or may not exist under static/.
func writeAssetGateSite(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "huan.yaml", `
baseURL: https://example.com/
title: Test
languageCode: zh-cn
publishDir: docs
`)
	writeFile(t, dir, filepath.Join("content", "posts", "foo.md"), `---
title: "Foo"
date: 2026-06-14T00:00:00Z
---
Hello world.
`)
	writeFile(t, dir, filepath.Join("layouts", "_default", "single.html"),
		`<html><head><link rel="stylesheet" href="/css/site.css"></head><body>{{ .Title }}</body></html>`)
}

// TestMultiSiteFailsWhenLanguageDropsReferencedAsset reproduces the historical
// shape: an English build whose pages reference /css/... while that language
// output no longer carries static/css. It used to publish unstyled pages
// silently; now it must fail and name the reference.
func TestMultiSiteFailsWhenLanguageDropsReferencedAsset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "huan.yaml", `
baseURL: https://example.com/
title: Test
languageCode: zh-cn
publishDir: docs
defaultContentLanguage: zh-cn
languages:
  zh-cn:
    weight: 1
    languageName: 中文
    languageCode: zh-cn
  en:
    weight: 2
    languageName: English
    languageCode: en
    baseURL: /en
    staticExclude: ["css/"]
`)
	writeFile(t, dir, filepath.Join("content", "posts", "foo.md"), `---
title: "Foo"
date: 2026-06-14T00:00:00Z
---
Hello world.
`)
	// The English sidecar gives the en language a page to render, which is how
	// the historical case lost its styles: the en page referenced /css/... but
	// the en output no longer carried static/css.
	writeFile(t, dir, filepath.Join("content", "posts", "foo.en.md"), `---
title: "Foo EN"
date: 2026-06-14T00:00:00Z
---
Hello world.
`)
	writeFile(t, dir, filepath.Join("static", "css", "site.css"), "body{}")
	writeFile(t, dir, filepath.Join("layouts", "_default", "single.html"),
		`<html><head><link rel="stylesheet" href="/css/site.css"></head><body>{{ .Title }}</body></html>`)

	_, err := BuildMultiSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "docs")})
	if err == nil {
		t.Fatal("expected the asset gate to fail: en output has no css but its pages reference it")
	}
	if !strings.Contains(err.Error(), "asset check failed") || !strings.Contains(err.Error(), "/css/site.css") {
		t.Errorf("error should name the gate and the reference, got: %v", err)
	}
}
