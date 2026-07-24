package sitemap

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// Config is the sitemap enhancer configuration from huan.yaml plugins.sitemap_enhancer.*
type Config struct {
	DefaultPriority   map[string]float64 `yaml:"defaultPriority"`
	DefaultChangefreq map[string]string  `yaml:"defaultChangefreq"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultPriority: map[string]float64{
			"home":     1.0,
			"page":     0.8,
			"section":  0.6,
			"taxonomy": 0.5,
			"term":     0.4,
		},
		DefaultChangefreq: map[string]string{
			"home":     "daily",
			"page":     "weekly",
			"section":  "weekly",
			"taxonomy": "weekly",
			"term":     "monthly",
		},
	}
}

// ParseConfig parses raw yaml config into Config.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["defaultPriority"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("defaultPriority: expected map, got %T", v)
		}
		parsed := make(map[string]float64, len(m))
		for k, val := range m {
			f, err := toFloat64(val)
			if err != nil {
				return nil, fmt.Errorf("defaultPriority.%s: %w", k, err)
			}
			parsed[k] = f
		}
		// Merge into defaults (overrides existing keys, keeps unoverridden defaults)
		for k, v := range parsed {
			cfg.DefaultPriority[k] = v
		}
	}
	if v, ok := raw["defaultChangefreq"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("defaultChangefreq: expected map, got %T", v)
		}
		parsed := make(map[string]string, len(m))
		for k, val := range m {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("defaultChangefreq.%s: expected string, got %T", k, val)
			}
			parsed[k] = s
		}
		// Merge into defaults (overrides existing keys, keeps unoverridden defaults)
		for k, v := range parsed {
			cfg.DefaultChangefreq[k] = v
		}
	}
	return cfg, nil
}

func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "defaultPriority", Type: "map", Required: false, Description: "per-kind priority values"},
		{Key: "defaultChangefreq", Type: "map", Required: false, Description: "per-kind changefreq values"},
	}}
}