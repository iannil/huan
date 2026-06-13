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
	"github.com/iannil/huan/internal/markdown"
	"github.com/iannil/huan/internal/output"
	"github.com/iannil/huan/internal/shortcode"
	"github.com/iannil/huan/internal/taxonomy"
	tmpl "github.com/iannil/huan/internal/template"
)

// Options controls a single BuildSite invocation.
type Options struct {
	SourceDir        string
	OutputDir        string // absolute path
	IncludeDrafts    bool
	IncludeFuture    bool
	IncludeExpired   bool
	InjectLiveReload bool   // serve-only; when true, LiveReloadURL must be set (Task E1 will use this)
	LiveReloadURL    string // empty disables injection
	BaseURLOverride  string // serve-only; when non-empty, overrides cfg.BaseURL so in-site links point to the dev server
	MinifyOverride   *bool  // nil = use config Minify; non-nil = force this value
	Logf             func(format string, args ...any)
}

// Result reports what happened during the build.
type Result struct {
	PagesRendered int
	FilesWritten  int
	BytesWritten  int64
	Errors        int
	Duration      time.Duration
}

func (o *Options) logf() func(string, ...any) {
	if o.Logf == nil {
		return func(format string, args ...any) { fmt.Printf(format, args...) }
	}
	return o.Logf
}

