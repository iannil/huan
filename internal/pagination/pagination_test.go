package pagination

import (
	"testing"

	"github.com/iannil/huan/internal/content"
)

func TestPaginate(t *testing.T) {
	pages := make([]*content.Page, 25)
	for i := range pages {
		pages[i] = &content.Page{Title: "p"}
	}

	pagers := Paginate(pages, 10, "/posts/")
	if len(pagers) != 3 {
		t.Fatalf("expected 3 pagers, got %d", len(pagers))
	}

	if len(pagers[0].Pages) != 10 {
		t.Errorf("first pager should have 10 pages, got %d", len(pagers[0].Pages))
	}
	if len(pagers[2].Pages) != 5 {
		t.Errorf("last pager should have 5 pages, got %d", len(pagers[2].Pages))
	}
}

func TestPagerLinks(t *testing.T) {
	pages := make([]*content.Page, 25)
	for i := range pages {
		pages[i] = &content.Page{Title: "p"}
	}

	pagers := Paginate(pages, 10, "/posts/")

	if pagers[0].HasPrev {
		t.Error("first pager should not have prev")
	}
	if !pagers[0].HasNext {
		t.Error("first pager should have next")
	}
	if !pagers[1].HasPrev {
		t.Error("second pager should have prev")
	}
	if pagers[2].HasNext {
		t.Error("last pager should not have next")
	}
	if pagers[1].Prev != pagers[0] {
		t.Error("second pager prev should point to first")
	}
}

func TestPagerURL(t *testing.T) {
	pages := make([]*content.Page, 25)
	for i := range pages {
		pages[i] = &content.Page{Title: "p"}
	}

	pagers := Paginate(pages, 10, "/posts/")
	if pagers[0].URL() != "/posts/" {
		t.Errorf("first pager URL should be /posts/, got %s", pagers[0].URL())
	}
	if pagers[1].URL() != "/posts/page/2/" {
		t.Errorf("second pager URL should be /posts/page/2/, got %s", pagers[1].URL())
	}
}

func TestPaginateEmpty(t *testing.T) {
	pagers := Paginate(nil, 10, "/posts/")
	if len(pagers) != 1 {
		t.Fatalf("expected 1 empty pager, got %d", len(pagers))
	}
	if len(pagers[0].Pages) != 0 {
		t.Errorf("empty pager should have no pages, got %d", len(pagers[0].Pages))
	}
}
