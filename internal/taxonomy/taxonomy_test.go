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
