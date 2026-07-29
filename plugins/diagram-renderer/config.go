package main

import (
	"fmt"

	"github.com/iannil/huan/pkg/plugin"
)

// Config is the diagram-renderer configuration from huan.yaml
// plugins.diagram_renderer.*
type Config struct {
	Enabled      bool
	KrokiURL     string
	Languages    []string
	CacheDir     string
	TimeoutMs    int
	FallbackMode string // "client" | "codeblock" | "fail"
	MermaidJS    string
	FigureClass  string
	IncludeKinds []string
	ExcludeKinds []string
}

// DefaultConfig returns a Config with the spec's default values.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		KrokiURL:     "http://localhost:8000",
		Languages:    []string{"mermaid", "plantuml", "graphviz", "d2"},
		CacheDir:     ".huan/cache/diagrams",
		TimeoutMs:    5000,
		FallbackMode: "client",
		MermaidJS:    "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js",
		FigureClass:  "diagram",
	}
}

// ParseConfig parses raw yaml config into Config, layering over defaults.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["enabled"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("enabled: expected bool, got %T", v)
		}
		cfg.Enabled = b
	}
	if v, ok := raw["krokiUrl"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("krokiUrl: expected string, got %T", v)
		}
		cfg.KrokiURL = s
	}
	if v, ok := raw["languages"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("languages: %w", err)
		}
		cfg.Languages = items
	}
	if v, ok := raw["cacheDir"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("cacheDir: expected string, got %T", v)
		}
		cfg.CacheDir = s
	}
	if v, ok := raw["timeoutMs"]; ok {
		f, ok := toFloat64(v)
		if !ok {
			return nil, fmt.Errorf("timeoutMs: expected number, got %T", v)
		}
		cfg.TimeoutMs = int(f)
	}
	if v, ok := raw["figureClass"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("figureClass: expected string, got %T", v)
		}
		cfg.FigureClass = s
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
	if v, ok := raw["fallback"]; ok {
		fb, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("fallback: expected map, got %T", v)
		}
		if mv, ok := fb["mode"]; ok {
			s, ok := mv.(string)
			if !ok {
				return nil, fmt.Errorf("fallback.mode: expected string, got %T", mv)
			}
			cfg.FallbackMode = s
		}
		if jv, ok := fb["mermaidJs"]; ok {
			s, ok := jv.(string)
			if !ok {
				return nil, fmt.Errorf("fallback.mermaidJs: expected string, got %T", jv)
			}
			cfg.MermaidJS = s
		}
	}
	return cfg, nil
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
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
		{Key: "enabled", Type: "bool", Required: false, Default: false, Description: "开关插件"},
		{Key: "krokiUrl", Type: "string", Required: false, Default: "http://localhost:8000", Description: "自托管 Kroki 端点"},
		{Key: "languages", Type: "string_slice", Required: false, Description: "允许渲染的图表语言"},
		{Key: "cacheDir", Type: "string", Required: false, Default: ".huan/cache/diagrams", Description: "SVG 缓存目录"},
		{Key: "timeoutMs", Type: "int", Required: false, Default: 5000, Description: "Kroki 请求超时(ms)"},
		{Key: "figureClass", Type: "string", Required: false, Default: "diagram", Description: "输出 figure 基础 class"},
		{Key: "includeKinds", Type: "string_slice", Required: false, Description: "仅对这些 page kind 渲染"},
		{Key: "excludeKinds", Type: "string_slice", Required: false, Description: "跳过这些 page kind"},
		{Key: "fallback", Type: "map", Required: false, Description: "失败降级: {mode, mermaidJs}"},
	}}
}
