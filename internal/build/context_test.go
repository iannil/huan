package build

import (
	"strings"
	"testing"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)

// TestBuildTaxonomyContext_RegularPagesAreTermPages verifies that the taxonomy
// listing context (/tags/) exposes term stub pages in RegularPages, matching
// Hugo's behavior. Hugo's taxonomy-list RSS iterates .Pages which for a
// taxonomy list page is the set of term pages (one per term), NOT the site's
// regular content pages.
func TestBuildTaxonomyContext_RegularPagesAreTermPages(t *testing.T) {
	earlier := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 27, 17, 7, 34, 0, time.UTC)

	// alpha tagged page is older; beta tagged page is newer
	alphaPage := &content.Page{Title: "alpha-post", RelPath: "posts/a.md", Kind: "page", DateParsed: earlier, LastmodParsed: earlier}
	betaPage := &content.Page{Title: "beta-post", RelPath: "posts/b.md", Kind: "page", DateParsed: later, LastmodParsed: later}

	alphaCtx := &tmpl.Context{Title: "alpha-post", RelPermalink: "/posts/a/", Permalink: "https://x/posts/a/", Date: earlier, Lastmod: earlier}
	betaCtx := &tmpl.Context{Title: "beta-post", RelPermalink: "/posts/b/", Permalink: "https://x/posts/b/", Date: later, Lastmod: later}

	lookup := map[*content.Page]*tmpl.Context{alphaPage: alphaCtx, betaPage: betaCtx}

	site := &content.Site{
		Pages:     []*content.Page{alphaPage, betaPage},
		Taxonomies: map[string]content.Taxonomy{
			"tags": {
				"alpha": {alphaPage},
				"beta":  {betaPage},
			},
		},
	}

	siteCtx := &tmpl.SiteContext{
		Title:        "TestSite",
		BaseURL:      "https://x/",
		Taxonomies:   map[string]tmpl.TaxonomyContext{},
		RegularPages: tmpl.PageSlice{alphaCtx, betaCtx},
	}

	cfg := &config.Config{}

	ctx := BuildTaxonomyContext(siteCtx, lookup, site, cfg)
	if ctx == nil {
		t.Fatal("expected non-nil taxonomy context")
	}

	// Each item in RegularPages should be a term-stub (Kind == "term")
	if got, want := len(ctx.RegularPages), 2; got != want {
		t.Fatalf("expected %d term-stub pages in RegularPages, got %d", want, got)
	}

	seen := map[string]bool{}
	for _, item := range ctx.RegularPages {
		c := tmpl.AsCtx(item)
		if c == nil {
			t.Errorf("nil context in RegularPages")
			continue
		}
		if c.Kind != "term" {
			t.Errorf("expected Kind=term for term stub, got %q (title=%q)", c.Kind, c.Title)
		}
		seen[c.Title] = true
		// Each term stub should have a non-empty permalink to /tags/{name}/
		if c.Permalink == "" {
			t.Errorf("term stub %q has empty Permalink", c.Title)
		}
	}
	if !seen["alpha"] || !seen["beta"] {
		t.Errorf("expected term stubs for both alpha and beta; seen=%v", seen)
	}
}

// TestBuildTaxonomyContext_TermStubsSortedByDateDesc verifies that term stubs
// in the taxonomy RSS are sorted by their effective date (newest first),
// matching Hugo's RSS output for /tags/index.xml.
func TestBuildTaxonomyContext_TermStubsSortedByDateDesc(t *testing.T) {
	earlier := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 27, 17, 7, 34, 0, time.UTC)

	alphaPage := &content.Page{Title: "alpha-post", RelPath: "posts/a.md", Kind: "page", DateParsed: earlier, LastmodParsed: earlier}
	betaPage := &content.Page{Title: "beta-post", RelPath: "posts/b.md", Kind: "page", DateParsed: later, LastmodParsed: later}

	alphaCtx := &tmpl.Context{Title: "alpha-post", RelPermalink: "/posts/a/", Permalink: "https://x/posts/a/", Date: earlier, Lastmod: earlier}
	betaCtx := &tmpl.Context{Title: "beta-post", RelPermalink: "/posts/b/", Permalink: "https://x/posts/b/", Date: later, Lastmod: later}

	lookup := map[*content.Page]*tmpl.Context{alphaPage: alphaCtx, betaPage: betaCtx}

	site := &content.Site{
		Taxonomies: map[string]content.Taxonomy{
			"tags": {
				"alpha": {alphaPage},
				"beta":  {betaPage},
			},
		},
	}

	siteCtx := &tmpl.SiteContext{
		Title:        "TestSite",
		BaseURL:      "https://x/",
		RegularPages: tmpl.PageSlice{alphaCtx, betaCtx},
	}

	ctx := BuildTaxonomyContext(siteCtx, lookup, site, &config.Config{})
	if ctx == nil {
		t.Fatal("expected non-nil taxonomy context")
	}

	// beta (newer) should come before alpha (older)
	if got := tmpl.AsCtx(ctx.RegularPages[0]); got.Title != "beta" {
		t.Errorf("expected beta first (newer date); got %q", got.Title)
	}
	if got := tmpl.AsCtx(ctx.RegularPages[1]); got.Title != "alpha" {
		t.Errorf("expected alpha second (older date); got %q", got.Title)
	}
}

