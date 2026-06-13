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

// TestURLEscapeForURL_PercentEncodesCJK verifies that URLEscapeForURL
// percent-encodes CJK characters, matching Hugo's permalink behavior for
// URLs in HTML/XML output (RSS <link>, <guid>, <atom:link>).
// Filesystem paths use URLEscape (CJK preserved); URLs use URLEscapeForURL.
func TestURLEscapeForURL_PercentEncodesCJK(t *testing.T) {
	got := URLEscapeForURL("专注")
	want := "%E4%B8%93%E6%B3%A8"
	if got != want {
		t.Errorf("URLEscapeForURL(%q) = %q, want %q", "专注", got, want)
	}
}

// TestURLEscapeForURL_PreservesASCII verifies ASCII passes through unchanged.
func TestURLEscapeForURL_PreservesASCII(t *testing.T) {
	got := URLEscapeForURL("apple")
	want := "apple"
	if got != want {
		t.Errorf("URLEscapeForURL(%q) = %q, want %q", "apple", got, want)
	}
}

// TestURLEscapeForURL_SpaceBecomesHyphen verifies spaces become hyphens,
// mirroring URLEscape (path version) for the ASCII parts.
func TestURLEscapeForURL_SpaceBecomesHyphen(t *testing.T) {
	got := URLEscapeForURL("hello world")
	want := "hello-world"
	if got != want {
		t.Errorf("URLEscapeForURL(%q) = %q, want %q", "hello world", got, want)
	}
}

// TestURLEscapeForURL_Mixed verifies a mixed ASCII + CJK term encodes the
// CJK portion while preserving ASCII.
func TestURLEscapeForURL_Mixed(t *testing.T) {
	// "go语言" → "go" + percent-encoded "语言"
	got := URLEscapeForURL("go语言")
	want := "go%E8%AF%AD%E8%A8%80"
	if got != want {
		t.Errorf("URLEscapeForURL(%q) = %q, want %q", "go语言", got, want)
	}
}

// TestBuildTermContext_PercentEncodesCJKInPermalink verifies that the single
// term page context (/tags/{tag}/) percent-encodes CJK in both Permalink
// and RelPermalink, matching Hugo's RSS output where the channel <link>
// is e.g. /tags/%E4%B8%93%E6%B3%A8/ not /tags/专注/.
func TestBuildTermContext_PercentEncodesCJKInPermalink(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	cfg := &config.Config{}
	site := &content.Site{}

	ctx := BuildTermContext(siteCtx, nil, site, cfg, "专注", tmpl.PageSlice{})
	if ctx == nil {
		t.Fatal("expected non-nil term context")
	}
	wantPerm := "https://x/tags/%E4%B8%93%E6%B3%A8/"
	if ctx.Permalink != wantPerm {
		t.Errorf("term Permalink: got %q, want %q", ctx.Permalink, wantPerm)
	}
	if ctx.RelPermalink != "/tags/%E4%B8%93%E6%B3%A8/" {
		t.Errorf("term RelPermalink: got %q, want %q", ctx.RelPermalink, "/tags/%E4%B8%93%E6%B3%A8/")
	}
}

// TestBuildTermContext_FiltersNeverListedPages verifies that
// BuildTermContext strips build.list=never pages from the term's page list.
// This matches Hugo's behavior: tags whose only pages are hidden still get
// /tags/{tag}/index.{html,xml} generated, but the page list (and thus RSS
// items) is empty.
func TestBuildTermContext_FiltersNeverListedPages(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	cfg := &config.Config{}
	site := &content.Site{}

	// visible page; never-listed page
	visibleCtx := &tmpl.Context{Title: "visible"}
	neverCtx := &tmpl.Context{
		Title: "never-listed",
		Build: config.BuildConfig{List: "never"},
	}
	pages := tmpl.PageSlice{visibleCtx, neverCtx}

	ctx := BuildTermContext(siteCtx, nil, site, cfg, "tag1", pages)
	if ctx == nil {
		t.Fatal("expected non-nil term context")
	}
	if len(ctx.RegularPages) != 1 {
		t.Fatalf("RegularPages: got %d, want 1 (never-listed excluded)", len(ctx.RegularPages))
	}
	got := tmpl.AsCtx(ctx.RegularPages[0]).Title
	if got != "visible" {
		t.Errorf("RegularPages[0].Title: got %q, want %q", got, "visible")
	}
}

