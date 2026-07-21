package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
)

// TestRunImagePipeline_NotConfigured tests that runImagePipeline returns nil
// when image_pipeline is not configured in plugins.
func TestRunImagePipeline_NotConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Plugins: map[string]map[string]any{}, // no image_pipeline
	}

	err := runImagePipeline(cfg, sourceDir, outputDir)
	if err != nil {
		t.Errorf("expected nil when image_pipeline not configured, got: %v", err)
	}
}

// TestRunImagePipeline_ConfiguredButPluginMissing tests that runImagePipeline
// returns an error when image_pipeline is configured but the .so file doesn't exist.
func TestRunImagePipeline_ConfiguredButPluginMissing(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	outputDir := filepath.Join(tmpDir, "output")
	pluginDir := filepath.Join(sourceDir, "plugins")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Plugins: map[string]map[string]any{
			"image_pipeline": {"quality": 85},
		},
	}

	err := runImagePipeline(cfg, sourceDir, outputDir)
	if err == nil {
		t.Fatal("expected error when plugin .so doesn't exist")
	}
	// Should contain "load plugin" in the error message
	if !strings.Contains(err.Error(), "load plugin") {
		t.Errorf("error should contain 'load plugin', got: %v", err)
	}
}

// containsString is a helper to check if s contains substr.
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}