package template

import (
	"html/template"
	"testing"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

func TestNewRenderer(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("hello {{ .Title }}"))
	r := NewRenderer(tmpl, template.FuncMap{})
	if r == nil {
		t.Fatal("expected non-nil renderer")
	}
}

func TestRenderer_Render(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("hello {{ .Title }}"))
	r := NewRenderer(tmpl, template.FuncMap{})

	result, err := r.Render("test", &Context{Title: "world"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Render: got %q, want 'hello world'", result)
	}
}

func TestRenderer_Render_MissingTemplate(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("content"))
	r := NewRenderer(tmpl, template.FuncMap{})

	_, err := r.Render("nonexistent", &Context{})
	if err == nil {
		t.Error("expected error for missing template")
	}
}

func TestSetPaginator(t *testing.T) {
	ctx := &Context{Kind: "page"}
	pager := &PaginatorContext{PageNumber: 1, TotalPages: 5}
	SetPaginator(ctx, pager)
	if ctx.paginatorCache == nil {
		t.Fatal("expected non-nil paginator")
	}
	if ctx.paginatorCache.PageNumber != 1 {
		t.Errorf("PageNumber: got %d, want 1", ctx.paginatorCache.PageNumber)
	}
}

func TestContext_Paginator(t *testing.T) {
	cfg := &config.Config{Paginate: 10}
	site := &SiteContext{Config: cfg}
	ctx := &Context{
		Kind:         "section",
		Site:         site,
		RelPermalink: "/posts/",
		RegularPages: PageSlice{
			&Context{Title: "p1"},
			&Context{Title: "p2"},
			&Context{Title: "p3"},
		},
	}

	pager := ctx.Paginator()
	if pager == nil {
		t.Fatal("expected non-nil paginator")
	}
	if pager.PageNumber != 1 {
		t.Errorf("PageNumber: got %d, want 1", pager.PageNumber)
	}
	if len(pager.Pages) != 3 {
		t.Errorf("Pages len: got %d, want 3", len(pager.Pages))
	}
}

func TestContext_Paginate(t *testing.T) {
	cfg := &config.Config{Paginate: 2}
	site := &SiteContext{Config: cfg}
	ctx := &Context{
		Kind:         "section",
		Site:         site,
		RelPermalink: "/posts/",
		RegularPages: PageSlice{
			&Context{Title: "p1"},
			&Context{Title: "p2"},
			&Context{Title: "p3"},
			&Context{Title: "p4"},
		},
	}

	// Default pagination
	pager := ctx.Paginate()
	if pager.TotalPages != 2 {
		t.Errorf("TotalPages: got %d, want 2", pager.TotalPages)
	}
	if !pager.HasNext {
		t.Error("expected HasNext=true")
	}
	if pager.Next == nil {
		t.Fatal("expected Next pager")
	}
	if pager.Next.PageNumber != 2 {
		t.Errorf("Next.PageNumber: got %d, want 2", pager.Next.PageNumber)
	}

	// Custom page size
	pager2 := ctx.Paginate(ctx.RegularPages, 1)
	if pager2.TotalPages != 4 {
		t.Errorf("TotalPages with size 1: got %d, want 4", pager2.TotalPages)
	}
}

func TestContext_Paginate_WithPageSlice(t *testing.T) {
	cfg := &config.Config{Paginate: 10}
	site := &SiteContext{Config: cfg}
	ctx := &Context{Site: site, RelPermalink: "/test/"}

	pages := PageSlice{&Context{Title: "a"}, &Context{Title: "b"}}
	pager := ctx.Paginate(pages, 1)
	if len(pager.Pages) != 1 {
		t.Errorf("Paginate with custom slice: got %d pages, want 1", len(pager.Pages))
	}
}

func TestContext_Paginate_Cache(t *testing.T) {
	cfg := &config.Config{Paginate: 10}
	site := &SiteContext{Config: cfg}
	ctx := &Context{
		Site:         site,
		RelPermalink: "/test/",
		RegularPages: PageSlice{&Context{}},
	}

	// First call via Paginator() sets the cache
	pager1 := ctx.Paginator()
	if pager1 == nil {
		t.Fatal("first Paginator returned nil")
	}

	// Check cache was set
	if ctx.paginatorCache == nil {
		t.Error("paginatorCache should be set after Paginator")
	}

	// Paginate() should check cache and return the same pager
	pager2 := ctx.Paginate()
	if pager2 != pager1 {
		t.Error("Paginate should return cached paginator from Paginator()")
	}
}

func TestDefaultPageOutputFormats(t *testing.T) {
	formats := DefaultPageOutputFormats("https://example.com/page/", "/page/")
	if formats == nil {
		t.Fatal("expected non-nil output formats")
	}

	// Should have HTML
	html := formats.Get("HTML")
	if html == nil {
		t.Fatal("expected HTML format")
	}

	// Should have RSS
	rss := formats.Get("RSS")
	if rss == nil {
		t.Fatal("expected RSS format")
	}
}

