### Task 1: ContentIndex — 加载与类型

**Files:**
- Create: `internal/daemon/contentindex/index.go` — Item/ContentIndex 类型 + LoadFromDir
- Create: `internal/daemon/contentindex/index_test.go` — 加载测试

**Interfaces:**
- Produces: `Item`, `ContentIndex`, `NewContentIndex(baseURL string) *ContentIndex`, `LoadFromDir(outputDir string) error`, `Len() int`

- [ ] **Step 1: 编写测试**

`internal/daemon/contentindex/index_test.go`：

```go
package contentindex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDir_LoadsAllSections(t *testing.T) {
	dir := t.TempDir()
	writeAPITestFile(t, dir, "posts.json", baseURL, []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01", "tags": []string{"go"}},
	})
	writeAPITestFile(t, dir, "books.json", baseURL, []map[string]any{
		{"title": "B1", "url": baseURL + "books/b1/", "date": "2026-01-02"},
	})

	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if ci.Len() != 2 {
		t.Errorf("Len = %d, want 2", ci.Len())
	}
}

func TestLoadFromDir_RelativeURL(t *testing.T) {
	dir := t.TempDir()
	writeAPITestFile(t, dir, "posts.json", baseURL, []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01"},
	})

	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	item, ok := ci.GetByURL("/posts/p1/")
	if !ok {
		t.Fatal("GetByURL not found")
	}
	if item.URL != "/posts/p1/" {
		t.Errorf("URL = %q, want /posts/p1/ (relative)", item.URL)
	}
	if item.Section != "posts" {
		t.Errorf("Section = %q, want posts", item.Section)
	}
}

func TestLoadFromDir_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	// Good file
	writeAPITestFile(t, dir, "posts.json", baseURL, []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01"},
	})
	// Bad file
	os.WriteFile(filepath.Join(dir, "api", "broken.json"), []byte("{not json"), 0644)

	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir should skip bad files, got: %v", err)
	}
	if ci.Len() != 1 {
		t.Errorf("Len = %d, want 1 (bad file skipped)", ci.Len())
	}
}

func TestLoadFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir empty dir: %v", err)
	}
	if ci.Len() != 0 {
		t.Errorf("Len = %d, want 0", ci.Len())
	}
}

func TestLoadFromDir_NoAPIDir(t *testing.T) {
	// outputDir without api/ subdir should not error.
	dir := t.TempDir()
	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir no api dir: %v", err)
	}
}

const baseURL = "https://example.com/"

// writeAPITestFile writes a section JSON array to <dir>/api/<name>.
func writeAPITestFile(t *testing.T, dir, name, base string, items []map[string]any) {
	t.Helper()
	apiDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatal(err)
	}
	data := mustJSON(items)
	if err := os.WriteFile(filepath.Join(apiDir, name), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(v any) []byte {
	b, err := jsonMarshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
```

Add the jsonMarshal helper usage — actually just use encoding/json directly. Replace `mustJSON` body:

```go
import "encoding/json"

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/contentindex/ -run "TestLoadFromDir" -v
```
Expected: COMPILATION ERROR (no index.go)

- [ ] **Step 3: 实现 index.go**

`internal/daemon/contentindex/index.go`：

```go
// Package contentindex provides an in-memory query index over the
// pre-built /api/{section}.json files. The daemon loads it at startup and
// after each build, then serves read-only content queries via /api/v1/*.
package contentindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Item is a single content entry returned by the query API. The Plain
// (full-text body) field from the source JSON is intentionally dropped —
// the API returns metadata + summary only; full content is served via the
// pre-built page or JIT rendering.
type Item struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`      // relative, e.g. /posts/hello/
	Section     string   `json:"section"`  // derived from source filename
	Date        string   `json:"date"`
	Description string   `json:"description,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// rawItem matches the source JSON (output.ContentItem) shape. Used only for
// decoding; URL is absolute, Section is filled from the filename.
type rawItem struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
}

// ContentIndex is the daemon's in-memory content query index.
// Thread-safe; callers reload via LoadFromDir after a build.
type ContentIndex struct {
	mu      sync.RWMutex
	items   []Item
	baseURL string
}

// NewContentIndex creates an empty index. baseURL is used to convert the
// absolute URLs in the source JSON back to relative paths.
func NewContentIndex(baseURL string) *ContentIndex {
	return &ContentIndex{baseURL: baseURL}
}

// LoadFromDir loads (or reloads) all section JSON files from
// <outputDir>/api/*.json. Malformed files are skipped with a warning.
// Missing api/ directory is not an error (yields an empty index).
func (ci *ContentIndex) LoadFromDir(outputDir string) error {
	apiDir := filepath.Join(outputDir, "api")
	entries, err := os.ReadDir(apiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no api dir → empty index
		}
		return fmt.Errorf("contentindex: read %s: %w", apiDir, err)
	}

	var items []Item
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		section := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.Join(apiDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "huan: contentindex: read %s: %v\n", entry.Name(), err)
			continue
		}
		var raw []rawItem
		if err := json.Unmarshal(data, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "huan: contentindex: parse %s: %v\n", entry.Name(), err)
			continue
		}
		for _, r := range raw {
			items = append(items, Item{
				Title:       r.Title,
				URL:         ci.toRelative(r.URL),
				Section:     section,
				Date:        r.Date,
				Description: r.Description,
				Summary:     r.Summary,
				Tags:        r.Tags,
			})
		}
	}

	ci.mu.Lock()
	ci.items = items
	ci.mu.Unlock()
	return nil
}

// Len returns the number of indexed items.
func (ci *ContentIndex) Len() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return len(ci.items)
}

// GetByURL returns the item with the given relative URL.
func (ci *ContentIndex) GetByURL(url string) (Item, bool) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	for _, it := range ci.items {
		if it.URL == url {
			return it, true
		}
	}
	return Item{}, false
}

// toRelative strips the configured baseURL prefix from an absolute URL,
// ensuring the result starts with "/".
func (ci *ContentIndex) toRelative(absURL string) string {
	rel := strings.TrimPrefix(absURL, ci.baseURL)
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	return rel
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/contentindex/ -run "TestLoadFromDir" -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/contentindex/index.go internal/daemon/contentindex/index_test.go
git commit -m "feat(contentindex): add ContentIndex with LoadFromDir"
```

---

