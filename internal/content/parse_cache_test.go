package content

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func writeParseCacheFile(t testing.TB, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestParseCacheIsolatesPageMutationsAndVersions(t *testing.T) {
	dir := t.TempDir()
	writeParseCacheFile(t, filepath.Join(dir, "post.md"), "---\ntitle: Original\ntags: [one]\nkeywords: [key]\nbuild:\n  render: always\n---\nOriginal body")
	c := NewParseCache(1 << 20)
	load := func() *Page {
		t.Helper()
		pages, err := LoadDirWithCache(dir, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pages[0]
	}
	first := load()
	oldVersion := first.Version
	first.Title = "polluted"
	first.Tags[0] = "polluted"
	first.Keywords[0] = "polluted"
	first.RawContent = "polluted"
	first.Build.Render = "never"
	first.Parent = first
	first.Pages = []*Page{first}
	first.RegularPages = first.Pages
	first.RegularPagesRecursive = first.Pages
	first.Sections = first.Pages
	first.Content = "polluted"
	first.Summary = "polluted"
	first.Plain = "polluted"
	first.WordCount = 100
	first.URL = "polluted"
	second := load()
	if first == second || second.Version == oldVersion || second.Version == 0 {
		t.Fatal("cache reused identity/version")
	}
	if second.Title != "Original" || second.Tags[0] != "one" || second.Keywords[0] != "key" || second.RawContent != "Original body" || second.Build.Render != "always" {
		t.Fatalf("cache polluted: %+v", second)
	}
	if second.Parent != nil || second.Pages != nil || second.RegularPages != nil || second.RegularPagesRecursive != nil || second.Sections != nil || second.Content != "" || second.Summary != "" || second.Plain != "" || second.WordCount != 0 || second.URL != "" {
		t.Fatalf("reused computed fields: %+v", second)
	}
	if s := c.Stats(); s.Hits != 1 || s.Misses != 1 {
		t.Fatalf("stats=%+v", s)
	}
}

func TestParseCacheAlwaysReadsAndObservesSameStatEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	writeParseCacheFile(t, path, "---\ntitle: Old\n---\nOne")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	c := NewParseCache(1 << 20)
	var observed []string
	observe := func(_ string, data []byte) { observed = append(observed, string(data)) }
	first, err := LoadDirWithCache(dir, c, observe)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadDirWithCache(dir, c, observe)
	if err != nil {
		t.Fatal(err)
	}
	writeParseCacheFile(t, path, "---\ntitle: New\n---\nTwo")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	third, err := LoadDirWithCache(dir, c, observe)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Title != "Old" || second[0].Title != "Old" || third[0].Title != "New" || third[0].RawContent != "Two" || len(observed) != 3 || !strings.Contains(observed[2], "New") {
		t.Fatalf("missed edit or observation: %+v, %v", third, observed)
	}
	if s := c.Stats(); s.Hits != 1 || s.Misses != 2 {
		t.Fatalf("stats=%+v", s)
	}
	// Parsing errors must not be hidden by a formerly successful entry.
	writeParseCacheFile(t, path, "---\ntitle: [\n---")
	if pages, err := LoadDirWithCache(dir, c, observe); err == nil || len(pages) != 0 || len(observed) != 4 {
		t.Fatalf("pages=%v err=%v observations=%d", pages, err, len(observed))
	}
}

func TestParseCachePathIdentityAndDirectoryChanges(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	c := NewParseCache(1 << 20)
	body := "---\ntitle: Same\n---\nBody"
	for _, path := range []string{filepath.Join(dir, "posts/a.md"), filepath.Join(dir, "posts/a.en.md"), filepath.Join(other, "posts/a.md")} {
		writeParseCacheFile(t, path, body)
	}
	load := func(root string) []*Page {
		t.Helper()
		pages, err := LoadDirWithCache(root, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pages
	}
	pages := load(dir)
	load(other)
	if s := c.Stats(); s.Misses != 3 || s.Hits != 0 {
		t.Fatalf("language/site path collision: %+v", s)
	}
	if pages[0].Language != "en" || pages[1].Language != "" || pages[0].RelPath != "posts/a.md" || pages[1].RelPath != "posts/a.md" {
		t.Fatalf("wrong sidecar identity: %+v %+v", pages[0], pages[1])
	}
	nested := load(filepath.Join(dir, "posts"))
	if nested[0].RelPath != "a.md" || nested[1].RelPath != "a.md" || c.Stats().Hits != 2 {
		t.Fatal("cache leaked previous root-relative paths")
	}
	if err := os.Rename(filepath.Join(dir, "posts/a.en.md"), filepath.Join(dir, "posts/b.fr.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "posts/a.md")); err != nil {
		t.Fatal(err)
	}
	writeParseCacheFile(t, filepath.Join(dir, "posts/new.md"), "New")
	pages = load(dir)
	if len(pages) != 2 || pages[0].RelPath != "posts/b.md" || pages[0].Language != "fr" || pages[1].RelPath != "posts/new.md" {
		t.Fatalf("stale directory membership: %+v", pages)
	}
}

func TestParseCacheLRUBounds(t *testing.T) {
	c := NewParseCache(5000)
	page := &Page{Title: "Title", RawContent: "Body"}
	a, b, d := parseCacheKey{path: "a"}, parseCacheKey{path: "b"}, parseCacheKey{path: "c"}
	c.put(a, page)
	c.put(b, page)
	if _, ok := c.get(a); !ok {
		t.Fatal("missing first page")
	}
	c.put(d, page)
	if _, ok := c.get(b); ok {
		t.Fatal("least recently used entry retained")
	}
	if _, ok := c.get(a); !ok {
		t.Fatal("recent entry evicted")
	}
	if s := c.Stats(); s.Entries != 2 || s.Bytes > 5000 || s.MaxBytes != 5000 {
		t.Fatalf("LRU stats=%+v", s)
	}
	c.put(b, &Page{RawContent: strings.Repeat("x", 6000)})
	if s := c.Stats(); s.Entries != 2 || s.Bytes > 5000 {
		t.Fatalf("oversized page entered cache: %+v", s)
	}
	for _, limit := range []int64{0, -1, 1} {
		disabled := NewParseCache(limit)
		disabled.put(a, page)
		if s := disabled.Stats(); s.Entries != 0 || s.Bytes != 0 {
			t.Fatalf("disabled cache stores entries: %+v", s)
		}
	}
}

func TestParseCacheConcurrentLoadsMatchUncached(t *testing.T) {
	dir := t.TempDir()
	writeParseCacheFile(t, filepath.Join(dir, "a.md"), "---\ntitle: Original\ntags: []\nkeywords: [one]\n---\nBody")
	c := NewParseCache(1 << 20)
	expected, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	expected[0].Version = 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				pages, err := LoadDirWithCache(dir, c, nil)
				if err != nil {
					t.Error(err)
					return
				}
				pages[0].Version = 0
				if !reflect.DeepEqual(pages, expected) {
					t.Errorf("cached page differs: %+v", pages[0])
					return
				}
				pages[0].Keywords[0] = "mutated"
			}
		}()
	}
	wg.Wait()
}

