// Package plugin provides canonical type definitions for the huan plugin system.
package plugin

import (
	"context"
	"html/template"
	"io/fs"
)

// ThemePlugin is the core capability interface for theme plugins.
// Themes provide templates, template functions, and static assets
// for site rendering.
//
// Note: Methods use interface{} and built-in types (not concrete structs)
// so that .so plugins loaded via Go's plugin.Open can satisfy this
// interface across module boundaries. Concrete struct types would be
// different named types in each module and fail type assertions.
type ThemePlugin interface {
	Plugin

	// Info returns the theme's metadata as a key-value map.
	Info() map[string]any

	// Templates returns the list of templates the theme provides.
	// Each entry has "path" and "content" keys.
	Templates() []map[string]string

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

// ShortcodeHandler is a function that renders a shortcode.
type ShortcodeHandler func(ctx ShortcodeContext) (string, error)

// ShortcodeContext carries the parameters and context for a shortcode invocation.
type ShortcodeContext struct {
	Params map[string]string
	Inner  string
}

// ShortcodeProvider is an optional interface that themes can implement
// to register custom shortcodes.
type ShortcodeProvider interface {
	Shortcodes() map[string]ShortcodeHandler
}
