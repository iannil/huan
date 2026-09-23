package build

import (
	"fmt"
	"github.com/iannil/huan/internal/content"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func rebuildFixture(t testing.TB, pages int) string {
	t.Helper()
	dir := t.TempDir()
	put := func(path, data string) {
		t.Helper()
		target := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	put("huan.yaml", `baseURL: https://example.com/
title: Cache test
languageCode: zh-cn
defaultContentLanguage: zh-cn
languages:
  zh-cn:
    weight: 1
  en:
    weight: 2
    baseURL: /en
`)
	put("layouts/_default/single.html", `<html><head><title>{{ .Title }}</title></head><body>{{ .Content }}{{ .Summary }}{{ .WordCount }}{{ range .Tags }}{{ . }}{{ end }}</body></html>`)
	put("layouts/_default/list.html", `{{ .Title }}{{ range .Pages }}{{ .Title }} {{ .Permalink }}{{ end }}`)
	put("layouts/_default/index.searchindex.json", `{{ range .Site.RegularPages }}{{ .Title }} {{ .Plain }}{{ end }}`)
	for i := 0; i < pages; i++ {
		for _, lang := range []string{"", ".en"} {
			put(fmt.Sprintf("content/posts/page-%03d%s.md", i, lang), fmt.Sprintf("---\ntitle: Page %d%s\ndate: 2026-01-01\ntags: [one]\n---\n# Heading %d%s\n\n", i, lang, i, lang)+strings.Repeat("中文 paragraph **bold**, `code`, [link](https://example.com).\n\n", 24))
		}
	}
	return dir
}

func outputSnapshot(t testing.TB, dir string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		result[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMarkdownCachedRebuildMatchesCleanMultiSite(t *testing.T) {
	t.Setenv("HUAN_STRICT_I18N", "")
	dir := rebuildFixture(t, 3)
	c := NewMarkdownCache(16 << 20)
	parseCache := content.NewParseCache(16 << 20)
	cached, clean := filepath.Join(t.TempDir(), "cached"), filepath.Join(t.TempDir(), "clean")
	run := func(out string, cache *MarkdownCache) {
		t.Helper()
		var inputs *content.ParseCache
		if cache != nil {
			inputs = parseCache
		}
		result, err := BuildMultiSite(Options{SourceDir: dir, OutputDir: out, ParseCache: inputs, MarkdownCache: cache, Logf: func(string, ...any) {}})
		if err != nil {
			t.Fatal(err)
		}
		for _, lang := range result.PerLanguage {
			if lang.Result.Errors != 0 {
				t.Fatalf("%s: %d render errors", lang.Code, lang.Result.Errors)
			}
		}
	}
	target := filepath.Join(dir, "content/posts/page-000.md")
	steps := []struct {
		name string
		edit func()
	}{
		{"initial", func() {}},
		{"no change", func() {}},
		{"body", func() {
			data, _ := os.ReadFile(target)
			if err := os.WriteFile(target, append(data, []byte("\nNew body[^1].\n\n[^1]: A footnote.\n")...), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"metadata", func() {
			data, _ := os.ReadFile(target)
			data = []byte(strings.ReplaceAll(string(data), "tags: [one]", "tags: [two]\nslug: changed"))
			if err := os.WriteFile(target, data, 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"rename", func() {
			if err := os.Rename(filepath.Join(dir, "content/posts/page-001.en.md"), filepath.Join(dir, "content/posts/moved.en.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete", func() {
			if err := os.Remove(filepath.Join(dir, "content/posts/page-002.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"markup config", func() {
			path := filepath.Join(dir, "huan.yaml")
			data, _ := os.ReadFile(path)
			if err := os.WriteFile(path, append(data, []byte("\nmarkup:\n  goldmark:\n    extensions:\n      typographer: false\n")...), 0644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.edit()
			run(cached, c)
			run(clean, nil)
			got, want := outputSnapshot(t, cached), outputSnapshot(t, clean)
			if !reflect.DeepEqual(got, want) {
				for path, data := range want {
					if got[path] != data {
						t.Errorf("output differs: %s", path)
					}
				}
				for path := range got {
					if _, ok := want[path]; !ok {
						t.Errorf("stale output: %s", path)
					}
				}
			}
		})
	}
	if c.Stats().Hits == 0 {
		t.Fatal("rebuilds never used cached markdown")
	}
}

func BenchmarkMultiSiteRebuild(b *testing.B) {
	dir := rebuildFixture(b, 100)
	for _, mode := range []string{"uncached", "cached"} {
		b.Run(mode, func(b *testing.B) {
			var cache *MarkdownCache
			if mode == "cached" {
				cache = NewMarkdownCache(128 << 20)
			}
			opts := Options{SourceDir: dir, OutputDir: filepath.Join(b.TempDir(), "out"), MarkdownCache: cache, Logf: func(string, ...any) {}}
			if _, err := BuildMultiSite(opts); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := BuildMultiSite(opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
