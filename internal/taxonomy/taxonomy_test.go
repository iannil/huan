package taxonomy

import (
	"testing"
	"time"

	"github.com/iannil/huan/internal/content"
)

func TestBuild(t *testing.T) {
	pages := []*content.Page{
		{Title: "A", Tags: []string{"go", "web"}, DateParsed: time.Now()},
		{Title: "B", Tags: []string{"go", "rust"}},
		{Title: "C", Tags: []string{"rust"}},
		{Title: "D", Tags: []string{}},
	}

	tax := Build(pages, "tags")
	if len(tax) != 3 {
		t.Fatalf("expected 3 tags (go, web, rust), got %d", len(tax))
	}
	if len(tax["go"]) != 2 {
		t.Errorf("expected 2 pages tagged 'go', got %d", len(tax["go"]))
	}
	if len(tax["rust"]) != 2 {
		t.Errorf("expected 2 pages tagged 'rust', got %d", len(tax["rust"]))
	}
	if len(tax["web"]) != 1 {
		t.Errorf("expected 1 page tagged 'web', got %d", len(tax["web"]))
	}
}

func TestBuildExcludesDrafts(t *testing.T) {
	pages := []*content.Page{
		{Title: "A", Tags: []string{"x"}, Draft: false},
		{Title: "B", Tags: []string{"x"}, Draft: true},
	}

	tax := Build(pages, "tags")
	if len(tax["x"]) != 1 {
		t.Errorf("expected 1 non-draft page tagged 'x', got %d", len(tax["x"]))
	}
}

func TestByCount(t *testing.T) {
	pages := []*content.Page{
		{Title: "A", Tags: []string{"rare"}},
		{Title: "B", Tags: []string{"common"}},
		{Title: "C", Tags: []string{"common"}},
		{Title: "D", Tags: []string{"common"}},
	}

	tax := Build(pages, "tags")
	entries := tax.ByCount()

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "common" || entries[0].Count != 3 {
		t.Errorf("expected 'common' first with count 3, got %s (%d)", entries[0].Name, entries[0].Count)
	}
}

// TestBuild_PreservesInputPageOrder documents the contract that Build does NOT
// reorder pages within a term — it appends in input order. Callers (e.g.
// build.go) are responsible for passing already-sorted pages (sorted via
// content.sortPagesDefault with the site's collator). This test guards against
// future regressions that might silently reorder taxonomy term members.
func TestBuild_PreservesInputPageOrder(t *testing.T) {
	d := time.Date(2025, 10, 14, 0, 0, 0, 0, time.UTC)
	p1 := &content.Page{Title: "苹果", Tags: []string{"fruit"}, DateParsed: d, RelPath: "/a.md"}
	p2 := &content.Page{Title: "香蕉", Tags: []string{"fruit"}, DateParsed: d, RelPath: "/b.md"}
	p3 := &content.Page{Title: "樱桃", Tags: []string{"fruit"}, DateParsed: d, RelPath: "/c.md"}

	// Pass pages in a deliberately scrambled order.
	tax := Build([]*content.Page{p2, p3, p1}, "tags")
	pages := tax["fruit"]
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
	// Output should preserve input order: p2, p3, p1.
	if pages[0] != p2 || pages[1] != p3 || pages[2] != p1 {
		gotTitles := []string{pages[0].Title, pages[1].Title, pages[2].Title}
		wantTitles := []string{p2.Title, p3.Title, p1.Title}
		t.Errorf("Build did not preserve input order:\n  got:  %v\n  want: %v", gotTitles, wantTitles)
	}
}
