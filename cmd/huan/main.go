package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/novel_ttl/huan/internal/config"
	"github.com/novel_ttl/huan/internal/content"
	"github.com/novel_ttl/huan/internal/encrypt"
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
	}

	// 5. Build content tree
	site, err := content.BuildTree(pages, cfg, sourceDir)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}
	site.Data = data

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
		if p.Sitemap.Disable {
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

	// Copy static assets
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

// resolveTemplateName maps a page to its template using Hugo's lookup rules.
func resolveTemplateName(tmpls *template.Template, p *content.Page) string {
	switch p.Kind {
	case "home":
		if t := tmpls.Lookup("index.html"); t != nil {
			return "index.html"
		}
		return "_default/list.html"

	case "section":
		// Try layouts/{section}/list.html, then _default/list.html
		if p.Section != "" {
			if t := tmpls.Lookup(p.Section + "/list.html"); t != nil {
				return p.Section + "/list.html"
			}
		}
		if p.Type != "" {
			if t := tmpls.Lookup(p.Type + "/list.html"); t != nil {
				return p.Type + "/list.html"
			}
		}
		if t := tmpls.Lookup("_default/list.html"); t != nil {
			return "_default/list.html"
		}
		return ""

	case "page":
		// Try layouts/{type}/single.html, layouts/{section}/single.html, _default/single.html
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
