package template

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// Context is the data object passed to every template.
// It mirrors Hugo's page context: .Title, .Content, .Site, .Params, etc.
type Context struct {
	// Page fields - accessed as .Title, .Content, etc.
	Title         string
	Date          time.Time
	Lastmod       time.Time
	Draft         bool
	Hidden        bool
	Type          string
	Slug          string
	Tags          []string
	Keywords      []string
	Description   string
	Author        string
	Image         string
	FeaturedImage string
	Section       string
	Kind          string
	Weight        int
	WordCount     int
	ReadingTime   int

	Content         template.HTML
	Summary         template.HTML
	Plain           string
	RelPermalink    string
	Permalink       string

	Params          map[string]interface{}
	Access          string
	EncryptGroup    string
	EncryptMode     string
	EncryptRatio    int
	Build           config.BuildConfig
	Cascade         config.CascadeConfig
	Sitemap         config.SitemapPageConfig

	File            *FileInfo
	Site            *SiteContext
	Pages           PageSlice
	RegularPages    PageSlice
	RegularPagesRecursive PageSlice
	Parent          *Context

	// Pagination
	paginatorCache  *PaginatorContext // populated by Paginator() on first access
	Paginated       bool               // true if this context is itself a paginated result

	// Output formats
	OutputFormats   *PageOutputFormats

	// Data from data files (or a DataAccessor for taxonomy pages)
	Data           interface{}

	// Scratch for template-scoped variables
	Scratch        *Scratch

	// Taxonomy data: term listing (for /tags/) and current term info (for /tags/X/)
	DataTerms     []TermSummaryExternal
	DataPlural    string

	// For taxonomy pages
	Data_          *TaxonomyDataContext
}

// FileInfo mirrors Hugo's .File object.
type FileInfo struct {
	Path          string
	Dir           string
	BaseFileName string
}

// SiteContext mirrors Hugo's .Site object.
type SiteContext struct {
	Title        string
	BaseURL      string
	Language     *LanguageContext
	Params       map[string]interface{}
	Menus        map[string][]config.MenuItem
	Pages        PageSlice
	RegularPages PageSlice
	Data         map[string]interface{}
	Taxonomies   map[string]TaxonomyContext
	Config       *config.Config
	LanguageCode string
	Author       *AuthorContext

	// Copyright string (Hugo compatibility)
	Copyright string

	// Output formats
	OutputFormats *OutputFormatsContext

	// page index for GetPage lookups
	pagesByPath map[string]*Context
}

// LanguageContext mirrors Hugo's .Site.Language object.
type LanguageContext struct {
	LanguageCode string
	LanguageName string
	LanguageDirection string
}

// TermSummaryExternal is a taxonomy term with its pages, exposed to templates.
// Used by /tags/ term listing via .Data.Terms.ByCount.
type TermSummaryExternal struct {
	Name  string
	Pages PageSlice
	Count int
}

// TermsList is a sortable list of terms for template use.
type TermsList []TermSummaryExternal

// ByCount sorts terms by count descending.
func (t TermsList) ByCount() TermsList {
	out := make(TermsList, len(t))
	copy(out, t)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if out[j].Count > out[j-1].Count {
				out[j], out[j-1] = out[j-1], out[j]
			} else {
				break
			}
		}
	}
	return out
}

// DataAccessor wraps DataTerms for `.Data.Terms.ByCount` template access.
type DataAccessor struct {
	Terms  TermsList
	Plural string
	Pages  PageSlice
}

// GroupByDate on DataAccessor.Pages mirrors PageSlice.GroupByDate.
func (d *DataAccessor) GroupByDate(layout string) []DateGroup {
	return d.Pages.GroupByDate(layout)
}

// TermsData returns a DataAccessor for the templates' `.Data.Terms` access.
func (c *Context) TermsData() *DataAccessor {
	return &DataAccessor{
		Terms:  TermsList(c.DataTerms),
		Plural: c.DataPlural,
	}
}

