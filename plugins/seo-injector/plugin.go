package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/pkg/plugin"
	"golang.org/x/net/html"
)

// collectHTMLFiles walks outputDir recursively and returns every .html file at
// any depth (root level included). It replaces filepath.Glob("**/*.html"),
// which — because Go's Glob has no recursive ** — matched only files exactly
// one directory deep, silently skipping root and nested pages.
func collectHTMLFiles(outputDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

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

// SEOInjector is the build.Hook plugin that injects SEO meta tags.
type SEOInjector struct {
	cfg  Config
	logf func(string, ...any)
}

// New creates a new SEOInjector plugin.
func New(cfg *Config) *SEOInjector {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &SEOInjector{cfg: *cfg, logf: func(format string, args ...any) {}}
}

// SetLogf sets the logger function.
func (p *SEOInjector) SetLogf(fn func(string, ...any)) {
	p.logf = fn
}

// Name returns the plugin name.
func (p *SEOInjector) Name() string { return "seo_injector" }

// PluginMetadata returns the plugin metadata.
func (p *SEOInjector) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"seo", "og", "twitter"},
		IsOfficial: true,
	}
}

// OnOutputWritten scans the output directory for HTML files and injects missing SEO meta tags.
func (p *SEOInjector) OnOutputWritten(ctx context.Context, outputDir string) error {
	entries, err := collectHTMLFiles(outputDir)
	if err != nil {
		p.logf("seo-injector: scan %s: %v\n", outputDir, err)
		return nil // collection-not-interruption: log warning, don't abort
	}

	for _, filePath := range entries {
		select {
		case <-ctx.Done():
			p.logf("seo-injector: cancelled\n")
			return nil
		default:
		}

		if err := p.processFile(filePath, outputDir); err != nil {
			p.logf("seo-injector: %s: %v\n", filePath, err)
		}
	}
	return nil
}

// processFile reads, injects, and writes a single HTML file.
func (p *SEOInjector) processFile(filePath, outputDir string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Compute relative URL
	rel, err := filepath.Rel(outputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel path: %w", err)
	}
	// Convert filesystem path to URL path. On Windows, use ToSlash.
	urlPath := "/" + strings.ReplaceAll(rel, string(filepath.Separator), "/")

	opts := &InjectOptions{
		DescriptionMaxLength: p.cfg.DescriptionMaxLength,
		DefaultOGImage:       p.cfg.DefaultOGImage,
		InjectOG:             p.cfg.InjectOG,
		InjectTwitter:        p.cfg.InjectTwitter,
		PageURL:              urlPath, // relative to site root; caller should prepend baseURL if needed
		PageKind:             p.guessKind(rel),
		PageTitle:            p.extractTitle(string(data)),
	}

	result, err := InjectHTML(string(data), opts)
	if err != nil {
		return fmt.Errorf("inject: %w", err)
	}

	if result == string(data) {
		return nil // no changes
	}

	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// guessKind tries to infer the page kind from the file path.
func (p *SEOInjector) guessKind(relPath string) string {
	// Files in the root (index.html) are "home"
	if relPath == "index.html" || relPath == "/index.html" {
		return "home"
	}
	// Section index files — but exclude pagination paths like /page/2/index.html
	if strings.HasSuffix(relPath, "/index.html") &&
		!strings.Contains(relPath, "/page/") {
		return "section"
	}
	// Everything else is a page
	return "page"
}

// extractTitle extracts the <title> from HTML.
func (p *SEOInjector) extractTitle(htmlSrc string) string {
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return ""
	}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil || title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = strings.TrimSpace(n.FirstChild.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title
}
