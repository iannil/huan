package build

import (
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInputPageIsolation(t *testing.T) {
	src := &content.Page{Tags: []string{"one"}, Keywords: []string{"key"}}
	src.Cascade.Build.Render = "always"
	src.Parent = src
	src.Pages, src.RegularPages, src.RegularPagesRecursive, src.Sections = []*content.Page{src}, []*content.Page{src}, []*content.Page{src}, []*content.Page{src}
	a, b := cloneInputPage(src), cloneInputPage(src)
	a.Tags[0], a.Keywords[0] = "changed", "changed"
	a.Cascade.Build.Render = "never"
	if b.Tags[0] != "one" || b.Keywords[0] != "key" || b.Cascade.Build.Render != "always" || src.Tags[0] != "one" {
		t.Fatal("language inputs share mutable state")
	}
	if a.Parent != nil || a.Pages != nil || a.RegularPages != nil || a.RegularPagesRecursive != nil || a.Sections != nil {
		t.Fatal("copied tree links")
	}
}
func TestInputDataIsolation(t *testing.T) {
	src := map[string]interface{}{"books": []interface{}{map[string]interface{}{"title": "A", "yaml": map[interface{}]interface{}{1: []interface{}{"original"}}}}}
	a := cloneInputData(src).(map[string]interface{})
	book := a["books"].([]interface{})[0].(map[string]interface{})
	book["title"] = "B"
	book["yaml"].(map[interface{}]interface{})[1].([]interface{})[0] = "changed"
	original := src["books"].([]interface{})[0].(map[string]interface{})
	if original["title"] != "A" || original["yaml"].(map[interface{}]interface{})[1].([]interface{})[0] != "original" {
		t.Fatal("data snapshot was mutated")
	}
}

func TestInputLoadFailures(t *testing.T) {
	for _, tc := range []struct{ name, path, body, want string }{
		{"content", "content/posts/foo.md", "---\ntitle: [\n---\nBad", "load content:"},
		{"data", "data/books.yaml", "books: [", "load data:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "content/posts/good.md", "---\ntitle: Good\n---\nGood")
			writeFile(t, dir, tc.path, tc.body)
			_, err := loadBuildInputs(dir, &config.Config{}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	dir := t.TempDir()
	writeFile(t, dir, "content/posts/good.md", "Good")
	in, err := loadBuildInputs(dir, &config.Config{}, nil)
	if err != nil || in.data == nil || len(in.data) != 0 {
		t.Fatalf("missing data: %v, %v", in, err)
	}
}

func TestInputStrictStale(t *testing.T) {
	t.Setenv("HUAN_STRICT_I18N", "1")
	dir := t.TempDir()
	writeFile(t, dir, "huan.yaml", `baseURL: https://example.com/
title: Test
defaultContentLanguage: zh-cn
languages:
  zh-cn:
    weight: 1
  en:
    weight: 2
`)
	writeFile(t, dir, "content/posts/foo.md", "---\ntitle: Source\n---\nSource")
	writeFile(t, dir, "content/posts/foo.en.md", "---\ntitle: English\nsource_hash: deadbeef\n---\nEnglish")
	_, err := BuildMultiSite(Options{SourceDir: dir, OutputDir: filepath.Join(dir, "out")})
	if err == nil || !strings.Contains(err.Error(), "build language zh-cn:") || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("strict error = %v", err)
	}
}

func TestInputPipelineCacheIsolation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "content/posts/foo.md", "---\ntitle: Source\ntags: [original]\n---\nSource")
	cfg := &config.Config{}
	in, err := loadBuildInputs(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	var copies []*content.Page
	for i := 0; i < 2; i++ {
		pc := NewPipelineCache()
		p := newPipeline(Options{SourceDir: dir, PipelineCache: pc, PageFilter: func(pg *content.Page) bool { pg.Tags[0] = "filtered"; return true }})
		p.cfg, p.inputs = cfg, in
		if err := p.loadContent(); err != nil {
			t.Fatal(err)
		}
		cached, err := pc.ContentCache.GetOrLoad("posts/foo.md", func(string) (*content.Page, time.Time, error) { t.Fatal("cache miss"); return nil, time.Time{}, nil })
		if err != nil || cached != p.pages[0] || cached == in.pages[0] {
			t.Fatalf("cache did not store current pipeline copy: %v", err)
		}
		copies = append(copies, cached)
	}
	if copies[0] == copies[1] || in.pages[0].Tags[0] != "original" {
		t.Fatal("filter/cache shared language input")
	}
}

func TestInputPipelineCachePairedLanguages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "content/posts/foo.md", "---\ntitle: Source\n---\nDefault body")
	writeFile(t, dir, "content/posts/foo.en.md", "---\ntitle: English\n---\nEnglish body")
	writeFile(t, dir, "content/gallery/image.md", "---\ntitle: Image\n---\nNeutral body")
	writeFile(t, dir, "content/gallery/image.en.md", "---\ntitle: Excluded\n---\nExcluded sidecar")
	cfg := &config.Config{}
	in, err := loadBuildInputs(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	var neutralCopies []*content.Page
	for _, lang := range []string{"", "en"} {
		t.Run("language_"+lang, func(t *testing.T) {
			calls := 0
			pc := NewPipelineCache()
			p := newPipeline(Options{SourceDir: dir, PipelineCache: pc, PageFilter: func(pg *content.Page) bool {
				calls++
				if strings.HasPrefix(pg.RelPath, "gallery/") {
					return pg.Language == ""
				}
				return pg.Language == lang
			}})
			p.cfg, p.inputs = cfg, in
			if err := p.loadContent(); err != nil {
				t.Fatal(err)
			}
			if calls != len(in.pages) || len(p.pages) != 2 {
				t.Fatalf("filter calls=%d retained=%d", calls, len(p.pages))
			}
			for _, retained := range p.pages {
				cached, err := pc.ContentCache.GetOrLoad(retained.RelPath, func(string) (*content.Page, time.Time, error) { t.Fatal("cache miss"); return nil, time.Time{}, nil })
				if err != nil || cached != retained || cached.Language != retained.Language || cached.RawContent != retained.RawContent {
					t.Fatalf("cache contains excluded variant for %s: cached=%+v retained=%+v err=%v", retained.RelPath, cached, retained, err)
				}
				if strings.HasPrefix(retained.RelPath, "gallery/") {
					if cached.Language != "" || !strings.Contains(cached.RawContent, "Neutral body") {
						t.Fatal("neutral gallery cache language changed")
					}
					neutralCopies = append(neutralCopies, cached)
				}
			}
		})
	}
	if len(neutralCopies) != 2 || neutralCopies[0] == neutralCopies[1] {
		t.Fatal("neutral cache copies share identity")
	}
}
