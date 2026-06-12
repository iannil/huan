package build

import (
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/i18n"
	tmpl "github.com/iannil/huan/internal/template"
)

// ResolveRSSOutput returns the RSS template name for a section/home page.
func ResolveRSSOutput(p *content.Page) string {
	// Hugo uses _default/rss.xml for all sections
	return "_default/rss.xml"
}

// BuildSitemapContext creates a root context for sitemap rendering.
// The sitemap template iterates .Pages, so we need a context with all pages populated.
func BuildSitemapContext(siteCtx *tmpl.SiteContext, lookup map[*content.Page]*tmpl.Context, site *content.Site, cfg *config.Config) *tmpl.Context {
	ctx := &tmpl.Context{
		Kind:   "home",
		Site:   siteCtx,
		Params: map[string]interface{}{},
		Data:   siteCtx.Data,
	}
	ctx.Pages = siteCtx.Pages
	return ctx
}

// FindHomeContext returns the home page context, used for search index rendering.
func FindHomeContext(lookup map[*content.Page]*tmpl.Context, site *content.Site) *tmpl.Context {
	for _, p := range site.Pages {
		if p.Kind == "home" {
			return lookup[p]
		}
	}
	return nil
}

// FilterMainSections returns pages whose Section is in mainSections.
func FilterMainSections(pages tmpl.PageSlice, mainSections []string) tmpl.PageSlice {
	sectionSet := map[string]bool{}
	for _, s := range mainSections {
		sectionSet[s] = true
	}
	var result tmpl.PageSlice
	for _, item := range pages {
		c := tmpl.AsCtx(item)
		if c == nil {
			continue
		}
		if sectionSet[c.Section] {
			result = append(result, c)
		}
	}
	return result
}

// CloneContextForPagination creates a shallow copy of homeCtx with a Paginator
// pointing at page N. The URL stays "/" so meta tags match Hugo's behavior.
func CloneContextForPagination(homeCtx *tmpl.Context, allItems tmpl.PageSlice, pageSize, pageNum, totalPages int) *tmpl.Context {
	start := (pageNum - 1) * pageSize
	end := start + pageSize
	if end > len(allItems) {
		end = len(allItems)
	}
	if start >= len(allItems) {
		start = len(allItems)
	}

	ctx := *homeCtx
	pager := &tmpl.PaginatorContext{
		PageNumber: pageNum,
		URL:        fmt.Sprintf("/page/%d/", pageNum),
		Pages:      allItems[start:end],
		TotalPages: totalPages,
		PagerSize:  pageSize,
		HasPrev:    pageNum > 1,
		HasNext:    pageNum < totalPages,
	}
	// Provide non-nil Prev/Next to avoid nil-pointer derefs in templates that
	// access $paginator.Prev.URL unconditionally. Hugo returns a zero paginator
	// when at the boundary; we do similar by reusing the same pager with HasPrev/HasNext=false.
	if !pager.HasPrev {
		pager.Prev = pager
	} else {
		prevStart := (pageNum - 2) * pageSize
		prevEnd := prevStart + pageSize
		if prevEnd > len(allItems) {
			prevEnd = len(allItems)
		}
		pager.Prev = &tmpl.PaginatorContext{
			PageNumber: pageNum - 1,
			URL:        fmt.Sprintf("/page/%d/", pageNum-1),
			Pages:      allItems[prevStart:prevEnd],
			HasNext:    true,
		}
		if pager.Prev.PageNumber == 1 {
			pager.Prev.URL = "/"
		}
	}
	if !pager.HasNext {
		pager.Next = pager
	} else {
		nextStart := pageNum * pageSize
		nextEnd := nextStart + pageSize
		if nextEnd > len(allItems) {
			nextEnd = len(allItems)
		}
		pager.Next = &tmpl.PaginatorContext{
			PageNumber: pageNum + 1,
			URL:        fmt.Sprintf("/page/%d/", pageNum+1),
			Pages:      allItems[nextStart:nextEnd],
			HasPrev:    true,
		}
	}
	tmpl.SetPaginator(&ctx, pager)
	return &ctx
}

func DetectThemeName(sourceDir string) string {
	themesDir := filepath.Join(sourceDir, "themes")
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return e.Name()
		}
	}
	return ""
}

