package build

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"sync"

	"github.com/iannil/huan/internal/config"
)

// MarkdownCache stores immutable rendering results across builds. It is safe to
// share between pipelines; it never retains pages or shortcode state. The zero
// value and nonpositive capacities disable storage. Do not copy after use.
type MarkdownCache struct {
	mu                      sync.Mutex
	maxBytes                int64
	bytes                   int64
	entries                 map[markdownKey]*list.Element
	lru                     list.List
	hits, misses, evictions uint64
	diskHits                uint64
	disk                    *markdownDisk
}

// MarkdownCacheStats is a consistent snapshot. Bytes counts owned string data
// plus a conservative fixed allowance for each entry's key and bookkeeping;
// it is a capacity budget, not a measurement of the Go allocator's heap usage.
type MarkdownCacheStats struct {
	Hits, Misses, Evictions uint64
	DiskHits                uint64
	Entries                 int
	Bytes, MaxBytes         int64
}

type markdownKey struct {
	Raw, Expanded, Markup [sha256.Size]byte
}

type markdownValue struct {
	HTML, Plain, Summary string
	WordCount            int
}

type markdownEntry struct {
	key   markdownKey
	value markdownValue
}

func (v markdownValue) size() int64 {
	// Includes both copies of the key (map + entry), string headers, list
	// pointers, and an allowance for map buckets/allocation bookkeeping.
	return 512 + int64(len(v.HTML)) + int64(len(v.Plain)) + int64(len(v.Summary))
}

// NewMarkdownCache creates a bounded LRU. A nonpositive limit disables storage.
func NewMarkdownCache(maxBytes int64) *MarkdownCache {
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &MarkdownCache{maxBytes: maxBytes}
}

// Clear discards all entries while retaining cumulative hit/miss/eviction counts.
// Call between builds to invalidate results after non-content changes.
func (c *MarkdownCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	c.lru.Init()
	c.bytes = 0
}

// Stats returns the current budget usage and lifetime counters.
func (c *MarkdownCache) Stats() MarkdownCacheStats {
	if c == nil {
		return MarkdownCacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return MarkdownCacheStats{DiskHits: c.diskHits, Hits: c.hits, Misses: c.misses, Evictions: c.evictions, Entries: len(c.entries), Bytes: c.bytes, MaxBytes: c.maxBytes}
}

func (c *MarkdownCache) get(key markdownKey) (markdownValue, bool) {
	if c == nil {
		return markdownValue{}, false
	}
	c.mu.Lock()
	if e := c.entries[key]; e != nil {
		c.hits++
		c.lru.MoveToFront(e)
		value := e.Value.(markdownEntry).value
		c.mu.Unlock()
		return value, true
	}
	c.mu.Unlock()
	if c.disk != nil {
		if value, ok := c.disk.load(key); ok {
			// load decodes each field into an independently owned string.
			c.putMemoryValue(key, value, true)
			c.mu.Lock()
			c.hits++
			c.diskHits++
			c.mu.Unlock()
			return value, true
		}
	}
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
	return markdownValue{}, false
}

func (c *MarkdownCache) put(key markdownKey, value markdownValue) {
	if c == nil {
		return
	}
	if c.disk != nil {
		c.disk.store(key, value)
	}
	c.putMemory(key, value)
}

func (c *MarkdownCache) putMemory(key markdownKey, value markdownValue) {
	c.putMemoryValue(key, value, false)
}

func (c *MarkdownCache) putMemoryValue(key markdownKey, value markdownValue, owned bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	size := value.size()
	if size > c.maxBytes {
		return
	}
	if e := c.entries[key]; e != nil {
		c.lru.MoveToFront(e)
		return
	}
	for c.bytes > c.maxBytes-size {
		e := c.lru.Back()
		entry := e.Value.(markdownEntry)
		delete(c.entries, entry.key)
		c.bytes -= entry.value.size()
		c.lru.Remove(e)
		c.evictions++
	}
	// Summary may be a tiny slice of a very large HTML string. Clone every
	// stored string so the counted lengths bound the backing data we retain.
	if !owned {
		value.HTML = strings.Clone(value.HTML)
		value.Plain = strings.Clone(value.Plain)
		value.Summary = strings.Clone(value.Summary)
	}
	if c.entries == nil {
		c.entries = make(map[markdownKey]*list.Element)
	}
	c.entries[key] = c.lru.PushFront(markdownEntry{key: key, value: value})
	c.bytes += size
}

func markdownConfigDigest(cfg config.MarkupConfig) ([sha256.Size]byte, error) {
	// JSON includes all exported configuration fields and deterministically
	// sorts map keys, so added rendering options automatically invalidate hits.
	data, err := json.Marshal(cfg)
	return sha256.Sum256(data), err
}
