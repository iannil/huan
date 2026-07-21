package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectSrcset(t *testing.T) {
	cfg := Config{
		InjectSrcset:      true,
		InjectPicture:     false,
		InjectLazyLoading: true,
	}
	cfg.defaults(map[string]any{})

	input := `<img src="/images/photo.jpg" alt="test">`

	processed := []ProcessedImage{
		{
			Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
			Variants: []ImageVariant{
				{RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
				{RelPath: "images/photo-768w.jpg", Width: 768, Format: "jpg"},
				{RelPath: "images/photo.webp", Width: 1200, Format: "webp"},
			},
		},
	}

	output := injectHTML(input, processed, cfg)
	if !strings.Contains(output, "srcset") {
		t.Error("output missing srcset attribute")
	}
	if !strings.Contains(output, "480w") {
		t.Error("output missing 480w descriptor")
	}
	if !strings.Contains(output, "loading=") {
		t.Error("output missing loading attribute")
	}
}

func TestInjectPicture(t *testing.T) {
	cfg := Config{
		InjectSrcset:      true,
		InjectPicture:     true,
		InjectLazyLoading: true,
	}
	cfg.defaults(map[string]any{})

	input := `<img src="/images/photo.jpg" alt="test">`
	output := injectHTML(input, []ProcessedImage{
		{
			Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
			Variants: []ImageVariant{
				{RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
				{RelPath: "images/photo-768w.jpg", Width: 768, Format: "jpg"},
				{RelPath: "images/photo.webp", Width: 1200, Format: "webp"},
				{RelPath: "images/photo-480w.webp", Width: 480, Format: "webp"},
				{RelPath: "images/photo-768w.webp", Width: 768, Format: "webp"},
			},
		},
	}, cfg)

	if !strings.Contains(output, "<picture>") {
		t.Error("output missing <picture> tag")
	}
	if !strings.Contains(output, "type=\"image/webp\"") {
		t.Error("output missing webp source type")
	}
	if !strings.Contains(output, "</picture>") {
		t.Error("output missing </picture>")
	}
}

func TestInjectSkipExistingSrcset(t *testing.T) {
	cfg := Config{InjectSrcset: true, InjectPicture: false}
	cfg.defaults(map[string]any{})

	input := `<img src="/images/photo.jpg" srcset="/images/photo-2x.jpg 2x" alt="test">`
	output := injectHTML(input, []ProcessedImage{
		{
			Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
			Variants: []ImageVariant{
				{RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
			},
		},
	}, cfg)

	if strings.Contains(output, "480w") {
		t.Error("should not modify img with existing srcset")
	}
}

func TestInjectSkipNoMatch(t *testing.T) {
	cfg := Config{InjectSrcset: true, InjectPicture: false}
	cfg.defaults(map[string]any{})

	input := `<img src="/images/other.jpg" alt="test">`
	output := injectHTML(input, []ProcessedImage{}, cfg)
	if output != input {
		t.Error("should not modify img with no matching processed image")
	}
}

func TestInjectHTMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "index.html")
	htmlContent := `<html><body><img src="/images/photo.jpg" alt="test"></body></html>`
	os.WriteFile(htmlPath, []byte(htmlContent), 0644)

	cfg := Config{InjectSrcset: true, InjectPicture: false}
	cfg.defaults(map[string]any{})

	processed := []ProcessedImage{
		{
			Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
			Variants: []ImageVariant{
				{RelPath: "images/photo-768w.jpg", Width: 768, Format: "jpg"},
			},
		},
	}

	err := InjectHTMLFiles(tmpDir, processed, cfg)
	if err != nil {
		t.Fatalf("InjectHTMLFiles: %v", err)
	}

	data, _ := os.ReadFile(htmlPath)
	if !strings.Contains(string(data), "srcset") {
		t.Error("HTML file missing srcset after injection")
	}
}

func TestInjectSkipInsidePicture(t *testing.T) {
	cfg := Config{InjectSrcset: true, InjectPicture: true}
	cfg.defaults(map[string]any{})

	// HTML with an img already inside a <picture> block should not be modified
	input := `<picture><source srcset="/images/photo.webp" type="image/webp"><img src="/images/photo.jpg" alt="test"></picture>`
	output := injectHTML(input, []ProcessedImage{
		{
			Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
			Variants: []ImageVariant{
				{RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
			},
		},
	}, cfg)

	if strings.Contains(output, "480w") {
		t.Error("should not modify img already inside <picture>")
	}
	// The original <picture> block should be preserved exactly
	if output != input {
		t.Errorf("expected unchanged output, got: %s", output)
	}
}
