package cache

import (
	"sync"
	"testing"

	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)

// newTestPage creates a minimal Page with the given version.
func newTestPage(version uint64) *content.Page {
	return &content.Page{
		Title:   "Test",
		RelPath: "test.md",
		Version: version,
	}
}

// newTestContext creates a minimal template Context.
func newTestContext(title string) *tmpl.Context {
	return &tmpl.Context{
		Title: title,
	}
}

func TestContextCache_GetSetHit(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)
	ctx := newTestContext("hello")

	c.Set(pg, ctx)
	got := c.Get(pg)
	if got == nil {
		t.Fatal("expected hit, got nil")
	}
	if got.Title != "hello" {
		t.Fatalf("expected Title=hello, got %s", got.Title)
	}
}

func TestContextCache_MissBeforeSet(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)

	got := c.Get(pg)
	if got != nil {
		t.Fatal("expected nil before Set")
	}
}

func TestContextCache_VersionMismatch(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)
	ctx := newTestContext("v1")

	c.Set(pg, ctx)

	// Simulate page reload: create a new Page with same pointer identity
	// won't work for map key, so we mutate the version in-place.
	pg.Version = 2

	got := c.Get(pg)
	if got != nil {
		t.Fatal("expected nil on version mismatch")
	}
}

func TestContextCache_UpdateAfterVersionChange(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)
	c.Set(pg, newTestContext("v1"))

	// Version changes, then re-Set
	pg.Version = 2
	c.Set(pg, newTestContext("v2"))

	got := c.Get(pg)
	if got == nil {
		t.Fatal("expected hit after re-Set")
	}
	if got.Title != "v2" {
		t.Fatalf("expected Title=v2, got %s", got.Title)
	}
}

func TestContextCache_Invalidate(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)
	c.Set(pg, newTestContext("hello"))

	c.Invalidate(pg)
	got := c.Get(pg)
	if got != nil {
		t.Fatal("expected nil after Invalidate")
	}
}

func TestContextCache_InvalidateNonExistent(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)

	// Should not panic
	c.Invalidate(pg)
	_ = c.Len()
}

func TestContextCache_Clear(t *testing.T) {
	c := NewContextCache()
	pg1 := newTestPage(1)
	pg2 := newTestPage(1)
	c.Set(pg1, newTestContext("a"))
	c.Set(pg2, newTestContext("b"))

	c.Clear()

	if c.Len() != 0 {
		t.Fatalf("expected 0 entries after Clear, got %d", c.Len())
	}
	if c.Get(pg1) != nil {
		t.Fatal("expected nil after Clear")
	}
}

func TestContextCache_Len(t *testing.T) {
	c := NewContextCache()
	if c.Len() != 0 {
		t.Fatalf("expected 0, got %d", c.Len())
	}

	pg1 := newTestPage(1)
	pg2 := newTestPage(1)
	c.Set(pg1, newTestContext("a"))
	if c.Len() != 1 {
		t.Fatalf("expected 1, got %d", c.Len())
	}
	c.Set(pg2, newTestContext("b"))
	if c.Len() != 2 {
		t.Fatalf("expected 2, got %d", c.Len())
	}
}

func TestContextCache_ConcurrentAccess(t *testing.T) {
	c := NewContextCache()
	pg := newTestPage(1)
	ctx := newTestContext("concurrent")

	var wg sync.WaitGroup
	n := 20

	// Concurrently set and get
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Set(pg, ctx)
			_ = c.Get(pg)
			_ = c.Len()
		}()
	}
	wg.Wait()

	// Concurrent Invalidate
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Invalidate(pg)
			c.Set(pg, ctx)
		}()
	}
	wg.Wait()

	// Concurrent Clear
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Clear()
		}()
	}
	wg.Wait()
}

func TestContextCache_DifferentPages(t *testing.T) {
	c := NewContextCache()
	pg1 := newTestPage(1)
	pg2 := newTestPage(1)

	c.Set(pg1, newTestContext("page1"))
	c.Set(pg2, newTestContext("page2"))

	if c.Get(pg1).Title != "page1" {
		t.Fatal("pg1 mismatch")
	}
	if c.Get(pg2).Title != "page2" {
		t.Fatal("pg2 mismatch")
	}
	if c.Len() != 2 {
		t.Fatalf("expected 2, got %d", c.Len())
	}
}

func TestContextCache_NewContextCache(t *testing.T) {
	c := NewContextCache()
	if c == nil {
		t.Fatal("NewContextCache returned nil")
	}
	if c.Len() != 0 {
		t.Fatalf("expected empty cache, got %d", c.Len())
	}
}