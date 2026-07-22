package build

import (
	"fmt"
	"strings"

	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)

// resolveSourceFromURL derives the source file path (relative to content/)
// from a page URL. Used by JIT rendering when the URL is not in the DAG
// (e.g., a newly-created draft not yet captured by a full build).
//
// URL conventions (match Hugo/huan content layout):
//   /                          → _index.md            (home)
//   /posts/                    → posts/_index.md      (section)
//   /posts/hello/              → posts/hello.md       (regular page)
//   /posts/2026/new-year/      → posts/2026/new-year.md
//   /posts/_index/             → posts/_index.md      (explicit)
//
// Returns "" only if the input cannot be parsed (effectively never for a
// normalized URL); callers should still verify the file exists on disk.
func resolveSourceFromURL(pageURL string) string {
	u := strings.Trim(pageURL, "/")
	if u == "" {
		return "_index.md" // home
	}
	parts := strings.Split(u, "/")
	last := parts[len(parts)-1]
	if last == "_index" {
		// /posts/_index/ → posts/_index.md
		return strings.Join(parts, "/") + ".md"
	}
	if len(parts) == 1 {
		// /posts/ → posts/_index.md (single segment = section index)
		return parts[0] + "/_index.md"
	}
	// /posts/hello/ → posts/hello.md
	return strings.Join(parts, "/") + ".md"
}

// RenderPageWithCache renders a single page on demand, reusing the cached
// rendering infrastructure (templates/i18n/markdown/writer). Unlike
// IncrementalRender, it returns the HTML without writing to disk, targets
// exactly one page identified by URL, and forces IncludeDrafts=true (JIT
// primarily serves draft pages that were skipped at build time).
//
// Used by daemon's JIT fallback when a requested URL is not in the
// pre-built static output. Content + contexts are always rebuilt (cheap)
// to keep list-page contexts consistent with the current on-disk content.
//
// When cache is nil or incomplete, falls back to a full pipeline setup
// (slower but always correct).
func RenderPageWithCache(opts Options, cache *PipelineCache, pageURL string) (string, error) {
	logf := opts.logf()
	p := newPipeline(opts)

	// JIT primarily serves drafts that were filtered out at build time.
	p.opts.IncludeDrafts = true

	// Stage 1: config. Reuse cached cfg when available.
	if cache != nil && cache.SiteCfg != nil {
		p.cfg = cache.SiteCfg
		if opts.BaseURLOverride != "" {
			p.cfg.BaseURL = opts.BaseURLOverride
		}
		if opts.MinifyOverride != nil {
			p.cfg.Minify = *opts.MinifyOverride
		}
	} else {
		if err := p.loadConfig(); err != nil {
			return "", fmt.Errorf("RenderPageWithCache loadConfig: %w", err)
		}
	}

	// Stage 2-3: reload ALL content + render markdown. Required for
	// correctness: list/tag/section page contexts reference child pages.
	if err := p.loadContent(); err != nil {
		return "", fmt.Errorf("RenderPageWithCache loadContent: %w", err)
	}
	if err := p.renderMarkdownAndTree(); err != nil {
		return "", fmt.Errorf("RenderPageWithCache renderMarkdownAndTree: %w", err)
	}

	// Stage 4: reuse cached rendering infrastructure OR rebuild.
	if cache != nil && cache.Templates != nil && cache.Writer != nil {
		p.tmpls = cache.Templates
		p.i18nBundle = cache.I18nBundle
		p.scRegistry = cache.SCRegistry
		p.md = cache.MDRenderer
		p.renderer = tmpl.NewRenderer(cache.Templates, tmpl.FuncMap(p.cfg.BaseURL))
		p.writer = cache.Writer
		tmpl.SetI18nBundle(p.i18nBundle)
	} else {
		if err := p.setupTemplatesAndWriter(); err != nil {
			return "", fmt.Errorf("RenderPageWithCache setupTemplatesAndWriter: %w", err)
		}
	}

	// Stage 5: rebuild contexts.
	p.buildContexts()

	// Stage 6: find the target page by URL.
	var target *content.Page
	for _, pg := range p.site.Pages {
		if pg.URL == pageURL {
			target = pg
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("RenderPageWithCache: page not found: %s", pageURL)
	}

	// Stage 7: render the single page (returns HTML, does NOT write to disk).
	html, err := p.renderSinglePage(target)
	if err != nil {
		return "", fmt.Errorf("RenderPageWithCache render %s: %w", pageURL, err)
	}

	// Stage 8: serve-mode LiveReload injection (parity with full build).
	if opts.InjectLiveReload && opts.LiveReloadURL != "" {
		html = InjectLiveReload(html, opts.LiveReloadURL)
	}

	logf("  JIT render: %s\n", pageURL)
	return html, nil
}
