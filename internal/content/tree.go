package content

import (
	"html/template"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/i18n"
)

// BuildTree takes raw pages and assembles the content tree:
//   - determines Kind (home, section, page)
//   - computes URL from file path
//   - determines Section
//   - builds parent/child relationships
//   - renders Markdown to HTML
//   - filters drafts
func BuildTree(pages []*Page, cfg *config.Config, sourceDir string) (*Site, error) {
	site := &Site{
		Title:    cfg.Title,
		BaseURL:  cfg.BaseURL,
		Language: cfg.LanguageCode,
		Params:   paramsToMap(cfg),
		Menus:    cfg.Menu,
		Config:   cfg,
		Sections: make(map[string]*Page),
		Data:     make(map[string]interface{}),
	}

	// First pass: compute URL, Kind, Section
	for _, p := range pages {
		computePageMeta(p)
	}

	// Second pass: build tree structure
	sectionMap := map[string]*Page{} // section path → section Page

	for _, p := range pages {
		// Only _index.md creates section pages. index.md is a leaf bundle page
		// (a regular page whose URL is its containing directory).
		base := filepath.Base(p.RelPath)
		if base == "_index.md" {
			p.Kind = "section"
			sectionDir := filepath.Dir(p.RelPath)
			if sectionDir == "." {
				p.Kind = "home"
			}
			sectionMap[sectionDir] = p
		}
	}

	// Hugo auto-creates a section page for every top-level content directory,
	// even when no _index.md is present. huan must do the same BEFORE parent
	// assignment so that pages nested under that directory (e.g.
	// posts/2026/05/2601.md) can find their section via the directory walk.
	// Without this, zhurongshuo's posts/ (no _index.md, organized as
	// year/month/day subdirs) would have no section page, and all its pages
	// would be parentless orphans — leaving posts/.RegularPagesRecursive empty.
	//
	// Skip leaf bundles: a directory with only an index.md (no _index.md and
	// no other content) is a leaf bundle, not a section.
	topLevelDirs := map[string]bool{}
	for _, p := range pages {
		if p.Kind == "page" {
			parts := strings.Split(filepath.ToSlash(p.RelPath), "/")
			if len(parts) > 1 {
				topLevelDirs[parts[0]] = true
			}
		}
	}
	for dir := range topLevelDirs {
		if _, exists := sectionMap[dir]; exists {
			continue // a section page (with _index.md) already covers this dir
		}
		// Check if this dir is a leaf bundle (only has index.md, no other pages)
		hasOtherContent := false
		for _, p := range pages {
			parts := strings.Split(filepath.ToSlash(p.RelPath), "/")
			if len(parts) > 1 && parts[0] == dir {
				base := parts[len(parts)-1]
				if base != "index.md" {
					hasOtherContent = true
					break
				}
			}
		}
		if !hasOtherContent {
			continue // skip creating section for leaf bundle
		}
		// Hugo title-cases the directory name when auto-creating a section
		// page (no _index.md present). Hyphens become spaces first, then
		// each word is title-cased: "posts" → "Posts", "my-notes" → "My Notes".
		sectionTitle := makeSectionTitle(dir)
		sectionPage := &Page{
			Title:   sectionTitle,
			Kind:    "section",
			Section: dir,
			URL:     "/" + dir + "/",
			RelPath: dir + "/_index.md",
		}
		computePageMeta(sectionPage)
		sectionMap[dir] = sectionPage
		pages = append(pages, sectionPage)
	}

	// Assign parents and children
	for _, p := range pages {
		if p.Kind == "home" {
			continue
		}

		dir := filepath.Dir(p.RelPath)
		base := filepath.Base(p.RelPath)

		if base == "_index.md" {
			// Section page: parent is the enclosing section (walk up until found)
			parentDir := filepath.Dir(dir)
			for parentDir != "." && parentDir != "" {
				if parent, ok := sectionMap[parentDir]; ok {
					p.Parent = parent
					break
				}
				parentDir = filepath.Dir(parentDir)
			}
		} else {
			// Regular page: parent is the nearest enclosing section.
			// Walk up the directory tree until a section _index.md is found.
			searchDir := dir
			for searchDir != "." && searchDir != "" {
				if section, ok := sectionMap[searchDir]; ok {
					p.Parent = section
					p.Kind = "page"
					break
				}
				searchDir = filepath.Dir(searchDir)
			}
		}
	}

	// Apply Hugo-style cascade inheritance: a section's _index.md may declare
	// `cascade.build.list: never` to exclude all descendant pages from lists
	// (RSS, sitemap, listings). Each page inherits its parent section's
	// Cascade.Build.List unless the page explicitly sets its own Build.List.
	// Walk up the parent chain so nested sections inherit from ancestors too.
	{
		// Index section pages by RelPath dir for cascade lookups.
		sectionByDir := map[string]*Page{} // e.g. "hidden" -> *_index.md page
		for _, p := range pages {
			if p.Kind == "section" {
				dir := filepath.Dir(p.RelPath)
				sectionByDir[dir] = p
			}
		}
		for _, p := range pages {
			if p.Kind != "page" {
				continue
			}
			if p.Build.List != "" {
				continue // page explicitly sets Build.List — don't inherit
			}
			// Walk up the page's directory chain looking for ancestor section
			// _index.md with a non-empty Cascade.Build.List.
			searchDir := filepath.Dir(p.RelPath)
			for searchDir != "." && searchDir != "" {
				if sec, ok := sectionByDir[searchDir]; ok {
					if sec.Cascade.Build.List != "" {
						p.Build.List = sec.Cascade.Build.List
						break
					}
				}
				searchDir = filepath.Dir(searchDir)
			}
		}
	}

	// Build section.Pages and section.RegularPages
	// Hugo's build.list=never directive: exclude from all lists (RSS, sitemap,
	// section listings, etc.). Pages inherit the directive via cascade above,
	// so we filter at the source — every list collection below then naturally
	// excludes them.
	isListed := func(p *Page) bool {
		return p.Build.List != "never"
	}
	for _, p := range pages {
		if p.Parent != nil {
			if p.Kind == "page" && !isListed(p) {
				// Excluded from lists: still link via tree (Parent), but skip
				// adding to any *Pages / RegularPages collection.
				continue
			}
			p.Parent.Pages = append(p.Parent.Pages, p)
			if p.Kind == "page" {
				p.Parent.RegularPages = append(p.Parent.RegularPages, p)
			}
			if p.Kind == "section" {
				p.Parent.Sections = append(p.Parent.Sections, p)
			}
		}
	}

	// Build the locale-aware collator once per build (construction is
	// expensive). Used by sortPagesDefault for the Title tie-break layer.
	coll := i18n.BuildCollator(cfg.LanguageCode)

	// Sort each section's Pages by Hugo's DefaultPageSort (weight → date → title → path)
	for _, p := range pages {
		if len(p.Pages) > 1 {
			sortPagesDefault(p.Pages, coll)
		}
		if len(p.RegularPages) > 1 {
			sortPagesDefault(p.RegularPages, coll)
		}
	}

	// Fill RegularPagesRecursive for every section (depth-first).
	// Done in a second pass so all direct children are populated first.
	for _, p := range pages {
		if p.Kind == "section" || p.Kind == "home" {
			p.RegularPagesRecursive = collectRegularPagesRecursive(p)
		}
	}

	// Collect all pages and regular pages for the site.
	// Apply build.list=never filter: never-listed pages are excluded from
	// site.RegularPages (and thus RSS, sitemap, related content).
	for _, p := range pages {
		site.Pages = append(site.Pages, p)
		if p.Kind == "page" && p.Build.List != "never" {
			site.RegularPages = append(site.RegularPages, p)
		}
	}

	// WordCount and Plain are computed in main.go after Markdown rendering
	// (from plainified HTML, matching Hugo). Don't recompute here.
	// Sort site.RegularPages by Hugo's DefaultPageSort (weight → date → title → path)
	if len(site.RegularPages) > 1 {
		sortPagesDefault(site.RegularPages, coll)
	}

	// Ensure a home page exists. Hugo creates a virtual home page even when
	// content/_index.md is missing, so templates that range over .Site.Pages
	// or that need a home context (e.g., for search.json) still work.
	homeExists := false
	for _, p := range site.Pages {
		if p.Kind == "home" {
			homeExists = true
			break
		}
	}
	if !homeExists {
		homePage := &Page{
			Title:  site.Title,
			Kind:   "home",
			URL:    "/",
			RelPath: "_index.md",
		}
		homePage.RegularPages = site.RegularPages
		homePage.Pages = append(homePage.Pages, site.RegularPages...)
		for _, sp := range site.Sections {
			homePage.Pages = append(homePage.Pages, sp)
			sp.Parent = homePage
		}
		homePage.RegularPagesRecursive = site.RegularPages
		site.Pages = append([]*Page{homePage}, site.Pages...)
	}

	// Build section map
	for dir, p := range sectionMap {
		if dir == "." {
			continue
		}
		sectionName := strings.Split(filepath.ToSlash(dir), "/")[0]
		site.Sections[sectionName] = p
	}

	return site, nil
}

