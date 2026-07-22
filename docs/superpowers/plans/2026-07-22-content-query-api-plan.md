# Content Query REST API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 daemon 增加面向终端用户的公开只读内容查询 REST API（`/api/v1/*`），从预构建的 `/api/{section}.json` 加载内存索引，支持 section/tag/全文/分页查询。

**Architecture:** ContentIndex 从预构建 JSON 加载内存索引（构建后刷新），HTTP Handler 暴露 `/api/v1/pages`、`/api/v1/pages/{url}`、`/api/v1/tags`、`/api/v1/sections`。复用已有 `GenerateContentAPI` 数据源，daemon 只做查询层。

**Tech Stack:** Go 1.26.2, net/http, encoding/json, daemon serving/builder

## Global Constraints

- 公开只读，无需 token（区别于 /admin/api/*）
- 数据源是预构建 `/api/{section}.json`（`cfg.AI.ContentAPI` 开启时生成），不重新加载 content 目录
- URL 规范化：预构建 JSON 的绝对 URL → 相对路径（去掉 BaseURL）
- draft/future/expired 已在数据源过滤，ContentIndex 不重复过滤
- limit 上限 50，默认 10；page 默认 1
- 路径前缀 `/api/v1/` 与预构建 `/api/{section}.json` 不冲突
- 所有现有测试必须通过

---

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

### Task 2: ContentIndex — 查询方法

**Files:**
- Modify: `internal/daemon/contentindex/index.go` — 新增 Filter/Result/Query/Tags/Sections
- Modify: `internal/daemon/contentindex/index_test.go` — 查询测试

**Interfaces:**
- Produces: `Filter`, `Result`, `Query(filter Filter) Result`, `Tags() map[string]int`, `Sections() map[string]int`

- [ ] **Step 1: 编写测试**

在 `internal/daemon/contentindex/index_test.go` 末尾添加：

```go
func TestQuery_SectionFilter(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{Section: "posts"})
	for _, it := range r.Data {
		if it.Section != "posts" {
			t.Errorf("section filter leaked %q", it.Section)
		}
	}
	if r.Total == 0 {
		t.Error("expected posts results")
	}
}

func TestQuery_TagFilter(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{Tag: "go"})
	for _, it := range r.Data {
		if !contains(it.Tags, "go") {
			t.Errorf("tag filter leaked item without 'go': %v", it.Tags)
		}
	}
}

func TestQuery_FullTextSearch(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{Query: "GOLANG"})
	if r.Total == 0 {
		t.Error("expected matches for 'GOLANG' (case-insensitive)")
	}
}

func TestQuery_Pagination(t *testing.T) {
	ci := loadTestIndex(t)
	r1 := ci.Query(Filter{Limit: 1, Page: 1})
	r2 := ci.Query(Filter{Limit: 1, Page: 2})
	if len(r1.Data) != 1 || len(r2.Data) != 1 {
		t.Fatalf("page sizes: %d, %d", len(r1.Data), len(r2.Data))
	}
	if r1.Data[0].URL == r2.Data[0].URL {
		t.Error("page 1 and 2 returned same item")
	}
}

func TestQuery_LimitCapped(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{Limit: 999})
	if r.Limit != 50 {
		t.Errorf("Limit = %d, want 50 (capped)", r.Limit)
	}
}

func TestQuery_Defaults(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{})
	if r.Page != 1 {
		t.Errorf("default Page = %d, want 1", r.Page)
	}
	if r.Limit != 10 {
		t.Errorf("default Limit = %d, want 10", r.Limit)
	}
}

func TestQuery_SortByDateDesc(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{Sort: "date"})
	for i := 1; i < len(r.Data); i++ {
		if r.Data[i-1].Date < r.Data[i].Date {
			t.Errorf("not desc: %q before %q", r.Data[i-1].Date, r.Data[i].Date)
		}
	}
}

func TestQuery_NoMatch(t *testing.T) {
	ci := loadTestIndex(t)
	r := ci.Query(Filter{Query: "zzznomatchzzz"})
	if r.Total != 0 || len(r.Data) != 0 {
		t.Errorf("expected empty result, got total=%d", r.Total)
	}
}

func TestGetByURL_NotFound(t *testing.T) {
	ci := loadTestIndex(t)
	if _, ok := ci.GetByURL("/nope/"); ok {
		t.Error("expected not found")
	}
}

func TestTags(t *testing.T) {
	ci := loadTestIndex(t)
	tags := ci.Tags()
	if tags["go"] == 0 {
		t.Errorf("expected 'go' tag count > 0, got %v", tags)
	}
}

func TestSections(t *testing.T) {
	ci := loadTestIndex(t)
	secs := ci.Sections()
	if secs["posts"] == 0 {
		t.Errorf("expected 'posts' section count > 0, got %v", secs)
	}
}

// loadTestIndex builds an in-memory index from hardcoded items for query tests.
func loadTestIndex(t *testing.T) *ContentIndex {
	t.Helper()
	ci := NewContentIndex(baseURL)
	ci.mu.Lock()
	ci.items = []Item{
		{Title: "Go Post", URL: "/posts/go/", Section: "posts", Date: "2026-02-01", Summary: "About GOLANG", Tags: []string{"go"}},
		{Title: "Rust Post", URL: "/posts/rust/", Section: "posts", Date: "2026-01-15", Summary: "About rust", Tags: []string{"rust"}},
		{Title: "Book One", URL: "/books/b1/", Section: "books", Date: "2026-01-01", Summary: "A book", Tags: []string{"go"}},
	}
	ci.mu.Unlock()
	return ci
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/contentindex/ -run "TestQuery|TestTags|TestSections|TestGetByURL_NotFound" -v
```
Expected: COMPILATION ERROR (no Filter/Result/Query)

- [ ] **Step 3: 实现查询方法**

在 `internal/daemon/contentindex/index.go` 末尾添加：

```go
// Filter controls a Query invocation.
type Filter struct {
	Section string // section filter
	Tag     string // tag filter
	Query   string // full-text (Title/Summary/Description, case-insensitive)
	Page    int    // 1-based page
	Limit   int    // page size, default 10, capped at 50
	Sort    string // "date" (default, desc); other values fall back to date desc
}

// Result is a paginated query response.
type Result struct {
	Data  []Item `json:"data"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

// Query returns a paginated, filtered, sorted slice of items.
func (ci *ContentIndex) Query(f Filter) Result {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 10
	}
	if f.Limit > 50 {
		f.Limit = 50
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	// Filter
	var matched []Item
	q := strings.ToLower(f.Query)
	for _, it := range ci.items {
		if f.Section != "" && it.Section != f.Section {
			continue
		}
		if f.Tag != "" && !containsString(it.Tags, f.Tag) {
			continue
		}
		if q != "" && !containsLower(it.Title, q) && !containsLower(it.Summary, q) && !containsLower(it.Description, q) {
			continue
		}
		matched = append(matched, it)
	}

	// Sort by date desc (stable)
	sortItemsByDateDesc(matched)

	total := len(matched)
	start := (f.Page - 1) * f.Limit
	end := start + f.Limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	var data []Item
	if start < end {
		data = matched[start:end]
	}
	if data == nil {
		data = []Item{}
	}

	return Result{Data: data, Total: total, Page: f.Page, Limit: f.Limit}
}

// Tags returns a map of tag → page count.
func (ci *ContentIndex) Tags() map[string]int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	out := map[string]int{}
	for _, it := range ci.items {
		for _, tag := range it.Tags {
			out[tag]++
		}
	}
	return out
}

// Sections returns a map of section → page count.
func (ci *ContentIndex) Sections() map[string]int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	out := map[string]int{}
	for _, it := range ci.items {
		out[it.Section]++
	}
	return out
}

// --- helpers ---

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func containsLower(s, lower string) bool {
	return strings.Contains(strings.ToLower(s), lower)
}

func sortItemsByDateDesc(items []Item) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j-1].Date < items[j].Date; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/contentindex/ -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/contentindex/index.go internal/daemon/contentindex/index_test.go
git commit -m "feat(contentindex): add Query/Tags/Sections with filtering and pagination"
```

---

### Task 3: HTTP Handler

**Files:**
- Create: `internal/daemon/contentindex/handler.go` — /api/v1/* 路由
- Create: `internal/daemon/contentindex/handler_test.go` — Handler 测试

**Interfaces:**
- Consumes: ContentIndex (Task 1-2)
- Produces: `Handler`, `NewHandler(index *ContentIndex) *Handler`, `ServeHTTP`

- [ ] **Step 1: 编写测试**

`internal/daemon/contentindex/handler_test.go`：

```go
package contentindex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_PagesList(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var r Result
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Total == 0 {
		t.Error("expected non-empty list")
	}
	if r.Limit != 10 {
		t.Errorf("default limit = %d, want 10", r.Limit)
	}
}

func TestHandler_PagesFilter(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages?section=books", "")
	var r Result
	json.Unmarshal(rec.Body.Bytes(), &r)
	for _, it := range r.Data {
		if it.Section != "books" {
			t.Errorf("filter leaked section %q", it.Section)
		}
	}
}

func TestHandler_PageDetail(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages/posts/go/", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var it Item
	json.Unmarshal(rec.Body.Bytes(), &it)
	if it.URL != "/posts/go/" {
		t.Errorf("URL = %q", it.URL)
	}
}

func TestHandler_PageDetail404(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages/nope/", "")
	if rec.Code != 404 {
		t.Errorf("code = %d, want 404", rec.Code)
	}
}

func TestHandler_Tags(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/tags", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var m map[string]int
	json.Unmarshal(rec.Body.Bytes(), &m)
	if m["go"] == 0 {
		t.Errorf("expected go tag, got %v", m)
	}
}

func TestHandler_Sections(t *testing.T) {
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/sections", "")
	var m map[string]int
	json.Unmarshal(rec.Body.Bytes(), &m)
	if m["posts"] == 0 {
		t.Errorf("expected posts section, got %v", m)
	}
}

func TestHandler_NoAuthRequired(t *testing.T) {
	// Public endpoint: no Authorization header, no token → still 200.
	h := NewHandler(loadTestIndex(t))
	rec := serveHandler(t, h, "GET", "/api/v1/pages", "")
	if rec.Code != 200 {
		t.Errorf("public endpoint returned %d", rec.Code)
	}
}

func TestHandler_IndexNotReady(t *testing.T) {
	// Empty index (Len 0) should still serve 200 with empty data, not 503.
	h := NewHandler(NewContentIndex(baseURL))
	rec := serveHandler(t, h, "GET", "/api/v1/pages", "")
	if rec.Code != 200 {
		t.Errorf("empty index code = %d, want 200", rec.Code)
	}
}

func serveHandler(t *testing.T, h *Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/contentindex/ -run "TestHandler" -v
```
Expected: COMPILATION ERROR (no Handler)

- [ ] **Step 3: 实现 handler.go**

`internal/daemon/contentindex/handler.go`：

```go
package contentindex

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Handler exposes the read-only content query API at /api/v1/*.
// No authentication — these endpoints are public.
type Handler struct {
	index *ContentIndex
}

// NewHandler creates a Handler backed by the given ContentIndex.
func NewHandler(index *ContentIndex) *Handler {
	return &Handler{index: index}
}

// ServeHTTP routes /api/v1/pages, /api/v1/pages/{url}, /api/v1/tags,
// /api/v1/sections.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody("method not allowed"))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "pages":
		h.handlePagesList(w, r)
	case strings.HasPrefix(path, "pages/"):
		h.handlePageDetail(w, strings.TrimPrefix(path, "pages/"))
	case path == "tags":
		h.handleTags(w, r)
	case path == "sections":
		h.handleSections(w, r)
	default:
		writeJSON(w, http.StatusNotFound, errBody("not found"))
	}
}

func (h *Handler) handlePagesList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{
		Section: q.Get("section"),
		Tag:     q.Get("tag"),
		Query:   q.Get("q"),
		Sort:    q.Get("sort"),
	}
	if v, err := strconv.Atoi(q.Get("page")); err == nil {
		f.Page = v
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = v
	}
	res := h.index.Query(f)
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handlePageDetail(w http.ResponseWriter, rest string) {
	// rest is like "posts/go/" → normalize to "/posts/go/"
	url := "/" + strings.TrimPrefix(rest, "/")
	item, ok := h.index.GetByURL(url)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("not found"))
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleTags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.index.Tags())
}

func (h *Handler) handleSections(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.index.Sections())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/contentindex/ -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/contentindex/handler.go internal/daemon/contentindex/handler_test.go
git commit -m "feat(contentindex): add HTTP handler for /api/v1/* query endpoints"
```

---

### Task 4: daemon 集成（serving + builder + daemon.go）

**Files:**
- Modify: `internal/daemon/serving.go` — 注册 /api/v1/ handler，ServingOptions 新增 ContentAPI
- Modify: `internal/daemon/builder.go` — ContentIndex 刷新钩子，BuilderOptions 新增 ContentIndex
- Modify: `internal/daemon/daemon.go` — 创建 ContentIndex + Handler，注入

**Interfaces:**
- Consumes: ContentIndex, Handler (Task 1-3)
- Produces: ServingOptions.ContentAPI, BuilderOptions.ContentIndex, daemon wiring

- [ ] **Step 1: ServingOptions 新增 ContentAPI 字段**

在 `internal/daemon/serving.go` 的 `ServingOptions` 结构体中添加字段：

```go
	ContentAPI    http.Handler              // optional /api/v1/* content query handler
```

- [ ] **Step 2: serving.go Start() 注册 /api/v1/**

在 `internal/daemon/serving.go` 的 `Start()` 方法中，注册 admin handler 之前（或 health 之后），添加：

```go
	// Content query API (public, read-only) — /api/v1/*
	if s.opts.ContentAPI != nil {
		mux.Handle("/api/v1/", s.opts.ContentAPI)
	}
```

放置位置：在 metrics handler 之后、admin handler 之前（确保 `/api/v1/` 精确匹配优先于 `/` catch-all）。

- [ ] **Step 3: BuilderOptions 新增 ContentIndex 字段**

在 `internal/daemon/builder.go` 的 `BuilderOptions` 结构体中添加：

```go
	// ContentIndex is reloaded from /api/*.json after each build so the
	// query API serves fresh data.
	ContentIndex *contentindex.ContentIndex
```

并在 builder.go 顶部添加 import（若不存在）：

```go
	"github.com/iannil/huan/internal/daemon/contentindex"
```

- [ ] **Step 4: builder.go 添加 ContentIndex 刷新钩子**

在 `executeFullBuild` 成功日志后（JITCache.Clear 附近）添加：

```go
	if b.opts.ContentIndex != nil {
		if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
			b.opts.Logf("builder: content index reload: %v", err)
		}
	}
```

在 `IncrementalBuild` 的 JITCache.Remove 循环之后添加同样代码：

```go
	if b.opts.ContentIndex != nil {
		if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
			b.opts.Logf("builder: content index reload: %v", err)
		}
	}
```

- [ ] **Step 5: daemon.go 创建并注入 ContentIndex + Handler**

在 `internal/daemon/daemon.go` 的 `Run()` 中，创建 Builder 之前，添加：

```go
	// 7.5 Init ContentIndex (for /api/v1/* query API)
	var contentAPI http.Handler
	var contentIdx *contentindex.ContentIndex
	if cfg.AI.ContentAPI {
		contentIdx = contentindex.NewContentIndex(cfg.BaseURL)
		if err := contentIdx.LoadFromDir(tmpDir); err != nil {
			log.Printf("daemon: content index load: %v", err)
		}
		contentAPI = contentindex.NewHandler(contentIdx)
		log.Println("daemon: content query API enabled (/api/v1/*)")
	} else {
		log.Println("daemon: content query API disabled (ai.contentAPI not set)")
	}
```

在 Builder 创建时注入 `ContentIndex: contentIdx`。

在 Serving 创建时注入 `ContentAPI: contentAPI`。

在 daemon.go 顶部添加 import：

```go
	"github.com/iannil/huan/internal/daemon/contentindex"
```

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 7: 运行 daemon 测试确保无回归**

```bash
go test ./internal/daemon/... -v
```
Expected: ALL PASS

- [ ] **Step 8: 提交**

```bash
git add internal/daemon/serving.go internal/daemon/builder.go internal/daemon/daemon.go
git commit -m "feat(daemon): wire ContentIndex and /api/v1/ query API into daemon"
```

---

### Task 5: 集成测试 + 全量验证

**Files:**
- Modify: `internal/daemon/daemon_test.go` — ContentAPI 集成测试
- Modify: `docs/superpowers/specs/2026-07-22-content-query-api-design.md` — 标记实现状态

- [ ] **Step 1: 编写集成测试**

在 `internal/daemon/daemon_test.go` 末尾添加：

```go
// TestDaemon_ContentAPI_AfterBuild verifies the query API returns content
// after a full build (which generates /api/{section}.json).
func TestDaemon_ContentAPI_AfterBuild(t *testing.T) {
	tmpDir := setupContentAPISite(t)
	cache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	contentIdx := contentindex.NewContentIndex("https://example.com/")
	handler := contentindex.NewHandler(contentIdx)
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dag.NewDependencyGraph(),
		JITCache:      cache_pkg.NewJITCache(100, 5*time.Minute),
		Logf:          t.Logf,
		PipelineCache: cache,
		ContentIndex:  contentIdx,
	})

	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Query via the handler.
	rec := httptestNewRequest(t, handler, "GET", "/api/v1/pages?section=posts")
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var r contentindex.Result
	jsonNewDecoder(rec.Body).Decode(&r)
	if r.Total == 0 {
		t.Error("expected posts in API after build")
	}
}

// TestDaemon_ContentAPI_RefreshOnBuild verifies the index reloads after a
// content change + rebuild.
func TestDaemon_ContentAPI_RefreshOnBuild(t *testing.T) {
	tmpDir := setupContentAPISite(t)
	contentIdx := contentindex.NewContentIndex("https://example.com/")
	handler := contentindex.NewHandler(contentIdx)
	builder := NewBuilder(BuilderOptions{
		SourceDir:    tmpDir,
		OutputDir:    tmpDir,
		Bus:          eventbus.NewChannelBus(),
		DAG:          dag.NewDependencyGraph(),
		JITCache:     cache_pkg.NewJITCache(100, 5*time.Minute),
		Logf:         t.Logf,
		ContentIndex: contentIdx,
	})
	builder.FullBuild(context.Background())

	// Initial count.
	rec1 := httptestNewRequest(t, handler, "GET", "/api/v1/pages", "")
	var r1 contentindex.Result
	jsonNewDecoder(rec1.Body).Decode(&r1)
	initial := r1.Total

	// Add a new page + rebuild.
	mustWriteIncFile(t, tmpDir, "content/posts/new.md", []byte("---\ntitle: \"New\"\ndate: \"2026-03-01\"\n---\nNew body.\n"))
	builder.FullBuild(context.Background())

	rec2 := httptestNewRequest(t, handler, "GET", "/api/v1/pages", "")
	var r2 contentindex.Result
	jsonNewDecoder(rec2.Body).Decode(&r2)
	if r2.Total <= initial {
		t.Errorf("total not refreshed: %d → %d", initial, r2.Total)
	}
}

// TestDaemon_ContentAPI_ExcludesDraft verifies drafts don't appear.
func TestDaemon_ContentAPI_ExcludesDraft(t *testing.T) {
	tmpDir := setupContentAPISite(t)
	contentIdx := contentindex.NewContentIndex("https://example.com/")
	builder := NewBuilder(BuilderOptions{
		SourceDir:    tmpDir,
		OutputDir:    tmpDir,
		Bus:          eventbus.NewChannelBus(),
		DAG:          dag.NewDependencyGraph(),
		JITCache:     cache_pkg.NewJITCache(100, 5*time.Minute),
		Logf:         t.Logf,
		ContentIndex: contentIdx,
	})
	// BuildDrafts=false → draft excluded from /api/*.json
	builder.FullBuild(context.Background())

	// The setupContentAPISite includes a draft page "secret"; it should NOT appear.
	handler := contentindex.NewHandler(contentIdx)
	rec := httptestNewRequest(t, handler, "GET", "/api/v1/pages?q=secret", "")
	var r contentindex.Result
	jsonNewDecoder(rec.Body).Decode(&r)
	if r.Total != 0 {
		t.Errorf("draft leaked into API: total=%d", r.Total)
	}
}

func setupContentAPISite(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	// ai.contentAPI must be true for GenerateContentAPI to emit /api/*.json
	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte("baseURL: \"https://example.com/\"\ntitle: \"API\"\npublishDir: \"docs\"\nai:\n  contentAPI: true\n"))
	mustWriteIncFile(t, tmpDir, "content/posts/p1.md", []byte("---\ntitle: \"Post One\"\ndate: \"2026-01-01\"\n---\nHello world.\n"))
	mustWriteIncFile(t, tmpDir, "content/posts/secret.md", []byte("---\ntitle: \"secret\"\ndate: \"2026-01-02\"\ndraft: true\n---\nDraft secret.\n"))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html", []byte("<html><body>{{ .Content }}</body></html>"))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html", []byte("<html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>"))
	return tmpDir
}
```

helper imports needed in daemon_test.go（若不存在则添加）：

```go
import (
	// ... existing
	"encoding/json"
	"net/http/httptest"

	"github.com/iannil/huan/internal/daemon/contentindex"
)

// httptestNewRequest issues a request to a handler and returns the recorder.
func httptestNewRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// jsonNewDecoder is json.NewDecoder (aliased to avoid import clashes if any).
var jsonNewDecoder = json.NewDecoder
```

Note: daemon_test.go likely already imports `cache` (github.com/iannil/huan/internal/daemon/cache). Use `cache.NewJITCache` not `cache_pkg`. Replace `cache_pkg.NewJITCache` with `cache.NewJITCache` in the tests above. The `contentindex` import is new — add it.

- [ ] **Step 2: 运行集成测试**

```bash
go test ./internal/daemon/ -run "TestDaemon_ContentAPI" -v
```
Expected: ALL PASS

- [ ] **Step 3: 全量编译 + vet**

```bash
go build ./... && go vet ./...
```
Expected: SUCCESS

- [ ] **Step 4: 全量测试**

```bash
go test ./... -count=1
```
Expected: ALL PASS

- [ ] **Step 5: 更新设计文档状态**

修改 `docs/superpowers/specs/2026-07-22-content-query-api-design.md`：`- **状态**：Draft` → `- **状态**：Implemented`

- [ ] **Step 6: 最终提交**

```bash
git add -A
git commit -m "feat: implement content query REST API (/api/v1/*)

- ContentIndex loads /api/{section}.json into memory, rebuilds on build
- Query supports section/tag/full-text/pagination/sort
- HTTP handler exposes /api/v1/pages, /pages/{url}, /tags, /sections
- Public read-only (no token), drafts excluded by data source
- daemon wires ContentIndex + handler, refreshes after full/incremental build

Co-Authored-By: Claude <noreply@anthropic.com>"
```