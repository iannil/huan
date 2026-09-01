package main

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"sort"

	"github.com/iannil/huan/pkg/plugin"
)

//go:embed templates/* templates/partials/charts/_shared.html
var templateFS embed.FS

//go:embed assets/*
var assetFS embed.FS

// ZhurongshuoTheme is the official zhurongshuo (祝融说) theme plugin.
// It provides templates, custom template functions, and static assets
// for the zhurongshuo content site.
type ZhurongshuoTheme struct{}

// Name returns the unique identifier for this theme plugin.
// It matches the theme name used in huan.yaml theme: field.
func (t *ZhurongshuoTheme) Name() string { return "zhurongshuo" }

// Info returns the theme's metadata.
func (t *ZhurongshuoTheme) Info() map[string]any {
	return map[string]any{
		"Name":        "zhurongshuo",
		"Version":     "0.1.0",
		"Author":      "iannil",
		"Description": "祝融说官方主题 — 中文内容排版优化",
		"Tags":        []string{"blog", "chinese", "philosophy"},
		"MinHuanVer":  "v0.7.0",
	}
}

// Templates returns all embedded template files.
// The "templates/" prefix is stripped from the path so that
// e.g. "templates/index.html" becomes "index.html".
func (t *ZhurongshuoTheme) Templates() []map[string]string {
	var entries []map[string]string
	fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, _ := templateFS.ReadFile(path)
		relPath := path[len("templates/"):]
		entries = append(entries, map[string]string{
			"path":    relPath,
			"content": string(data),
		})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i]["path"] < entries[j]["path"] })
	return entries
}

// FuncMap returns custom template functions available to templates.
func (t *ZhurongshuoTheme) FuncMap() template.FuncMap {
	return template.FuncMap{
		"readingTime":    readingTime,
		"relatedPosts":   relatedPosts,
		"toc":            toc,
		"darkModeToggle": darkModeToggle,
		"parseGuideYAML": parseGuideYAML,
		"svgTextWidth":   svgTextWidth,
		"svgTruncate":    svgTruncate,
		"svgWrap":        svgWrap,
	}
}

// BeforeRender implements plugin.ThemeHooks.
func (t *ZhurongshuoTheme) BeforeRender(ctx context.Context) error {
	return nil
}

// AfterRender implements plugin.ThemeHooks.
func (t *ZhurongshuoTheme) AfterRender(ctx context.Context) error {
	return nil
}

// Shortcodes implements plugin.ShortcodeProvider.
// Returns the shortcode handlers provided by this theme.
func (t *ZhurongshuoTheme) Shortcodes() map[string]plugin.ShortcodeHandler {
	return map[string]plugin.ShortcodeHandler{
		// zhurongshuo 主题暂不提供自定义 shortcode
		// 内置的 audio/img shortcode 由 huan 引擎提供
	}
}

// Assets returns the theme's static file system (CSS, JS, etc.).
func (t *ZhurongshuoTheme) Assets() fs.FS {
	sub, err := fs.Sub(assetFS, "assets")
	if err != nil {
		return assetFS
	}
	return sub
}
