// Package plugin defines the base plugin interface for huan extensions.
// This is a self-contained copy of the types needed by theme plugins,
// isolated so that .so plugins do not need to import huan's internal packages.
package plugin

import (
	"context"
	"html/template"
	"io/fs"
)

// Plugin is the base interface every plugin satisfies.
type Plugin interface {
	Name() string
}

// ThemePlugin is the core capability interface for theme plugins.
// Themes provide templates, template functions, and static assets
// for site rendering.
type ThemePlugin interface {
	Plugin

	// Info returns the theme's metadata.
	Info() ThemeInfo

	// Templates returns the list of templates the theme provides.
	Templates() []TemplateEntry

	// FuncMap returns the theme's custom template functions.
	FuncMap() template.FuncMap

	// Assets returns the theme's static asset filesystem.
	Assets() fs.FS
}

// ThemeInfo carries theme metadata.
type ThemeInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	Screenshot  string   `json:"screenshot,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	MinHuanVer  string   `json:"minHuanVer,omitempty"`
}

// TemplateEntry describes a single template file.
type TemplateEntry struct {
	Path    string // Logical path, e.g. "index.html"
	Content string // Template content
}

// ThemeHooks is an optional interface that themes can implement to inject
// lifecycle hooks into the render pipeline.
type ThemeHooks interface {
	BeforeRender(ctx context.Context) error
	AfterRender(ctx context.Context) error
}
