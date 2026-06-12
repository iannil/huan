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
