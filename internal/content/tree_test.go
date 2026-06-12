package content

import (
	"testing"
	"time"
)

func TestSortPagesByDateDesc_TiebreakerByLowerTitle(t *testing.T) {
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		{Title: "Zebra", RelPath: "/z.md", DateParsed: d},
		{Title: "apple", RelPath: "/a.md", DateParsed: d},
		{Title: "Mango", RelPath: "/m.md", DateParsed: d},
	}
	sortPagesByDateDesc(pages)

	gotTitles := []string{pages[0].Title, pages[1].Title, pages[2].Title}
	wantTitles := []string{"apple", "Mango", "Zebra"} // lower(title) asc
	for i, g := range gotTitles {
		if g != wantTitles[i] {
			t.Errorf("pos %d: got %q want %q (full order: %v)", i, g, wantTitles[i], gotTitles)
		}
	}
}

func TestSortPagesByDateDesc_TiebreakerByRelPathWhenTitleEqual(t *testing.T) {
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		{Title: "Same", RelPath: "/b.md", DateParsed: d},
		{Title: "Same", RelPath: "/a.md", DateParsed: d},
		{Title: "Same", RelPath: "/c.md", DateParsed: d},
	}
	sortPagesByDateDesc(pages)

	gotPaths := []string{pages[0].RelPath, pages[1].RelPath, pages[2].RelPath}
	wantPaths := []string{"/a.md", "/b.md", "/c.md"} // relpath asc
	for i, g := range gotPaths {
		if g != wantPaths[i] {
			t.Errorf("pos %d: got %q want %q", i, g, wantPaths[i])
		}
	}
}

func TestSortPagesByDateDesc_DateTakesPrecedence(t *testing.T) {
	d1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		{Title: "zzz", RelPath: "/old.md", DateParsed: d2}, // older but title later
		{Title: "aaa", RelPath: "/new.md", DateParsed: d1}, // newer
	}
	sortPagesByDateDesc(pages)
	if pages[0].RelPath != "/new.md" {
		t.Errorf("newer must come first; got %v", pages[0].RelPath)
	}
}
