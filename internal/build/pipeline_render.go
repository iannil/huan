package build

// pipeline_render.go: stage 5 — render pages, RSS for home/section,
// and Markdown mirrors for AI consumers.

import (
	"fmt"
	"os"
	"strings"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/output"
	tmpl "github.com/iannil/huan/internal/template"
)

// renderPages renders every eligible site page (filtered by draft/future/
// expired/render-never) into HTML, writes each to disk, and emits the
// associated outputs (Markdown mirror, RSS feed for home/section).
//
// Errors during a single page are counted but do not abort the build —
// partial output is preferable to no output, and the build log captures
// every warning for later inspection.
func (p *pipeline) renderPages() {
	renderedCount := 0
	errors := 0
	for _, pg := range p.site.Pages {
		if !p.shouldRender(pg) {
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

		// For section/list rendering, expose pages via .Data.Pages.
		if pg.Kind == "section" || pg.Kind == "home" {
			ctx.Data = &tmpl.DataAccessor{
				Pages: ctx.RegularPages,
			}
		}

		html, err := p.renderer.Render(tmplName, ctx)
		if err != nil {
			p.logf("  WARN: render %s with %s: %v\n", pg.RelPath, tmplName, err)
			errors++
			continue
		}

		// Inject LiveReload (serve mode only).
		if p.opts.InjectLiveReload && p.opts.LiveReloadURL != "" {
			html = InjectLiveReload(html, p.opts.LiveReloadURL)
		}

		outPath := output.URLToFilePath(pg.URL, "")
		if err := p.writer.Write(outPath, html); err != nil {
			p.logf("  WARN: write %s: %v\n", pg.URL, err)
			errors++
			continue
		}
		renderedCount++

		p.maybeWriteMarkdownMirror(pg)
		p.maybeWriteSectionRSS(pg, ctx)

		// Invoke OnPageRendered hooks
		p.runOnPageRendered(pg)
	}

	p.result.PagesRendered = renderedCount
	p.result.Errors = errors
}

// shouldRender reports whether pg passes the draft/future/expired/render-never
// filters. Centralizing this keeps the render loop body flat.
func (p *pipeline) shouldRender(pg *content.Page) bool {
	if pg.Draft && !p.opts.IncludeDrafts {
		return false
	}
	if !p.opts.IncludeFuture && !pg.PublishDateParsed.IsZero() && pg.PublishDateParsed.After(p.now) {
		return false
	}
	if !p.opts.IncludeExpired && !pg.ExpiryDateParsed.IsZero() && pg.ExpiryDateParsed.Before(p.now) {
		return false
	}
	if pg.Build.Render == "never" {
		return false
	}
	return true
}

// maybeWriteMarkdownMirror writes a sidecar .md copy of the page source
// alongside the HTML when cfg.AI.MarkdownMirror is enabled. Skipped for
// non-page kinds (section/home/term) since those don't have a 1:1 source.
func (p *pipeline) maybeWriteMarkdownMirror(pg *content.Page) {
	if !p.cfg.AI.MarkdownMirror || pg.Kind != "page" {
		return
	}
	mdRelPath := strings.TrimSuffix(pg.URL, "/") + "/index.md"
	mdRelPath = strings.TrimPrefix(mdRelPath, "/")
	data, err := os.ReadFile(pg.FilePath)
	if err != nil {
		p.logf("  WARN: mirror md %s: %v\n", pg.FilePath, err)
		return
	}
	if err := p.writer.Write(mdRelPath, string(data)); err != nil {
		p.logf("  WARN: write md %s: %v\n", mdRelPath, err)
	}
}

// maybeWriteSectionRSS emits /index.xml for home and section pages.
// Hugo emits RSS for these two kinds; term/taxonomy RSS is handled by
// the taxonomy stage.
func (p *pipeline) maybeWriteSectionRSS(pg *content.Page, ctx *tmpl.Context) {
	if pg.Kind != "home" && pg.Kind != "section" {
		return
	}
	rssName := ResolveRSSOutput(pg)
	if rssName == "" {
		return
	}
	rssHTML, err := p.renderer.Render(rssName, ctx)
	if err != nil {
		p.logf("  WARN: render RSS %s: %v\n", pg.URL, err)
		return
	}
	rssPath := strings.TrimSuffix(pg.URL, "/") + "/index.xml"
	rssPath = strings.TrimPrefix(rssPath, "/")
	if err := p.writer.Write(rssPath, rssHTML); err != nil {
		p.logf("  WARN: write RSS %s: %v\n", pg.URL, err)
	}
}

// renderSinglePage renders one page using the already-initialized pipeline state.
// The pipeline must have completed stages 1-4 (templates, i18n, lookup, renderer).
// Returns empty string if the page should not be rendered (draft/future/expired/never).
// This is used by the AfterBuild callback for JIT rendering in daemon mode.
func (p *pipeline) renderSinglePage(pg *content.Page) (string, error) {
	if !p.shouldRender(pg) {
		return "", nil
	}

	tmplName := ResolveTemplateName(p.tmpls, pg)
	if tmplName == "" {
		return "", nil
	}

	ctx := p.lookup[pg]
	if ctx == nil {
		return "", fmt.Errorf("renderSinglePage: no context for %s", pg.URL)
	}

	// For section/list rendering, expose pages via .Data.Pages.
	if pg.Kind == "section" || pg.Kind == "home" {
		ctx.Data = &tmpl.DataAccessor{
			Pages: ctx.RegularPages,
		}
	}

	html, err := p.renderer.Render(tmplName, ctx)
	if err != nil {
		return "", fmt.Errorf("render %s with %s: %w", pg.RelPath, tmplName, err)
	}

	return html, nil
}

// findPage returns the page matching the given RelPath from the pipeline's site.
// Returns nil if no matching page is found. Used by RenderPage to locate the
// target page in the freshly-built pipeline (caller's page object may not have
// the same pointer identity as the pages in the new pipeline).
func (p *pipeline) findPage(relPath string) *content.Page {
	for _, pg := range p.site.Pages {
		if pg.RelPath == relPath {
			return pg
		}
	}
	return nil
}
