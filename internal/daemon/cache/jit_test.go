package cache

import (
	"testing"
	"time"
)

func TestJITCache_SetGet(t *testing.T) {
	c := NewJITCache(10, 5*time.Minute)
	entry := &JITEntry{Path: "/test/", HTML: []byte("<h1>test</h1>"), TTL: 10 * time.Minute}
	c.Set("/test/", entry)

	got := c.Get("/test/")
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if string(got.HTML) != "<h1>test</h1>" {
		t.Errorf("expected '<h1>test</h1>', got '%s'", string(got.HTML))
	}
}

func TestJITCache_Miss(t *testing.T) {
	c := NewJITCache(10, 5*time.Minute)
	got := c.Get("/nonexistent/")
	if got != nil {
		t.Error("expected nil for missing entry")
	}
}

func TestJITCache_TTLExpiration(t *testing.T) {
	c := NewJITCache(10, 5*time.Minute)
	entry := &JITEntry{Path: "/test/", HTML: []byte("hello"), TTL: 10 * time.Millisecond}
	c.Set("/test/", entry)

	// Should be found immediately
	if c.Get("/test/") == nil {
		t.Fatal("expected entry before TTL expiry")
	}

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)
	got := c.Get("/test/")
	if got != nil {
		t.Error("expected nil after TTL expiry")
	}
}

func TestJITCache_LRUEviction(t *testing.T) {
	c := NewJITCache(3, 5*time.Minute)

	c.Set("/a/", &JITEntry{Path: "/a/", HTML: []byte("a")})
	c.Set("/b/", &JITEntry{Path: "/b/", HTML: []byte("b")})
	c.Set("/c/", &JITEntry{Path: "/c/", HTML: []byte("c")})

	// Access /a/ to make it most recently used
	c.Get("/a/")

	// Add /d/ — should evict /b/ (least recently used)
	c.Set("/d/", &JITEntry{Path: "/d/", HTML: []byte("d")})

	if c.Get("/a/") == nil {
		t.Error("/a/ should still be in cache")
	}
	if c.Get("/b/") != nil {
		t.Error("/b/ should have been evicted")
	}
	if c.Get("/c/") == nil {
		t.Error("/c/ should still be in cache")
	}
	if c.Get("/d/") == nil {
		t.Error("/d/ should be in cache")
	}
}

func TestJITCache_Clear(t *testing.T) {
	c := NewJITCache(10, 5*time.Minute)
	c.Set("/a/", &JITEntry{Path: "/a/", HTML: []byte("a")})
	c.Set("/b/", &JITEntry{Path: "/b/", HTML: []byte("b")})

	c.Clear()
	if c.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", c.Len())
	}
}

func TestJITCache_Update(t *testing.T) {
	c := NewJITCache(10, 5*time.Minute)
	c.Set("/a/", &JITEntry{Path: "/a/", HTML: []byte("old")})
	c.Set("/a/", &JITEntry{Path: "/a/", HTML: []byte("new")})

	got := c.Get("/a/")
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if string(got.HTML) != "new" {
		t.Errorf("expected 'new', got '%s'", string(got.HTML))
	}
}
