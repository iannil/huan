package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/pkg/plugin"
)

// Config is the HTML injector configuration from huan.yaml plugins.html_injector.*
type Config struct {
	Head         []string `yaml:"head"`
	BodyEnd      []string `yaml:"bodyEnd"`
	IncludeKinds []string `yaml:"includeKinds"`
	ExcludeKinds []string `yaml:"excludeKinds"`
}

// DefaultConfig returns a Config with sensible defaults (empty lists = no injection).
func DefaultConfig() *Config {
	return &Config{}
}

// ParseConfig parses raw yaml config into Config.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["head"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("head: %w", err)
		}
		cfg.Head = items
	}
	if v, ok := raw["bodyEnd"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("bodyEnd: %w", err)
		}
		cfg.BodyEnd = items
	}
	if v, ok := raw["includeKinds"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("includeKinds: %w", err)
		}
		cfg.IncludeKinds = items
	}
	if v, ok := raw["excludeKinds"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("excludeKinds: %w", err)
		}
		cfg.ExcludeKinds = items
	}
	return cfg, nil
}

func toStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", v)
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "head", Type: "string_slice", Required: false, Description: "HTML fragments to inject before </head>"},
		{Key: "bodyEnd", Type: "string_slice", Required: false, Description: "HTML fragments to inject before </body>"},
		{Key: "includeKinds", Type: "string_slice", Required: false, Description: "only inject for these page kinds"},
		{Key: "excludeKinds", Type: "string_slice", Required: false, Description: "skip injection for these page kinds"},
	}}
}

// HTMLInjector is the build.Hook plugin that injects custom HTML fragments.
type HTMLInjector struct {
	cfg  Config
	logf func(string, ...any)
}

// New creates a new HTMLInjector plugin.
func New(cfg *Config) *HTMLInjector {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &HTMLInjector{cfg: *cfg, logf: func(format string, args ...any) {}}
}

// SetLogf sets the logger function.
func (p *HTMLInjector) SetLogf(fn func(string, ...any)) {
	p.logf = fn
}

// Name returns the plugin name.
func (p *HTMLInjector) Name() string { return "html_injector" }

// PluginMetadata returns the plugin metadata.
func (p *HTMLInjector) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"html", "script", "css"},
		IsOfficial: true,
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (p *HTMLInjector) ConfigSchema() plugin.Schema {
	return p.cfg.ConfigSchema()
}

// OnContentLoaded is a no-op for this plugin.
func (p *HTMLInjector) OnContentLoaded(_ context.Context, pages []any) ([]any, error) {
	return nil, nil
}

// OnPageRendered is a no-op for this plugin.
func (p *HTMLInjector) OnPageRendered(_ context.Context, page any) error {
	return nil
}

// OnOutputWritten scans the output directory for HTML files and injects
// custom head/body HTML snippets.
func (p *HTMLInjector) OnOutputWritten(ctx context.Context, outputDir string) error {
	p.logf("html-injector: starting injection in %s\n", outputDir)
	p.logf("html-injector: head=%v bodyEnd=%v include=%v exclude=%v\n",
		p.cfg.Head, p.cfg.BodyEnd, p.cfg.IncludeKinds, p.cfg.ExcludeKinds)

	if len(p.cfg.Head) == 0 && len(p.cfg.BodyEnd) == 0 {
		p.logf("html-injector: no snippets configured, skipping\n")
		return nil
	}

	entries, err := filepath.Glob(filepath.Join(outputDir, "**/*.html"))
	if err != nil {
		p.logf("html-injector: glob %s: %v\n", outputDir, err)
		return nil
	}

	for _, filePath := range entries {
		select {
		case <-ctx.Done():
			p.logf("html-injector: cancelled\n")
			return nil
		default:
		}

		if err := p.processFile(filePath, outputDir); err != nil {
			p.logf("html-injector: %s: %v\n", filePath, err)
		}
	}
	return nil
}

// processFile reads, injects, and writes a single HTML file.
func (p *HTMLInjector) processFile(filePath, outputDir string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	rel, err := filepath.Rel(outputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel path: %w", err)
	}
	urlPath := "/" + strings.ReplaceAll(rel, string(filepath.Separator), "/")

	pageKind := p.guessKind(rel)

	result := injectHTML(string(data), &p.cfg, pageKind)
	if result == string(data) {
		return nil // no changes
	}

	p.logf("html-injector: injected into %s (kind=%s)\n", urlPath, pageKind)
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// guessKind tries to infer the page kind from the file path.
func (p *HTMLInjector) guessKind(relPath string) string {
	if relPath == "index.html" || relPath == "/index.html" {
		return "home"
	}
	if strings.HasSuffix(relPath, "/index.html") &&
		!strings.Contains(relPath, "/page/") {
		return "section"
	}
	return "page"
}

// injectHTML injects configured HTML fragments into the page content.
// Returns the modified HTML. If no injection is needed, returns the original.
func injectHTML(htmlSrc string, cfg *Config, pageKind string) string {
	if cfg == nil {
		return htmlSrc
	}

	// Check kind filters
	if len(cfg.IncludeKinds) > 0 {
		if !contains(cfg.IncludeKinds, pageKind) {
			return htmlSrc
		}
	}
	if len(cfg.ExcludeKinds) > 0 {
		if contains(cfg.ExcludeKinds, pageKind) {
			return htmlSrc
		}
	}

	// Inject Head fragments (before </head>)
	if len(cfg.Head) > 0 {
		headClose := strings.Index(htmlSrc, "</head>")
		if headClose >= 0 {
			injection := "\n" + strings.Join(cfg.Head, "\n") + "\n"
			htmlSrc = htmlSrc[:headClose] + injection + htmlSrc[headClose:]
		}
	}

	// Inject BodyEnd fragments (before </body>)
	if len(cfg.BodyEnd) > 0 {
		bodyClose := strings.Index(htmlSrc, "</body>")
		if bodyClose >= 0 {
			injection := "\n" + strings.Join(cfg.BodyEnd, "\n") + "\n"
			htmlSrc = htmlSrc[:bodyClose] + injection + htmlSrc[bodyClose:]
		}
	}

	return htmlSrc
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Ensure compile-time interface satisfaction.
var _ plugin.Plugin = (*HTMLInjector)(nil)
var _ plugin.SchemaProvider = (*HTMLInjector)(nil)
