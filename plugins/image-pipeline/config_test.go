package main

import (
	"testing"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil) error: %v", err)
	}
	if cfg.Quality != 80 {
		t.Errorf("default Quality = %d, want 80", cfg.Quality)
	}
	if !cfg.Enabled {
		t.Errorf("default Enabled = %v, want true", cfg.Enabled)
	}
}

func TestParseConfig_FromMap(t *testing.T) {
	raw := map[string]any{
		"quality":  60,
		"formats":  []any{"webp", "avif"},
		"sizes":    []any{320, 640},
		"enabled":  false,
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	if cfg.Quality != 60 {
		t.Errorf("Quality = %d, want 60", cfg.Quality)
	}
	if len(cfg.Formats) != 2 || cfg.Formats[0] != "webp" || cfg.Formats[1] != "avif" {
		t.Errorf("Formats = %v, want [webp avif]", cfg.Formats)
	}
	if len(cfg.Sizes) != 2 || cfg.Sizes[0] != 320 || cfg.Sizes[1] != 640 {
		t.Errorf("Sizes = %v, want [320 640]", cfg.Sizes)
	}
	if cfg.Enabled {
		t.Errorf("Enabled = %v, want false", cfg.Enabled)
	}
}

func TestImagePipelinePlugin_Name(t *testing.T) {
	p := &ImagePipelinePlugin{}
	if got := p.Name(); got != "image_pipeline" {
		t.Errorf("Name() = %q, want %q", got, "image_pipeline")
	}
}

func TestImagePipelinePlugin_Process_Stub(t *testing.T) {
	p := &ImagePipelinePlugin{}
	err := p.Process("/tmp/out", "/tmp/src")
	if err != nil {
		t.Errorf("Process() error: %v", err)
	}
}