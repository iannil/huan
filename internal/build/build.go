package build

// build.go: BuildSite — the orchestrator. Stage methods live in
// pipeline_*.go and operate on a *pipeline struct (see pipeline.go).

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/output"
	tmpl "github.com/iannil/huan/internal/template"
)

// Options controls a single BuildSite invocation.
type Options struct {
	SourceDir        string
	OutputDir        string // absolute path
	IncludeDrafts    bool
	IncludeFuture    bool
	IncludeExpired   bool
	InjectLiveReload bool   // serve-only; when true, LiveReloadURL must be set
	LiveReloadURL    string // empty disables injection
	BaseURLOverride  string // serve-only; overrides cfg.BaseURL for dev server
	MinifyOverride   *bool  // nil = use config Minify; non-nil = force this value
	Logf             func(format string, args ...any)

	// CfgOverride skips config.Load(opts.SourceDir) and uses this Config
	// directly. Used internally by BuildMultiSite to build per-language
	// variants without re-reading huan.yaml.
	CfgOverride *config.Config

	// PageFilter, when non-nil, excludes pages where the function returns
	// false. Used internally by BuildMultiSite to build per-language subsets.
	PageFilter func(*content.Page) bool

	// AvailableTranslations, when non-nil, maps each page's language-neutral
	// RelPath to the set of language codes with an actual sidecar file.
	// Used by template context to filter hreflang output to only languages
	// that actually exist for each page (prevents hreflang=en → 404).
	AvailableTranslations map[string]map[string]bool

	// AfterBuild is called after BuildSite completes successfully.
	// It receives the Result and a callback for single-page rendering.
	// Used by daemon to initialize JIT cache with build context.
	// Experimental: API may change in future versions.
	AfterBuild func(*Result, RenderPageFunc) error

	// AfterBuildSite is called after BuildSite completes successfully with
	// the built site. Used by daemon's DAG builder to construct the
	// dependency graph for incremental updates.
	// Experimental: API may change in future versions.
	AfterBuildSite func(*content.Site) error

	// PipelineCache, if non-nil, is populated with reusable build state
	// after BuildSite completes successfully. Used by daemon for
	// incremental builds. Pass a cache created by NewPipelineCache().
	// Experimental: API may change in future versions.
	PipelineCache *PipelineCache
}

// RenderPageFunc is the callback signature for single-page rendering.
// Experimental: API may change in future versions.
type RenderPageFunc func(pg *content.Page) (string, error)

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

// BuildSite renders the full site from SourceDir into OutputDir, orchestrating
// 7 stages via the pipeline type (see pipeline.go and pipeline_*.go):
//
//  1. loadConfig (pipeline.go)           — cfg + serve overrides + i18n inject
//  2. loadContent (pipeline.go)          — content/ + data/ + stale-i18n check
//  3. renderMarkdownAndTree (pipeline.go)— per-page goldmark + tree + taxonomies
//  4. setupTemplatesAndWriter (setup)    — templates + i18n bundle + writer
//  5. buildContexts (setup)              — siteCtx + per-page ctx + links
//  6. renderPages (render)               — HTML + Markdown mirror + section RSS
//  7. renderFeedsAndSpecials (feeds)     — taxonomy/pagination/404/sitemap/etc.
//  8. copyStaticAndFinalize (write)      — static assets + stats
//
// Any stage error aborts the build immediately. Errors during per-page render
// are accumulated into Result.Errors instead.
func BuildSite(opts Options) (*Result, error) {
	start := time.Now()
	p := newPipeline(opts)

	stages := []struct {
		name string
		fn   func() error
	}{
		{"load config", p.loadConfig},
		{"load content", p.loadContent},
		{"render markdown + tree", p.renderMarkdownAndTree},
		{"setup templates + writer", p.setupTemplatesAndWriter},
	}
	for _, s := range stages {
		if err := s.fn(); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
	}

	// buildContexts is infallible (no error return path).
	p.buildContexts()

	// render + postprocess stages: errors counted into Result.Errors,
	// not propagated. Build continues so partial output is produced.
	p.renderPages()
	p.renderFeedsAndSpecials()
	p.copyStaticAndFinalize()

	p.result.Duration = time.Since(start)

	// Invoke AfterBuild callback if provided (for daemon JIT cache setup).
	if opts.AfterBuild != nil {
		renderPageFn := func(pg *content.Page) (string, error) {
			return p.renderSinglePage(pg)
		}
		if err := opts.AfterBuild(p.result, renderPageFn); err != nil {
			return p.result, fmt.Errorf("AfterBuild callback: %w", err)
		}
	}

	// Invoke AfterBuildSite callback if provided (for daemon DAG builder).
	if opts.AfterBuildSite != nil {
		if err := opts.AfterBuildSite(p.site); err != nil {
			return p.result, fmt.Errorf("AfterBuildSite callback: %w", err)
		}
	}

	// Populate the pipeline cache if requested (for daemon incremental builds).
	if opts.PipelineCache != nil {
		p.populateCache(opts.PipelineCache)
	}

	return p.result, nil
}

