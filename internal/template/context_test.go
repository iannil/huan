package template

import (
	"testing"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// TestLinkPageRelationships_SectionContextRegularPagesFollowsNearestIndexMd
// verifies that a section context's .RegularPages uses Hugo's
// "nearest section ancestor" rule, NOT a recursive collection.
//
// Phase 3b previously wired ctx.RegularPages from p.RegularPagesRecursive
// for every section. That made posts/ work (zhurongshuo posts/ has no
// _index.md anywhere, so tree.go's posts.RegularPages is already recursive)
// but broke practices/ and books/ — which DO have intermediate _index.md.
// With the recursive override, practices/index.xml showed 20 chapter pages
// instead of Hugo's 0.
//
// After the fix, ctx.RegularPages mirrors tree.go's p.RegularPages directly:
//   - posts/ (no intermediate _index.md): every post attaches to posts, so
//     posts.RegularPages contains all of them.
//   - practices/ (has _index.md, intermediate dirs also have _index.md):
//     chapter pages attach to their nearest ancestor _index.md, so
//     practices.RegularPages is empty (or only top-level leaves).
func TestLinkPageRelationships_SectionContextRegularPagesFollowsNearestIndexMd(t *testing.T) {
	now := time.Now()
	// posts/ mirrors zhurongshuo: NO _index.md (auto-created), no nested
	// _index.md. Both posts attach directly to posts.
	p1 := &content.Page{
		Title: "Post 1", RelPath: "posts/2026/05/01.md", Kind: "page",
		Section: "posts", DateParsed: now,
	}
	p2 := &content.Page{
		Title: "Post 2", RelPath: "posts/2026/05/02.md", Kind: "page",
		Section: "posts", DateParsed: now.Add(-time.Hour),
	}
	// practices/ mirrors zhurongshuo: HAS _index.md, intermediate book
	// directories also have _index.md. The chapter attaches to the book
	// section, NOT practices.
	practices := &content.Page{
		Title: "Practices", RelPath: "practices/_index.md", Kind: "section",
		Section: "practices", DateParsed: now,
	}
	book := &content.Page{
		Title: "Book", RelPath: "practices/season-1/some-book/_index.md", Kind: "section",
		Section: "practices", DateParsed: now,
	}
	chapter := &content.Page{
		Title: "Chapter 1", RelPath: "practices/season-1/some-book/part-01/chapter-01.md",
		Kind: "page", Section: "practices", DateParsed: now,
	}
	pages := []*content.Page{p1, p2, practices, book, chapter}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := content.BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}

	// Find section pages in the built tree.
	var postsSection, practicesSection *content.Page
	for _, p := range site.Pages {
		if p.RelPath == "posts/_index.md" {
			postsSection = p
		}
		if p.RelPath == "practices/_index.md" {
			practicesSection = p
		}
	}
	if postsSection == nil {
		t.Fatal("posts section not auto-created")
	}
	if practicesSection == nil {
		t.Fatal("practices section not found")
	}

	// Build site context and per-page lookup (mirrors build.go wiring).
	siteCtx := NewSiteContext(site, cfg)
	lookup := map[*content.Page]*Context{}
	for _, p := range site.Pages {
		lookup[p] = NewContext(p, siteCtx, cfg)
	}
	for _, p := range site.Pages {
		if ctx, ok := lookup[p]; ok {
			LinkPageRelationships(ctx, p, lookup)
		}
	}

	// posts/ has no intermediate _index.md → all posts attach directly.
	postsCtx := lookup[postsSection]
	if got, want := len(postsCtx.RegularPages), 2; got != want {
		t.Errorf("posts context RegularPages len: got %d, want %d (no intermediate _index.md → all attach directly)", got, want)
		for i, v := range postsCtx.RegularPages {
			if c := AsCtx(v); c != nil {
				t.Logf("  [%d] %s", i, c.Title)
			}
		}
	}

	// practices/ has intermediate _index.md → chapter attaches to book section,
	// NOT practices. So practices.RegularPages must be empty (Hugo semantics).
	practicesCtx := lookup[practicesSection]
	if got, want := len(practicesCtx.RegularPages), 0; got != want {
		t.Errorf("practices context RegularPages len: got %d, want %d (chapter belongs to book section, not practices)", got, want)
		for i, v := range practicesCtx.RegularPages {
			if c := AsCtx(v); c != nil {
				t.Logf("  [%d] %s", i, c.Title)
			}
		}
	}

	// practices.RegularPagesRecursive should still include the chapter (the
	// recursive view is unchanged).
	if got, wantAtLeast := len(practicesCtx.RegularPagesRecursive), 1; got < wantAtLeast {
		t.Errorf("practices context RegularPagesRecursive len: got %d, want >= %d (recursive view must still include descendants)", got, wantAtLeast)
	}
}

// TestSiteGetPage_MissingReturnsNonNilZeroStub verifies that GetPage returns a
// non-nil zero-valued Context stub for unmatched refs, matching Hugo's
// observed behavior. Templates that guard with {{ if ne $page nil }} must
// therefore treat missing pages as found, emitting e.g. <a href=""> for a
// stub whose RelPermalink is "". This is what produces Hugo's <a href>Synton DB</a>
// in zhurongshuo's products/index.html for the data-only product (no .md file).
func TestSiteGetPage_MissingReturnsNonNilZeroStub(t *testing.T) {
	site := &SiteContext{
		Pages: PageSlice{
			&Context{Title: "p", RelPermalink: "/posts/foo/"},
		},
	}
	got := site.GetPage("/missing/path")
	if got == nil {
		t.Fatal("GetPage(missing): got nil, want non-nil zero Context stub")
	}
	if got.RelPermalink != "" {
		t.Errorf("stub RelPermalink: got %q, want %q", got.RelPermalink, "")
	}
	if got.Title != "" {
		t.Errorf("stub Title: got %q, want %q", got.Title, "")
	}
}

// TestSiteGetPage_FoundReturnsMatchingPage verifies that GetPage returns the
// matching page (unchanged) when one exists.
func TestSiteGetPage_FoundReturnsMatchingPage(t *testing.T) {
	page := &Context{Title: "Foo", RelPermalink: "/posts/foo/"}
	site := &SiteContext{Pages: PageSlice{page}}
	got := site.GetPage("/posts/foo")
	if got == nil {
		t.Fatal("GetPage(found): got nil, want the matching page")
	}
	if got != page {
		t.Errorf("GetPage(found): got %p, want %p (same page)", got, page)
	}
}

// TestSiteGetPage_EmptyArgsReturnsZeroStub verifies that GetPage with no args
// returns a zero Context stub (not nil) to mirror Hugo's "non-nil for missing"
// behavior.
func TestSiteGetPage_EmptyArgsReturnsZeroStub(t *testing.T) {
	site := &SiteContext{Pages: PageSlice{}}
	got := site.GetPage()
	if got == nil {
		t.Fatal("GetPage(): got nil, want non-nil zero Context stub")
	}
}
