package htmlinjector

import (
	"context"
	"fmt"
	"html/template"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/plugin"
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
	cfg *Config
}

// New creates a new HTMLInjector plugin.
func New(cfg *Config) *HTMLInjector {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &HTMLInjector{cfg: cfg}
}

// Name returns the plugin name.
func (p *HTMLInjector) Name() string { return "html_injector" }

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
func (p *HTMLInjector) OnContentLoaded(_ context.Context, pages []*content.Page) ([]*content.Page, error) {
	return nil, nil
}

// OnPageRendered injects configured HTML fragments into the page.
func (p *HTMLInjector) OnPageRendered(_ context.Context, page *content.Page) error {
	modified := InjectHTML(string(page.Content), p.cfg, page.Kind)
	if modified != string(page.Content) {
		page.Content = template.HTML(modified)
	}
	return nil
}

// OnOutputWritten is a no-op for this plugin.
func (p *HTMLInjector) OnOutputWritten(_ context.Context, outputDir string) error {
	return nil
}

// Ensure compile-time interface satisfaction.
var _ plugin.Plugin = (*HTMLInjector)(nil)
var _ plugin.SchemaProvider = (*HTMLInjector)(nil)