// RenderPage renders a single page using the provided build options.
// This is the entry point for daemon's JIT rendering and incremental updates.
//
// It creates a new pipeline, runs stages 1-5 (config, content, markdown, templates,
// contexts), finds the requested page by RelPath, and renders it.
//
// Experimental: API may change in future versions.
func RenderPage(opts Options, pg *content.Page) (string, error) {
	p := newPipeline(opts)

	// Stage 1-4: minimal pipeline setup
	if err := p.loadConfig(); err != nil {
		return "", fmt.Errorf("RenderPage loadConfig: %w", err)
	}
	if err := p.loadContent(); err != nil {
		return "", fmt.Errorf("RenderPage loadContent: %w", err)
	}
	if err := p.renderMarkdownAndTree(); err != nil {
		return "", fmt.Errorf("RenderPage renderMarkdownAndTree: %w", err)
	}
	if err := p.setupTemplatesAndWriter(); err != nil {
		return "", fmt.Errorf("RenderPage setupTemplatesAndWriter: %w", err)
	}

	// Stage 5: build contexts
	p.buildContexts()

	// Find the requested page in the site
	target := p.findPage(pg.RelPath)
	if target == nil {
		return "", fmt.Errorf("RenderPage: page not found: %s", pg.RelPath)
	}

	// Render single page
	html, err := p.renderSinglePage(target)
	if err != nil {
		return "", fmt.Errorf("RenderPage: %w", err)
	}

	return html, nil
}

// RenderPageToBytes renders a single page and returns the HTML as a byte slice.
// Convenience wrapper around RenderPage.
//
// Experimental: API may change in future versions.
func RenderPageToBytes(opts Options, pg *content.Page) ([]byte, error) {
	html, err := RenderPage(opts, pg)
	if err != nil {
		return nil, err
	}
	return []byte(html), nil
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

// IncrementalRender re-renders only the pages whose URLs are in affectedURLs,
// after a content change. It reuses cached templates/i18n/writer from a prior
// full build (via cache), but reloads all content + rebuilds contexts to
// ensure correctness (list pages must see updated page content).
//
// Only pages in affectedURLs are re-rendered and written to OutputDir;
// other pages are left untouched on disk. This skips the expensive
// per-page template rendering for the ~95% of unaffected pages.
//
// Use HasTemplateChanges() first: if templates changed, call BuildSite
// (full build) instead. Pass cache=nil to rebuild everything from scratch
// (slower, but always correct).
func IncrementalRender(opts Options, cache *PipelineCache, affectedURLs []string) error {
	if len(affectedURLs) == 0 {
		return nil
	}

	logf := opts.logf()
	p := newPipeline(opts)

	// Stage 1: load config. Reuse cached cfg when available (faster).
	if cache != nil && cache.SiteCfg != nil {
		p.cfg = cache.SiteCfg
		// Apply serve-mode overrides (they may differ from the cached build).
		if opts.BaseURLOverride != "" {
			p.cfg.BaseURL = opts.BaseURLOverride
		}
		if opts.MinifyOverride != nil {
			p.cfg.Minify = *opts.MinifyOverride
		}
	} else {
		if err := p.loadConfig(); err != nil {
			return fmt.Errorf("IncrementalRender loadConfig: %w", err)
		}
	}

	// Stage 2: reload ALL content. This is required for correctness:
	// list/tag/section pages reference child pages, so their contexts must
	// see the updated page content. Content loading is cheap relative to
	// template rendering.
	if err := p.loadContent(); err != nil {
		return fmt.Errorf("IncrementalRender loadContent: %w", err)
	}

	// Stage 3: render markdown + build content tree + taxonomies.
	if err := p.renderMarkdownAndTree(); err != nil {
		return fmt.Errorf("IncrementalRender renderMarkdownAndTree: %w", err)
	}

	// Stage 4: reuse cached rendering infrastructure OR rebuild.
	if cache != nil && cache.Templates != nil && cache.Writer != nil {
		p.tmpls = cache.Templates
		p.i18nBundle = cache.I18nBundle
		p.scRegistry = cache.SCRegistry
		p.md = cache.MDRenderer
		// Renderer wraps the cached templates with the current BaseURL FuncMap.
		p.renderer = tmpl.NewRenderer(cache.Templates, tmpl.FuncMap(p.cfg.BaseURL))
		p.writer = cache.Writer
		// Ensure the i18n bundle is wired into the template package (global state).
		tmpl.SetI18nBundle(p.i18nBundle)
	} else {
		if err := p.setupTemplatesAndWriter(); err != nil {
			return fmt.Errorf("IncrementalRender setupTemplatesAndWriter: %w", err)
		}
	}

	// Stage 5: build contexts (site-wide + per-page). Always rebuilt —
	// contexts reference page pointers that changed during content reload.
	p.buildContexts()

	// Stage 6: render ONLY affected pages. This is the performance win:
	// ~95% of pages are skipped.
	affectedSet := make(map[string]bool, len(affectedURLs))
	for _, u := range affectedURLs {
		affectedSet[u] = true
	}

	renderedCount := 0
	errors := 0
	for _, pg := range p.site.Pages {
		if !affectedSet[pg.URL] {
			continue
		}
		tmplName := ResolveTemplateName(p.tmpls, pg)
		if tmplName == "" {
			continue
		}
		ctx := p.lookup[pg]
		if ctx == nil {
			continue
		}
		// Section/home pages expose their pages via .Data.Pages.
		if pg.Kind == "section" || pg.Kind == "home" {
			ctx.Data = &tmpl.DataAccessor{Pages: ctx.RegularPages}
		}

		html, err := p.renderer.Render(tmplName, ctx)
		if err != nil {
			logf("  WARN: incremental render %s: %v\n", pg.URL, err)
			errors++
			continue
		}

		// Inject LiveReload (serve mode only) so re-rendered pages stay
		// live-reload-enabled without a full rebuild.
		if opts.InjectLiveReload && opts.LiveReloadURL != "" {
			html = InjectLiveReload(html, opts.LiveReloadURL)
		}

		outPath := output.URLToFilePath(pg.URL, "")
		if err := p.writer.Write(outPath, html); err != nil {
			logf("  WARN: incremental write %s: %v\n", pg.URL, err)
			errors++
			continue
		}
		renderedCount++
	}

	logf("  Incremental render: %d pages re-rendered (%d affected, %d errors)\n",
		renderedCount, len(affectedURLs), errors)
	return nil
}
