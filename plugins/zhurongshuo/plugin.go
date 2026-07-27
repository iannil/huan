package main

import (
	"embed"
	"html/template"
	"io/fs"
	"sort"

	"github.com/iannil/huan-plugin-zhurongshuo/plugin"
)

//go:embed templates/*
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
func (t *ZhurongshuoTheme) Info() plugin.ThemeInfo {
	return plugin.ThemeInfo{
		Name:        "zhurongshuo",
		Version:     "0.1.0",
		Author:      "iannil",
		Description: "祝融说官方主题 — 中文内容排版优化",
		Tags:        []string{"blog", "chinese", "philosophy"},
		MinHuanVer:  "v0.7.0",
	}
}

// Templates returns all embedded template files.
// The "templates/" prefix is stripped from the path so that
// e.g. "templates/index.html" becomes "index.html".
func (t *ZhurongshuoTheme) Templates() []plugin.TemplateEntry {
	var entries []plugin.TemplateEntry
	fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, _ := templateFS.ReadFile(path)
		relPath := path[len("templates/"):]
		entries = append(entries, plugin.TemplateEntry{
			Path:    relPath,
			Content: string(data),
		})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

// FuncMap returns custom template functions available to templates.
func (t *ZhurongshuoTheme) FuncMap() template.FuncMap {
	return template.FuncMap{
		"readingTime":    readingTime,
		"relatedPosts":   relatedPosts,
		"toc":            toc,
		"darkModeToggle": darkModeToggle,
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
