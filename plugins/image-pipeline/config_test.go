package main

import (
	"testing"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("ParseConfig empty: %v", err)
	}
	if len(cfg.Formats) != 1 || cfg.Formats[0] != "webp" {
		t.Errorf("default Formats = %v, want [webp]", cfg.Formats)
	}
	if cfg.Quality != 80 {
		t.Errorf("default Quality = %d, want 80", cfg.Quality)
	}
	if !cfg.InjectSrcset {
		t.Error("default InjectSrcset should be true")
	}
	if !cfg.InjectPicture {
		t.Error("default InjectPicture should be true")
	}
	if !cfg.InjectLazyLoading {
		t.Error("default InjectLazyLoading should be true")
	}
	if !cfg.SkipLarger {
		t.Error("default SkipLarger should be true")
	}
}

func TestParseConfig_Override(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{
		"quality":           90,
		"formats":           []any{"webp", "avif"},
		"sizes":             []any{480, 768, 1200},
		"inject_srcset":     false,
		"max_dimension":     2048,
	})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Quality != 90 {
		t.Errorf("Quality = %d, want 90", cfg.Quality)
	}
	if len(cfg.Formats) != 2 || cfg.Formats[1] != "avif" {
		t.Errorf("Formats = %v, want [webp avif]", cfg.Formats)
	}
	if len(cfg.Sizes) != 3 || cfg.Sizes[1] != 768 {
		t.Errorf("Sizes = %v, want [480 768 1200]", cfg.Sizes)
	}
	if cfg.InjectSrcset {
		t.Error("InjectSrcset should be false")
	}
	if cfg.MaxDimension != 2048 {
		t.Errorf("MaxDimension = %d, want 2048", cfg.MaxDimension)
	}
}

func TestParseConfig_InvalidFormats(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"formats": []any{"gif"},
	})
	if err == nil {
		t.Error("expected error for invalid format 'gif'")
	}
}

func TestParseConfig_InvalidQuality(t *testing.T) {
	_, err := ParseConfig(map[string]any{
		"quality": 150,
	})
	if err == nil {
		t.Error("expected error for quality > 100")
	}
}

func TestParseConfig_NilInput(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil) error: %v", err)
	}
	if len(cfg.Formats) != 1 || cfg.Formats[0] != "webp" {
		t.Errorf("default Formats = %v, want [webp]", cfg.Formats)
	}
	if cfg.Quality != 80 {
		t.Errorf("default Quality = %d, want 80", cfg.Quality)
	}
}

func TestImagePipelinePlugin_Name(t *testing.T) {
	p := &ImagePipelinePlugin{}
	if got := p.Name(); got != "image_pipeline" {
		t.Errorf("Name() = %q, want %q", got, "image_pipeline")
	}
}

func TestImagePipelinePlugin_Process_Stub(t *testing.T) {
	// Create a temp output directory
	tmpDir := t.TempDir()
	p := &ImagePipelinePlugin{cfg: Config{InjectSrcset: false, InjectPicture: false}}
	err := p.Process(tmpDir, "/tmp/src")
	// With empty directory, Process should return nil (no images to process)
	if err != nil {
		t.Errorf("Process() error: %v", err)
	}
}