func BenchmarkLoadDirParseCache(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 300; i++ {
		writeParseCacheFile(b, filepath.Join(dir, fmt.Sprintf("page-%04d.md", i)), fmt.Sprintf("---\ntitle: Page %d\ndate: 2026-01-01\ntags: [one, two]\nkeywords: [a, b]\ndescription: A description\nbuild:\n  render: always\n---\n", i)+strings.Repeat("Markdown body.\n", 100))
	}
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprintf("cached=%v", cached), func(b *testing.B) {
			var c *ParseCache
			if cached {
				c = NewParseCache(16 << 20)
			}
			if _, err := LoadDirWithCache(dir, c, nil); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := LoadDirWithCache(dir, c, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestParseCachePreservesErrorAndObservationOrder(t *testing.T) {
	dir := t.TempDir()
	writeParseCacheFile(t, filepath.Join(dir, "a.md"), "First")
	writeParseCacheFile(t, filepath.Join(dir, "c.md"), "Last")
	cache := NewParseCache(1 << 20)
	if _, err := LoadDirWithCache(dir, cache, nil); err != nil {
		t.Fatal(err)
	}
	writeParseCacheFile(t, filepath.Join(dir, "b.md"), "---\ntitle: [\n---")
	var paths []string
	pages, err := LoadDirWithCache(dir, cache, func(path string, _ []byte) { paths = append(paths, filepath.Base(path)) })
	if err == nil || !strings.Contains(err.Error(), "parse "+filepath.Join(dir, "b.md")) || len(pages) != 1 || pages[0].RawContent != "First" {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
	if !reflect.DeepEqual(paths, []string{"a.md", "b.md"}) {
		t.Fatalf("observation order=%v", paths)
	}
	if s := cache.Stats(); s.Hits != 1 || s.Entries != 2 {
		t.Fatalf("failure cached or later page loaded: %+v", s)
	}
}

func TestParseCacheBudgetIncludesMetadataAndSliceStrings(t *testing.T) {
	large := strings.Repeat("x", 6000)
	for _, page := range []*Page{{Title: large}, {Description: large}, {Tags: []string{large}}, {Keywords: []string{large}}, {RawContent: large}} {
		c := NewParseCache(5000)
		c.put(parseCacheKey{path: "a"}, page)
		if s := c.Stats(); s.Entries != 0 || s.Bytes != 0 {
			t.Fatalf("oversized metadata cached: %+v", s)
		}
	}
	c := NewParseCache(5000)
	c.put(parseCacheKey{path: large}, &Page{})
	if s := c.Stats(); s.Entries != 0 {
		t.Fatalf("oversized key cached: %+v", s)
	}
}
