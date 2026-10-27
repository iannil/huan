package main

import (
	"bytes"
	"context"
	"github.com/iannil/huan/pkg/plugin"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestPublicationConfigNeutralAndExplicit(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Publication.BaseURL != "" || cfg.Publication.Identity(content.LangEN).Author != "" {
		t.Fatal("implicit publisher")
	}
	cfg, err = ParseConfig(map[string]any{"publication": map[string]any{
		"base_url": "https://press.example/", "cover_background": "assets/cover.png",
		"languages": map[string]any{"en": map[string]any{"author": "Other Author", "publisher": "Other Press", "copyright": "Copyright Other Press"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Publication.Identity(content.LangEN); got.Author != "Other Author" || got.Publisher != "Other Press" || got.Copyright != "Copyright Other Press" {
		t.Fatalf("identity: %+v", got)
	}
	if got := cfg.Publication.Identity(content.LangZH); got.Author != "" {
		t.Fatalf("unexpected language fallback: %+v", got)
	}
	for _, bad := range []any{false, map[string]any{"base_url": 42}, map[string]any{"base_url": "javascript:alert(1)"}, map[string]any{"languages": map[string]any{"en": map[string]any{"author": false}}}} {
		if _, err := ParseConfig(map[string]any{"publication": bad}); err == nil {
			t.Fatalf("accepted invalid publication: %#v", bad)
		}
	}
}

func TestPublicationFingerprintTracksCopyAndBackground(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cover.png")
	writePNG := func(c color.Color) {
		t.Helper()
		im := image.NewRGBA(image.Rect(0, 0, 1, 1))
		im.Set(0, 0, c)
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig()
	cfg.Publication.CoverBackground = "cover.png"
	writePNG(color.White)
	first, err := loadPublication(root, cfg.Publication)
	if err != nil {
		t.Fatal(err)
	}
	same, err := loadPublication(root, cfg.Publication)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint() != same.Fingerprint() {
		t.Fatal("unstable fingerprint")
	}
	writePNG(color.Black)
	second, err := loadPublication(root, cfg.Publication)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint() == second.Fingerprint() {
		t.Fatal("background bytes not hashed")
	}
	cfg.Publication.BaseURL = "https://press.example"
	third, err := loadPublication(root, cfg.Publication)
	if err != nil {
		t.Fatal(err)
	}
	if third.Fingerprint() == second.Fingerprint() {
		t.Fatal("config not hashed")
	}
	cfg.Publication.CoverBackground = "missing.png"
	if _, err := loadPublication(root, cfg.Publication); err == nil {
		t.Fatal("missing configured background ignored")
	}
}

func TestExportPublicationInvalidatesCache(t *testing.T) {
	root := writeBookProject(t)
	cfg := DefaultConfig()
	req := plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "docx"}
	run := func(wantSkip bool) {
		t.Helper()
		result, err := New(cfg).Export(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Failed) > 0 {
			t.Fatal(result.Failed)
		}
		if wantSkip {
			if len(result.Skipped) != 1 {
				t.Fatalf("expected skip: %+v", result)
			}
		} else if len(result.Succeeded) != 1 {
			t.Fatalf("expected export: %+v", result)
		}
	}
	run(false)
	run(true)
	cfg.Publication.Languages = map[content.Lang]content.PublicationIdentity{content.LangZH: {Author: "Another Author"}}
	run(false)
	run(true)
	cfg.Publication.CoverBackground = "background.png"
	write := func(c color.Color) {
		im := image.NewRGBA(image.Rect(0, 0, 1, 1))
		im.Set(0, 0, c)
		var b bytes.Buffer
		png.Encode(&b, im)
		if err := os.WriteFile(filepath.Join(root, "background.png"), b.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(color.White)
	run(false)
	run(true)
	write(color.Black)
	run(false)
}