// computeSummary returns the page summary as Hugo does: HTML content up to
// <!--more--> divider, or the first ~70 words rendered as HTML.
// Hugo's default summary is HTML (with <p> tags), not plain text.
func computeSummary(rawContent, renderedHTML string) template.HTML {
	if idx := strings.Index(rawContent, "<!--more-->"); idx >= 0 {
		// We don't have access to the markdown renderer here easily,
		// so we return the plain text version. This is a fallback.
		before := rawContent[:idx]
		stripped := stripMarkdownForCount(before)
		return template.HTML("<p>" + strings.TrimSpace(stripped) + "</p>")
	}

	// Truncate rendered HTML to ~70 words.
	// Hugo's default summaryLength is 70 (words), but zhurongshuo sets 120.
	// We approximate by truncating the rendered HTML.
	// For RSS, the template often uses .Summary | plainify, so HTML tags survive.
	return template.HTML(renderedHTML)
}

// stripMarkdownForCount removes markdown syntax to approximate the page's plain
// text content for word counting. Hugo's WordCount runs on plainified HTML
// (rendered HTML with tags stripped), so code block content IS counted.
func stripMarkdownForCount(src string) string {
	// Remove code fence markers but keep their content (Hugo counts code text).
	s := strings.ReplaceAll(src, "```", "")

	// Strip common markdown markers
	replacers := []struct{ from, to string }{
		{"#", ""},
		{"*", ""},
		{"_", ""},
		{"`", ""},
		{"[", ""},
		{"]", ""},
		{"(", ""},
		{")", ""},
		{">", ""},
		{"-", ""},
	}
	for _, r := range replacers {
		s = strings.ReplaceAll(s, r.from, r.to)
	}
	return s
}

