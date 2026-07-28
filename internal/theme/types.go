// Package theme defines the ThemePlugin capability interface for huan's theme
// plugin system. Themes are plugins that provide templates, template functions,
// and static assets for site rendering.
//
// All types are aliased from pkg/plugin so .so plugins share the same type identity.
package theme

import (
	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// All types are aliased from pkg/plugin so .so plugins share the same type identity.
type ThemePlugin = pkgplugin.ThemePlugin
type ThemeHooks = pkgplugin.ThemeHooks
type ShortcodeProvider = pkgplugin.ShortcodeProvider

// ThemeInfo is a helper type for accessing theme metadata from a map.
// The ThemePlugin.Info() method returns map[string]any for cross-module
// compatibility. Use DecodeInfo() to parse it.
type ThemeInfo struct {
	Name        string
	Version     string
	Author      string
	Description string
	Screenshot  string
	Tags        []string
	MinHuanVer  string
}

// DecodeInfo parses a map returned by ThemePlugin.Info() into ThemeInfo.
func DecodeInfo(m map[string]any) ThemeInfo {
	return ThemeInfo{
		Name:        toString(m["Name"]),
		Version:     toString(m["Version"]),
		Author:      toString(m["Author"]),
		Description: toString(m["Description"]),
		Screenshot:  toString(m["Screenshot"]),
		Tags:        toStringSlice(m["Tags"]),
		MinHuanVer:  toString(m["MinHuanVer"]),
	}
}

// TemplateEntry is a helper type for accessing template data from a map.
type TemplateEntry struct {
	Path    string
	Content string
}

// DecodeTemplates parses template entries returned by ThemePlugin.Templates().
func DecodeTemplates(entries []map[string]string) []TemplateEntry {
	out := make([]TemplateEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, TemplateEntry{
			Path:    e["path"],
			Content: e["content"],
		})
	}
	return out
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}