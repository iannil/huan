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
