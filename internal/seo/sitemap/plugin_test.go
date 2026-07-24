package sitemap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig_Default(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	if cfg.DefaultPriority["home"] != 1.0 {
		t.Errorf("home priority = %f, want 1.0", cfg.DefaultPriority["home"])
	}
	if cfg.DefaultChangefreq["page"] != "weekly" {
		t.Errorf("page changefreq = %q, want weekly", cfg.DefaultChangefreq["page"])
	}
}

func TestParseConfig_Overrides(t *testing.T) {
	raw := map[string]any{
		"defaultPriority": map[string]any{
			"home": 0.9,
			"page": 0.7,
		},
		"defaultChangefreq": map[string]any{
			"home": "hourly",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.DefaultPriority["home"] != 0.9 {
		t.Errorf("home priority = %f, want 0.9", cfg.DefaultPriority["home"])
	}
	if cfg.DefaultPriority["page"] != 0.7 {
		t.Errorf("page priority = %f, want 0.7", cfg.DefaultPriority["page"])
	}
	if cfg.DefaultChangefreq["home"] != "hourly" {
		t.Errorf("home changefreq = %q, want hourly", cfg.DefaultChangefreq["home"])
	}
	// Unoverridden defaults should still be present
	if cfg.DefaultChangefreq["page"] != "weekly" {
		t.Errorf("page changefreq = %q, want weekly", cfg.DefaultChangefreq["page"])
	}
}

func TestGuessKindFromURL(t *testing.T) {
	tests := []struct {
		loc  string
		kind string
	}{
		{"https://example.com/", "home"},
		{"https://example.com/index.html", "home"},
		{"/", "home"},
		{"https://example.com/posts/", "section"},
		{"https://example.com/posts/my-post/", "page"},
		{"https://example.com/about/", "section"},
		{"https://example.com/2024/01/post/", "page"},
		{"https://example.com/tags/", "taxonomy"},
		{"https://example.com/tags/golang/", "term"},
		{"https://example.com/categories/", "taxonomy"},
		{"https://example.com/categories/dev/", "term"},
		{"https://example.com/page/2/", "page"},
		{"/page/2/", "page"},
	}
	for _, tt := range tests {
		got := GuessKindFromURL(tt.loc)
		if got != tt.kind {
			t.Errorf("GuessKindFromURL(%q) = %q, want %q", tt.loc, got, tt.kind)
		}
	}
}

func TestEnhanceSitemap_AddsMissingPriority(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
    <lastmod>2026-01-01T00:00:00-07:00</lastmod>
  </url>
  <url>
    <loc>https://example.com/posts/</loc>
  </url>
  <url>
    <loc>https://example.com/posts/my-post/</loc>
    <priority>0.9</priority>
  </url>
</urlset>`

	opts := &EnhanceOptions{
		DefaultPriority: map[string]float64{
			"home":    1.0,
			"section": 0.6,
			"page":    0.8,
		},
	}
	result := EnhanceSitemap(input, opts)

	// Home gets priority 1.0
	if !strings.Contains(result, "<priority>1</priority>") {
		t.Errorf("expected home priority 1, got: %s", result)
	}
	// Section gets priority 0.6
	if !strings.Contains(result, "<priority>0.6</priority>") {
		t.Errorf("expected section priority 0.6, got: %s", result)
	}
	// Existing priority 0.9 should NOT be overwritten
	if !strings.Contains(result, "<priority>0.9</priority>") {
		t.Errorf("expected existing priority 0.9 preserved, got: %s", result)
	}
}

func TestEnhanceSitemap_AddsMissingChangefreq(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
  </url>
  <url>
    <loc>https://example.com/posts/my-post/</loc>
    <changefreq>hourly</changefreq>
  </url>
</urlset>`

	opts := &EnhanceOptions{
		DefaultChangefreq: map[string]string{
			"home": "daily",
			"page": "weekly",
		},
	}
	result := EnhanceSitemap(input, opts)

	if !strings.Contains(result, "<changefreq>daily</changefreq>") {
		t.Errorf("expected home changefreq daily, got: %s", result)
	}
	// Existing changefreq should NOT be overwritten
	if !strings.Contains(result, "<changefreq>hourly</changefreq>") {
		t.Errorf("expected existing changefreq hourly preserved, got: %s", result)
	}
}

func TestEnhanceSitemap_InvalidXML(t *testing.T) {
	input := "not valid xml"
	result := EnhanceSitemap(input, &EnhanceOptions{})
	if result != input {
		t.Errorf("expected unchanged for invalid XML")
	}
}

func TestEnhanceSitemap_NoChanges(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
    <priority>1.0</priority>
    <changefreq>daily</changefreq>
  </url>
</urlset>`
	// All fields already present, no options
	result := EnhanceSitemap(input, nil)
	if result != input {
		t.Errorf("expected unchanged when all fields present and no options")
	}
}

func TestEnhanceSitemap_NilOptions(t *testing.T) {
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
  </url>
</urlset>`
	result := EnhanceSitemap(input, nil)
	if result != input {
		t.Errorf("expected unchanged when opts is nil")
	}
}

func TestNewSitemapEnhancer(t *testing.T) {
	p := New(nil)
	if p.Name() != "sitemap_enhancer" {
		t.Errorf("Name() = %q, want sitemap_enhancer", p.Name())
	}
}

func TestSitemapEnhancer_HooksReturnNil(t *testing.T) {
	p := New(nil)
	pages, err := p.OnContentLoaded(context.Background(), nil)
	if err != nil || pages != nil {
		t.Errorf("OnContentLoaded: err=%v pages=%v", err, pages)
	}
	err = p.OnPageRendered(context.Background(), nil)
	if err != nil {
		t.Errorf("OnPageRendered: %v", err)
	}
}

func TestOnOutputWritten_EnhancesSitemap(t *testing.T) {
	tmpDir := t.TempDir()
	sitemapPath := filepath.Join(tmpDir, "sitemap.xml")
	input := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/</loc>
  </url>
  <url>
    <loc>https://example.com/posts/</loc>
  </url>
</urlset>`
	if err := os.WriteFile(sitemapPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	p := New(nil)
	p.SetLogf(t.Logf)
	err := p.OnOutputWritten(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("OnOutputWritten: %v", err)
	}

	data, err := os.ReadFile(sitemapPath)
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)
	// Home should get priority 1.0
	if !strings.Contains(result, "<priority>1</priority>") {
		t.Errorf("expected home priority 1, got: %s", result)
	}
	// Section should get priority 0.6
	if !strings.Contains(result, "<priority>0.6</priority>") {
		t.Errorf("expected section priority 0.6, got: %s", result)
	}
}

func TestOnOutputWritten_NoSitemap(t *testing.T) {
	tmpDir := t.TempDir()
	p := New(nil)
	p.SetLogf(t.Logf)
	err := p.OnOutputWritten(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("OnOutputWritten on empty dir: %v", err)
	}
}
