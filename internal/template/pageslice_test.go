package template

import (
	"testing"
	"time"
)

// byteCompare is a fallback CompareFunc that compares strings byte-by-byte.
// Used in tests where we don't need a locale-aware collator.
func byteCompare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// TestPageSlice_SortDefault_DateDesc verifies that SortDefault orders pages
// by Date descending when weights are equal (Hugo's DefaultPageSort layer 2).
func TestPageSlice_SortDefault_DateDesc(t *testing.T) {
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)

	ps := PageSlice{
		&Context{Title: "earlier", Date: earlier, RelPermalink: "/a/"},
		&Context{Title: "later", Date: later, RelPermalink: "/b/"},
	}
	ps.SortDefault(byteCompare)

	got0 := ps[0].(*Context).Title
	if got0 != "later" {
		t.Errorf("SortDefault[0]: got %q, want %q (newer date sorts first)", got0, "later")
	}
}

// TestPageSlice_SortDefault_TitleTiebreak verifies that pages with equal
// weight and date are sorted by Title via the supplied CompareFunc (Hugo's
// DefaultPageSort layer 3).
func TestPageSlice_SortDefault_TitleTiebreak(t *testing.T) {
	d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ps := PageSlice{
		&Context{Title: "banana", Date: d, RelPermalink: "/b/"},
		&Context{Title: "apple", Date: d, RelPermalink: "/a/"},
	}
	ps.SortDefault(byteCompare)

	got0 := ps[0].(*Context).Title
	if got0 != "apple" {
		t.Errorf("SortDefault[0]: got %q, want %q (Title asc tiebreak)", got0, "apple")
	}
}

// TestPageSlice_SortDefault_PathFinalTiebreak verifies that pages with equal
// weight, date, and title fall back to RelPermalink byte comparison (Hugo's
// DefaultPageSort layer 4).
func TestPageSlice_SortDefault_PathFinalTiebreak(t *testing.T) {
	d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ps := PageSlice{
		&Context{Title: "same", Date: d, RelPermalink: "/books/"},
		&Context{Title: "same", Date: d, RelPermalink: "/practices/"},
	}
	ps.SortDefault(byteCompare)

	got0 := ps[0].(*Context).RelPermalink
	if got0 != "/books/" {
		t.Errorf("SortDefault[0]: got %q, want %q (path byte asc)", got0, "/books/")
	}
}

// TestPageSlice_SortDefault_EmptySliceIsNoop verifies that SortDefault on an
// empty slice does not panic and leaves the slice empty.
func TestPageSlice_SortDefault_EmptySliceIsNoop(t *testing.T) {
	ps := PageSlice{}
	ps.SortDefault(byteCompare)
	if len(ps) != 0 {
		t.Errorf("SortDefault on empty: got len %d, want 0", len(ps))
	}
}

// TestPageSlice_GroupByDate_SortsByDateDescWithinGroup verifies that
// pages within each date group are sorted by Date desc (matching Hugo's
// behavior). Hugo's GroupByDate produces groups in reverse chronological
// order, and within each group pages are also ordered by Date desc with
// Path desc as tiebreaker.
func TestPageSlice_GroupByDate_SortsByDateDescWithinGroup(t *testing.T) {
	t1 := mustParseTime(t, "2023-04-14T12:34:42+08:00")
	t2 := mustParseTime(t, "2023-04-14T12:34:42+08:00") // same Date
	t3 := mustParseTime(t, "2023-10-06T19:00:24+08:00")
	site := &SiteContext{LanguageCode: "zh-cn"}
	p1 := &Context{Title: "选择权", File: &FileInfo{Path: "posts/2023/04/1401.md", BaseFileName: "1401"}, Date: t1, RelPermalink: "/posts/2023/04/1401/", Site: site}
	p2 := &Context{Title: "有选择权", File: &FileInfo{Path: "posts/2023/04/1402.md", BaseFileName: "1402"}, Date: t2, RelPermalink: "/posts/2023/04/1402/", Site: site}
	p3 := &Context{Title: "处理观念", File: &FileInfo{Path: "posts/2023/10/0605.md", BaseFileName: "0605"}, Date: t3, RelPermalink: "/posts/2023/10/0605/", Site: site}

	in := PageSlice{p1, p2, p3} // input order: p1, p2, p3
	groups := in.GroupByDate("2006")
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	// Within group: Date desc, tiebreak by RelPermalink desc
	// p3 (10-06) first, then p2 (path /1402/ > /1401/), then p1
	wantOrder := []string{"处理观念", "有选择权", "选择权"}
	if len(g.Pages) != len(wantOrder) {
		t.Fatalf("expected %d pages, got %d", len(wantOrder), len(g.Pages))
	}
	for i, want := range wantOrder {
		c := AsCtx(g.Pages[i])
		if c == nil || c.Title != want {
			t.Errorf("idx %d: got %v, want %s", i, c, want)
		}
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}
