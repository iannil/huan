### Task 3: JITCache（LRU + TTL）

**Files:**
- Create: `internal/daemon/cache/jit.go`
- Create: `internal/daemon/cache/jit_test.go`

**Interfaces:**
- Consumes: 无（纯数据结构）
- Produces: `JITCache` 类型, `JITEntry` 类型, `Get()`, `Set()`, `Remove()`, `Clear()`, `Len()`

- [ ] **Step 1: Write jit.go**

```go
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
```

- [ ] **Step 2: Run `go vet` to verify jit.go compiles**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go vet ./internal/daemon/cache/...
```
Expected: no errors

- [ ] **Step 3: Write jit_test.go**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go test ./internal/daemon/cache/... -v -count=1
```
Expected: 6 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/cache/
git commit -m "feat(daemon): add JITCache with LRU eviction + TTL expiration

- JITCache 实现：LRU 淘汰 + TTL 过期
- Get/Set/Remove/Clear/Len 接口
- 默认 maxSize 1000，默认 TTL 5 分钟
- 6 个测试覆盖：基础 SetGet、Miss、TTL 过期、LRU 淘汰、Clear、更新覆盖

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

