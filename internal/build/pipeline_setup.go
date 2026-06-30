package build

// pipeline_setup.go: stage 3 (templates + i18n + writer) and stage 4
// (siteCtx + per-page context + link relationships).

import (
	"os"
	"path/filepath"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/i18n"
	"github.com/iannil/huan/internal/output"
	tmpl "github.com/iannil/huan/internal/template"
)

// setupTemplatesAndWriter loads templates + i18n bundle, configures the
// renderer with the cfg.BaseURL FuncMap, and constructs the writer with
// minify + canonify options matching cfg.
//
// Order matters: SetI18nBundle must happen BEFORE FuncMap is built (so
// i18nFunc reads the right bundle), and BEFORE any template that calls
// `i18n` is executed.
func (p *pipeline) setupTemplatesAndWriter() error {
	tmpls, err := tmpl.LoadAllTemplates(p.opts.SourceDir, p.cfg.BaseURL)
	if err != nil {
		return err
	}
	p.tmpls = tmpls

	p.i18nBundle = p.loadI18nBundle()
	tmpl.SetI18nBundle(p.i18nBundle)

	templateCount := 0
	for range tmpls.Templates() {
		templateCount++
	}
	p.logf("  Templates:    %d\n", templateCount)

	p.renderer = tmpl.NewRenderer(tmpls, tmpl.FuncMap(p.cfg.BaseURL))

	if p.cfg.Minify {
		p.writer = output.NewWriterWithMinify(p.opts.OutputDir)
	} else {
		p.writer = output.NewWriter(p.opts.OutputDir)
	}
	// Enable canonifyURLs to rewrite root-relative paths to absolute URLs.
	p.writer.SetCanonify(output.CanonifyOptions{BaseURL: p.cfg.BaseURL})
	return nil
}

// loadI18nBundle loads translations for the current language. Multi-language
// builds load ONLY the current lang file from theme + project; single-lang
// builds load all files (backward compat).
//
// The two-mode branch (multi vs single) preserves the pre-existing behavior
// captured in build.go before the BuildSite refactor — see ADR 0007.
func (p *pipeline) loadI18nBundle() *i18n.Bundle {
	bundle := i18n.New()
	currentLang := p.cfg.LanguageCode
	if p.cfg.IsMultiLanguage() && currentLang != "" {
		// Multi-language: load only the current language file.
		themeFile := filepath.Join(p.opts.SourceDir, "themes", DetectThemeName(p.opts.SourceDir), "i18n", currentLang+".yaml")
		if _, err := os.Stat(themeFile); err == nil {
			_ = bundle.LoadFile(themeFile)
		}
		projectFile := filepath.Join(p.opts.SourceDir, "i18n", currentLang+".yaml")
		if _, err := os.Stat(projectFile); err == nil {
			_ = bundle.LoadFile(projectFile)
		}
		p.logf("  i18n bundle:  %s (%d keys)\n", currentLang, bundle.Keys())
	} else {
		// Single-language backward compat: load all files in i18n/ dirs.
		themeDir := filepath.Join(p.opts.SourceDir, "themes", DetectThemeName(p.opts.SourceDir), "i18n")
		if _, err := os.Stat(themeDir); err == nil {
			_ = bundle.LoadDir(themeDir)
		}
		projectDir := filepath.Join(p.opts.SourceDir, "i18n")
		if _, err := os.Stat(projectDir); err == nil {
			_ = bundle.LoadDir(projectDir)
		}
	}
	return bundle
}

// buildContexts constructs the site-wide SiteContext, one per-page Context
// per sitePage, and wires Page↔Page link relationships (Prev/Next/Parent).
// After this stage, every page has a fully populated Context ready for
// template rendering.
func (p *pipeline) buildContexts() {
	siteCtx := tmpl.NewSiteContext(p.site, p.cfg)
	siteCtx.AvailableTranslations = p.opts.AvailableTranslations

	lookup := map[*content.Page]*tmpl.Context{}
	for _, pg := range p.site.Pages {
		lookup[pg] = tmpl.NewContext(pg, siteCtx, p.cfg)
	}
	for _, pg := range p.site.Pages {
		if ctx, ok := lookup[pg]; ok {
			tmpl.LinkPageRelationships(ctx, pg, lookup)
		}
	}
	tmpl.PopulateSitePages(siteCtx, p.site, lookup)

	p.siteCtx = siteCtx
	p.lookup = lookup
}
