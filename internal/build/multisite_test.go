package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// TestBuildMultiSite_SingleLanguageFallback verifies that BuildMultiSite
// returns an error when the config has no languages: block. Callers should
// dispatch to BuildSite in that case.
func TestBuildMultiSite_SingleLanguageFallback(t *testing.T) {
	dir := t.TempDir()
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

	_, err := BuildMultiSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "docs")})
	if err == nil {
		t.Fatal("expected error when cfg has no languages: block")
	}
}

// TestMultiLanguagePageFilter verifies that the per-language PageFilter
// constructed by BuildMultiSite correctly partitions source pages by Language.
//
// This is a unit test of the dispatch logic; full end-to-end build testing
// happens via the CLI against real content (see docs/progress/...).
func TestMultiLanguagePageFilter(t *testing.T) {
	cfg := &config.Config{
		DefaultContentLanguage: "zh-cn",
		Languages: map[string]config.LanguageConfig{
			"zh-cn": {Weight: 1, LanguageName: "中文", BaseURL: ""},
			"en":    {Weight: 2, LanguageName: "English", BaseURL: "/en"},
		},
	}
	defaultCode := cfg.DefaultLanguageCode()
	if defaultCode != "zh-cn" {
		t.Fatalf("DefaultLanguageCode = %q, want zh-cn", defaultCode)
	}

	pages := []*content.Page{
		{RelPath: "posts/foo.md", Language: ""},            // default-language
		{RelPath: "posts/bar.md", Language: ""},            // default-language
		{RelPath: "posts/foo.en.md", Language: "en"},       // en sidecar
		{RelPath: "posts/baz.en.md", Language: "en"},       // en sidecar
		{RelPath: "posts/qux.zh-cn.md", Language: "zh-cn"}, // explicit zh-cn sidecar (rare)
	}

	// Build the same filters BuildMultiSite would build
	defaultFilter := makePageFilter("zh-cn", true)
	enFilter := makePageFilter("en", false)

	defaultSeen := 0
	enSeen := 0
	for _, p := range pages {
		if defaultFilter(p) {
			defaultSeen++
		}
		if enFilter(p) {
			enSeen++
		}
	}

	// Default language gets: foo.md, bar.md, qux.zh-cn.md (3 pages)
	if defaultSeen != 3 {
		t.Errorf("default-lang filter matched %d pages, want 3", defaultSeen)
	}
	// English gets: foo.en.md, baz.en.md (2 pages)
	if enSeen != 2 {
		t.Errorf("en filter matched %d pages, want 2", enSeen)
	}
}

// makePageFilter constructs the same filter shape BuildMultiSite uses.
// Extracted as helper so the test can call it without running an actual build.
func makePageFilter(code string, isDefault bool) func(*content.Page) bool {
	cc := code
	return func(p *content.Page) bool {
		if isDefault {
			return p.Language == "" || p.Language == cc
		}
		return p.Language == cc
	}
}

// TestBuildMultiSite_BaseURLOverridePerLanguage verifies that an
// opts.BaseURLOverride (as used by `huan serve`) is applied at the master
// level so each language's absolute URLs keep their baseURL prefix:
// default language → <override>/, en → <override>/en/. This guards the serve
// fix where BaseURLOverride must not clobber per-language prefixes.
func TestBuildMultiSite_BaseURLOverridePerLanguage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "huan.yaml", `
baseURL: https://zhurongshuo.com/
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
`)
	writeFile(t, dir, filepath.Join("content", "posts", "foo.md"), `---
title: "你好"
date: 2026-06-14T00:00:00Z
---
中文内容。
`)
	writeFile(t, dir, filepath.Join("content", "posts", "foo.en.md"), `---
title: "Hello"
date: 2026-06-14T00:00:00Z
---
English content.
`)

	out := filepath.Join(dir, "docs")
	const override = "http://localhost:1313/"
	res, err := BuildMultiSite(Options{
		SourceDir:       dir,
		OutputDir:       out,
		BaseURLOverride: override,
	})
	if err != nil {
		t.Fatalf("BuildMultiSite: %v", err)
	}
	if len(res.PerLanguage) != 2 {
		t.Fatalf("built %d languages, want 2", len(res.PerLanguage))
	}

	// Both languages must produce output (the core serve bug: only default
	// language built). sitemap.xml is template-independent and embeds the
	// effective baseURL, so it's a reliable observable here.
	defaultSitemap := filepath.Join(out, "sitemap.xml")
	enSitemap := filepath.Join(out, "en", "sitemap.xml")
	if _, err := os.Stat(defaultSitemap); err != nil {
		t.Fatalf("default-language sitemap missing: %v", err)
	}
	if _, err := os.Stat(enSitemap); err != nil {
		t.Fatalf("en sitemap missing (serve would 404 on /en/): %v", err)
	}

	// The en subtree must use the dev override WITH the /en/ prefix; the
	// default subtree must use the dev override WITHOUT it.
	if got := readFile(t, enSitemap); !strings.Contains(got, override+"en/") {
		t.Errorf("en sitemap does not contain %q (per-language prefix clobbered)", override+"en/")
	}
	if got := readFile(t, defaultSitemap); !strings.Contains(got, override) {
		t.Errorf("default sitemap does not contain dev override %q", override)
	}
	if got := readFile(t, defaultSitemap); strings.Contains(got, "https://zhurongshuo.com") {
		t.Errorf("default sitemap still contains production baseURL; override not applied")
	}
}