// BuildSite renders the full site from SourceDir into OutputDir.
// Behavior matches the existing huan build command for byte-level Hugo parity.
func BuildSite(opts Options) (*Result, error) {
	start := time.Now()
	r := &Result{}
	logf := opts.logf()

	cfg, err := config.Load(opts.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// serve mode overrides cfg.BaseURL so all in-site absolute URLs (canonify,
	// permalinks, RSS, sitemap, OG metadata) point at the dev server instead
	// of the production domain. Empty = use cfg.BaseURL as-is (production build).
	if opts.BaseURLOverride != "" {
		cfg.BaseURL = opts.BaseURLOverride
	}

	// --minify flag overrides config
	if opts.MinifyOverride != nil {
		cfg.Minify = *opts.MinifyOverride
	}

	now := time.Now()

	logf("Building site: %s\n", cfg.Title)
	logf("  Source:      %s\n", opts.SourceDir)
	logf("  Output:      %s\n", opts.OutputDir)
	logf("  BaseURL:     %s\n", cfg.BaseURL)

	// 1. Load content
	contentDir := filepath.Join(opts.SourceDir, "content")
	pages, err := content.LoadDir(contentDir)
	if err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	logf("  Pages loaded: %d\n", len(pages))

	// 2. Load data files
	dataDir := filepath.Join(opts.SourceDir, "data")
	data, err := content.LoadDataFiles(dataDir)
	if err != nil {
		return nil, fmt.Errorf("load data: %w", err)
	}
	logf("  Data files:   %d\n", len(data))

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
			return nil, fmt.Errorf("shortcode %s: %w", p.RelPath, err)
		}

		html, err := md.Render(expanded)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", p.RelPath, err)
		}
		p.Content = template.HTML(html)

		// Compute Plain and WordCount from rendered HTML (matches Hugo's
		// behavior of counting words in plainified HTML).
		plain := StripHTMLTagsForSummary(html)
		p.Plain = plain
		p.WordCount = CountWordsInPlain(plain)

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
			p.Summary = template.HTML(TruncateHTMLToBlockBoundary(content, 120))
		}
	}

	// 5. Build content tree
	site, err := content.BuildTree(pages, cfg, opts.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}
	site.Data = data

	// 6. Build taxonomies
	// Pass site.Pages (all non-draft pages including hidden/never-listed ones)
	// so that tags whose only pages are hidden still appear as taxonomy keys.
	// Hugo generates /tags/{tag}/ pages (HTML + RSS) for these "empty" tags
	// even though their page list is filtered to zero. BuildTaxonomyContext
	// then filters each term's page list to exclude never-listed pages.
	// Pages here are already sorted via content.sortPagesDefault with the
	// site's collator, so taxonomy term members and per-tag RSS items emit
	// in the same order Hugo produces via its DefaultPageSort.
	// We also capture original-cased tag names so term-page titles render as
	// Hugo does (e.g. <title>FANFAN on ...</title>) while the urlized key
	// (fanfan) is still used for filesystem paths and URL paths.
	taxonomies, originalCases := taxonomy.BuildAllWithOriginalCase(site.Pages)
	taxCount := 0
	if tax, ok := taxonomies["tags"]; ok {
		taxCount = len(tax)
	}
	logf("  Tags:         %d unique\n", taxCount)
	site.Taxonomies = map[string]content.Taxonomy{}
	for name, tax := range taxonomies {
		converted := content.Taxonomy{}
		for term, pages := range tax {
			converted[term] = pages
		}
		site.Taxonomies[name] = converted
	}
	site.TaxonomyOriginalCase = originalCases

	// 8. Load templates
	tmpls, err := tmpl.LoadAllTemplates(opts.SourceDir, cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}

	// Load i18n bundles (theme first, then project overrides).
	i18nBundle := i18n.New()
	themeI18nDir := filepath.Join(opts.SourceDir, "themes", DetectThemeName(opts.SourceDir), "i18n")
	if _, err := os.Stat(themeI18nDir); err == nil {
		_ = i18nBundle.LoadDir(themeI18nDir)
	}
	projectI18nDir := filepath.Join(opts.SourceDir, "i18n")
	if _, err := os.Stat(projectI18nDir); err == nil {
		_ = i18nBundle.LoadDir(projectI18nDir)
	}
	tmpl.SetI18nBundle(i18nBundle)

	// Count loaded templates
	templateCount := 0
	for range tmpls.Templates() {
		templateCount++
	}
	logf("  Templates:    %d\n", templateCount)

	// 9. Render pages
	renderer := tmpl.NewRenderer(tmpls, tmpl.FuncMap(cfg.BaseURL))
	var writer *output.Writer
	if cfg.Minify {
		writer = output.NewWriterWithMinify(opts.OutputDir)
	} else {
		writer = output.NewWriter(opts.OutputDir)
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
		if p.Draft && !opts.IncludeDrafts {
			continue
		}
		if !opts.IncludeFuture && !p.PublishDateParsed.IsZero() && p.PublishDateParsed.After(now) {
			continue
		}
		if !opts.IncludeExpired && !p.ExpiryDateParsed.IsZero() && p.ExpiryDateParsed.Before(now) {
			continue
		}
		if p.Build.Render == "never" {
			continue
		}

		tmplName := ResolveTemplateName(tmpls, p)
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
			logf("  WARN: render %s with %s: %v\n", p.RelPath, tmplName, err)
			errors++
			continue
		}

		// Inject LiveReload script if requested (serve mode only)
		if opts.InjectLiveReload && opts.LiveReloadURL != "" {
			html = InjectLiveReload(html, opts.LiveReloadURL)
		}

		outPath := output.URLToFilePath(p.URL, "")
		if err := writer.Write(outPath, html); err != nil {
			logf("  WARN: write %s: %v\n", p.URL, err)
			errors++
			continue
		}
		renderedCount++

		// Markdown mirror: copy source .md alongside HTML for AI consumption
		if cfg.AI.MarkdownMirror && p.Kind == "page" {
			mdRelPath := strings.TrimSuffix(p.URL, "/") + "/index.md"
			mdRelPath = strings.TrimPrefix(mdRelPath, "/")
			data, err := os.ReadFile(p.FilePath)
			if err != nil {
				logf("  WARN: mirror md %s: %v\n", p.FilePath, err)
			} else if err := writer.Write(mdRelPath, string(data)); err != nil {
				logf("  WARN: write md %s: %v\n", mdRelPath, err)
			}
		}

		// RSS output for home and section pages
		if p.Kind == "home" || p.Kind == "section" {
			if rssName := ResolveRSSOutput(p); rssName != "" {
				if rssHTML, err := renderer.Render(rssName, ctx); err == nil {
					rssPath := strings.TrimSuffix(p.URL, "/") + "/index.xml"
					rssPath = strings.TrimPrefix(rssPath, "/")
					if err := writer.Write(rssPath, rssHTML); err != nil {
						logf("  WARN: write RSS %s: %v\n", p.URL, err)
					}
				} else {
					logf("  WARN: render RSS %s: %v\n", p.URL, err)
				}
			}
		}
	}

	// Generate taxonomy term pages: /tags/ and /tags/{tag}/
	if taxCtx := BuildTaxonomyContext(siteCtx, lookup, site, cfg); taxCtx != nil {
		// /tags/ - the terms listing page
		if html, err := renderer.Render("_default/terms.html", taxCtx); err == nil {
			writer.Write("tags/index.html", html)
		} else {
			logf("  WARN: terms: %v\n", err)
		}

		// /tags/index.xml - RSS for tags listing
		if html, err := renderer.Render("_default/rss.xml", taxCtx); err == nil {
			writer.Write("tags/index.xml", html)
		}

		// /tags/{tag}/ - one page per term (HTML + RSS)
		if termsTmpl := tmpls.Lookup("_default/list.html"); termsTmpl != nil {
			for _, term := range taxCtx.DataTerms {
				termCtx := BuildTermContext(siteCtx, lookup, site, cfg, term.Name, term.Pages)
				if termCtx != nil {
					tagSlug := URLEscape(term.Name)
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
	if catCtx := BuildEmptyTaxonomyContext(siteCtx, "Categories", "categories"); catCtx != nil {
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
	if homeCtx := FindHomeContext(lookup, site); homeCtx != nil {
		mainPageItems := FilterMainSections(siteCtx.RegularPages, cfg.Params.MainSections)
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
			logf("  WARN: write page/1: %v\n", err)
		}

		// /page/2/, /page/3/, ... are actual paginated home pages
		for i := 2; i <= totalPages; i++ {
			pagedCtx := CloneContextForPagination(homeCtx, mainPageItems, pageSize, i, totalPages)
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
	if siteMapCtx := BuildSitemapContext(siteCtx, lookup, site, cfg); siteMapCtx != nil {
		if html, err := renderer.Render("_default/sitemap.xml", siteMapCtx); err == nil {
			writer.Write("sitemap.xml", html)
		} else {
			logf("  WARN: sitemap: %v\n", err)
		}
	}

	// Generate search.json from home page context
	if homeCtx := FindHomeContext(lookup, site); homeCtx != nil {
		if html, err := renderer.Render("_default/index.searchindex.json", homeCtx); err == nil {
			if werr := writer.Write("search.json", html); werr != nil {
				logf("  WARN: write search.json: %v\n", werr)
			}
		} else {
			logf("  WARN: search: %v\n", err)
		}
	}

	// Generate llms.txt for AI crawlers
	if cfg.AI.LlmsTxt {
		if err := output.GenerateLlmsTxt(opts.OutputDir, opts.SourceDir, cfg); err != nil {
			logf("  WARN: llms.txt: %v\n", err)
		}
	}

	// Generate /api/{section}.json for AI consumption
	if cfg.AI.ContentAPI {
		if err := output.GenerateContentAPI(opts.OutputDir, site, cfg, opts.IncludeDrafts, opts.IncludeFuture, opts.IncludeExpired, now); err != nil {
			logf("  WARN: content api: %v\n", err)
		}
	}

	// Copy static assets: theme static first, then project static (overrides)
	themeName := DetectThemeName(opts.SourceDir)
	if themeName != "" {
		themeStaticDir := filepath.Join(opts.SourceDir, "themes", themeName, "static")
		if _, err := os.Stat(themeStaticDir); err == nil {
			if err := writer.CopyStatic(themeStaticDir); err != nil {
				logf("  WARN: theme static: %v\n", err)
			}
		}
	}
	staticDir := filepath.Join(opts.SourceDir, "static")
	if err := writer.CopyStatic(staticDir); err != nil {
		logf("  WARN: static: %v\n", err)
	}

	files, bytes := writer.Stats()
	r.FilesWritten = files
	r.BytesWritten = bytes
	r.PagesRendered = renderedCount
	r.Errors = errors
	logf("  Rendered:     %d pages\n", renderedCount)
	logf("  Output:       %d files, %.1f KB\n", files, float64(bytes)/1024)
	if errors > 0 {
		logf("  Errors:       %d\n", errors)
	}
	logf("Build complete.\n")

	r.Duration = time.Since(start)
	return r, nil
}

// InjectLiveReload inserts the livereload <script> before </head>.
// Falls back to before </body> if </head> is absent.
// If neither is present, appends the script at the end.
func InjectLiveReload(html, wsURL string) string {
	host := hostFromURL(wsURL)
	port := portFromURL(wsURL)
	tag := `<script src="http://` + host + `:` + port +
		`/livereload.js?mindelay=10&v=2" data-livereload-port="` + port +
		`" data-livereload-host="` + host + `"></script>`
	if idx := strings.Index(html, "</head>"); idx >= 0 {
		return html[:idx] + tag + html[idx:]
	}
	if idx := strings.Index(html, "</body>"); idx >= 0 {
		return html[:idx] + tag + html[idx:]
	}
	return html + tag
}

func portFromURL(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil || u.Port() == "" {
		return "1313"
	}
	return u.Port()
}

func hostFromURL(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil || u.Hostname() == "" {
		return "localhost"
	}
	return u.Hostname()
}
