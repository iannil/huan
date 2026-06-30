package build

// pipeline.go holds the Pipeline type that orchestrates BuildSite stages.
// See build.go::BuildSite for the entry point and Result/Options types.

import (
	"fmt"
	"html/template"
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

// pipeline holds the accumulated state across build stages. Each stage is a
// method on *pipeline that reads prior state and produces the next piece.
//
// Why a struct (not a chain of free functions): the stages share 6+
// long-lived values (cfg, site, siteCtx, lookup, writer, renderer) plus
// options/result. A struct keeps these out of every function signature and
// lets each stage read what it needs without ~10-arg plumbing.
type pipeline struct {
	opts   Options
	logf   func(string, ...any)
	result *Result

	// Stage 1 (loadConfig)
	cfg *config.Config
	now time.Time

	// Stage 2 (loadContent)
	pages []*content.Page
	data  map[string]interface{}
	site  *content.Site

	// Stage 3 (setupRendering)
	scRegistry *shortcode.Registry
	md         *markdown.Renderer
	tmpls      *template.Template
	i18nBundle *i18n.Bundle
	renderer   *tmpl.Renderer
	writer     *output.Writer

	// Stage 4 (buildContext)
	siteCtx *tmpl.SiteContext
	lookup  map[*content.Page]*tmpl.Context

	// Stage 5 (renderPages) — renderedCount and errors accumulate into result.
}

// newPipeline initializes the struct with options + result. Stages mutate it.
func newPipeline(opts Options) *pipeline {
	return &pipeline{
		opts:   opts,
		logf:   opts.logf(),
		result: &Result{},
		now:    time.Now(),
	}
}

// --- Stage 1: Load + apply config ---

// loadConfig resolves cfg from CfgOverride (per-language path) or disk,
// then applies serve-mode overrides (BaseURLOverride, MinifyOverride) and
// the multi-language site_translations injection.
//
// All side effects on cfg are confined to this stage — later stages see a
// stable cfg that doesn't shift under them.
func (p *pipeline) loadConfig() error {
	if p.opts.CfgOverride != nil {
		p.cfg = p.opts.CfgOverride
	} else {
		cfg, err := config.Load(p.opts.SourceDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		p.cfg = cfg
	}

	// serve mode overrides cfg.BaseURL so all in-site absolute URLs point at
	// the dev server. Empty = use cfg.BaseURL as-is (production build).
	if p.opts.BaseURLOverride != "" {
		p.cfg.BaseURL = p.opts.BaseURLOverride
	}
	if p.opts.MinifyOverride != nil {
		p.cfg.Minify = *p.opts.MinifyOverride
	}

	// Inject site_translations BEFORE BuildTree (which calls paramsToMap) so
	// the per-language Params propagate to templates. Multi-language only.
	if p.cfg.IsMultiLanguage() && !p.cfg.IsDefaultLanguageCurrent() {
		injectSiteTranslations(p.cfg, p.cfg.LanguageCode)
		p.logf("  i18n inject: site_translations for %s\n", p.cfg.LanguageCode)
	}

	p.logf("Building site: %s\n", p.cfg.Title)
	p.logf("  Source:      %s\n", p.opts.SourceDir)
	p.logf("  Output:      %s\n", p.opts.OutputDir)
	p.logf("  BaseURL:     %s\n", p.cfg.BaseURL)
	return nil
}

// --- Stage 2: Load content + data files ---

// loadContent scans content/, applies the optional PageFilter (per-language
// subsets), and loads data/*.yaml. The returned site has Taxonomies set up
// by buildContentTree; this stage only loads raw pages + data.
func (p *pipeline) loadContent() error {
	contentDir := filepath.Join(p.opts.SourceDir, "content")

	if err := p.checkStaleTranslations(contentDir); err != nil {
		return err
	}

	pages, err := content.LoadDir(contentDir)
	if err != nil {
		return fmt.Errorf("load content: %w", err)
	}
	if p.opts.PageFilter != nil {
		filtered := make([]*content.Page, 0, len(pages))
		for _, pg := range pages {
			if p.opts.PageFilter(pg) {
				filtered = append(filtered, pg)
			}
		}
		p.logf("  Pages after filter: %d (of %d loaded)\n", len(filtered), len(pages))
		pages = filtered
	}
	p.logf("  Pages loaded: %d\n", len(pages))
	p.pages = pages

	dataDir := filepath.Join(p.opts.SourceDir, "data")
	data, err := content.LoadDataFiles(dataDir)
	if err != nil {
		return fmt.Errorf("load data: %w", err)
	}
	p.logf("  Data files:   %d\n", len(data))
	p.data = data
	return nil
}

// checkStaleTranslations surfaces stale .en.md sidecars (source_hash
// mismatch). In strict mode (CI), fails the build; locally, logs warnings.
func (p *pipeline) checkStaleTranslations(contentDir string) error {
	if !p.cfg.IsMultiLanguage() {
		return nil
	}
	report, err := checkStaleTranslations(contentDir)
	if err != nil {
		p.logf("  WARN: i18n stale check error: %v\n", err)
		return nil
	}
	if report.Checked == 0 && report.Stale == 0 && report.Missing == 0 {
		return nil
	}
	p.logf("  i18n stale check: %d checked, %d stale, %d missing source_hash\n",
		report.Checked, report.Stale, report.Missing)
	if strictI18nEnabled() && (report.Stale > 0 || report.Missing > 0) {
		return fmt.Errorf("%s", report.Error())
	}
	if report.Stale > 0 || report.Missing > 0 {
		p.logf("  WARN: stale translations found (run `huan translate qwen3` to refresh):\n%s\n",
			report.Error())
	}
	return nil
}

// --- Stage 3: Render Markdown per page, build content tree, taxonomies ---

// renderMarkdownAndTree runs shortcode expansion + goldmark rendering on
// each page, computes Plain/WordCount/Summary, then builds the content tree
// (sections, hierarchy) and the tag/category taxonomies.
func (p *pipeline) renderMarkdownAndTree() error {
	p.scRegistry = shortcode.NewRegistry()
	p.md = markdown.NewRenderer(&p.cfg.Markup)

	for _, pg := range p.pages {
		if pg.RawContent == "" {
			continue
		}
		if err := p.renderPageMarkdown(pg); err != nil {
			return err
		}
	}

	site, err := content.BuildTree(p.pages, p.cfg, p.opts.SourceDir)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}
	site.Data = p.data
	p.site = site
	p.buildTaxonomies()
	return nil
}

// renderPageMarkdown expands shortcodes, renders Markdown to HTML, and
// computes Plain/WordCount/Summary. Pulled out so the loop body stays flat.
func (p *pipeline) renderPageMarkdown(pg *content.Page) error {
	expanded, err := p.scRegistry.Expand(pg.RawContent, pg, nil)
	if err != nil {
		return fmt.Errorf("shortcode %s: %w", pg.RelPath, err)
	}
	html, err := p.md.Render(expanded)
	if err != nil {
		return fmt.Errorf("render %s: %w", pg.RelPath, err)
	}
	pg.Content = template.HTML(html)

	// Plain + WordCount from rendered HTML (matches Hugo's word counting).
	plain := StripHTMLTagsForSummary(html)
	pg.Plain = plain
	pg.WordCount = CountWordsInPlain(plain)

	// Summary: content up to <!--more--> marker, else first ~120 words
	// truncated at the enclosing block boundary (Hugo's behavior).
	if idx := strings.Index(pg.RawContent, "<!--more-->"); idx >= 0 {
		before := pg.RawContent[:idx]
		if beforeHTML, err := p.md.Render(before); err == nil {
			pg.Summary = template.HTML(beforeHTML)
		}
	} else {
		pg.Summary = template.HTML(TruncateHTMLToBlockBoundary(string(pg.Content), 120))
	}
	return nil
}

// buildTaxonomies constructs the tag taxonomy from site.Pages (all non-draft
// pages including hidden/never-listed) and converts to content.Taxonomy.
// Also stores original-cased tag names so term-page titles render as Hugo
// does (e.g. "FANFAN on ...") while URL paths use the lowercased key.
func (p *pipeline) buildTaxonomies() {
	taxonomies, originalCases := taxonomy.BuildAllWithOriginalCase(p.site.Pages)
	taxCount := 0
	if tax, ok := taxonomies["tags"]; ok {
		taxCount = len(tax)
	}
	p.logf("  Tags:         %d unique\n", taxCount)
	converted := map[string]content.Taxonomy{}
	for name, tax := range taxonomies {
		entry := content.Taxonomy{}
		for term, pages := range tax {
			entry[term] = pages
		}
		converted[name] = entry
	}
	p.site.Taxonomies = converted
	p.site.TaxonomyOriginalCase = originalCases
}
