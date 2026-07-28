package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/pkg/plugin"
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
	cfg  Config
	logf func(string, ...any)
}

// New creates a new SitemapEnhancer plugin.
func New(cfg *Config) *SitemapEnhancer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &SitemapEnhancer{cfg: *cfg, logf: func(format string, args ...any) {}}
}

// SetLogf sets the logger function.
func (p *SitemapEnhancer) SetLogf(fn func(string, ...any)) {
	p.logf = fn
}

// Name returns the plugin name.
func (p *SitemapEnhancer) Name() string { return "sitemap_enhancer" }

// PluginMetadata returns the plugin metadata.
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
func (p *SitemapEnhancer) OnContentLoaded(_ context.Context, pages []any) ([]any, error) {
	return nil, nil
}

// OnPageRendered is a no-op for this plugin.
func (p *SitemapEnhancer) OnPageRendered(_ context.Context, page any) error {
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

// ─────────────────────────────────────────────
// Sitemap enhancement logic (copied from internal/seo/sitemap/enhance.go)
// ─────────────────────────────────────────────

// EnhanceOptions carries parameters for a single sitemap enhancement.
type EnhanceOptions struct {
	DefaultPriority   map[string]float64
	DefaultChangefreq map[string]string
}

// urlEntry represents a single <url> element in sitemap.xml.
type urlEntry struct {
	Loc        string  `xml:"loc"`
	Lastmod    string  `xml:"lastmod,omitempty"`
	Changefreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// urlSet is the root element of sitemap.xml.
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

// EnhanceSitemap reads sitemap XML, fills in missing priority/changefreq, and returns the enhanced XML.
// If the XML cannot be parsed, returns the original src unchanged.
func EnhanceSitemap(src string, opts *EnhanceOptions) string {
	var us urlSet
	if err := xml.Unmarshal([]byte(src), &us); err != nil {
		return src
	}
	if len(us.URLs) == 0 {
		return src
	}

	changed := false
	for i, u := range us.URLs {
		kind := GuessKindFromURL(u.Loc)

		// Fill priority if missing
		if u.Priority == 0 && opts != nil && opts.DefaultPriority != nil {
			if pri, ok := opts.DefaultPriority[kind]; ok {
				us.URLs[i].Priority = pri
				changed = true
			}
		}

		// Fill changefreq if missing
		if u.Changefreq == "" && opts != nil && opts.DefaultChangefreq != nil {
			if cf, ok := opts.DefaultChangefreq[kind]; ok {
				us.URLs[i].Changefreq = cf
				changed = true
			}
		}
	}

	if !changed {
		return src
	}

	// Re-marshal with nice formatting
	output, err := xml.MarshalIndent(us, "", "  ")
	if err != nil {
		return src
	}

	return xml.Header + string(output) + "\n"
}

// GuessKindFromURL infers the page kind from a sitemap <loc> URL.
// The URL may be absolute (https://example.com/posts/) or relative (/posts/).
func GuessKindFromURL(loc string) string {
	// Extract path from absolute URL if needed
	pathPart := loc
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		// Find the path after the host
		for i := 8; i < len(loc); i++ {
			if loc[i] == '/' {
				pathPart = loc[i:]
				break
			}
		}
	}

	// Normalize: remove trailing slash, remove index.html
	clean := strings.TrimSuffix(pathPart, "/")
	clean = strings.TrimSuffix(clean, "/index.html")

	// Root path → home
	if clean == "" || clean == "/" {
		return "home"
	}

	// Split into segments
	clean = strings.TrimPrefix(clean, "/")
	segments := strings.Split(clean, "/")

	// /tags/ → taxonomy, /tags/something/ → term
	if len(segments) >= 1 && segments[0] == "tags" {
		if len(segments) == 1 {
			return "taxonomy"
		}
		return "term"
	}

	// /categories/ → taxonomy, /categories/something/ → term
	if len(segments) >= 1 && segments[0] == "categories" {
		if len(segments) == 1 {
			return "taxonomy"
		}
		return "term"
	}

	// /page/N/ → page (paginated home)
	if len(segments) >= 1 && segments[0] == "page" {
		return "page"
	}

	// Single segment → section (e.g. /posts/, /about/)
	if len(segments) == 1 {
		return "section"
	}

	// Multiple segments → page (e.g. /posts/my-post/, /2024/01/post/)
	return "page"
}
