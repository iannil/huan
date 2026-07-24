package injector

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// Config is the SEO injector configuration from huan.yaml plugins.seo_injector.*
type Config struct {
	DescriptionMaxLength int    `yaml:"descriptionMaxLength"`
	DefaultOGImage       string `yaml:"defaultOGImage"`
	InjectOG             bool   `yaml:"injectOG"`
	InjectTwitter        bool   `yaml:"injectTwitter"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DescriptionMaxLength: 160,
		InjectOG:             true,
		InjectTwitter:        true,
	}
}

// ParseConfig parses raw yaml config into Config.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["descriptionMaxLength"]; ok {
		if f, ok := toFloat64(v); ok {
			cfg.DescriptionMaxLength = int(f)
		} else {
			return nil, fmt.Errorf("descriptionMaxLength: expected number, got %T", v)
		}
	}
	if v, ok := raw["defaultOGImage"]; ok {
		if s, ok := v.(string); ok {
			cfg.DefaultOGImage = s
		} else {
			return nil, fmt.Errorf("defaultOGImage: expected string, got %T", v)
		}
	}
	if v, ok := raw["injectOG"]; ok {
		if b, ok := v.(bool); ok {
			cfg.InjectOG = b
		} else {
			return nil, fmt.Errorf("injectOG: expected bool, got %T", v)
		}
	}
	if v, ok := raw["injectTwitter"]; ok {
		if b, ok := v.(bool); ok {
			cfg.InjectTwitter = b
		} else {
			return nil, fmt.Errorf("injectTwitter: expected bool, got %T", v)
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

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "descriptionMaxLength", Type: "int", Required: false, Default: 160, Description: "meta description 最大长度"},
		{Key: "defaultOGImage", Type: "string", Required: false, Description: "默认 OG image 路径"},
		{Key: "injectOG", Type: "bool", Required: false, Default: true, Description: "是否注入 OG 标签"},
		{Key: "injectTwitter", Type: "bool", Required: false, Default: true, Description: "是否注入 Twitter Card 标签"},
	}}
}
