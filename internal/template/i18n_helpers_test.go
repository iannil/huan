package template

import (
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

func TestHreflangFunc_MultiLanguage(t *testing.T) {
	cfg := &config.Config{
		BaseURL:                "https://example.com/",
		DefaultContentLanguage: "zh-cn",
		Languages: map[string]config.LanguageConfig{
			"zh-cn": {Weight: 1, LanguageName: "中文", BaseURL: ""},
			"en":    {Weight: 2, LanguageName: "English", BaseURL: "/en"},
		},
	}
	page := &content.Page{
		RelPath: "posts/foo.md",
		URL:     "/posts/foo/",
	}
	siteCtx := &SiteContext{Config: cfg}
	ctx := NewContext(page, siteCtx, cfg)

	got := string(hreflangFunc(ctx))
	// Should contain both zh-cn and en alternate links
	if !strings.Contains(got, `hreflang="zh-cn"`) {
		t.Errorf("missing zh-cn alternate: %s", got)
	}
	if !strings.Contains(got, `hreflang="en"`) {
		t.Errorf("missing en alternate: %s", got)
	}
	// Should contain x-default
	if !strings.Contains(got, `hreflang="x-default"`) {
		t.Errorf("missing x-default: %s", got)
	}
	// en URL should have /en/ prefix
	if !strings.Contains(got, "https://example.com/en/posts/foo/") {
		t.Errorf("en URL missing /en/ prefix: %s", got)
	}
	// zh-cn URL should be at root
	if !strings.Contains(got, "https://example.com/posts/foo/") {
		t.Errorf("zh-cn URL missing root path: %s", got)
	}
}

func TestHreflangFunc_SingleLanguage(t *testing.T) {
	// Single-language: no languages: block → empty output
	cfg := &config.Config{
		BaseURL:      "https://example.com/",
		LanguageCode: "zh-cn",
	}
	page := &content.Page{URL: "/posts/foo/"}
	siteCtx := &SiteContext{Config: cfg}
	ctx := NewContext(page, siteCtx, cfg)

	got := string(hreflangFunc(ctx))
	if got != "" {
		t.Errorf("expected empty for single-language, got %s", got)
	}
}

func TestLangPrefixFunc(t *testing.T) {
	// After PR3 v2: effective language is derived from cfg.LanguageCode
	// (per-build language set by BuildMultiSite), not from page.Language.
	// This is because in a multi-language build, every page in that build
	// shares the same language context.
	tests := []struct {
		name        string
		langCfg     map[string]config.LanguageConfig
		buildLang   string // cfg.LanguageCode (per-build language)
		pageLang    string // p.Language (from filename suffix)
		want        string
	}{
		{
			name:      "zh-cn build default prefix",
			langCfg:   map[string]config.LanguageConfig{"zh-cn": {BaseURL: ""}, "en": {BaseURL: "/en"}},
			buildLang: "zh-cn",
			pageLang:  "",
			want:      "",
		},
		{
			name:      "en build prefix",
			langCfg:   map[string]config.LanguageConfig{"zh-cn": {BaseURL: ""}, "en": {BaseURL: "/en"}},
			buildLang: "en",
			pageLang:  "en",
			want:      "/en",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				BaseURL:                "https://example.com/",
				LanguageCode:           tc.buildLang,
				DefaultContentLanguage: "zh-cn",
				Languages:              tc.langCfg,
			}
			page := &content.Page{URL: "/posts/foo/", Language: tc.pageLang}
			siteCtx := &SiteContext{Config: cfg}
			ctx := NewContext(page, siteCtx, cfg)
			got := langPrefixFunc(ctx)
			if got != tc.want {
				t.Errorf("langPrefix = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTranslationLinksFunc(t *testing.T) {
	cfg := &config.Config{
		BaseURL:                "https://example.com/",
		DefaultContentLanguage: "zh-cn",
		Languages: map[string]config.LanguageConfig{
			"zh-cn": {Weight: 1, LanguageName: "中文", BaseURL: ""},
			"en":    {Weight: 2, LanguageName: "English", BaseURL: "/en"},
		},
	}
	page := &content.Page{URL: "/posts/foo/"}
	siteCtx := &SiteContext{Config: cfg}
	ctx := NewContext(page, siteCtx, cfg)

	links := translationLinksFunc(ctx)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	// Verify LanguageName populated from cfg
	names := map[string]string{}
	for _, l := range links {
		names[l.Lang] = l.LanguageName
	}
	if names["zh-cn"] != "中文" {
		t.Errorf("zh-cn LanguageName = %q", names["zh-cn"])
	}
	if names["en"] != "English" {
		t.Errorf("en LanguageName = %q", names["en"])
	}
}

func TestContext_IsTranslated(t *testing.T) {
	cfg := &config.Config{
		BaseURL:                "https://example.com/",
		DefaultContentLanguage: "zh-cn",
		Languages: map[string]config.LanguageConfig{
			"zh-cn": {Weight: 1, BaseURL: ""},
			"en":    {Weight: 2, BaseURL: "/en"},
		},
	}
	page := &content.Page{URL: "/posts/foo/"}
	siteCtx := &SiteContext{Config: cfg}
	ctx := NewContext(page, siteCtx, cfg)

	if !ctx.IsTranslated() {
		t.Error("expected IsTranslated=true for multi-language page")
	}

	// Single-language: IsTranslated=false
	singleCfg := &config.Config{BaseURL: "https://example.com/", LanguageCode: "zh-cn"}
	singleSiteCtx := &SiteContext{Config: singleCfg}
	singleCtx := NewContext(page, singleSiteCtx, singleCfg)
	if singleCtx.IsTranslated() {
		t.Error("expected IsTranslated=false for single-language page")
	}
}
