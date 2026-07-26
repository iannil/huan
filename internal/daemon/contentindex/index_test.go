package contentindex

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const baseURL = "https://example.com/"

func TestLoadFromDir_LoadsAllSections(t *testing.T) {
	dir := t.TempDir()
	writeAPITestFile(t, dir, "posts.json", []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01", "tags": []string{"go"}},
	})
	writeAPITestFile(t, dir, "books.json", []map[string]any{
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
	writeAPITestFile(t, dir, "posts.json", []map[string]any{
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
	writeAPITestFile(t, dir, "posts.json", []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01"},
	})
	// Bad file
	if err := os.WriteFile(filepath.Join(dir, "api", "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

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

// writeAPITestFile writes a section JSON array to <dir>/api/<name>.
func writeAPITestFile(t *testing.T, dir, name string, items []map[string]any) {
	t.Helper()
	apiDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := mustJSON(items)
	if err := os.WriteFile(filepath.Join(apiDir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

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

// TestQuery_HugePageNoPanic guards against integer overflow on the public
// endpoint. A page of math.MaxInt64 used to compute a negative start index
// that bypassed the bounds guard and panicked on matched[start:end].
func TestQuery_HugePageNoPanic(t *testing.T) {
	ci := loadTestIndex(t)

	// Must not panic and must return empty data.
	r := ci.Query(Filter{Page: math.MaxInt64, Limit: 10})
	if len(r.Data) != 0 {
		t.Errorf("expected empty data for huge page, got %d items", len(r.Data))
	}
	if r.Page != 100000 {
		t.Errorf("Page = %d, want 100000 (capped)", r.Page)
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

func TestEmptyIndex(t *testing.T) {
	ci := NewContentIndex(baseURL)
	if ci.Len() != 0 {
		t.Errorf("Len = %d, want 0", ci.Len())
	}

	_, ok := ci.GetByURL("/nonexistent/")
	if ok {
		t.Error("expected false for GetByURL on empty index")
	}

	r := ci.Query(Filter{})
	if r.Total != 0 {
		t.Errorf("Total = %d, want 0", r.Total)
	}
	if len(r.Data) != 0 {
		t.Errorf("Data len = %d, want 0", len(r.Data))
	}

	tags := ci.Tags()
	if len(tags) != 0 {
		t.Errorf("Tags = %v, want empty", tags)
	}
	secs := ci.Sections()
	if len(secs) != 0 {
		t.Errorf("Sections = %v, want empty", secs)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	ci := loadTestIndex(t)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ci.Query(Filter{})
			_ = ci.Tags()
			_ = ci.Sections()
			_, _ = ci.GetByURL("/posts/go/")
		}()
	}
	wg.Wait()
}