// TestBuildTaxonomyContext_TaxonomyListPermalink verifies that the taxonomy
// list page (/tags/) has a non-empty Permalink and RelPermalink so the RSS
// channel <link> is populated. Also confirms no double-slash in the URL
// (BaseURL already ends with "/").
func TestBuildTaxonomyContext_TaxonomyListPermalink(t *testing.T) {
	now := time.Now()
	page := &content.Page{Title: "p", RelPath: "posts/a.md", Kind: "page", DateParsed: now, LastmodParsed: now}
	pageCtx := &tmpl.Context{Title: "p", Date: now, Lastmod: now}
	lookup := map[*content.Page]*tmpl.Context{page: pageCtx}
	site := &content.Site{
		Taxonomies: map[string]content.Taxonomy{
			"tags": {"alpha": {page}},
		},
	}
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/", RegularPages: tmpl.PageSlice{pageCtx}}

	ctx := BuildTaxonomyContext(siteCtx, lookup, site, &config.Config{})
	if ctx == nil {
		t.Fatal("expected non-nil taxonomy context")
	}
	if ctx.Permalink != "https://x/tags/" {
		t.Errorf("Permalink: got %q, want %q", ctx.Permalink, "https://x/tags/")
	}
	if ctx.RelPermalink != "/tags/" {
		t.Errorf("RelPermalink: got %q, want %q", ctx.RelPermalink, "/tags/")
	}
	if strings.Contains(ctx.Permalink, "//tags") {
		t.Errorf("Permalink has double slash: %q", ctx.Permalink)
	}
}

// TestBuildTaxonomyContext_TermStubURLPercentEncoded verifies that CJK tag
// names are percent-encoded in the stub Permalink, matching Hugo's behavior.
// Hugo's taxonomy-list RSS emits /tags/%E5%85%B1%E8%AF%86/ not /tags/共识/.
func TestBuildTaxonomyContext_TermStubURLPercentEncoded(t *testing.T) {
	now := time.Now()
	page := &content.Page{Title: "p", RelPath: "posts/a.md", Kind: "page", DateParsed: now, LastmodParsed: now}
	pageCtx := &tmpl.Context{Title: "p", Date: now, Lastmod: now}
	lookup := map[*content.Page]*tmpl.Context{page: pageCtx}
	site := &content.Site{
		Taxonomies: map[string]content.Taxonomy{
			"tags": {"共识": {page}},
		},
	}
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/", RegularPages: tmpl.PageSlice{pageCtx}}

	ctx := BuildTaxonomyContext(siteCtx, lookup, site, &config.Config{})
	if ctx == nil {
		t.Fatal("expected non-nil taxonomy context")
	}
	stub := tmpl.AsCtx(ctx.RegularPages[0])
	wantPerm := "https://x/tags/%E5%85%B1%E8%AF%86/"
	if stub.Permalink != wantPerm {
		t.Errorf("CJK stub Permalink: got %q, want %q", stub.Permalink, wantPerm)
	}
	if stub.RelPermalink != "/tags/%E5%85%B1%E8%AF%86/" {
		t.Errorf("CJK stub RelPermalink: got %q", stub.RelPermalink)
	}
}
