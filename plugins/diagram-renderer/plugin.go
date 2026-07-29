package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/pkg/plugin"
)

// DiagramRenderer renders diagram fenced code blocks to inline SVG via Kroki.
type DiagramRenderer struct {
	cfg   Config
	cache *Cache
	kroki *KrokiClient
	logf  func(string, ...any)
}

// New creates a DiagramRenderer from cfg.
func New(cfg *Config) *DiagramRenderer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &DiagramRenderer{
		cfg:   *cfg,
		cache: NewCache(cfg.CacheDir),
		kroki: NewKrokiClient(cfg.KrokiURL, time.Duration(cfg.TimeoutMs)*time.Millisecond),
		logf:  func(string, ...any) {},
	}
}

// SetLogf sets the logger function.
func (p *DiagramRenderer) SetLogf(fn func(string, ...any)) { p.logf = fn }

// Name returns the plugin's unique identifier.
func (p *DiagramRenderer) Name() string { return "diagram_renderer" }

// PluginMetadata returns human-readable metadata.
func (p *DiagramRenderer) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"diagram", "mermaid", "kroki", "svg"},
		IsOfficial: true,
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (p *DiagramRenderer) ConfigSchema() plugin.Schema { return p.cfg.ConfigSchema() }

// collectHTMLFiles walks outputDir recursively for every .html file.
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

// OnOutputWritten scans output HTML and renders diagram blocks to SVG.
func (p *DiagramRenderer) OnOutputWritten(ctx context.Context, outputDir string) error {
	if !p.cfg.Enabled || len(p.cfg.Languages) == 0 {
		p.logf("diagram-renderer: disabled or no languages, skipping\n")
		return nil
	}
	files, err := collectHTMLFiles(outputDir)
	if err != nil {
		p.logf("diagram-renderer: scan %s: %v\n", outputDir, err)
		return nil
	}
	for _, fp := range files {
		select {
		case <-ctx.Done():
			p.logf("diagram-renderer: cancelled\n")
			return nil
		default:
		}
		if err := p.processFile(ctx, fp, outputDir); err != nil {
			p.logf("diagram-renderer: %s: %v\n", fp, err)
		}
	}
	return nil
}

// processFile renders every diagram block in one HTML file.
func (p *DiagramRenderer) processFile(ctx context.Context, filePath, outputDir string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	rel, err := filepath.Rel(outputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel: %w", err)
	}
	if !p.kindAllowed(p.guessKind(rel)) {
		return nil
	}

	src := string(data)
	blocks := findDiagramBlocks(src, p.cfg.Languages)
	if len(blocks) == 0 {
		return nil
	}

	needMermaidJS := false
	for _, b := range blocks {
		replacement, injectJS := p.renderBlock(ctx, b)
		src = strings.Replace(src, b.Full, replacement, 1)
		if injectJS {
			needMermaidJS = true
		}
	}
	if needMermaidJS {
		src = injectMermaidJS(src, p.cfg.MermaidJS)
	}

	if src == string(data) {
		return nil
	}
	if err := os.WriteFile(filePath, []byte(src), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// renderBlock returns the replacement HTML for one diagram block and whether the
// page needs mermaid.js injected. It tries cache, then Kroki, then fallback.
func (p *DiagramRenderer) renderBlock(ctx context.Context, b diagramBlock) (string, bool) {
	key := p.cache.Key(b.Lang, b.Source)
	if svg, ok := p.cache.Get(key); ok {
		p.logf("diagram-renderer: cache hit lang=%s hash=%s\n", b.Lang, key)
		return wrapSVG(svg, b.Lang, p.cfg.FigureClass), false
	}
	svg, err := p.kroki.Render(ctx, b.Lang, b.Source)
	if err != nil {
		p.logf("diagram-renderer: WARN kroki failed lang=%s: %v; falling back\n", b.Lang, err)
		return fallbackReplacement(b, &p.cfg)
	}
	if putErr := p.cache.Put(key, svg); putErr != nil {
		p.logf("diagram-renderer: cache put lang=%s: %v\n", b.Lang, putErr)
	}
	p.logf("diagram-renderer: rendered lang=%s hash=%s\n", b.Lang, key)
	return wrapSVG(svg, b.Lang, p.cfg.FigureClass), false
}

// kindAllowed applies include/exclude kind filters.
func (p *DiagramRenderer) kindAllowed(kind string) bool {
	if len(p.cfg.IncludeKinds) > 0 && !contains(p.cfg.IncludeKinds, kind) {
		return false
	}
	if len(p.cfg.ExcludeKinds) > 0 && contains(p.cfg.ExcludeKinds, kind) {
		return false
	}
	return true
}

// guessKind infers a page kind from its relative path (matches html-injector).
func (p *DiagramRenderer) guessKind(relPath string) string {
	if relPath == "index.html" || relPath == "/index.html" {
		return "home"
	}
	if strings.HasSuffix(relPath, "/index.html") && !strings.Contains(relPath, "/page/") {
		return "section"
	}
	return "page"
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Compile-time interface assertions.
var _ plugin.PostBuildHook = (*DiagramRenderer)(nil)
var _ plugin.SchemaProvider = (*DiagramRenderer)(nil)
