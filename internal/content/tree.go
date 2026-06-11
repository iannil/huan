package content

import (
	"path/filepath"
	"strings"

	"github.com/novel_ttl/huan/internal/config"
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
		// Handle _index.md pages (section indexes)
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

	// Assign parents and children
	for _, p := range pages {
		if p.Kind == "home" {
			continue
		}

		dir := filepath.Dir(p.RelPath)
		base := filepath.Base(p.RelPath)

		if base == "_index.md" {
			// Section page: parent is the enclosing section
			parentDir := filepath.Dir(dir)
			if parentDir != "." {
				if parent, ok := sectionMap[parentDir]; ok {
					p.Parent = parent
				}
			}
		} else {
			// Regular page: parent is the section it lives in
			if section, ok := sectionMap[dir]; ok {
				p.Parent = section
				p.Kind = "page"
			}
		}
	}

	// Build section.Pages and section.RegularPages
	for _, p := range pages {
		if p.Parent != nil {
			p.Parent.Pages = append(p.Parent.Pages, p)
			if p.Kind == "page" {
				p.Parent.RegularPages = append(p.Parent.RegularPages, p)
			}
			if p.Kind == "section" {
				p.Parent.Sections = append(p.Parent.Sections, p)
			}
		}
	}

	// Sort each section's Pages by Date descending (Hugo default order)
	for _, p := range pages {
		if len(p.Pages) > 1 {
			sortPagesByDateDesc(p.Pages)
		}
		if len(p.RegularPages) > 1 {
			sortPagesByDateDesc(p.RegularPages)
		}
	}

	// Fill RegularPagesRecursive for every section (depth-first).
	// Done in a second pass so all direct children are populated first.
	for _, p := range pages {
		if p.Kind == "section" || p.Kind == "home" {
			p.RegularPagesRecursive = collectRegularPagesRecursive(p)
		}
	}

	// Collect all pages and regular pages for the site
	for _, p := range pages {
		site.Pages = append(site.Pages, p)
		if p.Kind == "page" {
			site.RegularPages = append(site.RegularPages, p)
		}
	}
	// Sort site.RegularPages by Date descending (Hugo default)
	if len(site.RegularPages) > 1 {
		sortPagesByDateDesc(site.RegularPages)
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

// sortPagesByDateDesc sorts pages by Date descending (newest first).
func sortPagesByDateDesc(pages []*Page) {
	for i := 1; i < len(pages); i++ {
		for j := i; j > 0; j-- {
			if pages[j].DateParsed.After(pages[j-1].DateParsed) {
				pages[j], pages[j-1] = pages[j-1], pages[j]
			} else {
				break
			}
		}
	}
}

// computePageMeta sets Kind, URL, Section from the file path.
func computePageMeta(p *Page) {
	rel := filepath.ToSlash(p.RelPath)
	base := filepath.Base(rel)
	dir := filepath.Dir(rel)

	// Determine Kind
	if base == "_index.md" {
		if dir == "." {
			p.Kind = "home"
		} else {
			p.Kind = "section"
		}
	} else {
		p.Kind = "page"
	}

	// Determine Section (first path component)
	parts := strings.Split(rel, "/")
	if len(parts) > 0 {
		p.Section = parts[0]
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
//   - _index.md in subdir → "/{subdir}/" (e.g., "posts/_index.md" → "/posts/")
//   - regular .md → strip extension, use path as URL
//     (e.g., "posts/2020/08/2601.md" → "/posts/2020/08/2601/")
//   - If slug is set in frontmatter, use slug as the last segment
func computeURL(p *Page) string {
	rel := filepath.ToSlash(p.RelPath)
	base := filepath.Base(rel)
	dir := filepath.Dir(rel)

	if base == "_index.md" {
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
func paramsToMap(cfg *config.Config) map[string]interface{} {
	m := map[string]interface{}{
		"subTitle":       cfg.Params.SubTitle,
		"footerSlogan":   cfg.Params.FooterSlogan,
		"keywords":       cfg.Params.Keywords,
		"description":    cfg.Params.Description,
		"copyrights":     cfg.Params.Copyrights,
		"enableMathJax":  cfg.Params.EnableMathJax,
		"enableSummary":  cfg.Params.EnableSummary,
		"mainSections":   cfg.Params.MainSections,
		"googleAnalytics": cfg.Params.GoogleAnalytics,
		"cdnURL":         cfg.Params.CDNURL,
		"author":         map[string]string{"name": cfg.Params.Author.Name},
	}

	if cfg.Params.EncryptGroups != nil {
		groups := make(map[string]interface{})
		for k, v := range cfg.Params.EncryptGroups {
			groups[k] = map[string]interface{}{
				"hint":  v.Hint,
				"mode":  v.Mode,
				"ratio": v.Ratio,
			}
		}
		m["encryptGroups"] = groups
	}

	return m
}
