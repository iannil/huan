package build

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)

// JITRenderFast renders a single page for JIT, skipping the full pipeline.
// Unlike RenderPageWithCache which reloads ALL content + rebuilds ALL contexts,
// JITRenderFast loads ONLY the target page, renders ONLY its markdown, and
// builds ONLY its context. This reduces JIT latency from O(N) to O(1).
//
// Preconditions:
//   - cache is non-nil and has been populated by at least one full build
//   - cache.ContentCache is non-nil
//   - The page exists on disk (caller should verify before calling)
//
// Returns "" with no error when the page should not be rendered (draft/future).
func JITRenderFast(opts Options, cache *PipelineCache, pageURL string) (string, error) {
	// 1. Resolve source file from URL.
	sourceRel := ResolveSourceFromURL(pageURL)
	if sourceRel == "" {
		return "", fmt.Errorf("JITRenderFast: cannot resolve source for %s", pageURL)
	}

	sourcePath := filepath.Join(opts.SourceDir, "content", sourceRel)
	if _, err := os.Stat(sourcePath); err != nil {
		return "", fmt.Errorf("JITRenderFast: source not found: %s", sourcePath)
	}

	// 2. Load single page via ContentCache.
	pg, err := cache.ContentCache.GetOrLoad(sourceRel, func(path string) (*content.Page, time.Time, error) {
		data, rerr := os.ReadFile(sourcePath)
		if rerr != nil {
			return nil, time.Time{}, rerr
		}
		page, perr := content.LoadPageFromFrontmatterData(data, sourceRel)
		if perr != nil {
			return nil, time.Time{}, perr
		}
		page.FilePath = sourcePath
		fi, serr := os.Stat(sourcePath)
		if serr != nil {
			return nil, time.Time{}, serr
		}
		return page, fi.ModTime(), nil
	})
	if err != nil {
		return "", fmt.Errorf("JITRenderFast: load %s: %w", sourceRel, err)
	}

	// 3. Check draft/future/expired filters.
	if pg.Draft && !opts.IncludeDrafts {
		return "", nil
	}
	now := time.Now()
	if !opts.IncludeFuture && !pg.PublishDateParsed.IsZero() && pg.PublishDateParsed.After(now) {
		return "", nil
	}
	if !opts.IncludeExpired && !pg.ExpiryDateParsed.IsZero() && pg.ExpiryDateParsed.Before(now) {
		return "", nil
	}

	// 4. Render markdown for this single page.
	if pg.RawContent != "" && pg.Content == "" {
		expanded, serr := cache.SCRegistry.Expand(pg.RawContent, pg, nil)
		if serr != nil {
			return "", fmt.Errorf("JITRenderFast: shortcode %s: %w", sourceRel, serr)
		}
		htmlContent, rerr := cache.MDRenderer.Render(expanded)
		if rerr != nil {
			return "", fmt.Errorf("JITRenderFast: render %s: %w", sourceRel, rerr)
		}
		pg.Content = template.HTML(htmlContent)
		pg.Plain = StripHTMLTagsForSummary(htmlContent)
		pg.WordCount = CountWordsInPlain(pg.Plain)
	}

	// 5. Build single-page context (no site-wide context, no DAG).
	siteCtx := &tmpl.SiteContext{
		Config: cache.SiteCfg,
	}

	tmplName := ResolveTemplateName(cache.Templates, pg)
	if tmplName == "" {
		return "", fmt.Errorf("JITRenderFast: no template for %s", sourceRel)
	}

	ctx := tmpl.NewContext(pg, siteCtx, cache.SiteCfg)
	if pg.Kind == "section" || pg.Kind == "home" {
		return "", fmt.Errorf("JITRenderFast: list page %s not supported, use RenderPageWithCache", pageURL)
	}

	// 6. Render the single page.
	renderer := tmpl.NewRenderer(cache.Templates, tmpl.FuncMap(cache.SiteCfg.BaseURL))
	out, err := renderer.Render(tmplName, ctx)
	if err != nil {
		return "", fmt.Errorf("JITRenderFast: render %s: %w", pageURL, err)
	}

	// 7. Serve-mode LiveReload injection.
	if opts.InjectLiveReload && opts.LiveReloadURL != "" {
		out = InjectLiveReload(out, opts.LiveReloadURL)
	}

	return out, nil
}