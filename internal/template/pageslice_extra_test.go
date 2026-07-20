package template

import (
	"testing"
	"time"
)

func TestPageSlice_ByDate(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	ctx1 := &Context{Date: t1, Kind: "page"}
	ctx2 := &Context{Date: t2, Kind: "page"}
	ps := PageSlice{ctx1, ctx2}
	sorted := ps.ByDate()
	if len(sorted) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sorted))
	}
	// Should be sorted oldest first (ascending)
	if AsCtx(sorted[0]).Date != t1 {
		t.Error("expected oldest first")
	}
}

func TestPageSlice_ByDate_Empty(t *testing.T) {
	ps := PageSlice{}
	sorted := ps.ByDate()
	if len(sorted) != 0 {
		t.Errorf("expected empty slice, got %d", len(sorted))
	}
}

func TestPageSlice_ByLastmod(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	ctx1 := &Context{Lastmod: t1, Kind: "page"}
	ctx2 := &Context{Lastmod: t2, Kind: "page"}
	ps := PageSlice{ctx1, ctx2}
	sorted := ps.ByLastmod()
	if len(sorted) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sorted))
	}
	// Should be sorted oldest first (ascending)
	if AsCtx(sorted[0]).Lastmod != t1 {
		t.Error("expected oldest Lastmod first")
	}
}

func TestPageSlice_ByPublishDate(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	ctx1 := &Context{Date: t1, Kind: "page"}
	ctx2 := &Context{Date: t2, Kind: "page"}
	ps := PageSlice{ctx1, ctx2}
	sorted := ps.ByPublishDate()
	if len(sorted) != 2 {
		t.Fatalf("expected 2 items, got %d", len(sorted))
	}
	// ByPublishDate is an alias for ByDate
	if AsCtx(sorted[0]).Date != t1 {
		t.Error("expected oldest date first")
	}
}

func TestPageSlice_Reverse(t *testing.T) {
	ctx1 := &Context{Kind: "page", Title: "a"}
	ctx2 := &Context{Kind: "page", Title: "b"}
	ctx3 := &Context{Kind: "page", Title: "c"}
	ps := PageSlice{ctx1, ctx2, ctx3}
	reversed := ps.Reverse()
	if len(reversed) != 3 {
		t.Fatalf("expected 3 items, got %d", len(reversed))
	}
	if AsCtx(reversed[0]).Title != "c" {
		t.Errorf("expected 'c' first, got %s", AsCtx(reversed[0]).Title)
	}
	if AsCtx(reversed[2]).Title != "a" {
		t.Errorf("expected 'a' last, got %s", AsCtx(reversed[2]).Title)
	}
}

func TestPageSlice_Reverse_Empty(t *testing.T) {
	ps := PageSlice{}
	reversed := ps.Reverse()
	if len(reversed) != 0 {
		t.Errorf("expected empty slice, got %d", len(reversed))
	}
}

func TestPageSlice_Len(t *testing.T) {
	ps := PageSlice{&Context{}, &Context{}, &Context{}}
	if ps.Len() != 3 {
		t.Errorf("expected len 3, got %d", ps.Len())
	}

	ps2 := PageSlice{}
	if ps2.Len() != 0 {
		t.Errorf("expected len 0, got %d", ps2.Len())
	}
}

func TestPageSlice_First(t *testing.T) {
	ctx1 := &Context{Kind: "page", Title: "first"}
	ctx2 := &Context{Kind: "page", Title: "second"}
	ps := PageSlice{ctx1, ctx2}

	first := ps.First()
	if first == nil {
		t.Fatal("expected non-nil Context")
	}
	if first.Title != "first" {
		t.Errorf("expected 'first', got %s", first.Title)
	}
}

func TestPageSlice_First_Empty(t *testing.T) {
	ps := PageSlice{}
	first := ps.First()
	if first == nil {
		t.Fatal("expected non-nil empty Context (not nil)")
	}
	// Should return zero-valued Context
	if first.Title != "" {
		t.Errorf("expected empty title, got %s", first.Title)
	}
}

func TestPageSlice_Latest(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	ctx1 := &Context{Lastmod: t1, Title: "old"}
	ctx2 := &Context{Lastmod: t2, Title: "new"}
	ps := PageSlice{ctx1, ctx2}

	latest := ps.latest()
	if latest == nil {
		t.Fatal("expected non-nil Context")
	}
	if latest.Title != "new" {
		t.Errorf("expected 'new', got %s", latest.Title)
	}
}

func TestPageSlice_Latest_Empty(t *testing.T) {
	ps := PageSlice{}
	latest := ps.latest()
	if latest != nil {
		t.Errorf("expected nil for empty slice, got %v", latest)
	}
}

func TestAsCtx(t *testing.T) {
	ctx := &Context{Title: "test"}
	result := AsCtx(ctx)
	if result == nil || result.Title != "test" {
		t.Errorf("AsCtx(*Context): got %v, want title='test'", result)
	}

	// Non-Context input
	result2 := AsCtx("not a context")
	if result2 != nil {
		t.Errorf("AsCtx(string): got %v, want nil", result2)
	}
}

func TestPageSlice_WithNilContext(t *testing.T) {
	ps := PageSlice{nil, &Context{Title: "valid"}, nil}
	// Operations should handle nil gracefully
	sorted := ps.ByDate()
	if len(sorted) != 3 {
		t.Errorf("ByDate with nils: got %d items, want 3", len(sorted))
	}
}
