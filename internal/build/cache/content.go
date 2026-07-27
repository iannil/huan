// Package cache provides caching utilities for the build pipeline.
//
// ContentCache (the main type in this package) caches loaded page content
// keyed by content-relative path, with LRU eviction and mtime-based
// invalidation. It is safe for concurrent use.
package cache

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/iannil/huan/internal/content"
)

// LoadPageFunc loads a single page from the given content-relative path.
// Returns the loaded page and the file's modification time.
type LoadPageFunc func(path string) (*content.Page, time.Time, error)

// pageEntry is a single entry in the ContentCache.
type pageEntry struct {
	key     string
	page    *content.Page
	mtime   time.Time
	element *list.Element
}

// ContentCache provides LRU caching for loaded content pages.
// Key is the content-relative path (e.g. "posts/hello.md").
// Cache entries are invalidated when the file's mtime changes.
type ContentCache struct {
	mu    sync.RWMutex
	items map[string]*pageEntry
	lru   *list.List
	max   int
}

// NewContentCache creates a ContentCache with the given max entries.
// max must be > 0; values <= 0 are treated as 5000.
func NewContentCache(max int) *ContentCache {
	if max <= 0 {
		max = 5000
	}
	return &ContentCache{
		items: make(map[string]*pageEntry),
		lru:   list.New(),
		max:   max,
	}
}

// GetOrLoad returns the cached page for path if it exists.
// If the cache misses, it calls loadFn to load the page, caches it (evicting
// the LRU entry if at capacity), and returns the result. loadFn is only called
// when needed.
//
// NOTE: This cache uses caller-driven invalidation — callers must call
// Invalidate/InvalidateByPrefix/Clear when files change (typically via the
// file watcher in daemon mode). The cache does NOT re-stat the filesystem
// on GetOrLoad, so stale entries are returned until explicitly invalidated.
//
// Concurrent calls to GetOrLoad with the same path are safe: only one
// goroutine calls loadFn; the others wait and receive the cached result.
func (c *ContentCache) GetOrLoad(path string, loadFn LoadPageFunc) (*content.Page, error) {
	// Fast path: check cache under read lock.
	c.mu.RLock()
	if entry, exists := c.items[path]; exists {
		c.lru.MoveToFront(entry.element)
		c.mu.RUnlock()
		return entry.page, nil
	}
	c.mu.RUnlock()

	// Slow path: acquire write lock, double-check, then load if needed.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check: another goroutine may have loaded and stored while we
	// were between RUnlock and Lock.
	if entry, exists := c.items[path]; exists {
		c.lru.MoveToFront(entry.element)
		return entry.page, nil
	}

	// Load via loadFn.
	pg, mtime, err := loadFn(path)
	if err != nil {
		return nil, fmt.Errorf("content cache: load %s: %w", path, err)
	}

	// Evict LRU if at capacity.
	for c.lru.Len() >= c.max {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		oldKey := oldest.Value.(*pageEntry).key
		delete(c.items, oldKey)
		c.lru.Remove(oldest)
	}

	elem := c.lru.PushFront(&pageEntry{
		key:   path,
		page:  pg,
		mtime: mtime,
	})
	c.items[path] = &pageEntry{
		key:     path,
		page:    pg,
		mtime:   mtime,
		element: elem,
	}

	return pg, nil
}

// Invalidate removes a single entry from the cache by path.
func (c *ContentCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, exists := c.items[path]; exists {
		c.lru.Remove(entry.element)
		delete(c.items, path)
	}
}

// InvalidateByPrefix removes all entries whose key starts with the given prefix.
// Useful for directory-level invalidation when files are added or removed.
func (c *ContentCache) InvalidateByPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			c.lru.Remove(entry.element)
			delete(c.items, key)
		}
	}
}

// Clear removes all entries from the cache.
func (c *ContentCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*pageEntry)
	c.lru = list.New()
}

// Store inserts or updates a cache entry for path with the given page and mtime.
// This is a single-lock write operation without the double-checked loading
// overhead of GetOrLoad. Use it when you already have fresh data and just
// want to populate the cache (e.g., write-through pattern from full builds).
//
// Unlike GetOrLoad, Store does not call loadFn and does not check for existing
// entries — it unconditionally writes the entry.
func (c *ContentCache) Store(path string, pg *content.Page, mtime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If an entry already exists, remove it from the LRU list first.
	if entry, exists := c.items[path]; exists {
		c.lru.Remove(entry.element)
		delete(c.items, path)
	}

	// Evict LRU if at capacity.
	for c.lru.Len() >= c.max {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		oldKey := oldest.Value.(*pageEntry).key
		delete(c.items, oldKey)
		c.lru.Remove(oldest)
	}

	elem := c.lru.PushFront(&pageEntry{
		key:   path,
		page:  pg,
		mtime: mtime,
	})
	c.items[path] = &pageEntry{
		key:     path,
		page:    pg,
		mtime:   mtime,
		element: elem,
	}
}

// Len returns the current number of cached entries.
func (c *ContentCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}