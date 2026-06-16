package config

import (
	"sort"
	"strings"
)

// LanguageConfig holds per-language settings declared under the `languages:`
// block in huan.yaml. Mirrors Hugo's languages map structure for familiarity.
//
// Example yaml:
//
//	languages:
//	  zh-cn:
//	    weight: 1
//	    languageName: 中文
//	    baseURL: ""           # root path
//	    title: "祝融说。"
//	  en:
//	    weight: 2
//	    languageName: English
//	    baseURL: "/en"        # subpath prefix
//	    title: "Zhurong Says"
type LanguageConfig struct {
	// Weight controls sort order in sitemap.xml and language switcher UI.
	// Lower weight sorts first. Default 0.
	Weight int `yaml:"weight"`

	// LanguageName is the display name (e.g. "中文", "English").
	LanguageName string `yaml:"languageName"`

	// BaseURL is the URL prefix for this language's content. Empty string
	// means root path (typically the default language). Non-empty (e.g. "/en")
	// prepends to all URLs for this language's pages.
	BaseURL string `yaml:"baseURL"`

	// ContentDir overrides the default content/ directory for this language.
	// Empty means use the project's top-level content/ dir (default behavior).
	// When set, huan loads pages from this dir INSTEAD of looking for
	// .<lang>.md sidecars in the main content/ dir.
	ContentDir string `yaml:"contentDir"`

	// Title overrides the top-level cfg.Title for this language.
	Title string `yaml:"title"`

	// LanguageCode is the BCP-47 language code (e.g. "zh-cn", "en").
	// Empty means use the map key from `languages:` block.
	LanguageCode string `yaml:"languageCode"`

	// ExcludeSections lists top-level content sections (e.g. "books",
	// "gallery") that should NOT appear in this language. Pages under these
	// sections are dropped from the build (no content/listing pages, no
	// sitemap entries), and templates use IsSectionExcludedForLang to hide
	// their entry points (menu items, social icons). Typically used to keep
	// untranslated or intentionally-monolingual sections off a translated
	// site. Empty means no exclusion (default).
	ExcludeSections []string `yaml:"excludeSections"`

	// CatalogSections lists top-level sections rendered in this language as
	// "catalog-only": the section index page renders (from its `_index.<lang>.md`
	// sidecar), but the section's content pages are dropped from the build (no
	// content pages, no sitemap entries). Used to advertise an untranslated
	// section's contents (e.g. a disabled book index) without publishing the
	// individual pages.
	CatalogSections []string `yaml:"catalogSections"`

	// NeutralSections lists top-level sections whose content is language-neutral
	// (e.g. an image gallery). In this (non-default) language's build, the
	// section's default-language content pages are included as-is, while the
	// section index uses this language's `_index.<lang>.md` sidecar.
	NeutralSections []string `yaml:"neutralSections"`
}

// IsMultiLanguage reports whether cfg has a non-empty Languages map with at
// least one entry. Used by build pipeline to decide between the single-
// language fast path and the multi-language BuildMultiSite path.
func (c *Config) IsMultiLanguage() bool {
	return len(c.Languages) > 0
}

// DefaultLanguageCode returns the language code of the default (lowest-weight,
// or defaultContentLanguage) language. Returns cfg.LanguageCode when no
// languages: block is configured (single-language backward compat).
func (c *Config) DefaultLanguageCode() string {
	if !c.IsMultiLanguage() {
		return c.LanguageCode
	}
	// If defaultContentLanguage is set and exists in map, use it
	if c.DefaultContentLanguage != "" {
		if _, ok := c.Languages[c.DefaultContentLanguage]; ok {
			return c.DefaultContentLanguage
		}
	}
	// Otherwise pick lowest-weight language
	return c.lowestWeightLanguageCode()
}

