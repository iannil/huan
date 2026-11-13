package build

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// MultiSiteResult aggregates per-language build results. Returned by
// BuildMultiSite when huan.yaml declares more than one language under the
// `languages:` block (see docs/adr/0007-i18n-build-system.md).
type MultiSiteResult struct {
	// PerLanguage holds the build result for each configured language, with
	// the default language first, then the rest in weight-ascending then
	// code-alphabetical order (matches config.SortedLanguages(), with the
	// default hoisted to the front).
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
//     go to the default build; sidecar pages (Language="<code>") go to
//     their respective language build.
//     f. Call BuildSite with CfgOverride + PageFilter + AvailableTranslations.
//
// Single-language backward compatibility: when cfg.Languages is empty, callers
// should use BuildSite directly. BuildMultiSite returns an error in that case
// (caller bug, not a runtime condition).
func BuildMultiSite(opts Options) (*MultiSiteResult, error) {
	multiStart := time.Now()

	// Load master config once.
	var masterCfg *config.Config
	err := opts.Timings.Measure("multi", "load config", func() error { var err error; masterCfg, err = config.Load(opts.SourceDir); return err })
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if !masterCfg.IsMultiLanguage() {
		return nil, fmt.Errorf("BuildMultiSite: cfg has no languages: block; use BuildSite for single-language builds")
	}

	// serve mode passes a dev BaseURL override; apply it at the master level so
	// per-language prefixes (e.g. /en/) are computed from the dev URL. Cleared
	// per-language below so BuildSite doesn't re-clobber the computed prefix.
	if opts.BaseURLOverride != "" {
		masterCfg.BaseURL = opts.BaseURLOverride
	}

	// Load raw content and data once and derive AvailableTranslations.
	// This lets hreflang output skip languages that don't have sidecar files
	// for a given page (prevents SEO 404s).
	var inputs *buildInputs
	err = opts.Timings.Measure("multi", "shared input preparation", func() error {
		var err error
		inputs, err = loadBuildInputsWithCache(opts.SourceDir, masterCfg, opts.Timings, opts.ParseCache)
		return err
	})
	if err != nil {
		return nil, err
	}

	available := inputs.available
	defaultCode := masterCfg.DefaultLanguageCode()
	result := &MultiSiteResult{}

	// SortedLanguages orders by weight/code only, which does NOT guarantee the
	// default language comes first (a user may give a non-default language a
	// smaller weight). Since the default language builds into the publishDir
	// root and cleanPublishDir wipes it, it MUST build first — otherwise a
	// later default build would delete other languages' output under the root.
	// Move the default language to the front, preserving weight/code order
	// among the rest.
	langs := masterCfg.SortedLanguages()
	for i, entry := range langs {
		if entry.Code == defaultCode {
			langs = slices.Delete(langs, i, i+1)
			langs = slices.Insert(langs, 0, entry)
			break
		}
	}

	// Prepare one build spec per language. Serial mode writes straight into
	// opts.OutputDir (default language first, then each language under its
	// baseURL prefix). Parallel mode writes each language into an isolated
	// staging dir and renames them into place at the end, because the
	// default language's cleanPublishDir would otherwise wipe the root while
	// a concurrent language is still writing under it.
	specs := make([]languageSpec, 0, len(langs))
	for _, entry := range langs {
		specs = append(specs, makeLanguageSpec(opts, masterCfg, entry, defaultCode, available))
	}

	if opts.ParallelLanguages && len(specs) > 1 {
		if err := opts.Timings.Measure("multi", "parallel language builds", func() error {
			return buildLanguagesParallel(opts, specs, inputs, masterCfg)
		}); err != nil {
			return result, err
		}
	} else {
		for i := range specs {
			built, err := buildSiteWithInputs(specs[i].opts, inputs)
			if err != nil {
				return result, fmt.Errorf("build language %s: %w", specs[i].code, err)
			}
			specs[i].result = built
		}
	}

	for _, s := range specs {
		result.PerLanguage = append(result.PerLanguage, LanguageBuildResult{
			Code:   s.code,
			Result: s.result,
		})
	}

	result.TotalDuration = time.Since(multiStart)
	return result, nil
}

// languageSpec is one language's fully prepared build.
type languageSpec struct {
	code       string
	isDefault  bool
	urlPrefix  string // trimmed lang.BaseURL, empty for default/root
	stagingDir string // parallel mode only
	opts       Options
	result     *Result
}

func makeLanguageSpec(opts Options, masterCfg *config.Config, entry config.LanguageEntry, defaultCode string, available map[string]map[string]bool) languageSpec {
	code := entry.Code
	lang := entry.Lang
	isDefault := code == defaultCode
	urlPrefix := strings.Trim(lang.BaseURL, "/")

	langCfg := *masterCfg // shallow copy; only top-level fields overridden
	if lang.Title != "" {
		langCfg.Title = lang.Title
	}
	applyLanguageParams(&langCfg.Params, lang.Params)
	if lang.LanguageCode != "" {
		langCfg.LanguageCode = lang.LanguageCode
	} else {
		langCfg.LanguageCode = code
	}
	if lang.BaseURL != "" {
		base := strings.TrimRight(masterCfg.BaseURL, "/")
		langCfg.BaseURL = base + "/" + strings.Trim(lang.BaseURL, "/") + "/"
	}

	langOpts := opts
	langOpts.BaseURLOverride = ""
	if isDefault {
		langOpts.OutputDir = opts.OutputDir
	} else {
		langOpts.OutputDir = filepath.Join(opts.OutputDir, filepath.FromSlash(urlPrefix))
	}
	langOpts.CfgOverride = &langCfg
	langOpts.AvailableTranslations = available
	langOpts.StaticExclude = lang.StaticExclude
	langOpts.StaticRoot = lang.StaticRoot
	// Every language is verified to carry the shared static tree: the language
	// versions must correspond file-for-file, never borrow each other's assets.
	langOpts.CheckStaticParity = true
	langOpts.PageFilter = makeLanguagePageFilter(code, defaultCode, isDefault, lang)

	return languageSpec{code: code, isDefault: isDefault, urlPrefix: urlPrefix, opts: langOpts}
}

func buildLanguagesParallel(opts Options, specs []languageSpec, inputs *buildInputs, masterCfg *config.Config) error {
	stagingRoot, err := os.MkdirTemp(filepath.Dir(opts.OutputDir), ".huan-multi-*")
	if err != nil {
		return fmt.Errorf("create multi-language staging: %w", err)
	}
	defer os.RemoveAll(stagingRoot)

	perLangWorkers := parallelLanguageWorkers(len(specs), opts.MaxWorkers)
	var wg sync.WaitGroup
	errs := make([]error, len(specs))
	for i := range specs {
		i := i
		stageDir := filepath.Join(stagingRoot, sanitizeStagingName(specs[i].code))
		specs[i].opts.OutputDir = stageDir
		specs[i].opts.MaxWorkers = perLangWorkers
		specs[i].stagingDir = stageDir
		wg.Add(1)
		go func() {
			defer wg.Done()
			built, buildErr := buildSiteWithInputs(specs[i].opts, inputs)
			specs[i].result = built
			errs[i] = buildErr
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("build language %s: %w", specs[i].code, err)
		}
	}
	return mergeStagedLanguageOutputs(opts.OutputDir, specs)
}

func parallelLanguageWorkers(langCount, requested int) int {
	if langCount < 1 {
		langCount = 1
	}
	total := requested
	if total <= 0 {
		total = runtime.GOMAXPROCS(0)
	}
	if total < 1 {
		total = 1
	}
	share := total / langCount
	if share < 1 {
		share = 1
	}
	return share
}

func sanitizeStagingName(code string) string {
	code = strings.ReplaceAll(code, "/", "_")
	code = strings.ReplaceAll(code, "\\", "_")
	if code == "" {
		return "default"
	}
	return code
}

func makeLanguagePageFilter(code, defaultCode string, isDefault bool, lang config.LanguageConfig) func(*content.Page) bool {
	excluded := lang.ExcludeSections
	catalog := lang.CatalogSections
	neutral := lang.NeutralSections
	cc := code
	def := defaultCode
	return func(p *content.Page) bool {
		if isDefault {
			return p.Language == "" || p.Language == cc
		}
		top := config.TopSection(p.RelPath)
		isIndex := strings.HasPrefix(filepath.Base(p.RelPath), "_index.")
		switch {
		case sectionInList(top, excluded):
			return false
		case sectionInList(top, catalog):
			return isIndex && p.Language == cc
		case sectionInList(top, neutral):
			if isIndex {
				return p.Language == cc
			}
			return p.Language == "" || p.Language == def
		default:
			return p.Language == cc
		}
	}
}

func mergeStagedLanguageOutputs(finalDir string, staged []languageSpec) error {
	if err := os.RemoveAll(finalDir); err != nil {
		return fmt.Errorf("remove previous publish dir: %w", err)
	}
	// Prefer rename over copy. Default language staging becomes the publish
	// root; other languages rename under their URL prefix. Do NOT MkdirAll
	// finalDir first — an empty destination forces a full tree copy.
	var defaultLang *languageSpec
	others := make([]languageSpec, 0, len(staged))
	for i := range staged {
		if staged[i].isDefault {
			defaultLang = &staged[i]
		} else {
			others = append(others, staged[i])
		}
	}
	if defaultLang == nil {
		return fmt.Errorf("merge staged outputs: missing default language")
	}
	if err := promoteStagedDir(defaultLang.stagingDir, finalDir); err != nil {
		return fmt.Errorf("merge language %s: %w", defaultLang.code, err)
	}
	for _, s := range others {
		if s.urlPrefix == "" {
			return fmt.Errorf("language %s: non-default language needs baseURL prefix for parallel merge", s.code)
		}
		dest := filepath.Join(finalDir, filepath.FromSlash(s.urlPrefix))
		if err := promoteStagedDir(s.stagingDir, dest); err != nil {
			return fmt.Errorf("merge language %s: %w", s.code, err)
		}
	}
	return nil
}

func promoteStagedDir(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		// Destination exists unexpectedly; merge by copy.
		return copyDirMerge(src, dest)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	// Cross-device rename failure → copy fallback.
	return copyDirMerge(src, dest)
}

func copyDirMerge(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFileReplace(path, target)
	})
}

func copyFileReplace(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// sectionInList reports whether the given top-level section name is present in
// list. Used by the per-language PageFilter to classify pages.
func sectionInList(top string, list []string) bool {
	return slices.Contains(list, top)
}

// applyLanguageParams overrides top-level Params with per-language values.
// Only non-empty per-language fields win; empty fields keep the top-level
// value (partial override, matching the previous site_translations behavior).
func applyLanguageParams(dst *config.ParamsConfig, lang config.LanguageParamsConfig) {
	if lang.SubTitle != "" {
		dst.SubTitle = lang.SubTitle
	}
	if lang.FooterSlogan != "" {
		dst.FooterSlogan = lang.FooterSlogan
	}
	if lang.Description != "" {
		dst.Description = lang.Description
	}
	if len(lang.Keywords) > 0 {
		dst.Keywords = lang.Keywords
	}
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
	return availableTranslationsFromPages(pages, cfg), nil
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