// countWordsCJK counts words in mixed CJK + ASCII text.
// Hugo counts each CJK character as one word; ASCII words are space-separated.
func countWordsCJK(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			count++
			inWord = false
		} else if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
	}
	_ = utf8.RuneCountInString // keep import
	return count
}

// collectRegularPagesRecursive walks a section's descendants and returns all
// regular pages beneath it, in document order (matches Hugo's RegularPagesRecursive).
func collectRegularPagesRecursive(section *Page) []*Page {
	var result []*Page
	// Direct regular pages first
	for _, p := range section.RegularPages {
		if !p.Draft {
			result = append(result, p)
		}
	}
	// Recurse into sub-sections
	for _, sub := range section.Sections {
		result = append(result, collectRegularPagesRecursive(sub)...)
	}
	return result
}

// makeSectionTitle produces a Hugo-compatible display title from a section
// directory name when no _index.md is present. Hugo replaces hyphens with
// spaces and title-cases each word: "posts" → "Posts", "my-notes" → "My Notes".
// Underscores are also treated as word separators to match Hugo's path
// sanitization. The caser uses the English locale by default (Hugo's default
// for title-casing auto-generated section titles).
func makeSectionTitle(dir string) string {
	if dir == "" {
		return dir
	}
	// Replace path separators with spaces.
	name := strings.NewReplacer("-", " ", "_", " ").Replace(dir)
	caser := cases.Title(language.English)
	return caser.String(name)
}

// sortPagesDefault sorts pages using Hugo's DefaultPageSort algorithm
// (resources/page/pages_sort.go:DefaultPageSort). The tie-break chain is:
//
//  1. Weight ascending — but weight 0 sorts last (Hugo's "unweighted goes last" rule)
//  2. DateParsed descending
//  3. Collator on Title ascending (Hugo uses LinkTitle with Title fallback;
//     zhurongshuo has no LinkTitle so Title is used directly). Language-aware
//     (zh-cn → pinyin order, en → alphabetical, etc.)
//  4. RelPath ascending (byte-level, NOT through collator)
//
// Note: Hugo also has Ordinal and Weight0 (taxonomy term weight) layers
// earlier in the chain, but huan doesn't model those concepts. zhurongshuo
// pages have no Ordinal / Weight0 set, so omitting those layers is safe.
//
// The Collator must be built by the caller (typically once per build via
// i18n.BuildCollator(site.LanguageCode)) and reused — collator construction
// is expensive.
func sortPagesDefault(pages []*Page, coll *collate.Collator) {
	sort.SliceStable(pages, func(i, j int) bool {
		a, b := pages[i], pages[j]

		// Layer 1: Weight (0 sorts last; otherwise ascending)
		if a.Weight == 0 && b.Weight != 0 {
			return false // a sorts after b
		}
		if a.Weight != 0 && b.Weight == 0 {
			return true // a sorts before b
		}
		if a.Weight != b.Weight {
			return a.Weight < b.Weight
		}

		// Layer 2: Date desc
		if !a.DateParsed.Equal(b.DateParsed) {
			return a.DateParsed.After(b.DateParsed)
		}

		// Layer 3: Collator on Title asc
		if c := coll.CompareString(a.Title, b.Title); c != 0 {
			return c < 0
		}

		// Layer 4: RelPath asc (byte-level)
		return a.RelPath < b.RelPath
	})
}

