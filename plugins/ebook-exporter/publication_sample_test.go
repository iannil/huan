package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/pkg/plugin"
	"gopkg.in/yaml.v3"
)

// Opt-in site configuration smoke test; all outputs stay in a temporary folder
// unless EBOOK_PUBLICATION_SAMPLE_OUT explicitly selects another destination.
func TestSitePublicationSamples(t *testing.T) {
	site := os.Getenv("EBOOK_PUBLICATION_SITE")
	if site == "" {
		t.Skip("set EBOOK_PUBLICATION_SITE to exercise a site's publication config")
	}
	data, err := os.ReadFile(filepath.Join(site, "huan.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Plugins struct {
			Ebook map[string]any `yaml:"ebook_exporter"`
		} `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(raw.Plugins.Ebook)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := loadPublication(site, cfg.Publication)
	if err != nil {
		t.Fatal(err)
	}
	root := writeBookProject(t)
	bookDir := filepath.Join(root, "content/books/volume-1/demo-book")
	for _, name := range []string{"introduction", "part-01/chapter-01"} {
		if err := os.WriteFile(filepath.Join(bookDir, name+".en.md"), []byte("---\ntitle: Sample chapter\n---\n\nA bilingual publication smoke test.\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg.Publication = publication
	cfg.Publication.CoverBackground = ""
	cfg.OutputDir = os.Getenv("EBOOK_PUBLICATION_SAMPLE_OUT")
	if cfg.OutputDir == "" {
		cfg.OutputDir = t.TempDir()
	}
	res, err := New(cfg).Export(context.Background(), plugin.ExportRequest{SourceDir: root, Type: "books", Slug: "demo-book", Level: "individual", Format: "all", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) > 0 || len(res.Succeeded) != 6 {
		t.Fatalf("sample exports: %+v", res)
	}
	t.Logf("six bilingual artifacts: %s", cfg.OutputDir)
}
