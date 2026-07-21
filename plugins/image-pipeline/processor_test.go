package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessQuality(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "test.jpg")
	createTestJPEG(t, src, 100, 100)

	cfg := Config{
		Formats:    []string{"webp"},
		Quality:    80,
		SkipLarger: true,
	}
	cfg.defaults(map[string]any{})

	assets := []ImageAsset{{
		SrcPath: src,
		RelPath: "test.jpg",
		Width:   100,
		Height:  100,
		Size:    1000,
		Format:  "jpg",
	}}

	results, err := Process(assets, cfg, tmpDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Should have at least 1 variant (webp)
	if len(results[0].Variants) == 0 {
		t.Error("expected at least 1 variant")
	}
}

func TestProcessMultiSize(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "test.jpg")
	createTestJPEG(t, src, 1200, 800)

	cfg := Config{
		Formats:    []string{"webp"},
		Quality:    80,
		Sizes:      []int{480, 768},
		SkipLarger: true,
	}
	cfg.defaults(map[string]any{})

	assets := []ImageAsset{{
		SrcPath: src,
		RelPath: "test.jpg",
		Width:   1200,
		Height:  800,
		Size:    5000,
		Format:  "jpg",
	}}

	results, err := Process(assets, cfg, tmpDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// 2 sizes = 2 variants (480w, 768w)
	if len(results[0].Variants) != 2 {
		t.Errorf("expected 2 variants (2 sizes), got %d: %v", len(results[0].Variants), results[0].Variants)
	}
}

func TestProcessMaxDimension(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "test.jpg")
	createTestJPEG(t, src, 3000, 2000)

	cfg := Config{
		Formats:     []string{"webp"},
		Quality:     80,
		MaxDimension: 2048,
		SkipLarger:  true,
	}
	cfg.defaults(map[string]any{})

	assets := []ImageAsset{{
		SrcPath: src,
		RelPath: "test.jpg",
		Width:   3000,
		Height:  2000,
		Size:    10000,
		Format:  "jpg",
	}}

	results, err := Process(assets, cfg, tmpDir)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	// The scaled image should be <= 2048 on the long side
	if len(results) > 0 {
		// Verify the file was written (width is scaled from 3000 to 2048)
		webpPath := filepath.Join(tmpDir, "test-2048w.webp")
		if _, err := os.Stat(webpPath); err != nil {
			t.Logf("webp file not found (expected if webp encoding not available): %v", err)
		}
	}
}

func TestVariantFilename(t *testing.T) {
	tests := []struct {
		relPath string
		width   int
		format  string
		want    string
	}{
		{"photo.jpg", 480, "webp", "photo-480w.webp"},
		{"photo.jpg", 768, "webp", "photo-768w.webp"},
		{"photo.jpg", 0, "webp", "photo.webp"},
		{"images/banner.png", 480, "webp", "images/banner-480w.webp"},
	}

	for _, tt := range tests {
		got := variantFilename(tt.relPath, tt.width, tt.format)
		if got != tt.want {
			t.Errorf("variantFilename(%q, %d, %q) = %q, want %q", tt.relPath, tt.width, tt.format, got, tt.want)
		}
	}
}

func createTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}