// TestBuildTermContext_EmptyWhenAllPagesNeverListed verifies that a term
// whose only pages are never-listed produces an empty page list. Hugo still
// generates /tags/{tag}/index.{html,xml} for these tags (file exists) but
// with zero items in RSS.
func TestBuildTermContext_EmptyWhenAllPagesNeverListed(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	cfg := &config.Config{}
	site := &content.Site{}

	neverCtx := &tmpl.Context{
		Title: "hidden-page",
		Build: config.BuildConfig{List: "never"},
	}
	pages := tmpl.PageSlice{neverCtx}

	ctx := BuildTermContext(siteCtx, nil, site, cfg, "phantom", pages)
	if ctx == nil {
		t.Fatal("expected non-nil term context (file must still be generated)")
	}
	if len(ctx.RegularPages) != 0 {
		t.Errorf("RegularPages: got %d, want 0 (all pages never-listed)", len(ctx.RegularPages))
	}
	if len(ctx.Pages) != 0 {
		t.Errorf("Pages: got %d, want 0", len(ctx.Pages))
	}
}

// TestBuildTermContext_TitleUsesOriginalCase verifies that the term-page
// Title uses the original-cased tag name from frontmatter (e.g. "FANFAN")
// rather than the urlized key (e.g. "fanfan"). Hugo's term-page RSS emits
// <title>FANFAN on ...</title> while the URL uses /tags/fanfan/.
func TestBuildTermContext_TitleUsesOriginalCase(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	cfg := &config.Config{}
	site := &content.Site{
		TaxonomyOriginalCase: map[string]map[string]string{
			"tags": {"fanfan": "FANFAN"},
		},
	}

	ctx := BuildTermContext(siteCtx, nil, site, cfg, "fanfan", tmpl.PageSlice{})
	if ctx == nil {
		t.Fatal("expected non-nil term context")
	}
	if ctx.Title != "FANFAN" {
		t.Errorf("term Title: got %q, want %q (original case from frontmatter)", ctx.Title, "FANFAN")
	}
	// Filesystem path / URL must still use the urlized key.
	if ctx.RelPermalink != "/tags/fanfan/" {
		t.Errorf("term RelPermalink: got %q, want %q", ctx.RelPermalink, "/tags/fanfan/")
	}
}

// TestBuildTermContext_TitleFallsBackToKeyWhenNoOriginalCase verifies that
// when no original-case mapping is available, the term Title falls back to
// the (urlized) key. This preserves prior behavior for taxonomies built
// without the original-case tracking.
func TestBuildTermContext_TitleFallsBackToKeyWhenNoOriginalCase(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	cfg := &config.Config{}
	site := &content.Site{} // no TaxonomyOriginalCase

	ctx := BuildTermContext(siteCtx, nil, site, cfg, "plain", tmpl.PageSlice{})
	if ctx == nil {
		t.Fatal("expected non-nil term context")
	}
	if ctx.Title != "plain" {
		t.Errorf("term Title: got %q, want %q (fallback to key)", ctx.Title, "plain")
	}
}

// TestBuildTermContext_SetsSectionToTags verifies that the term-page context
// has Section="tags" so templates (e.g. GA page-context script) see the
// correct section. Hugo's term pages under /tags/{tag}/ report Section="tags".
func TestBuildTermContext_SetsSectionToTags(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	cfg := &config.Config{}
	site := &content.Site{}

	ctx := BuildTermContext(siteCtx, nil, site, cfg, "anytag", tmpl.PageSlice{})
	if ctx == nil {
		t.Fatal("expected non-nil term context")
	}
	if ctx.Section != "tags" {
		t.Errorf("term Section: got %q, want %q", ctx.Section, "tags")
	}
}

// TestBuildTaxonomyContext_SetsSectionToPlural verifies that the taxonomy
// listing context (/tags/) has Section set to the taxonomy plural ("tags"),
// matching Hugo's GA page-context output (section:"tags"). Templates that
// reference {{ .Section }} must see the plural even though the page itself
// is a taxonomy-list page, not a content page.
func TestBuildTaxonomyContext_SetsSectionToPlural(t *testing.T) {
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
	if ctx.Section != "tags" {
		t.Errorf("taxonomy Section: got %q, want %q", ctx.Section, "tags")
	}
}

// TestBuildEmptyTaxonomyContext_SetsSectionToPlural verifies that the empty
// taxonomy listing context (e.g. /categories/ when no categories exist) has
// Section set to the plural passed in. Hugo's GA script for /categories/
// emits section:"categories".
func TestBuildEmptyTaxonomyContext_SetsSectionToPlural(t *testing.T) {
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/"}
	ctx := BuildEmptyTaxonomyContext(siteCtx, "Categories", "categories")
	if ctx == nil {
		t.Fatal("expected non-nil empty taxonomy context")
	}
	if ctx.Section != "categories" {
		t.Errorf("empty taxonomy Section: got %q, want %q", ctx.Section, "categories")
	}
}

