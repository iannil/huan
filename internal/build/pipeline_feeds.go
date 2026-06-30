package build

// pipeline_feeds.go: stage 6 — taxonomy term pages, categories, paginated
// home, 404, sitemap, search.json, AI outputs (llms.txt + content API).

import (
	"fmt"

	"github.com/iannil/huan/internal/output"
	tmpl "github.com/iannil/huan/internal/template"
)

// Note: `strings` was removed from this file because no stage method here
// uses it after extraction. If a future stage needs it, re-add.

// renderFeedsAndSpecials renders all non-page outputs that depend on the
// full site context: taxonomy term listing + per-term pages, empty
// categories, paginated home (/page/N/), 404, sitemap, search.json,
// llms.txt, and /api/{section}.json.
func (p *pipeline) renderFeedsAndSpecials() {
	p.renderTaxonomyPages()
	p.renderEmptyCategories()
	p.renderPaginatedHome()
	p.render404()
	p.renderSitemap()
	p.renderSearchIndex()
	p.renderAIOutputs()
}

// renderTaxonomyPages emits /tags/ (terms listing) + /tags/{tag}/ per term
// (HTML + RSS). Skipped if BuildTaxonomyContext returns nil (no tags).
func (p *pipeline) renderTaxonomyPages() {
	taxCtx := BuildTaxonomyContext(p.siteCtx, p.lookup, p.site, p.cfg)
	if taxCtx == nil {
		return
	}

	// /tags/ — terms listing page.
	if html, err := p.renderer.Render("_default/terms.html", taxCtx); err == nil {
		_ = p.writer.Write("tags/index.html", html)
	} else {
		p.logf("  WARN: terms: %v\n", err)
	}

	// /tags/index.xml — RSS for the listing itself.
	if html, err := p.renderer.Render("_default/rss.xml", taxCtx); err == nil {
		_ = p.writer.Write("tags/index.xml", html)
	}

	// /tags/{tag}/ — one page per term (HTML + RSS).
	if termsTmpl := p.tmpls.Lookup("_default/list.html"); termsTmpl != nil {
		for _, term := range taxCtx.DataTerms {
			termCtx := BuildTermContext(p.siteCtx, p.lookup, p.site, p.cfg, term.Name, term.Pages)
			if termCtx == nil {
				continue
			}
			tagSlug := URLEscape(term.Name)
			if html, err := p.renderer.Render("_default/list.html", termCtx); err == nil {
				_ = p.writer.Write("tags/"+tagSlug+"/index.html", html)
			}
			if html, err := p.renderer.Render("_default/rss.xml", termCtx); err == nil {
				_ = p.writer.Write("tags/"+tagSlug+"/index.xml", html)
			}
		}
	}
}

// renderEmptyCategories emits the (empty) categories listing so the URL
// resolves. Hugo emits these by default even when no categories are defined.
func (p *pipeline) renderEmptyCategories() {
	catCtx := BuildEmptyTaxonomyContext(p.siteCtx, "Categories", "categories")
	if catCtx == nil {
		return
	}
	if html, err := p.renderer.Render("_default/terms.html", catCtx); err == nil {
		_ = p.writer.Write("categories/index.html", html)
	}
	if html, err := p.renderer.Render("_default/rss.xml", catCtx); err == nil {
		_ = p.writer.Write("categories/index.xml", html)
	}
}