// GetPage mirrors Hugo's .Site.GetPage - finds a page by ref/path.
// Usage: {{ .Site.GetPage "/posts/foo" }} or {{ .Site.GetPage "section" "name" }}
func (s *SiteContext) GetPage(args ...string) *Context {
	if len(args) == 0 {
		return nil
	}
	// Last arg is the path/ref
	ref := args[len(args)-1]
	ref = strings.TrimPrefix(ref, "/")
	ref = strings.TrimSuffix(ref, "/")

	for _, item := range s.Pages {
		p := asCtx(item)
		if p == nil {
			continue
		}
		if strings.TrimSuffix(strings.TrimPrefix(p.RelPermalink, "/"), "/") == ref {
			return p
		}
	}
	return nil
}

type AuthorContext struct {
	Name string
}

type TaxonomyContext map[string][]*Context

// TaxonomyDataContext for taxonomy listing pages.
type TaxonomyDataContext struct {
	Terms TaxonomyTerms
}

type TaxonomyTerms []TermCount

type TermCount struct {
	Term  string
	Pages []*Context
	Count int
}

// ByCount sorts terms by count (descending), matching Hugo behavior.
func (t TaxonomyTerms) ByCount() TaxonomyTerms {
	sorted := make(TaxonomyTerms, len(t))
	copy(sorted, t)
	// Simple bubble sort (small dataset)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Count > sorted[i].Count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

// ByCount_Alphabetical sorts terms alphabetically.
func (t TaxonomyTerms) Alphabetical() TaxonomyTerms {
	sorted := make(TaxonomyTerms, len(t))
	copy(sorted, t)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Term < sorted[i].Term {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

// PaginatorContext mirrors Hugo's .Paginator object.
type PaginatorContext struct {
	PageNumber int
	URL        string
	Pages      PageSlice
	HasPrev    bool
	HasNext    bool
	Prev       *PaginatorContext
	Next       *PaginatorContext
	First      *PaginatorContext
	Last       *PaginatorContext
	PagerSize  int
	TotalPages int
}

// OutputFormatsContext for .Site.OutputFormats.
type OutputFormatsContext struct {
	Formats []OutputFormat
}

type OutputFormat struct {
	Name      string
	MediaType string
	BaseName  string
	Rel       string
}

// PageOutputFormats wraps output formats for a single page.
type PageOutputFormats struct {
	formats []PageOutputFormat
}

func (p *PageOutputFormats) Get(name string) *PageOutputFormat {
	for i := range p.formats {
		if strings.EqualFold(p.formats[i].Name, name) {
			return &p.formats[i]
		}
	}
	return nil
}

// PageOutputFormat is a single output format for a page, with a computed Permalink.
type PageOutputFormat struct {
	Name         string
	Rel          string
	MediaType    MediaType
	Permalink    string
	RelPermalink string
}

// MediaType is a simple media type wrapper supporting .Type in templates.
type MediaType struct {
	Type string
}

// HTMLOnlyOutputFormats returns output formats with just HTML (no RSS).
// Used for pages like 404 that shouldn't have an RSS feed.
func HTMLOnlyOutputFormats(permalink, relPermalink string) *PageOutputFormats {
	return &PageOutputFormats{
		formats: []PageOutputFormat{
			{Name: "HTML", Rel: "alternate", MediaType: MediaType{Type: "text/html"}, Permalink: permalink, RelPermalink: relPermalink},
		},
	}
}

// DefaultPageOutputFormats returns a reasonable default for an HTML page.
func DefaultPageOutputFormats(permalink, relPermalink string) *PageOutputFormats {
	return &PageOutputFormats{
		formats: []PageOutputFormat{
			{Name: "HTML", Rel: "alternate", MediaType: MediaType{Type: "text/html"}, Permalink: permalink, RelPermalink: relPermalink},
			{Name: "RSS", Rel: "alternate", MediaType: MediaType{Type: "application/rss+xml"}, Permalink: permalink + "index.xml", RelPermalink: relPermalink + "index.xml"},
		},
	}
}

// NewContext creates a template context from a Page and a shared SiteContext.
// The site context must be built once via NewSiteContext() and passed in.
func NewContext(p *content.Page, siteCtx *SiteContext, cfg *config.Config) *Context {
	ctx := &Context{
		Title:           p.Title,
		Date:            p.DateParsed,
		Lastmod:         p.LastmodParsed,
		Draft:           p.Draft,
		Hidden:          p.Hidden,
		Type:            pageType(p),
		Slug:            p.Slug,
		Tags:            p.Tags,
		Keywords:        p.Keywords,
		Description:     p.Description,
		Author:          p.Author,
		Image:           p.Image,
		FeaturedImage:   p.FeaturedImage,
		Section:         p.Section,
		Kind:            p.Kind,
		Weight:          p.Weight,
		WordCount:       p.WordCount,
		Content:         p.Content,
		Summary:         p.Summary,
		Plain:           p.Plain,
		RelPermalink:    permalinkEncode(p.URL),
		Permalink:       permalinkEncode(cfg.BaseURL + strings.TrimPrefix(p.URL, "/")),
		Params:          pageParams(p),
		Access:          p.Access,
		EncryptGroup:    p.EncryptGroup,
		EncryptMode:     p.EncryptMode,
		EncryptRatio:    p.EncryptRatio,
		Build:           p.Build,
		Cascade:         p.Cascade,
		Sitemap:         mergeSitemap(cfg.Sitemap, p.Sitemap),
		Data:           siteCtx.Data,
		Site:           siteCtx,
		Scratch:        NewScratch(),
	}

	if p.FilePath != "" {
		ctx.File = &FileInfo{
			Path:          p.RelPath,
			Dir:           filepath.Dir(p.RelPath) + "/",
			BaseFileName:  strings.TrimSuffix(filepath.Base(p.RelPath), ".md"),
		}
	}

	// Initialize OutputFormats based on page kind.
	// Hugo's default outputs: home/section/taxonomy/term include RSS, page doesn't.
	switch p.Kind {
	case "home", "section", "taxonomy", "term":
		ctx.OutputFormats = DefaultPageOutputFormats(ctx.Permalink, ctx.RelPermalink)
	default:
		ctx.OutputFormats = &PageOutputFormats{
			formats: []PageOutputFormat{
				{Name: "HTML", Rel: "alternate", MediaType: MediaType{Type: "text/html"}, Permalink: ctx.Permalink, RelPermalink: ctx.RelPermalink},
			},
		}
	}

	return ctx
}

// NewSiteContext builds the shared SiteContext once, before any page context.
func NewSiteContext(site *content.Site, cfg *config.Config) *SiteContext {
	return &SiteContext{
		Title:        site.Title,
		BaseURL:      site.BaseURL,
		Language:     &LanguageContext{LanguageCode: cfg.LanguageCode},
		LanguageCode: cfg.LanguageCode,
		Params:       site.Params,
		Menus:        site.Menus,
		Data:         site.Data,
		Config:       cfg,
		// Copyright: Hugo's .Site.Copyright comes from the top-level `copyright`
		// config field, not params. zhurongshuo doesn't set it, so leave empty
		// to match Hugo's behavior (RSS template's `{{ with .Site.Copyright }}`
		// then skips the <copyright> element).
		Author:       &AuthorContext{Name: cfg.Author.Name},
		Taxonomies:   buildTaxonomyContexts(site.Taxonomies),
		OutputFormats: &OutputFormatsContext{},
	}
}

// mergeSitemap applies page-level sitemap overrides on top of the site default.
func mergeSitemap(siteDefault config.SitemapConfig, page config.SitemapPageConfig) config.SitemapPageConfig {
	result := config.SitemapPageConfig{
		Disable:    page.Disable,
		ChangeFreq: siteDefault.ChangeFreq,
		Priority:   siteDefault.Priority,
	}
	if page.ChangeFreq != "" {
		result.ChangeFreq = page.ChangeFreq
	}
	if page.Priority > 0 {
		result.Priority = page.Priority
	}
	return result
}
// all page contexts exist.
func PopulateSitePages(siteCtx *SiteContext, site *content.Site, lookup map[*content.Page]*Context) {
	for _, p := range site.RegularPages {
		if c, ok := lookup[p]; ok {
			siteCtx.RegularPages = append(siteCtx.RegularPages, c)
		}
	}
	for _, p := range site.Pages {
		if c, ok := lookup[p]; ok {
			siteCtx.Pages = append(siteCtx.Pages, c)
		}
	}
}

// LinkPageRelationships fills in Pages/RegularPages/RegularPagesRecursive
// on a Context after all contexts are built, via a page→context lookup.
func LinkPageRelationships(ctx *Context, p *content.Page, lookup map[*content.Page]*Context) {
	for _, child := range p.Pages {
		if c, ok := lookup[child]; ok {
			ctx.Pages = append(ctx.Pages, c)
		}
	}
	for _, child := range p.RegularPages {
		if c, ok := lookup[child]; ok {
			ctx.RegularPages = append(ctx.RegularPages, c)
		}
	}
	for _, child := range p.RegularPagesRecursive {
		if c, ok := lookup[child]; ok {
			ctx.RegularPagesRecursive = append(ctx.RegularPagesRecursive, c)
		}
	}
	if p.Parent != nil {
		if c, ok := lookup[p.Parent]; ok {
			ctx.Parent = c
		}
	}
}

// Paginator mirrors Hugo's .Paginator - auto-paginates this section's pages.
// Returns the first pager; subsequent pagers are linked via Next.
func (c *Context) Paginator() *PaginatorContext {
	if c.paginatorCache != nil {
		return c.paginatorCache
	}
	c.paginatorCache = c.Paginate(c.RegularPages)
	return c.paginatorCache
}

// SetPaginator overrides the cached paginator (used by pagination page generation).
func SetPaginator(c *Context, p *PaginatorContext) {
	c.paginatorCache = p
}
// pages of cfg.Paginate size and returns the first pager as PaginatorContext.
// Hugo variadic: .Paginate, .Paginate pages, .Paginate pages size.
// If a paginator is already cached (e.g., for /page/N/ rendering), returns it.
func (c *Context) Paginate(args ...interface{}) *PaginatorContext {
	if c.paginatorCache != nil {
		return c.paginatorCache
	}

	var pages PageSlice
	size := 10
	if c.Site != nil && c.Site.Config != nil {
		size = c.Site.Config.Paginate
		if size <= 0 {
			size = 10
		}
	}

	if len(args) == 0 {
		// No args: paginate the section's own pages
		pages = c.RegularPages
	} else {
		switch v := args[0].(type) {
		case PageSlice:
			pages = v
		case []*Context:
			pages = make(PageSlice, len(v))
			for i, c := range v {
				pages[i] = c
			}
		case []interface{}:
			for _, item := range v {
				if ctx, ok := item.(*Context); ok {
					pages = append(pages, ctx)
				}
			}
		default:
			pages = c.RegularPages
		}
		if len(args) >= 2 {
			if n, ok := args[1].(int); ok && n > 0 {
				size = n
			}
		}
	}

	return buildPaginator(pages, size, c.RelPermalink)
}

// buildPaginator creates a PaginatorContext chain from a list of pages.
func buildPaginator(pages PageSlice, size int, sectionURL string) *PaginatorContext {
	totalPages := (len(pages) + size - 1) / size
	if totalPages == 0 {
		totalPages = 1
	}

	pagers := make([]*PaginatorContext, totalPages)
	for i := 0; i < totalPages; i++ {
		start := i * size
		end := start + size
		if end > len(pages) {
			end = len(pages)
		}

		pageNum := i + 1
		var pageURL string
		if pageNum == 1 {
			pageURL = sectionURL
		} else {
			pageURL = strings.TrimRight(sectionURL, "/") + fmt.Sprintf("/page/%d/", pageNum)
		}

		pagers[i] = &PaginatorContext{
			PageNumber: pageNum,
			URL:        pageURL,
			Pages:      pages[start:end],
			TotalPages: totalPages,
			PagerSize:  size,
		}
	}

	for i, p := range pagers {
		if i > 0 {
			p.HasPrev = true
			p.Prev = pagers[i-1]
		}
		if i < len(pagers)-1 {
			p.HasNext = true
			p.Next = pagers[i+1]
		}
		p.First = pagers[0]
		p.Last = pagers[len(pagers)-1]
	}

	return pagers[0]
}
func (c *Context) IsHome() bool { return c.Kind == "home" }

// IsTranslated returns false (no multi-language support in huan).
func (c *Context) IsTranslated() bool { return false }

// Translations returns an empty slice (no multi-language support).
func (c *Context) Translations() PageSlice { return PageSlice{} }

// PublishDate returns the page's publication date (Hugo compatibility).
func (c *Context) PublishDate() time.Time { return c.Date }

// IsPage returns true if this is a regular page.
func (c *Context) IsPage() bool { return c.Kind == "page" }

// IsSection returns true if this is a section page.
func (c *Context) IsSection() bool { return c.Kind == "section" }

// Format formats the date with Go's time format.
func (c *Context) Format(layout string) string {
	if c.Date.IsZero() {
		return ""
	}
	return c.Date.Format(layout)
}

// FormatDate formats a specific time with the given layout.
func FormatDate(layout string, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(layout)
}

// permalinkEncode percent-encodes non-ASCII characters in a URL using uppercase
// hex, matching Hugo's Permalink output. ASCII characters (including / : . - _ ~)
// are preserved.
func permalinkEncode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 {
			// ASCII: keep as-is (URL-safe ASCII chars)
			b.WriteRune(r)
			continue
		}
		// Non-ASCII: percent-encode UTF-8 bytes with uppercase hex
		for _, c := range []byte(string(r)) {
			const hex = "0123456789ABCDEF"
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xF])
		}
	}
	return b.String()
}

