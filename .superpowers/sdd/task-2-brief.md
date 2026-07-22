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