// URLEscape mirrors Hugo's urlize behavior for tag URLs:
//   - lowercase ASCII letters
//   - spaces become "-"
//   - CJK characters are preserved as-is (NOT URL-encoded)
//   - ASCII letters/digits are preserved (after lowercasing)
//   - other special chars (parens, etc.) are URL-encoded
func URLEscape(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
			r >= 0x3040 && r <= 0x309F, // Hiragana
			r >= 0x30A0 && r <= 0x30FF, // Katakana
			r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
			b.WriteRune(r)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '/':
			b.WriteRune(r)
		default:
			encoded := url.PathEscape(string(r))
			b.WriteString(encoded)
		}
	}
	return b.String()
}

// BuildTaxonomyContext creates the context for /tags/ (terms listing).
func BuildTaxonomyContext(siteCtx *tmpl.SiteContext, lookup map[*content.Page]*tmpl.Context, site *content.Site, cfg *config.Config) *tmpl.Context {
	tags, ok := site.Taxonomies["tags"]
	if !ok || len(tags) == 0 {
		return nil
	}

	// Sort terms by count desc, then alphabetical
	type termEntry struct {
		Name  string
		Pages tmpl.PageSlice
	}
	var entries []termEntry
	for term, pages := range tags {
		var ps tmpl.PageSlice
		for _, p := range pages {
			if c, ok := lookup[p]; ok {
				ps = append(ps, c)
			}
		}
		entries = append(entries, termEntry{Name: term, Pages: ps})
	}
	// Sort by count desc, then by name asc
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			if len(entries[j].Pages) > len(entries[j-1].Pages) ||
				(len(entries[j].Pages) == len(entries[j-1].Pages) && entries[j].Name < entries[j-1].Name) {
				entries[j], entries[j-1] = entries[j-1], entries[j]
			} else {
				break
			}
		}
	}

	dataTerms := make([]tmpl.TermSummaryExternal, 0, len(entries))
	for _, e := range entries {
		dataTerms = append(dataTerms, tmpl.TermSummaryExternal{
			Name:  e.Name,
			Pages: e.Pages,
			Count: len(e.Pages),
		})
	}

	// Build term-stub contexts for the taxonomy-list RSS. Hugo's /tags/index.xml
	// iterates .Pages (= .RegularPages for the taxonomy-list page), where each
	// item is a *term page* (one per term), not a regular content page.
	// Each stub carries: Kind=term, Title=term name, Permalink=/tags/{encoded}/,
	// and Date/Lastmod = the most recent page's date under that term. Hugo sorts
	// these by DefaultPageSort: Weight → Date desc → LinkTitle (site collator)
	// → Path. We mirror this for byte-exact RSS output.
	termStubs := make(tmpl.PageSlice, 0, len(entries))
	for _, e := range entries {
		// Hugo percent-encodes the term name in the permalink (e.g. 共识 →
		// %E5%85%B1%E8%AF%86) for XML/URL output, while keeping the raw
		// CJK in filesystem paths.
		encoded := url.PathEscape(e.Name)
		relURL := "/tags/" + encoded + "/"
		permURL := siteCtx.BaseURL + "tags/" + encoded + "/"
		// Effective date = newest Lastmod/Date among the term's pages
		var effective time.Time
		for _, item := range e.Pages {
			c := tmpl.AsCtx(item)
			if c == nil {
				continue
			}
			if c.Lastmod.After(effective) {
				effective = c.Lastmod
			}
			if c.Date.After(effective) {
				effective = c.Date
			}
		}
		stub := &tmpl.Context{
			Kind:         "term",
			Title:        e.Name,
			Date:         effective,
			Lastmod:      effective,
			RelPermalink: relURL,
			Permalink:    permURL,
			Site:         siteCtx,
		}
		termStubs = append(termStubs, stub)
	}
	// Sort stubs to mirror Hugo's DefaultPageSort:
	//   Weight (all 0 here) → Date desc → LinkTitle (collator asc) → Path asc
	// We use the site collator for the LinkTitle tiebreak.
	coll := i18n.BuildCollator(cfg.LanguageCode)
	sortedStubs := make(tmpl.PageSlice, len(termStubs))
	copy(sortedStubs, termStubs)
	for i := 1; i < len(sortedStubs); i++ {
		for j := i; j > 0; j-- {
			a := tmpl.AsCtx(sortedStubs[j])
			b := tmpl.AsCtx(sortedStubs[j-1])
			if a == nil || b == nil {
				break
			}
			// Date desc: newer sorts earlier
			if !a.Date.Equal(b.Date) {
				if a.Date.After(b.Date) {
					sortedStubs[j], sortedStubs[j-1] = sortedStubs[j-1], sortedStubs[j]
				}
				continue
			}
			// Date tie: LinkTitle collator asc
			if c := coll.CompareString(a.Title, b.Title); c < 0 {
				sortedStubs[j], sortedStubs[j-1] = sortedStubs[j-1], sortedStubs[j]
			}
		}
	}

	// Channel-level permalink for the taxonomy listing. siteCtx.BaseURL ends
	// with "/", so we concatenate without an extra slash to avoid "//".
	taxRelURL := "/tags/"
	taxPermURL := siteCtx.BaseURL + "tags/"

	return &tmpl.Context{
		Kind:        "taxonomy",
		Title:       "Tags",
		Site:        siteCtx,
		Data: &tmpl.DataAccessor{
			Terms:  tmpl.TermsList(dataTerms),
			Plural: "tags",
		},
		Scratch:       tmpl.NewScratch(),
		DataTerms:     dataTerms,
		RegularPages:  sortedStubs,
		Pages:         sortedStubs,
		RelPermalink:  taxRelURL,
		Permalink:     taxPermURL,
		OutputFormats: tmpl.DefaultPageOutputFormats(taxPermURL, taxRelURL),
	}
}