// DefaultLanguage returns the LanguageConfig for the default language.
// Returns a zero-value LanguageConfig when single-language (caller should
// fall back to cfg-level Title / BaseURL / LanguageCode in that case).
func (c *Config) DefaultLanguage() LanguageConfig {
	code := c.DefaultLanguageCode()
	if lang, ok := c.Languages[code]; ok {
		return lang
	}
	return LanguageConfig{}
}

// LanguageEntry pairs a language code with its LanguageConfig, returned by
// SortedLanguages for deterministic iteration.
type LanguageEntry struct {
	Code string
	Lang LanguageConfig
}

// SortedLanguages returns Languages sorted by Weight (ascending), then by
// language code (alphabetical) for stable iteration. Used by templates and
// sitemap to enumerate languages in deterministic order.
func (c *Config) SortedLanguages() []LanguageEntry {
	out := make([]LanguageEntry, 0, len(c.Languages))
	for code, lang := range c.Languages {
		out = append(out, LanguageEntry{Code: code, Lang: lang})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lang.Weight != out[j].Lang.Weight {
			return out[i].Lang.Weight < out[j].Lang.Weight
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// lowestWeightLanguageCode returns the language code with the smallest Weight.
// Ties broken alphabetically by code. Used as fallback when
// defaultContentLanguage is unset or invalid.
func (c *Config) lowestWeightLanguageCode() string {
	sorted := c.SortedLanguages()
	if len(sorted) == 0 {
		return ""
	}
	return sorted[0].Code
}

// LanguageBaseURL returns the BaseURL prefix for the given language code.
// Returns empty string for unknown languages or single-language configs.
func (c *Config) LanguageBaseURL(langCode string) string {
	if !c.IsMultiLanguage() {
		return ""
	}
	if lang, ok := c.Languages[langCode]; ok {
		return lang.BaseURL
	}
	return ""
}

// LanguageName returns the display name for the given language code.
// Returns the code itself when not configured.
func (c *Config) LanguageName(langCode string) string {
	if lang, ok := c.Languages[langCode]; ok && lang.LanguageName != "" {
		return lang.LanguageName
	}
	return langCode
}

// IsDefaultLanguageCurrent returns true when cfg.LanguageCode matches the
// default language code (i.e. this build is rendering the default language).
// Used to decide whether site_translations injection applies.
//
// Returns true for single-language configs.
func (c *Config) IsDefaultLanguageCurrent() bool {
	if !c.IsMultiLanguage() {
		return true
	}
	return c.LanguageCode == c.DefaultLanguageCode()
}

// TopSection returns the first path segment of a URL or content-relative path,
// e.g. "/books/foo/" → "books", "products/x.md" → "products",
// "http://h/books/" → "books". Returns "" when there is no segment.
func TopSection(urlOrPath string) string {
	s := urlOrPath
	// Strip scheme://host if present.
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			s = rest[slash:]
		} else {
			return ""
		}
	}
	s = strings.TrimLeft(s, "/")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}

// IsSectionExcludedForLang reports whether the section that urlOrPath belongs
// to is excluded for the given language code. The language's BaseURL prefix
// (e.g. "/en") is stripped first so both raw config URLs ("/books/") and
// language-prefixed URLs ("/en/books/") resolve to the same top section.
func (c *Config) IsSectionExcludedForLang(code, urlOrPath string) bool {
	lang, ok := c.Languages[code]
	if !ok || len(lang.ExcludeSections) == 0 {
		return false
	}
	s := urlOrPath
	// Strip scheme://host so we can trim a leading language prefix uniformly.
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			s = rest[slash:]
		} else {
			s = "/"
		}
	}
	if base := strings.Trim(lang.BaseURL, "/"); base != "" {
		trimmed := strings.TrimLeft(s, "/")
		if trimmed == base {
			trimmed = ""
		} else if rest, found := strings.CutPrefix(trimmed, base+"/"); found {
			trimmed = rest
		}
		s = "/" + trimmed
	}
	top := TopSection(s)
	for _, ex := range lang.ExcludeSections {
		if ex == top {
			return true
		}
	}
	return false
}