// pageType returns the effective type for a page.
// Hugo: if frontmatter doesn't set `type`, it defaults to the section name.
func pageType(p *content.Page) string {
	if p.Type != "" {
		return p.Type
	}
	return p.Section
}

// pageParams converts page fields to a map for .Params access.
func pageParams(p *content.Page) map[string]interface{} {
	m := map[string]interface{}{
		"title":          p.Title,
		"date":           p.Date,
		"draft":          p.Draft,
		"hidden":         p.Hidden,
		"tags":           p.Tags,
		"keywords":       p.Keywords,
		"description":    p.Description,
		"slug":           p.Slug,
		"type":           p.Type,
		"access":         p.Access,
		"encryptGroup":   p.EncryptGroup,
		"encryptMode":    p.EncryptMode,
		"encryptRatio":   p.EncryptRatio,
		"image":          p.Image,
		"featured_image": p.FeaturedImage,
	}
	// Only include author if it's actually set in frontmatter (so templates
	// can use `if .Params.author` to detect its presence).
	if p.Author != "" {
		m["author"] = p.Author
	}
	return m
}

func buildTaxonomyContexts(taxonomies map[string]content.Taxonomy) map[string]TaxonomyContext {
	result := make(map[string]TaxonomyContext)
	for name, tax := range taxonomies {
		ctx := make(TaxonomyContext)
		for term, pages := range tax {
			contexts := make([]*Context, len(pages))
			for i, p := range pages {
				contexts[i] = &Context{Title: p.Title, RelPermalink: p.URL}
			}
			ctx[term] = contexts
		}
		result[name] = ctx
	}
	return result
}

