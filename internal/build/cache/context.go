package cache

import (
	"sync"

	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)

// ContextCache caches template contexts per page.
// Cache entries are invalidated when the page's version changes.
// It uses a simple map (no LRU) since it is scoped to a single build
// and does not survive across builds like ContentCache.
type ContextCache struct {
	mu    sync.RWMutex
	items map[*content.Page]*cachedContext
}

type cachedContext struct {
	ctx         *tmpl.Context
	pageVersion uint64
}

// NewContextCache creates a new ContextCache.
func NewContextCache() *ContextCache {
	return &ContextCache{
		items: make(map[*content.Page]*cachedContext),
	}
}

// Get returns the cached template context for pg if the page's version
// matches the cached version. Returns nil on miss or version mismatch.
func (c *ContextCache) Get(pg *content.Page) *tmpl.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.items[pg]
	if !exists {
		return nil
	}
	if entry.pageVersion != pg.Version {
		return nil
	}
	return entry.ctx
}

// Set stores a template context for pg, keyed by the page pointer and
// associated with pg.Version at the time of storage.
func (c *ContextCache) Set(pg *content.Page, ctx *tmpl.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[pg] = &cachedContext{
		ctx:         ctx,
		pageVersion: pg.Version,
	}
}

// Invalidate removes the cache entry for pg, if any.
func (c *ContextCache) Invalidate(pg *content.Page) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, pg)
}

// Clear removes all entries from the cache.
func (c *ContextCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[*content.Page]*cachedContext)
}

// Len returns the number of cached entries.
func (c *ContextCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}