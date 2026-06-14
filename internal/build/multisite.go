package build

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// MultiSiteResult aggregates per-language build results. Returned by
// BuildMultiSite when huan.yaml declares more than one language under the
// `languages:` block (see docs/adr/0007-i18n-build-system.md).
type MultiSiteResult struct {
	// PerLanguage holds the build result for each configured language, in
	// weight-ascending then code-alphabetical order (matches
	// config.SortedLanguages()).
	PerLanguage []LanguageBuildResult

	// TotalDuration is the wall-clock time of the entire multi-language build.
	TotalDuration time.Duration
}

// LanguageBuildResult pairs a language code with its build Result.
type LanguageBuildResult struct {
	Code   string
	Result *Result
}

// BuildMultiSite renders the site once per configured language, outputting
// each language's files under its baseURL prefix.
//
// Behavior:
//  1. Load config + content once (shared across languages).
//  2. Build AvailableTranslations map: for each page's RelPath, which languages
//     have actual sidecar files. Used by buildTranslationLinks to filter
//     hreflang output to only existing translations.
//  3. For each language (sorted by weight):
//     a. Clone the master cfg.
//     b. Override Title/LanguageCode per `languages.<code>` config.
//     c. Append language baseURL to cfg.BaseURL when non-empty (e.g. /en).
//     d. Append language baseURL to opts.OutputDir when non-empty.
//     e. Set PageFilter: default-lang pages (Language="" or matches default code)
//        go to the default build; sidecar pages (Language="<code>") go to
//        their respective language build.
//     f. Call BuildSite with CfgOverride + PageFilter + AvailableTranslations.
//
// Single-language backward compatibility: when cfg.Languages is empty, callers
// should use BuildSite directly. BuildMultiSite returns an error in that case
// (caller bug, not a runtime condition).
func BuildMultiSite(opts Options) (*MultiSiteResult, error) {
	multiStart := time.Now()

	// Load master config once.
	masterCfg, err := config.Load(opts.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if !masterCfg.IsMultiLanguage() {
		return nil, fmt.Errorf("BuildMultiSite: cfg has no languages: block; use BuildSite for single-language builds")
	}

	// Pre-scan content directory to build the AvailableTranslations map.
	// This lets hreflang output skip languages that don't have sidecar files
	// for a given page (prevents SEO 404s).
	available, err := buildAvailableTranslations(opts.SourceDir, masterCfg)
	if err != nil {
		return nil, fmt.Errorf("scan translations: %w", err)
	}

	defaultCode := masterCfg.DefaultLanguageCode()
	result := &MultiSiteResult{}

	for _, entry := range masterCfg.SortedLanguages() {
		code := entry.Code
		lang := entry.Lang

		// Clone master cfg for this language
		langCfg := *masterCfg // shallow copy is sufficient; we only override top-level fields

		// Override Title per-language config
		if lang.Title != "" {
			langCfg.Title = lang.Title
		}
		// Override LanguageCode: explicit lang.LanguageCode wins, else use code
		if lang.LanguageCode != "" {
			langCfg.LanguageCode = lang.LanguageCode
		} else {
			langCfg.LanguageCode = code
		}

		// Adjust BaseURL: append language prefix when set (e.g. /en)
		// Example: cfg.BaseURL = "https://zhurongshuo.com/", lang.BaseURL = "/en"
		//      → effective BaseURL = "https://zhurongshuo.com/en/"
		if lang.BaseURL != "" {
			base := strings.TrimRight(masterCfg.BaseURL, "/")
			langCfg.BaseURL = base + "/" + strings.Trim(lang.BaseURL, "/") + "/"
		}

		// Per-language Options
		langOpts := opts
		// Append language baseURL to OutputDir for non-default languages
		if lang.BaseURL != "" {
			langOpts.OutputDir = filepath.Join(opts.OutputDir, lang.BaseURL)
		}
		// Inject cfg override + page filter + translations map
		langOpts.CfgOverride = &langCfg
		langOpts.AvailableTranslations = available
		cc := code
		isDefault := code == defaultCode
		langOpts.PageFilter = func(p *content.Page) bool {
			if isDefault {
				// Default language: pages with empty Language OR explicit default code
				return p.Language == "" || p.Language == cc
			}
			// Non-default language: only pages with explicit matching code
			return p.Language == cc
		}

		// Dispatch to single-language BuildSite
		built, err := BuildSite(langOpts)
		if err != nil {
			return result, fmt.Errorf("build language %s: %w", code, err)
		}
		result.PerLanguage = append(result.PerLanguage, LanguageBuildResult{
			Code:   code,
			Result: built,
		})
	}

	result.TotalDuration = time.Since(multiStart)
	return result, nil
}

// buildAvailableTranslations scans the content directory and returns a map
// from language-neutral RelPath (e.g. "posts/foo.md") to the set of language
// codes that have an actual sidecar file.
//
// Examples:
//   - "posts/foo.md" + "posts/foo.en.md" exists → {"posts/foo.md": {"": true, "en": true}}
//   - "posts/bar.md" only → {"posts/bar.md": {"": true}}
//
// The default language (Language="") is always available for any page that
// exists at all. Non-default languages are only marked available when the
// .<lang>.md sidecar file is present.
//
// Auto-created pages (home, auto-section) are NOT in this map — they're
// synthesized per-build, not from .md files. buildTranslationLinks treats
// absence from the map as "all languages available" for these pages.
func buildAvailableTranslations(sourceDir string, cfg *config.Config) (map[string]map[string]bool, error) {
	contentDir := filepath.Join(sourceDir, "content")
	pages, err := content.LoadDir(contentDir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]bool)
	for _, p := range pages {
		if _, ok := out[p.RelPath]; !ok {
			out[p.RelPath] = make(map[string]bool)
		}
		// p.Language is the language code from filename suffix ("" for default)
		// Normalize empty to default language code for consistent lookup.
		lang := p.Language
		if lang == "" {
			lang = cfg.DefaultLanguageCode()
		}
		out[p.RelPath][lang] = true
	}
	return out, nil
}

// SummarizeMultiSite returns a one-line summary string for a MultiSiteResult,
// suitable for CLI output. Format: "built N languages: zh-cn=X pages en=Y pages (Dduration)".
func SummarizeMultiSite(r *MultiSiteResult) string {
	if r == nil || len(r.PerLanguage) == 0 {
		return "no languages built"
	}
	var parts []string
	for _, lb := range r.PerLanguage {
		if lb.Result == nil {
			parts = append(parts, fmt.Sprintf("%s=?", lb.Code))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d pages", lb.Code, lb.Result.PagesRendered))
	}
	return fmt.Sprintf("built %d languages: %s (%s)",
		len(r.PerLanguage), strings.Join(parts, " "), r.TotalDuration.Round(time.Millisecond))
}
