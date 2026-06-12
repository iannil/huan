package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/novel_ttl/huan/internal/build"
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
		plain := build.StripHTMLTagsForSummary(html)
		p.Plain = plain
		p.WordCount = build.CountWordsInPlain(plain)

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
			p.Summary = template.HTML(build.TruncateHTMLByWords(content, 120))
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
	themeI18nDir := filepath.Join(sourceDir, "themes", build.DetectThemeName(sourceDir), "i18n")
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

		tmplName := build.ResolveTemplateName(tmpls, p)
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
			if rssName := build.ResolveRSSOutput(p); rssName != "" {
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
	if taxCtx := build.BuildTaxonomyContext(siteCtx, lookup, site, cfg); taxCtx != nil {
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
				termCtx := build.BuildTermContext(siteCtx, lookup, site, cfg, term.Name, term.Pages)
				if termCtx != nil {
					tagSlug := build.URLEscape(term.Name)
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
	if catCtx := build.BuildEmptyTaxonomyContext(siteCtx, "Categories", "categories"); catCtx != nil {
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
	if homeCtx := build.FindHomeContext(lookup, site); homeCtx != nil {
		mainPageItems := build.FilterMainSections(siteCtx.RegularPages, cfg.Params.MainSections)
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
			pagedCtx := build.CloneContextForPagination(homeCtx, mainPageItems, pageSize, i, totalPages)
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
	if siteMapCtx := build.BuildSitemapContext(siteCtx, lookup, site, cfg); siteMapCtx != nil {
		if html, err := renderer.Render("_default/sitemap.xml", siteMapCtx); err == nil {
			writer.Write("sitemap.xml", html)
		} else {
			fmt.Printf("  WARN: sitemap: %v\n", err)
		}
	}

	// Generate search.json from home page context
	if homeCtx := build.FindHomeContext(lookup, site); homeCtx != nil {
		if html, err := renderer.Render("_default/index.searchindex.json", homeCtx); err == nil {
			if werr := writer.Write("search.json", html); werr != nil {
				fmt.Printf("  WARN: write search.json: %v\n", werr)
			}
		} else {
			fmt.Printf("  WARN: search: %v\n", err)
		}
	}

	// Copy static assets: theme static first, then project static (overrides)
	themeName := build.DetectThemeName(sourceDir)
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
