package cache

import (
	"container/list"
	"sync"
	"time"
)

// JITEntry holds a single JIT-rendered page.
type JITEntry struct {
	Path       string
	HTML       []byte
	Size       int64
	ContentType string
	RenderedAt time.Time
	TTL        time.Duration
}

// JITCache provides LRU + TTL caching for JIT-rendered pages.
// Not safe for concurrent use — callers must hold the lock.
type JITCache struct {
	mu       sync.RWMutex
	entries  map[string]*list.Element
	ll       *list.List // LRU order: front = most recently used
	maxSize  int
	defaultTTL time.Duration
}

type jitCacheItem struct {
	key   string
	entry *JITEntry
}

// NewJITCache creates a JITCache with the given max entry count and default TTL.
func NewJITCache(maxSize int, defaultTTL time.Duration) *JITCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}
	return &JITCache{
		entries:    make(map[string]*list.Element),
		ll:         list.New(),
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
	}
}

// Get retrieves a cached entry. Returns nil if not found or expired.
func (c *JITCache) Get(path string) *JITEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[path]
	if !ok {
		return nil
	}

	item := elem.Value.(*jitCacheItem)

	// Check TTL expiration
	if time.Since(item.entry.RenderedAt) > item.entry.TTL {
		c.removeElement(elem)
		return nil
	}

	// Move to front (most recently used)
	c.ll.MoveToFront(elem)
	return item.entry
}

// Set adds or updates a cached entry.
func (c *JITCache) Set(path string, entry *JITEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry.TTL == 0 {
		entry.TTL = c.defaultTTL
	}
	entry.Size = int64(len(entry.HTML))
	entry.RenderedAt = time.Now()

	// If exists, update in place
	if elem, ok := c.entries[path]; ok {
		item := elem.Value.(*jitCacheItem)
		item.entry = entry
		c.ll.MoveToFront(elem)
		return
	}

	// Evict if at capacity
	if c.ll.Len() >= c.maxSize {
		c.evictOldest()
	}

	item := &jitCacheItem{key: path, entry: entry}
	elem := c.ll.PushFront(item)
	c.entries[path] = elem
}

// Remove deletes a cached entry.
func (c *JITCache) Remove(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[path]; ok {
		c.removeElement(elem)
	}
}

// Clear removes all entries.
func (c *JITCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element)
	c.ll = list.New()
}

// Len returns the number of cached entries.
func (c *JITCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ll.Len()
}

// evictOldest removes the least recently used entry.
func (c *JITCache) evictOldest() {
	elem := c.ll.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

func (c *JITCache) removeElement(elem *list.Element) {
	item := elem.Value.(*jitCacheItem)
	delete(c.entries, item.key)
	c.ll.Remove(elem)
}