// TestBuildEmptyTaxonomyContext_EmptyRegularPagesAndZeroDate verifies that the
// empty taxonomy listing has empty RegularPages/Pages and a zero Date. This
// matches Hugo's output for /categories/index.xml when no categories exist:
// no <item>s and the <lastBuildDate> element is omitted entirely (because
// Hugo's RSS template gates on {{ if not .Date.IsZero }}).
func TestBuildEmptyTaxonomyContext_EmptyRegularPagesAndZeroDate(t *testing.T) {
	siteCtx := &tmpl.SiteContext{
		Title:        "T",
		BaseURL:      "https://x/",
		RegularPages: tmpl.PageSlice{&tmpl.Context{Title: "p"}},
	}
	ctx := BuildEmptyTaxonomyContext(siteCtx, "Categories", "categories")
	if ctx == nil {
		t.Fatal("expected non-nil empty taxonomy context")
	}
	if len(ctx.RegularPages) != 0 {
		t.Errorf("empty taxonomy RegularPages: got %d, want 0 (taxonomy listing has terms, not site pages)", len(ctx.RegularPages))
	}
	if len(ctx.Pages) != 0 {
		t.Errorf("empty taxonomy Pages: got %d, want 0", len(ctx.Pages))
	}
	if !ctx.Date.IsZero() {
		t.Errorf("empty taxonomy Date: got %v, want zero (Hugo omits <lastBuildDate> for empty taxonomy)", ctx.Date)
	}
}

// TestBuildTaxonomyContext_SetsDateToNewestTermStub verifies that the taxonomy
// listing context's Date is set to the newest term stub's effective date.
// Hugo's RSS template emits <lastBuildDate> only when .Date is non-zero, so
// the tags listing (which has terms) must have a non-zero Date to match.
func TestBuildTaxonomyContext_SetsDateToNewestTermStub(t *testing.T) {
	earlier := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 27, 17, 7, 34, 0, time.UTC)

	alphaPage := &content.Page{Title: "alpha-post", RelPath: "posts/a.md", Kind: "page", DateParsed: earlier, LastmodParsed: earlier}
	betaPage := &content.Page{Title: "beta-post", RelPath: "posts/b.md", Kind: "page", DateParsed: later, LastmodParsed: later}

	alphaCtx := &tmpl.Context{Title: "alpha-post", Date: earlier, Lastmod: earlier}
	betaCtx := &tmpl.Context{Title: "beta-post", Date: later, Lastmod: later}

	lookup := map[*content.Page]*tmpl.Context{alphaPage: alphaCtx, betaPage: betaCtx}
	site := &content.Site{
		Pages: []*content.Page{alphaPage, betaPage},
		Taxonomies: map[string]content.Taxonomy{
			"tags": {
				"alpha": {alphaPage},
				"beta":  {betaPage},
			},
		},
	}
	siteCtx := &tmpl.SiteContext{Title: "T", BaseURL: "https://x/", RegularPages: tmpl.PageSlice{alphaCtx, betaCtx}}

	ctx := BuildTaxonomyContext(siteCtx, lookup, site, &config.Config{})
	if ctx == nil {
		t.Fatal("expected non-nil taxonomy context")
	}
	if ctx.Date.IsZero() {
		t.Errorf("taxonomy Date: got zero, want non-zero (Hugo emits <lastBuildDate> for tags listing)")
	}
	if !ctx.Date.Equal(later) {
		t.Errorf("taxonomy Date: got %v, want %v (newest stub effective date)", ctx.Date, later)
	}
}

// TestRssLastBuildDate_EmptyForEmptyRegularPages verifies that
// rssLastBuildDate returns "" when RegularPages is empty. This matches
// Hugo's <lastBuildDate/> output for tags whose only pages are hidden.
func TestRssLastBuildDate_EmptyForEmptyRegularPages(t *testing.T) {
	ctx := &tmpl.Context{RegularPages: tmpl.PageSlice{}}
	got := tmpl.RssLastBuildDate(ctx)
	if got != "" {
		t.Errorf("rssLastBuildDate(empty): got %q, want %q", got, "")
	}
}

// TestRssLastBuildDate_FormatsLatestLastmod verifies that rssLastBuildDate
// returns the latest Lastmod formatted with Hugo's RSS date layout when
// RegularPages is non-empty.
func TestRssLastBuildDate_FormatsLatestLastmod(t *testing.T) {
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 27, 17, 7, 34, 0, time.UTC)
	ctx := &tmpl.Context{
		RegularPages: tmpl.PageSlice{
			&tmpl.Context{Lastmod: earlier},
			&tmpl.Context{Lastmod: later},
		},
	}
	got := tmpl.RssLastBuildDate(ctx)
	// Hugo's RSS layout: "Mon, 02 Jan 2006 15:04:05 -0700"
	want := later.Format("Mon, 02 Jan 2006 15:04:05 -0700")
	if got != want {
		t.Errorf("rssLastBuildDate: got %q, want %q", got, want)
	}
}