func TestHTMLOnlyOutputFormats(t *testing.T) {
	formats := HTMLOnlyOutputFormats("https://example.com/page/", "/page/")
	if formats == nil {
		t.Fatal("expected non-nil output formats")
	}

	// Should have HTML
	html := formats.Get("HTML")
	if html == nil {
		t.Fatal("expected HTML format")
	}

	// Should NOT have RSS
	rss := formats.Get("RSS")
	if rss != nil {
		t.Error("expected no RSS format")
	}
}

func TestPageOutputFormats_Get(t *testing.T) {
	formats := &PageOutputFormats{
		formats: []PageOutputFormat{
			{Name: "HTML", Permalink: "https://example.com/page/"},
			{Name: "RSS", Permalink: "https://example.com/page/index.xml"},
		},
	}

	// Case insensitive lookup
	html := formats.Get("html")
	if html == nil {
		t.Error("expected to find HTML with lowercase lookup")
	}

	// Missing format
	missing := formats.Get("JSON")
	if missing != nil {
		t.Error("expected nil for missing format")
	}
}

func TestContext_IsHome(t *testing.T) {
	ctx := &Context{Kind: "home"}
	if !ctx.IsHome() {
		t.Error("IsHome(): got false, want true")
	}

	ctx2 := &Context{Kind: "page"}
	if ctx2.IsHome() {
		t.Error("IsHome() for page: got true, want false")
	}
}

func TestContext_IsPage(t *testing.T) {
	ctx := &Context{Kind: "page"}
	if !ctx.IsPage() {
		t.Error("IsPage(): got false, want true")
	}
}

func TestContext_IsSection(t *testing.T) {
	ctx := &Context{Kind: "section"}
	if !ctx.IsSection() {
		t.Error("IsSection(): got false, want true")
	}
}

func TestContext_PublishDate(t *testing.T) {
	now := time.Now()
	ctx := &Context{Date: now}
	if !ctx.PublishDate().Equal(now) {
		t.Error("PublishDate should return Date")
	}
}

func TestContext_Format(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	ctx := &Context{Date: now}

	result := ctx.Format("2006-01-02")
	if result != "2026-07-20" {
		t.Errorf("Format: got %s, want 2026-07-20", result)
	}

	// Zero date
	ctx2 := &Context{}
	if ctx2.Format("2006") != "" {
		t.Error("Format with zero date should return empty string")
	}
}

func TestFormatDate(t *testing.T) {
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	result := FormatDate("2006-01-02", now)
	if result != "2026-07-20" {
		t.Errorf("FormatDate: got %s, want 2026-07-20", result)
	}

	// Zero time
	if FormatDate("2006", time.Time{}) != "" {
		t.Error("FormatDate with zero time should return empty")
	}
}

func TestContext_Translations(t *testing.T) {
	ctx := &Context{}
	translations := ctx.Translations()
	if translations == nil {
		t.Error("Translations should return non-nil slice")
	}
}

func TestContext_AllTranslationLinks(t *testing.T) {
	ctx := &Context{
		TranslationLinks: []TranslationLink{
			{Lang: "en", URL: "https://example.com/en/page/"},
		},
	}
	links := ctx.AllTranslationLinks()
	if len(links) != 1 {
		t.Errorf("AllTranslationLinks: got %d, want 1", len(links))
	}
}

func TestContext_TermsData(t *testing.T) {
	ctx := &Context{
		DataTerms: []TermSummaryExternal{
			{Name: "tag1", Count: 5},
			{Name: "tag2", Count: 3},
		},
		DataPlural: "tags",
	}

	data := ctx.TermsData()
	if data.Plural != "tags" {
		t.Errorf("TermsData.Plural: got %s, want tags", data.Plural)
	}
	if len(data.Terms) != 2 {
		t.Errorf("TermsData.Terms len: got %d, want 2", len(data.Terms))
	}
}

func TestDataAccessor_GroupByDate(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := &DataAccessor{
		Pages: PageSlice{
			&Context{Date: t1, Title: "p1"},
			&Context{Date: t2, Title: "p2"},
		},
	}

	groups := d.GroupByDate("2006")
	if len(groups) == 0 {
		t.Error("GroupByDate: expected at least one group")
	}
}

func TestTermsList_ByCount(t *testing.T) {
	list := TermsList{
		{Name: "a", Count: 3},
		{Name: "b", Count: 10},
		{Name: "c", Count: 1},
	}

	sorted := list.ByCount()
	if sorted[0].Count != 10 {
		t.Errorf("ByCount[0]: got %d, want 10", sorted[0].Count)
	}
}

func TestTaxonomyTerms_ByCount(t *testing.T) {
	terms := TaxonomyTerms{
		{Term: "a", Count: 3},
		{Term: "b", Count: 10},
	}

	sorted := terms.ByCount()
	if sorted[0].Count != 10 {
		t.Errorf("TaxonomyTerms.ByCount[0]: got %d, want 10", sorted[0].Count)
	}
}

func TestTaxonomyTerms_Alphabetical(t *testing.T) {
	terms := TaxonomyTerms{
		{Term: "z"},
		{Term: "a"},
		{Term: "m"},
	}

	sorted := terms.Alphabetical()
	if sorted[0].Term != "a" {
		t.Errorf("Alphabetical[0]: got %s, want a", sorted[0].Term)
	}
}