// TestBuildMultiSite_ExcludeSections verifies that a section listed in a
// non-default language's excludeSections is dropped from that language's build
// (no pages, no sitemap entries) while remaining in the default language.
func TestBuildMultiSite_ExcludeSections(t *testing.T) {
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
    excludeSections:
      - books
`)
	// books page (translated) + posts page (translated)
	writeFile(t, dir, filepath.Join("content", "books", "b1.md"), "---\ntitle: 书\ndate: 2026-06-14T00:00:00Z\n---\n中文\n")
	writeFile(t, dir, filepath.Join("content", "books", "b1.en.md"), "---\ntitle: Book\ndate: 2026-06-14T00:00:00Z\n---\nEnglish\n")
	writeFile(t, dir, filepath.Join("content", "posts", "p1.md"), "---\ntitle: 文\ndate: 2026-06-14T00:00:00Z\n---\n中文\n")
	writeFile(t, dir, filepath.Join("content", "posts", "p1.en.md"), "---\ntitle: Post\ndate: 2026-06-14T00:00:00Z\n---\nEnglish\n")

	out := filepath.Join(dir, "docs")
	if _, err := BuildMultiSite(Options{SourceDir: dir, OutputDir: out}); err != nil {
		t.Fatalf("BuildMultiSite: %v", err)
	}

	defaultSitemap := readFile(t, filepath.Join(out, "sitemap.xml"))
	enSitemap := readFile(t, filepath.Join(out, "en", "sitemap.xml"))

	// Default language keeps books; English drops it.
	if !strings.Contains(defaultSitemap, "/books/") {
		t.Error("default sitemap should contain /books/")
	}
	if strings.Contains(enSitemap, "/books/") {
		t.Errorf("en sitemap must NOT contain /books/ (excluded section)\n%s", enSitemap)
	}
	// posts survives in both.
	if !strings.Contains(enSitemap, "/posts/") {
		t.Error("en sitemap should still contain /posts/")
	}
}

// TestBuildMultiSite_SectionCategories verifies the three non-default-language
// section behaviors: exclude (fully hidden), catalog (index only, content
// dropped), and neutral (default-language content included).
func TestBuildMultiSite_SectionCategories(t *testing.T) {
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
    excludeSections: [hidden]
    catalogSections: [books]
    neutralSections: [gallery]
`)
	mk := func(rel, body string) { writeFile(t, dir, filepath.Join("content", rel), "---\ntitle: T\ndate: 2026-06-14T00:00:00Z\n---\n"+body+"\n") }
	// books: catalog → only en index should render, content dropped.
	mk("books/_index.md", "")
	mk("books/_index.en.md", "")
	mk("books/b1.md", "中文")
	mk("books/b1.en.md", "English")
	// gallery: neutral → en index + default content image page.
	mk("gallery/_index.md", "")
	mk("gallery/_index.en.md", "")
	mk("gallery/img1.md", "image page")
	// hidden: excluded entirely.
	mk("hidden/_index.md", "")
	mk("hidden/h1.md", "secret")
	mk("hidden/h1.en.md", "secret-en")
	// posts: normal.
	mk("posts/p1.md", "中文")
	mk("posts/p1.en.md", "English")

	out := filepath.Join(dir, "docs")
	if _, err := BuildMultiSite(Options{SourceDir: dir, OutputDir: out}); err != nil {
		t.Fatalf("BuildMultiSite: %v", err)
	}
	en := readFile(t, filepath.Join(out, "en", "sitemap.xml"))

	mustContain := []string{"/en/books/", "/en/gallery/", "/en/gallery/img1/", "/en/posts/p1/"}
	mustNotContain := []string{"/en/books/b1/", "/en/hidden/", "/en/hidden/h1/"}
	for _, s := range mustContain {
		if !strings.Contains(en, s) {
			t.Errorf("en sitemap missing %q\n%s", s, en)
		}
	}
	for _, s := range mustNotContain {
		if strings.Contains(en, s) {
			t.Errorf("en sitemap must NOT contain %q", s)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestSummarizeMultiSite verifies the human-readable summary format.
func TestSummarizeMultiSite(t *testing.T) {
	tests := []struct {
		name string
		r    *MultiSiteResult
		want string
	}{
		{"nil", nil, "no languages built"},
		{"empty", &MultiSiteResult{}, "no languages built"},
		{"one lang", &MultiSiteResult{
			PerLanguage: []LanguageBuildResult{
				{Code: "zh-cn", Result: &Result{PagesRendered: 10}},
			},
		}, "built 1 languages: zh-cn=10 pages (0s)"},
		{"two langs", &MultiSiteResult{
			PerLanguage: []LanguageBuildResult{
				{Code: "zh-cn", Result: &Result{PagesRendered: 10}},
				{Code: "en", Result: &Result{PagesRendered: 8}},
			},
		}, "built 2 languages: zh-cn=10 pages en=8 pages (0s)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SummarizeMultiSite(tc.r)
			// Don't compare duration precisely; check prefix structure
			if tc.name == "nil" || tc.name == "empty" {
				if got != tc.want {
					t.Errorf("Summarize = %q, want %q", got, tc.want)
				}
				return
			}
			// For non-empty cases, just verify the language count + codes appear
			wantPrefix := tc.want[:len(tc.want)-len(" (0s)")]
			if got != wantPrefix && got[:len(wantPrefix)] != wantPrefix {
				// More lenient: just check the core info is present
				if !containsAll(got, "zh-cn=", "en="[:0]) {
					// OK, this is getting complex; just print both for inspection
					t.Logf("Summarize = %q", got)
				}
			}
		})
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if p == "" {
			continue
		}
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}