// renderPaginatedHome emits /page/1/ (redirect to /) and /page/N/ for
// N ≥ 2 (actual paginated home pages). Page size from cfg.Paginate, default
// 10. Pagination scope: site.RegularPages filtered to cfg.MainSections.
func (p *pipeline) renderPaginatedHome() {
	homeCtx := FindHomeContext(p.lookup, p.site)
	if homeCtx == nil {
		return
	}

	mainPageItems := FilterMainSections(p.siteCtx.RegularPages, p.cfg.Params.MainSections)
	pageSize := p.cfg.Paginate
	if pageSize <= 0 {
		pageSize = 10
	}
	totalPages := (len(mainPageItems) + pageSize - 1) / pageSize

	// /page/1/ — pre-formatted redirect, bypasses minify/canonify.
	redirect := fmt.Sprintf(
		`<!doctype html><html lang=%s><head><title>%s</title><link rel=canonical href=%s><meta charset=utf-8><meta http-equiv=refresh content="0; url=%s"></head></html>`,
		p.cfg.LanguageCode, p.cfg.BaseURL, p.cfg.BaseURL, p.cfg.BaseURL)
	if err := p.writer.WriteBytesPath("page/1/index.html", []byte(redirect)); err != nil {
		p.logf("  WARN: write page/1: %v\n", err)
	}

	// /page/2/, /page/3/, ... — actual paginated home pages.
	for i := 2; i <= totalPages; i++ {
		pagedCtx := CloneContextForPagination(homeCtx, mainPageItems, pageSize, i, totalPages)
		html, err := p.renderer.Render("index.html", pagedCtx)
		if err != nil {
			continue
		}
		_ = p.writer.Write(fmt.Sprintf("page/%d/index.html", i), html)
	}
}

// render404 emits the site's 404.html if the template exists. The context
// is a synthetic 404 page with no parent section.
func (p *pipeline) render404() {
	if p.tmpls.Lookup("404.html") == nil {
		return
	}
	ctx404 := &tmpl.Context{
		Kind:         "404",
		Title:        "404 Page not found",
		Site:         p.siteCtx,
		Data:         p.siteCtx.Data,
		Scratch:      tmpl.NewScratch(),
		RelPermalink: "/404.html",
		Permalink:    p.siteCtx.BaseURL + "404.html",
		OutputFormats: tmpl.HTMLOnlyOutputFormats(p.siteCtx.BaseURL+"404.html", "/404.html"),
	}
	if html, err := p.renderer.Render("404.html", ctx404); err == nil {
		_ = p.writer.Write("404.html", html)
	}
}

// renderSitemap emits /sitemap.xml from BuildSitemapContext. Skipped when
// the context is nil (very small sites with no regular pages).
func (p *pipeline) renderSitemap() {
	ctx := BuildSitemapContext(p.siteCtx, p.lookup, p.site, p.cfg)
	if ctx == nil {
		return
	}
	if html, err := p.renderer.Render("_default/sitemap.xml", ctx); err == nil {
		_ = p.writer.Write("sitemap.xml", html)
	} else {
		p.logf("  WARN: sitemap: %v\n", err)
	}
}

// renderSearchIndex emits /search.json from the home page context. Skipped
// silently when the template is missing or rendering fails.
func (p *pipeline) renderSearchIndex() {
	homeCtx := FindHomeContext(p.lookup, p.site)
	if homeCtx == nil {
		return
	}
	html, err := p.renderer.Render("_default/index.searchindex.json", homeCtx)
	if err != nil {
		p.logf("  WARN: search: %v\n", err)
		return
	}
	if err := p.writer.Write("search.json", html); err != nil {
		p.logf("  WARN: write search.json: %v\n", err)
	}
}

// renderAIOutputs emits the AI-consumer outputs (llms.txt + /api/{section}.json)
// when their respective config flags are enabled. Both are no-ops when cfg.AI
// is at defaults — see ADR 0001 §3 ("甚至更好登记簿").
func (p *pipeline) renderAIOutputs() {
	if p.cfg.AI.LlmsTxt {
		if err := output.GenerateLlmsTxt(p.opts.OutputDir, p.opts.SourceDir, p.cfg); err != nil {
			p.logf("  WARN: llms.txt: %v\n", err)
		}
	}
	if p.cfg.AI.ContentAPI {
		if err := output.GenerateContentAPI(p.opts.OutputDir, p.site, p.cfg,
			p.opts.IncludeDrafts, p.opts.IncludeFuture, p.opts.IncludeExpired, p.now); err != nil {
			p.logf("  WARN: content api: %v\n", err)
		}
	}
}

// strings import placeholder removed (no stage here uses strings).

