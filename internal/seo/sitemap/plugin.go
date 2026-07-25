package sitemap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iannil/huan/internal/content"
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

// SitemapEnhancer is the build.Hook plugin that enhances sitemap.xml with
// priority and changefreq values based on page kind.
type SitemapEnhancer struct {
	cfg  *Config
	logf func(string, ...any)
}

// New creates a new SitemapEnhancer plugin.
func New(cfg *Config) *SitemapEnhancer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &SitemapEnhancer{cfg: cfg, logf: func(format string, args ...any) {}}
}

// SetLogf sets the logger function.
func (p *SitemapEnhancer) SetLogf(fn func(string, ...any)) {
	p.logf = fn
}

// Name returns the plugin name.
func (p *SitemapEnhancer) Name() string { return "sitemap_enhancer" }

func (p *SitemapEnhancer) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"seo", "sitemap"},
		IsOfficial: true,
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (p *SitemapEnhancer) ConfigSchema() plugin.Schema {
	return p.cfg.ConfigSchema()
}

// OnContentLoaded is a no-op for this plugin.
func (p *SitemapEnhancer) OnContentLoaded(_ context.Context, pages []*content.Page) ([]*content.Page, error) {
	return nil, nil
}

// OnPageRendered is a no-op for this plugin.
func (p *SitemapEnhancer) OnPageRendered(_ context.Context, page *content.Page) error {
	return nil
}

// OnOutputWritten enhances the sitemap.xml in the output directory.
func (p *SitemapEnhancer) OnOutputWritten(ctx context.Context, outputDir string) error {
	sitemapPath := filepath.Join(outputDir, "sitemap.xml")

	data, err := os.ReadFile(sitemapPath)
	if err != nil {
		p.logf("sitemap-enhancer: read %s: %v\n", sitemapPath, err)
		return nil // collection-not-interruption
	}

	opts := &EnhanceOptions{
		DefaultPriority:   p.cfg.DefaultPriority,
		DefaultChangefreq: p.cfg.DefaultChangefreq,
	}

	result := EnhanceSitemap(string(data), opts)
	if result == string(data) {
		return nil // no changes
	}

	if err := os.WriteFile(sitemapPath, []byte(result), 0644); err != nil {
		p.logf("sitemap-enhancer: write %s: %v\n", sitemapPath, err)
		return nil
	}

	return nil
}