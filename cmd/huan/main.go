package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	neturl "net/url"

	"github.com/novel_ttl/huan/internal/config"
	"github.com/novel_ttl/huan/internal/content"
	"github.com/novel_ttl/huan/internal/encrypt"
	"github.com/novel_ttl/huan/internal/i18n"
	"github.com/novel_ttl/huan/internal/markdown"
	"github.com/novel_ttl/huan/internal/output"
	"github.com/novel_ttl/huan/internal/shortcode"
	"github.com/novel_ttl/huan/internal/taxonomy"
	tmpl "github.com/novel_ttl/huan/internal/template"
	"github.com/spf13/cobra"
)

var sourceDir string

func main() {
	rootCmd := &cobra.Command{
		Use:   "huan",
		Short: "A static site generator",
		Long:  "huan is a static site generator written in Go, designed to replace Hugo for zhurongshuo.com.",
	}

	rootCmd.PersistentFlags().StringVarP(&sourceDir, "source", "s", ".", "source directory containing huan.yaml and content/")

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build the site",
		RunE:  runBuild,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the development server",
		RunE:  runServe,
	}

	serveCmd.Flags().String("port", "1313", "port to serve on")

	rootCmd.AddCommand(buildCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runBuild(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("Building site: %s\n", cfg.Title)
	fmt.Printf("  Source:      %s\n", sourceDir)
	fmt.Printf("  Output:      %s\n", cfg.PublishDir)
	fmt.Printf("  BaseURL:     %s\n", cfg.BaseURL)

	// 1. Load content
	contentDir := filepath.Join(sourceDir, "content")
	pages, err := content.LoadDir(contentDir)
	if err != nil {
		return fmt.Errorf("load content: %w", err)
	}
	fmt.Printf("  Pages loaded: %d\n", len(pages))

	// 2. Load data files
	dataDir := filepath.Join(sourceDir, "data")
	data, err := content.LoadDataFiles(dataDir)
	if err != nil {
		return fmt.Errorf("load data: %w", err)
	}
	fmt.Printf("  Data files:   %d\n", len(data))

	// 3. Set up shortcode registry and markdown renderer
	scRegistry := shortcode.NewRegistry()
	md := markdown.NewRenderer(&cfg.Markup)

	// 4. Render Markdown for each page (with shortcode expansion first)
	for _, p := range pages {
		if p.RawContent == "" {
			continue
		}

		// Expand shortcodes BEFORE markdown rendering
		expanded, err := scRegistry.Expand(p.RawContent, p, nil)
		if err != nil {
			return fmt.Errorf("shortcode %s: %w", p.RelPath, err)
		}

		html, err := md.Render(expanded)
		if err != nil {
			return fmt.Errorf("render %s: %w", p.RelPath, err)
		}
		p.Content = template.HTML(html)

		// Compute Plain and WordCount from rendered HTML (matches Hugo's
		// behavior of counting words in plainified HTML).
		plain := stripHTMLTagsForSummary(html)
		p.Plain = plain
		p.WordCount = countWordsInPlain(plain)

		// Compute Summary from rendered HTML: content up to <!--more-->, else
		// the first ~120 words (Hugo's default summaryLength).
		if idx := strings.Index(p.RawContent, "<!--more-->"); idx >= 0 {
			before := p.RawContent[:idx]
			beforeHTML, err := md.Render(before)
			if err == nil {
				p.Summary = template.HTML(beforeHTML)
			}
		} else {
			content := string(p.Content)
			p.Summary = template.HTML(truncateHTMLByWords(content, 120))
		}
	}

	// 5. Build content tree
	site, err := content.BuildTree(pages, cfg, sourceDir)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}
	site.Data = data
	for _, p := range site.Pages {
		_ = p
	}

	// 6. Set up encrypt engine and apply access control to protected pages
	encryptGroups := map[string]encrypt.EncryptGroupConfig{}
	for name, g := range cfg.Params.EncryptGroups {
		encryptGroups[name] = encrypt.EncryptGroupConfig{
			Hint:  g.Hint,
			Mode:  g.Mode,
			Ratio: g.Ratio,
		}
	}

	var encryptedContent interface{}
	if enc, ok := data["encrypted"]; ok {
		if m, ok := enc.(map[string]interface{}); ok {
			encryptedContent = m["content"]
		}
	}

	encEngine := encrypt.NewEngine(encryptedContent, encryptGroups)
	protectedCount := 0
	for _, p := range pages {
		if p.Access == "protected" {
			protectedCount++
		}
		content, err := encEngine.Render(p, scRegistry, site)
		if err != nil {
			return fmt.Errorf("encrypt %s: %w", p.RelPath, err)
		}
		p.Content = content
	}

	// 7. Build taxonomies
	taxonomies := taxonomy.BuildAll(pages)
	taxCount := 0
	if tax, ok := taxonomies["tags"]; ok {
		taxCount = len(tax)
	}
	fmt.Printf("  Tags:         %d unique\n", taxCount)
	site.Taxonomies = map[string]content.Taxonomy{}
	for name, tax := range taxonomies {
		converted := content.Taxonomy{}
		for term, pages := range tax {
			converted[term] = pages
		}
		site.Taxonomies[name] = converted
	}

	// 8. Load templates
	tmpls, err := tmpl.LoadAllTemplates(sourceDir, cfg.BaseURL)
	if err != nil {
		return fmt.Errorf("load templates: %w", err)
	}

	// Load i18n bundles (theme first, then project overrides).
	i18nBundle := i18n.New()
	themeI18nDir := filepath.Join(sourceDir, "themes", detectThemeName(sourceDir), "i18n")
	if _, err := os.Stat(themeI18nDir); err == nil {
		_ = i18nBundle.LoadDir(themeI18nDir)
	}
	projectI18nDir := filepath.Join(sourceDir, "i18n")
	if _, err := os.Stat(projectI18nDir); err == nil {
		_ = i18nBundle.LoadDir(projectI18nDir)
	}
	tmpl.SetI18nBundle(i18nBundle)

	// Count loaded templates
	templateCount := 0
	for range tmpls.Templates() {
		templateCount++
	}
	fmt.Printf("  Templates:    %d\n", templateCount)
	if protectedCount > 0 {
		fmt.Printf("  Protected:    %d pages\n", protectedCount)
	}

	// 9. Render pages
	renderer := tmpl.NewRenderer(tmpls, tmpl.FuncMap(cfg.BaseURL))
	var writer *output.Writer
	if cfg.Minify {
		writer = output.NewWriterWithMinify(filepath.Join(sourceDir, cfg.PublishDir))
	} else {
		writer = output.NewWriter(filepath.Join(sourceDir, cfg.PublishDir))
	}
	// Enable canonifyURLs to rewrite root-relative paths to absolute URLs.
	writer.SetCanonify(output.CanonifyOptions{BaseURL: cfg.BaseURL})

	// Build shared site context and page→context lookup
	siteCtx := tmpl.NewSiteContext(site, cfg)

	lookup := map[*content.Page]*tmpl.Context{}
	for _, p := range site.Pages {
		lookup[p] = tmpl.NewContext(p, siteCtx, cfg)
	}
	for _, p := range site.Pages {
		if ctx, ok := lookup[p]; ok {
			tmpl.LinkPageRelationships(ctx, p, lookup)
		}
	}
	tmpl.PopulateSitePages(siteCtx, site, lookup)

	renderedCount := 0
	errors := 0
	for _, p := range site.Pages {
		if p.Draft {
			continue
		}
		if p.Build.Render == "never" {
			continue
		}

		tmplName := resolveTemplateName(tmpls, p)
		if tmplName == "" {
			continue
		}

		ctx := lookup[p]
		if ctx == nil {
			continue
		}

		// For section/list rendering, expose pages via .Data.Pages
		if p.Kind == "section" || p.Kind == "home" {
			ctx.Data = &tmpl.DataAccessor{
				Pages: ctx.RegularPages,
			}
		}

		html, err := renderer.Render(tmplName, ctx)
		if err != nil {
			fmt.Printf("  WARN: render %s with %s: %v\n", p.RelPath, tmplName, err)
			errors++
			continue
		}

		outPath := output.URLToFilePath(p.URL, "")
		if err := writer.Write(outPath, html); err != nil {
			fmt.Printf("  WARN: write %s: %v\n", p.URL, err)
			errors++
			continue
		}
		renderedCount++

		// RSS output for home and section pages
		if p.Kind == "home" || p.Kind == "section" {
			if rssName := resolveRSSOutput(p); rssName != "" {
				if rssHTML, err := renderer.Render(rssName, ctx); err == nil {
					rssPath := strings.TrimSuffix(p.URL, "/") + "/index.xml"
					rssPath = strings.TrimPrefix(rssPath, "/")
					if err := writer.Write(rssPath, rssHTML); err != nil {
						fmt.Printf("  WARN: write RSS %s: %v\n", p.URL, err)
					}
				} else {
					fmt.Printf("  WARN: render RSS %s: %v\n", p.URL, err)
				}
			}
		}
	}

	// Generate taxonomy term pages: /tags/ and /tags/{tag}/
	if taxCtx := buildTaxonomyContext(siteCtx, lookup, site, cfg); taxCtx != nil {
		// /tags/ - the terms listing page
		if html, err := renderer.Render("_default/terms.html", taxCtx); err == nil {
			writer.Write("tags/index.html", html)
		} else {
			fmt.Printf("  WARN: terms: %v\n", err)
		}

		// /tags/index.xml - RSS for tags listing
		if html, err := renderer.Render("_default/rss.xml", taxCtx); err == nil {
			writer.Write("tags/index.xml", html)
		}

		// /tags/{tag}/ - one page per term (HTML + RSS)
		if termsTmpl := tmpls.Lookup("_default/list.html"); termsTmpl != nil {
			for _, term := range taxCtx.DataTerms {
				termCtx := buildTermContext(siteCtx, lookup, site, cfg, term.Name, term.Pages)
				if termCtx != nil {
					tagSlug := urlEscape(term.Name)
					if html, err := renderer.Render("_default/list.html", termCtx); err == nil {
						writer.Write("tags/"+tagSlug+"/index.html", html)
					}
					if html, err := renderer.Render("_default/rss.xml", termCtx); err == nil {
						writer.Write("tags/"+tagSlug+"/index.xml", html)
					}
				}
			}
		}
	}

	// Generate empty /categories/ taxonomy (Hugo default, even when unused).
	if catCtx := buildEmptyTaxonomyContext(siteCtx, "Categories", "categories"); catCtx != nil {
		if html, err := renderer.Render("_default/terms.html", catCtx); err == nil {
			writer.Write("categories/index.html", html)
		}
		if html, err := renderer.Render("_default/rss.xml", catCtx); err == nil {
			writer.Write("categories/index.xml", html)
		}
	}

	// Generate paginated home pages: /page/2/, /page/3/, etc.
	// Hugo's default mainSections pagination: site.RegularPages filtered to mainSections.
	// /page/1/ is generated as a redirect to / (Hugo alias behavior).
	if homeCtx := findHomeContext(lookup, site); homeCtx != nil {
		mainPageItems := filterMainSections(siteCtx.RegularPages, cfg.Params.MainSections)
		pageSize := cfg.Paginate
		if pageSize <= 0 {
			pageSize = 10
		}
		totalPages := (len(mainPageItems) + pageSize - 1) / pageSize

		// /page/1/ is a redirect to /. Hugo emits this exact minified form.
		homeURL := cfg.BaseURL
		redirect := fmt.Sprintf(`<!doctype html><html lang=%s><head><title>%s</title><link rel=canonical href=%s><meta charset=utf-8><meta http-equiv=refresh content="0; url=%s"></head></html>`,
			cfg.LanguageCode, homeURL, homeURL, homeURL)
		// Bypass minify/canonify for this pre-formatted redirect.
		if err := writer.WriteBytesPath("page/1/index.html", []byte(redirect)); err != nil {
			fmt.Printf("  WARN: write page/1: %v\n", err)
		}

		// /page/2/, /page/3/, ... are actual paginated home pages
		for i := 2; i <= totalPages; i++ {
			pagedCtx := cloneContextForPagination(homeCtx, mainPageItems, pageSize, i, totalPages)
			html, err := renderer.Render("index.html", pagedCtx)
			if err != nil {
				continue
			}
			_ = writer.Write(fmt.Sprintf("page/%d/index.html", i), html)
		}
	}

	// Generate 404 page
	if t := tmpls.Lookup("404.html"); t != nil {
		ctx404 := &tmpl.Context{
			Kind:          "404",
			Title:         "404 Page not found",
			Site:          siteCtx,
			Data:          siteCtx.Data,
			Scratch:       tmpl.NewScratch(),
			RelPermalink:  "/404.html",
			Permalink:     siteCtx.BaseURL + "404.html",
			OutputFormats: tmpl.HTMLOnlyOutputFormats(siteCtx.BaseURL+"404.html", "/404.html"),
		}
		if html, err := renderer.Render("404.html", ctx404); err == nil {
			_ = writer.Write("404.html", html)
		}
	}

	// Generate sitemap.xml
	if siteMapCtx := buildSitemapContext(siteCtx, lookup, site, cfg); siteMapCtx != nil {
		if html, err := renderer.Render("_default/sitemap.xml", siteMapCtx); err == nil {
			writer.Write("sitemap.xml", html)
		} else {
			fmt.Printf("  WARN: sitemap: %v\n", err)
		}
	}

	// Generate search.json from home page context
	if homeCtx := findHomeContext(lookup, site); homeCtx != nil {
		if html, err := renderer.Render("_default/index.searchindex.json", homeCtx); err == nil {
			if werr := writer.Write("search.json", html); werr != nil {
				fmt.Printf("  WARN: write search.json: %v\n", werr)
			}
		} else {
			fmt.Printf("  WARN: search: %v\n", err)
		}
	}

	// Copy static assets: theme static first, then project static (overrides)
	themeName := detectThemeName(sourceDir)
	if themeName != "" {
		themeStaticDir := filepath.Join(sourceDir, "themes", themeName, "static")
		if _, err := os.Stat(themeStaticDir); err == nil {
			if err := writer.CopyStatic(themeStaticDir); err != nil {
				fmt.Printf("  WARN: theme static: %v\n", err)
			}
		}
	}
	staticDir := filepath.Join(sourceDir, "static")
	if err := writer.CopyStatic(staticDir); err != nil {
		fmt.Printf("  WARN: static: %v\n", err)
	}

	files, bytes := writer.Stats()
	fmt.Printf("  Rendered:     %d pages\n", renderedCount)
	fmt.Printf("  Output:       %d files, %.1f KB\n", files, float64(bytes)/1024)
	if errors > 0 {
		fmt.Printf("  Errors:       %d\n", errors)
	}

	fmt.Println("Build complete.")
	return nil
}