func TestBuildTaxonomyContexts(t *testing.T) {
	taxonomies := map[string]content.Taxonomy{
		"tags": {
			"go":   {&content.Page{Title: "p1"}},
			"rust": {&content.Page{Title: "p2"}},
		},
	}

	result := buildTaxonomyContexts(taxonomies)
	if len(result) != 1 {
		t.Fatalf("expected 1 taxonomy context, got %d", len(result))
	}
	tags := result["tags"]
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestPermalinkEncode(t *testing.T) {
	// ASCII stays unchanged
	if result := permalinkEncode("/posts/hello/"); result != "/posts/hello/" {
		t.Errorf("permalinkEncode ASCII: got %s, want /posts/hello/", result)
	}

	// Non-ASCII gets encoded
	if result := permalinkEncode("/posts/书稿/"); !contains(result, "%") {
		t.Errorf("permalinkEncode non-ASCII: got %s, want percent-encoded", result)
	}
}

func TestPageType(t *testing.T) {
	p := &content.Page{Type: "custom", Section: "posts"}
	if result := pageType(p); result != "custom" {
		t.Errorf("pageType with Type: got %s, want custom", result)
	}

	p2 := &content.Page{Section: "posts"}
	if result := pageType(p2); result != "posts" {
		t.Errorf("pageType without Type: got %s, want posts", result)
	}
}

func TestPageParams(t *testing.T) {
	p := &content.Page{
		Title:       "Test",
		Draft:       false,
		Tags:        []string{"a", "b"},
		Keywords:    []string{"k1"},
		Description: "desc",
		Slug:        "test-slug",
		Type:        "page",
		Image:       "img.jpg",
		Author:      "author",
	}

	params := pageParams(p)
	if params["title"] != "Test" {
		t.Error("pageParams missing title")
	}
	if params["author"] != "author" {
		t.Error("pageParams missing author")
	}

	// No author
	p.Author = ""
	params2 := pageParams(p)
	if _, ok := params2["author"]; ok {
		t.Error("pageParams should not include empty author")
	}
}

func TestCollapseDoubleSlashes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com//path", "https://example.com/path"},
		{"https://example.com/path", "https://example.com/path"},
		{"http://test//a//b", "http://test/a/b"},
	}

	for _, tt := range tests {
		result := collapseDoubleSlashes(tt.input)
		if result != tt.expected {
			t.Errorf("collapseDoubleSlashes(%s): got %s, want %s", tt.input, result, tt.expected)
		}
	}
}

func TestMergeSitemap(t *testing.T) {
	siteDefault := config.SitemapConfig{
		ChangeFreq: "weekly",
		Priority:   0.5,
	}
	page := config.SitemapPageConfig{
		Disable:    false,
		ChangeFreq: "daily",
		Priority:   0.8,
	}

	result := mergeSitemap(siteDefault, page)
	if result.ChangeFreq != "daily" {
		t.Errorf("mergeSitemap ChangeFreq: got %s, want daily", result.ChangeFreq)
	}
	if result.Priority != 0.8 {
		t.Errorf("mergeSitemap Priority: got %f, want 0.8", result.Priority)
	}

	// Page without overrides
	page2 := config.SitemapPageConfig{}
	result2 := mergeSitemap(siteDefault, page2)
	if result2.ChangeFreq != "weekly" {
		t.Errorf("mergeSitemap fallback: got %s, want weekly", result2.ChangeFreq)
	}
}

func TestPopulateSitePages(t *testing.T) {
	p1 := &content.Page{Title: "p1"}
	p2 := &content.Page{Title: "p2"}
	site := &content.Site{
		Pages:        []*content.Page{p1, p2},
		RegularPages: []*content.Page{p1},
	}

	siteCtx := &SiteContext{}
	lookup := map[*content.Page]*Context{
		p1: &Context{Title: "p1"},
		p2: &Context{Title: "p2"},
	}

	PopulateSitePages(siteCtx, site, lookup, true)

	if len(siteCtx.Pages) != 2 {
		t.Errorf("PopulateSitePages Pages: got %d, want 2", len(siteCtx.Pages))
	}
	if len(siteCtx.RegularPages) != 1 {
		t.Errorf("PopulateSitePages RegularPages: got %d, want 1", len(siteCtx.RegularPages))
	}
}

func TestNewSiteContext(t *testing.T) {
	site := &content.Site{
		Title:   "Test Site",
		BaseURL: "https://example.com/",
		Params:  map[string]interface{}{"key": "value"},
	}
	cfg := &config.Config{
		Author: config.AuthorConfig{Name: "Test Author"},
	}

	siteCtx := NewSiteContext(site, cfg)
	if siteCtx.Title != "Test Site" {
		t.Errorf("NewSiteContext Title: got %s, want Test Site", siteCtx.Title)
	}
	if siteCtx.Author == nil || siteCtx.Author.Name != "Test Author" {
		t.Error("NewSiteContext Author not set correctly")
	}
}