// Renderer handles executing templates with context data.
// It keeps a "factory" template (never executed) and clones it for each render,
// so context-specific functions like `site` can be injected per-call.
type Renderer struct {
	tmpl    *template.Template // factory, never executed directly
	funcMap template.FuncMap
}

// NewRenderer creates a new template renderer.
func NewRenderer(tmpl *template.Template, funcMap template.FuncMap) *Renderer {
	return &Renderer{tmpl: tmpl, funcMap: funcMap}
}

// Render executes a named template with the given context.
// For each render, it clones the factory template and sets it as the active
// template reference (so `partial` closures resolve to this clone), then injects
// the `site` function returning the current Site context.
func (r *Renderer) Render(templateName string, ctx *Context) (string, error) {
	t := r.tmpl.Lookup(templateName)
	if t == nil {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	cloned, err := r.tmpl.Clone()
	if err != nil {
		return "", fmt.Errorf("clone template: %w", err)
	}

	// Make this clone the active template so `partial` closures resolve correctly.
	SetActiveTemplate(cloned)

	cloned.Funcs(template.FuncMap{
		"site": func() *SiteContext { return ctx.Site },
	})

	ct := cloned.Lookup(templateName)
	if ct == nil {
		return "", fmt.Errorf("cloned template not found: %s", templateName)
	}

	var buf strings.Builder
	if err := ct.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// LoadAllTemplates is a convenience function that loads all templates.
func LoadAllTemplates(sourceDir, baseURL string) (*template.Template, error) {
	// Determine theme name (look for theme directory)
	themeName := ""
	themesDir := filepath.Join(sourceDir, "themes")
	if entries, err := os.ReadDir(themesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				themeName = e.Name()
				break
			}
		}
	}

	funcMap := FuncMap(baseURL)
	loader := NewLoader(sourceDir, themeName, funcMap)
	return loader.LoadAll()
}
