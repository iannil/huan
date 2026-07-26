package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iannil/huan/internal/content"
)

// tempContentDir creates a temporary content/ directory and returns its path.
func tempContentDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	contentDir := filepath.Join(dir, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeContentFile creates a content file at the given content-relative path
// within the temp directory and returns its mtime.
func writeContentFile(t *testing.T, dir, relPath string) time.Time {
	t.Helper()
	fullPath := filepath.Join(dir, "content", relPath)
	_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, []byte("---\ntitle: Test\n---\nBody"), 0644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime()
}

// makeLoadFn creates a LoadPageFunc that reads from the given temp directory.
func makeLoadFn(t *testing.T, dir string, counter *int) (LoadPageFunc, string) {
	t.Helper()
	contentDir := filepath.Join(dir, "content")
	return func(path string) (*content.Page, time.Time, error) {
		if counter != nil {
			*counter++
		}
		fullPath := filepath.Join(contentDir, path)
		fi, err := os.Stat(fullPath)
		if err != nil {
			return nil, time.Time{}, err
		}
		pg := &content.Page{
			RelPath: path,
			Title:   "Test",
		}
		return pg, fi.ModTime(), nil
	}, contentDir
}

func TestContentCache_GetOrLoad(t *testing.T) {
	dir := tempContentDir(t)
	mtime := writeContentFile(t, dir, "test.md")
	c := NewContentCache(100)

	fn := func(path string) (*content.Page, time.Time, error) {
		pg := &content.Page{RelPath: path, Title: "Test"}
		return pg, mtime, nil
	}

	pg, err := c.GetOrLoad("test.md", fn)
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if pg.Title != "Test" {
		t.Fatalf("expected Title=Test, got %s", pg.Title)
	}
}

func TestContentCache_Hit(t *testing.T) {
	dir := tempContentDir(t)
	writeContentFile(t, dir, "hit.md")
	c := NewContentCache(100)
	callCount := 0
	fn, _ := makeLoadFn(t, dir, &callCount)

	// First call: miss
	_, _ = c.GetOrLoad("hit.md", fn)
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	// Second call: hit (file hasn't changed)
	_, _ = c.GetOrLoad("hit.md", fn)
	if callCount != 1 {
		t.Fatalf("expected 1 call on hit, got %d", callCount)
	}
}

func TestContentCache_Invalidate(t *testing.T) {
	dir := tempContentDir(t)
	writeContentFile(t, dir, "inv.md")
	c := NewContentCache(100)
	callCount := 0
	fn, _ := makeLoadFn(t, dir, &callCount)

	_, _ = c.GetOrLoad("inv.md", fn)
	c.Invalidate("inv.md")
	_, _ = c.GetOrLoad("inv.md", fn)

	if callCount != 2 {
		t.Fatalf("expected 2 calls after invalidate, got %d", callCount)
	}
}

func TestContentCache_Clear(t *testing.T) {
	dir := tempContentDir(t)
	writeContentFile(t, dir, "a.md")
	writeContentFile(t, dir, "b.md")
	c := NewContentCache(100)
	callCount := 0
	fn, _ := makeLoadFn(t, dir, &callCount)

	_, _ = c.GetOrLoad("a.md", fn)
	_, _ = c.GetOrLoad("b.md", fn)
	c.Clear()
	_, _ = c.GetOrLoad("a.md", fn)

	if callCount != 3 {
		t.Fatalf("expected 3 calls after clear, got %d", callCount)
	}
}

func TestContentCache_LRUEviction(t *testing.T) {
	dir := tempContentDir(t)
	writeContentFile(t, dir, "a.md")
	writeContentFile(t, dir, "b.md")
	writeContentFile(t, dir, "c.md")
	c := NewContentCache(2) // only 2 entries
	callCount := 0
	fn, _ := makeLoadFn(t, dir, &callCount)

	_, _ = c.GetOrLoad("a.md", fn) // 1
	_, _ = c.GetOrLoad("b.md", fn) // 2
	_, _ = c.GetOrLoad("c.md", fn) // 3 — evicts "a.md"
	_, _ = c.GetOrLoad("a.md", fn) // 4 — reload after eviction

	if callCount != 4 {
		t.Fatalf("expected 4 calls (3 loads + 1 reload after eviction), got %d", callCount)
	}
}

func TestContentCache_InvalidateByPrefix(t *testing.T) {
	dir := tempContentDir(t)
	writeContentFile(t, dir, "posts/a.md")
	writeContentFile(t, dir, "posts/b.md")
	writeContentFile(t, dir, "pages/c.md")
	c := NewContentCache(100)
	callCount := 0
	fn, _ := makeLoadFn(t, dir, &callCount)

	_, _ = c.GetOrLoad("posts/a.md", fn)
	_, _ = c.GetOrLoad("posts/b.md", fn)
	_, _ = c.GetOrLoad("pages/c.md", fn)

	c.InvalidateByPrefix("posts/")
	_, _ = c.GetOrLoad("posts/a.md", fn)
	_, _ = c.GetOrLoad("pages/c.md", fn) // should be hit

	if callCount != 4 {
		t.Fatalf("expected 4 calls (3 loads + 1 reload after prefix invalidation), got %d", callCount)
	}
}

func TestContentCache_ConcurrentAccess(t *testing.T) {
	dir := tempContentDir(t)
	writeContentFile(t, dir, "concurrent.md")
	c := NewContentCache(100)
	fn, _ := makeLoadFn(t, dir, nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := c.GetOrLoad("concurrent.md", fn)
			if err != nil {
				t.Errorf("concurrent GetOrLoad failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if c.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", c.Len())
	}
}

func TestContentCache_ZeroMax(t *testing.T) {
	c := NewContentCache(0) // should default to 5000
	if c.max != 5000 {
		t.Fatalf("expected max=5000, got %d", c.max)
	}
}

func TestContentCache_NegativeMax(t *testing.T) {
	c := NewContentCache(-1) // should default to 5000
	if c.max != 5000 {
		t.Fatalf("expected max=5000, got %d", c.max)
	}
}