// computePageMeta sets Kind, URL, Section from the file path.
func computePageMeta(p *Page) {
	rel := filepath.ToSlash(p.RelPath)
	base := filepath.Base(rel)
	dir := filepath.Dir(rel)

	// _index.md is a section/home page.
	// index.md at root is also home; index.md in subdir is a leaf bundle page.
	// Everything else is a regular page.
	if base == "_index.md" {
		if dir == "." {
			p.Kind = "home"
		} else {
			p.Kind = "section"
		}
	} else if base == "index.md" && dir == "." {
		p.Kind = "home"
	} else {
		p.Kind = "page"
	}

	// Determine Section (first path component).
	// Hugo: leaf bundle (index.md in subdir) has empty Section because it IS
	// the section's main page, not a child of a separate section.
	parts := strings.Split(rel, "/")
	if len(parts) > 1 && base != "index.md" {
		p.Section = parts[0]
	} else if len(parts) > 0 && base == "_index.md" {
		// _index.md in subdir: section name is its directory
		if len(parts) > 1 {
			p.Section = parts[0]
		}
	}
	// Home page has no section
	if p.Kind == "home" {
		p.Section = ""
	}

	// Compute URL
	p.URL = computeURL(p)
}

// computeURL derives the page URL from its file path, matching Hugo's behavior.
//
// Rules:
//   - _index.md at root → "/"
//   - _index.md in subdir → "/{subdir}/"
//   - index.md in subdir → "/{subdir}/" (leaf bundle: page rendered at directory URL)
//   - regular .md → strip extension, use path as URL
//   - If slug is set in frontmatter, use slug as the last segment
func computeURL(p *Page) string {
	rel := filepath.ToSlash(p.RelPath)
	base := filepath.Base(rel)
	dir := filepath.Dir(rel)

	// _index.md → directory URL
	if base == "_index.md" {
		if dir == "." {
			return "/"
		}
		return "/" + dir + "/"
	}

	// index.md (leaf bundle) → directory URL
	if base == "index.md" {
		if dir == "." {
			return "/"
		}
		return "/" + dir + "/"
	}

	// Use slug if set, otherwise use filename without extension
	lastSegment := strings.TrimSuffix(base, ".md")
	if p.Slug != "" {
		lastSegment = p.Slug
	}

	if dir == "." {
		return "/" + lastSegment + "/"
	}

	return "/" + dir + "/" + lastSegment + "/"
}

// paramsToMap converts Config.Params to map[string]interface{} for template access.
// Hugo templates use both lowercase (.Params.subTitle) and PascalCase
// (.Params.SubTitle, .Params.Author.name) interchangeably. We expose both forms.
func paramsToMap(cfg *config.Config) map[string]interface{} {
	// Build social list as []map (sortable via "sort" template function by weight)
	socialLower := []interface{}{}
	socialPascal := []interface{}{}
	for _, s := range cfg.Social {
		entry := map[string]interface{}{
			"name":   s.Name,
			"url":    s.URL,
			"weight": s.Weight,
		}
		socialLower = append(socialLower, entry)
		socialPascal = append(socialPascal, entry)
	}

	m := map[string]interface{}{
		"subTitle":        cfg.Params.SubTitle,
		"subtitle":        cfg.Params.SubTitle,
		"SubTitle":        cfg.Params.SubTitle,
		"Subtitle":        cfg.Params.SubTitle,
		"SUBTITLE":        cfg.Params.SubTitle,
		"footerSlogan":    cfg.Params.FooterSlogan,
		"FooterSlogan":    cfg.Params.FooterSlogan,
		"keywords":        cfg.Params.Keywords,
		"description":     cfg.Params.Description,
		"copyrights":      cfg.Params.Copyrights,
		"enableMathJax":   cfg.Params.EnableMathJax,
		"enableSummary":   cfg.Params.EnableSummary,
		"mainSections":    cfg.Params.MainSections,
		"googleAnalytics": cfg.Params.GoogleAnalytics,
		"cdnURL":          cfg.Params.CDNURL,
		"author":          map[string]interface{}{"name": cfg.Params.Author.Name},
		"Author":          map[string]interface{}{"name": cfg.Params.Author.Name},
		"social":          socialLower,
		"Social":          socialPascal,
	}

	if cfg.Params.EncryptGroups != nil {
		groupsLower := make(map[string]interface{})
		groupsPascal := make(map[string]interface{})
		for k, v := range cfg.Params.EncryptGroups {
			entry := map[string]interface{}{
				"hint":  v.Hint,
				"mode":  v.Mode,
				"ratio": v.Ratio,
			}
			groupsLower[k] = entry
			groupsPascal[k] = entry
		}
		m["encryptGroups"] = groupsLower
		m["EncryptGroups"] = groupsPascal
	}

	return m
}
