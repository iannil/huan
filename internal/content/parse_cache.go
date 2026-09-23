package content

import (
	"container/list"
	"crypto/sha256"
	"slices"
	"strings"
	"sync"
)

// ParseCache reuses immutable parsed Markdown inputs across directory loads.
// It is safe to share across builds and sites. The zero value disables storage.
// Capacity bounds owned string data and a conservative bookkeeping allowance,
// not the Go allocator's exact heap size. Do not copy after first use.
type ParseCache struct {
	mu           sync.Mutex
	maxBytes     int64
	bytes        int64
	entries      map[parseCacheKey]*list.Element
	lru          list.List
	hits, misses uint64
}

type parseCacheKey struct {
	path string
	hash [sha256.Size]byte
}

type parseCacheEntry struct {
	key  parseCacheKey
	page *Page
	size int64
}

// ParseCacheStats describes cumulative lookups and retained input versions.
type ParseCacheStats struct {
	Hits, Misses    uint64
	Entries         int
	Bytes, MaxBytes int64
}

// NewParseCache creates an LRU with a bounded byte budget for parsed inputs.
// Nonpositive capacities disable storage. All files are still read each load;
// only identical path-and-content pairs reuse their parsed fields.
func NewParseCache(maxBytes int64) *ParseCache {
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &ParseCache{maxBytes: maxBytes}
}

// Stats returns a consistent snapshot of the cache counters and capacity.
func (c *ParseCache) Stats() ParseCacheStats {
	if c == nil {
		return ParseCacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return ParseCacheStats{Hits: c.hits, Misses: c.misses, Entries: len(c.entries), Bytes: c.bytes, MaxBytes: c.maxBytes}
}

func (c *ParseCache) get(key parseCacheKey) (*Page, bool) {
	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		c.misses++
		c.mu.Unlock()
		return nil, false
	}
	c.hits++
	c.lru.MoveToFront(e)
	page := e.Value.(parseCacheEntry).page
	c.mu.Unlock()
	result := cloneParsedPage(page)
	result.Version = versionCounter.Add(1)
	return result, true
}

func (c *ParseCache) put(key parseCacheKey, page *Page) {
	c.mu.Lock()
	defer c.mu.Unlock()
	size := parsedPageSize(key, page)
	if size > c.maxBytes {
		return
	}
	if e, ok := c.entries[key]; ok {
		c.lru.MoveToFront(e)
		return
	}
	for c.bytes > c.maxBytes-size {
		oldest := c.lru.Back()
		entry := oldest.Value.(parseCacheEntry)
		delete(c.entries, entry.key)
		c.bytes -= entry.size
		c.lru.Remove(oldest)
	}
	if c.entries == nil {
		c.entries = make(map[parseCacheKey]*list.Element)
	}
	stored := cloneParsedPage(page)
	// Paths are bound by the current directory load, never reused from another
	// scan root. The key retains the actual filename, including language suffix.
	stored.FilePath, stored.RelPath, stored.Language = "", "", ""
	for _, field := range parsedPageStrings(stored) {
		*field = strings.Clone(*field)
	}
	for i, tag := range stored.Tags {
		stored.Tags[i] = strings.Clone(tag)
	}
	for i, keyword := range stored.Keywords {
		stored.Keywords[i] = strings.Clone(keyword)
	}
	key.path = strings.Clone(key.path)
	c.entries[key] = c.lru.PushFront(parseCacheEntry{key: key, page: stored, size: size})
	c.bytes += size
}

func parsedPageSize(key parseCacheKey, page *Page) int64 {
	// Page, map/list entries, two keys, slice headers, date locations, and
	// allocation overhead. All retained string backing data is cloned below.
	size := int64(2048 + len(key.path))
	for _, field := range parsedPageStrings(page) {
		size += int64(len(*field))
	}
	for _, tag := range page.Tags {
		size += 32 + int64(len(tag))
	}
	for _, keyword := range page.Keywords {
		size += 32 + int64(len(keyword))
	}
	return size
}

// Keep ownership and size accounting together for every parsed string field.
func parsedPageStrings(p *Page) []*string {
	return []*string{
		&p.Title, &p.Date, &p.PublishDate, &p.ExpiryDate, &p.Lastmod,
		&p.Type, &p.Slug, &p.Description, &p.Author, &p.Image, &p.FeaturedImage,
		&p.RawContent, &p.Build.List, &p.Build.Render,
		&p.Cascade.Build.List, &p.Cascade.Build.Render,
		&p.Cascade.Sitemap.ChangeFreq, &p.Sitemap.ChangeFreq,
	}
}

// cloneParsedPage is only used on freshly parsed or cache-owned input pages,
// before rendering/tree construction. Build/Cascade/Sitemap contain values.
func cloneParsedPage(page *Page) *Page {
	result := *page
	result.Tags = slices.Clone(page.Tags)
	result.Keywords = slices.Clone(page.Keywords)
	result.Parent = nil
	result.Pages, result.RegularPages, result.RegularPagesRecursive, result.Sections = nil, nil, nil, nil
	return &result
}
