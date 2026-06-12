package template

import (
	"testing"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// TestLinkPageRelationships_SectionContextRegularPagesIsRecursive asserts that
// a section context's .RegularPages is recursive (matches Hugo). zhurongshuo's
// posts/ section is organized by year/month/day subdirs; the posts section's
// direct children are year subsections (not regular pages), so direct
// p.RegularPages is empty. Hugo's .RegularPages in section context collects
// every regular page beneath the section — huan must do the same by wiring
// ctx.RegularPages from p.RegularPagesRecursive for section pages.
func TestLinkPageRelationships_SectionContextRegularPagesIsRecursive(t *testing.T) {
	now := time.Now()
	posts := &content.Page{
		Title: "Posts", RelPath: "posts/_index.md", Kind: "section",
		Section: "posts", DateParsed: now,
	}
	sub := &content.Page{
		Title: "2026", RelPath: "posts/2026/_index.md", Kind: "section",
		Section: "posts", DateParsed: now,
	}
	p1 := &content.Page{
		Title: "Post 1", RelPath: "posts/2026/05/01.md", Kind: "page",
		Section: "posts", DateParsed: now,
	}
	p2 := &content.Page{
		Title: "Post 2", RelPath: "posts/2026/05/02.md", Kind: "page",
		Section: "posts", DateParsed: now.Add(-time.Hour),
	}
	pages := []*content.Page{posts, sub, p1, p2}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := content.BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}

	// Find the posts section in the built tree.
	var postsSection *content.Page
	for _, p := range site.Pages {
		if p.RelPath == "posts/_index.md" {
			postsSection = p
			break
		}
	}
	if postsSection == nil {
		t.Fatal("posts section not found in site.Pages")
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

	ctx := lookup[postsSection]

	// Hugo-aligned: section context's RegularPages should be recursive,
	// containing both nested posts (Post 1 and Post 2), not empty.
	if got := len(ctx.RegularPages); got != 2 {
		t.Errorf("section context RegularPages len: got %d, want 2 (recursive)", got)
		for i, v := range ctx.RegularPages {
			if c := AsCtx(v); c != nil {
				t.Logf("  [%d] %s", i, c.Title)
			}
		}
	}
}