// resolveRSSOutput returns the RSS template name for a section/home page.
func resolveRSSOutput(p *content.Page) string {
	// Hugo uses _default/rss.xml for all sections
	return "_default/rss.xml"
}

// buildSitemapContext creates a root context for sitemap rendering.
// The sitemap template iterates .Pages, so we need a context with all pages populated.
func buildSitemapContext(siteCtx *tmpl.SiteContext, lookup map[*content.Page]*tmpl.Context, site *content.Site, cfg *config.Config) *tmpl.Context {
	ctx := &tmpl.Context{
		Kind:   "home",
		Site:   siteCtx,
		Params: map[string]interface{}{},
		Data:   siteCtx.Data,
	}
	ctx.Pages = siteCtx.Pages
	return ctx
}

// findHomeContext returns the home page context, used for search index rendering.
func findHomeContext(lookup map[*content.Page]*tmpl.Context, site *content.Site) *tmpl.Context {
	for _, p := range site.Pages {
		if p.Kind == "home" {
			return lookup[p]
		}
	}
	return nil
}

// filterMainSections returns pages whose Section is in mainSections.
func filterMainSections(pages tmpl.PageSlice, mainSections []string) tmpl.PageSlice {
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

// cloneContextForPagination creates a shallow copy of homeCtx with a Paginator
// pointing at page N. The URL stays "/" so meta tags match Hugo's behavior.
func cloneContextForPagination(homeCtx *tmpl.Context, allItems tmpl.PageSlice, pageSize, pageNum, totalPages int) *tmpl.Context {
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
// truncateHTMLByWords truncates HTML content to approximately N "words"
// (CJK chars count as 1 word each, ASCII words split by whitespace).
// It walks the HTML tracking word count and cuts at the next </p> boundary
// after reaching N words, matching Hugo's summary behavior.
func truncateHTMLByWords(htmlStr string, n int) string {
	if n <= 0 {
		return htmlStr
	}
	count := 0
	inTag := false
	inWord := false
	for i := 0; i < len(htmlStr); i++ {
		c := htmlStr[i]
		if inTag {
			if c == '>' {
				inTag = false
			}
			continue
		}
		if c == '<' {
			inTag = true
			inWord = false
			continue
		}
		// ASCII byte check
		if c >= 0x80 {
			// CJK / multi-byte char: count as 1 word
			if c&0xC0 != 0x80 {
				count++
				inWord = false
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
		if count >= n {
			// Find next </p> and cut there
			rest := htmlStr[i+1:]
			closeIdx := strings.Index(rest, "</p>")
			if closeIdx >= 0 {
				return htmlStr[:i+1+closeIdx+len("</p>")]
			}
			return htmlStr[:i+1]
		}
	}
	return htmlStr
}

// stripHTMLTagsForSummary strips HTML tags for plain text summary.
func stripHTMLTagsForSummary(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// countWordsInPlain counts words in plain text using Hugo's algorithm:
// each CJK character counts as 1 word; ASCII words (split by whitespace) count as 1.
func countWordsInPlain(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			count++
			inWord = false
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}

func detectThemeName(sourceDir string) string {
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

// urlEscape mirrors Hugo's urlize behavior for tag URLs:
//   - lowercase ASCII letters
//   - spaces become "-"
//   - CJK characters are preserved as-is (NOT URL-encoded)
//   - ASCII letters/digits are preserved (after lowercasing)
//   - other special chars (parens, etc.) are URL-encoded
func urlEscape(s string) string {
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
			encoded := neturl.PathEscape(string(r))
			b.WriteString(encoded)
		}
	}
	return b.String()
}

// buildTaxonomyContext creates the context for /tags/ (terms listing).
func buildTaxonomyContext(siteCtx *tmpl.SiteContext, lookup map[*content.Page]*tmpl.Context, site *content.Site, cfg *config.Config) *tmpl.Context {
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

	return &tmpl.Context{
		Kind:        "taxonomy",
		Title:       "Tags",
		Site:        siteCtx,
		Data: &tmpl.DataAccessor{
			Terms:  tmpl.TermsList(dataTerms),
			Plural: "tags",
		},
		Scratch:      tmpl.NewScratch(),
		DataTerms:    dataTerms,
		RegularPages: siteCtx.RegularPages,
		OutputFormats: tmpl.DefaultPageOutputFormats(siteCtx.BaseURL+"/tags/", "/tags/"),
	}
}

// buildEmptyTaxonomyContext creates a context for an empty taxonomy listing
// (e.g., /categories/ when no categories are defined). Hugo generates these
// by default.
func buildEmptyTaxonomyContext(siteCtx *tmpl.SiteContext, title, plural string) *tmpl.Context {
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

// buildTermContext creates the context for /tags/{tag}/ (single term page).
func buildTermContext(siteCtx *tmpl.SiteContext, lookup map[*content.Page]*tmpl.Context, site *content.Site, cfg *config.Config, term string, pages tmpl.PageSlice) *tmpl.Context {
	relURL := "/tags/" + urlEscape(term) + "/"
	permURL := siteCtx.BaseURL + "tags/" + urlEscape(term) + "/"
	return &tmpl.Context{
		Kind:        "term",
		Title:       term,
		Site:        siteCtx,
		Data: &tmpl.DataAccessor{
			Pages:  pages,
			Plural: "tags",
		},
		Scratch:      tmpl.NewScratch(),
		RegularPages: pages,
		Pages:        pages,
		RelPermalink: relURL,
		Permalink:    permURL,
		OutputFormats: tmpl.DefaultPageOutputFormats(permURL, relURL),
	}
}

// resolveTemplateName maps a page to its template using Hugo's lookup rules.
func resolveTemplateName(tmpls *template.Template, p *content.Page) string {
	switch p.Kind {
	case "home":
		if t := tmpls.Lookup("index.html"); t != nil {
			return "index.html"
		}
		return "_default/list.html"

	case "section":
		// Hugo lookup order: {type}/list.html → {section}/list.html → _default/list.html
		if p.Type != "" {
			if t := tmpls.Lookup(p.Type + "/list.html"); t != nil {
				return p.Type + "/list.html"
			}
		}
		if p.Section != "" {
			if t := tmpls.Lookup(p.Section + "/list.html"); t != nil {
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
			if t := tmpls.Lookup(p.Type + "/single.html"); t != nil {
				return p.Type + "/single.html"
			}
		}
		if p.Section != "" {
			if t := tmpls.Lookup(p.Section + "/single.html"); t != nil {
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

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	port, _ := cmd.Flags().GetString("port")

	fmt.Printf("Serving site: %s\n", cfg.Title)
	fmt.Printf("  Source:      %s\n", sourceDir)
	fmt.Printf("  Output:      %s\n", cfg.PublishDir)
	fmt.Printf("  URL:         http://localhost:%s\n", port)

	// TODO: build, then serve via HTTP
	fmt.Println("Serve not yet implemented.")
	return nil
}