// BuildEmptyTaxonomyContext creates a context for an empty taxonomy listing
// (e.g., /categories/ when no categories are defined). Hugo generates these
// by default.
func BuildEmptyTaxonomyContext(siteCtx *tmpl.SiteContext, title, plural string) *tmpl.Context {
	relURL := "/" + plural + "/"
	permURL := siteCtx.BaseURL + plural + "/"
	return &tmpl.Context{
		Kind:          "taxonomy",
		Title:         title,
		Site:          siteCtx,
		Data: &tmpl.DataAccessor{
			Terms:  tmpl.TermsList{},
			Plural: plural,
		},
		Scratch:       tmpl.NewScratch(),
		RegularPages:  siteCtx.RegularPages,
		RelPermalink:  relURL,
		Permalink:     permURL,
		OutputFormats: tmpl.DefaultPageOutputFormats(permURL, relURL),
	}
}

// BuildTermContext creates the context for /tags/{tag}/ (single term page).
func BuildTermContext(siteCtx *tmpl.SiteContext, lookup map[*content.Page]*tmpl.Context, site *content.Site, cfg *config.Config, term string, pages tmpl.PageSlice) *tmpl.Context {
	relURL := "/tags/" + URLEscape(term) + "/"
	permURL := siteCtx.BaseURL + "tags/" + URLEscape(term) + "/"
	return &tmpl.Context{
		Kind:        "term",
		Title:       term,
		Site:        siteCtx,
		Data: &tmpl.DataAccessor{
			Pages:  pages,
			Plural: "tags",
		},
		Scratch:       tmpl.NewScratch(),
		RegularPages:  pages,
		Pages:         pages,
		RelPermalink:  relURL,
		Permalink:     permURL,
		OutputFormats: tmpl.DefaultPageOutputFormats(permURL, relURL),
	}
}

// ResolveTemplateName maps a page to its template using Hugo's lookup rules.
func ResolveTemplateName(tmpls *template.Template, p *content.Page) string {
	switch p.Kind {
	case "home":
		if t := tmpls.Lookup("index.html"); t != nil {
			return "index.html"
		}
		return "_default/list.html"

	case "section":
		// Hugo lookup order: {type}/list.html → {section}/list.html → _default/list.html
		if p.Type != "" {
			if t := tmpls.Lookup(p.Type+"/list.html"); t != nil {
				return p.Type + "/list.html"
			}
		}
		if p.Section != "" {
			if t := tmpls.Lookup(p.Section+"/list.html"); t != nil {
				return p.Section + "/list.html"
			}
		}
		if t := tmpls.Lookup("_default/list.html"); t != nil {
			return "_default/list.html"
		}
		return ""

	case "page":
		// Hugo lookup order: {type}/single.html → {section}/single.html → _default/single.html
		if p.Type != "" {
			if t := tmpls.Lookup(p.Type+"/single.html"); t != nil {
				return p.Type + "/single.html"
			}
		}
		if p.Section != "" {
			if t := tmpls.Lookup(p.Section+"/single.html"); t != nil {
				return p.Section + "/single.html"
			}
		}
		if t := tmpls.Lookup("_default/single.html"); t != nil {
			return "_default/single.html"
		}
		return ""

	default:
		return ""
	}
}
