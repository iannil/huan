// Package image_test tests the ImageProcessor capability interface.
package image_test

import (
	"testing"

	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
)

// mockImageProcessor is a test double that implements ImageProcessor.
type mockImageProcessor struct {
	name string
}

func (m *mockImageProcessor) Name() string {
	return m.name
}

func (m *mockImageProcessor) Process(outputDir, sourceDir string) error {
	return nil
}

// TestImageProcessorEmbedsPlugin verifies that ImageProcessor embeds plugin.Plugin.
func TestImageProcessorEmbedsPlugin(t *testing.T) {
	var _ image.ImageProcessor = &mockImageProcessor{}
	var _ plugin.Plugin = &mockImageProcessor{}
}

// TestImageProcessorHasProcessMethod verifies the Process method signature.
func TestImageProcessorHasProcessMethod(t *testing.T) {
	proc := &mockImageProcessor{name: "test"}
	err := proc.Process("/output", "/source")
	if err != nil {
		t.Errorf("Process() returned unexpected error: %v", err)
	}
}

// TestImageProcessorCanBeRegistered verifies ImageProcessor works with plugin.Registry.
func TestImageProcessorCanBeRegistered(t *testing.T) {
	registry := plugin.NewRegistry()
	proc := &mockImageProcessor{name: "test-plugin"}

	err := registry.Register(proc)
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	// Verify it can be found via plugin.Find
	processors := plugin.Find[image.ImageProcessor](registry)
	if len(processors) != 1 {
		t.Errorf("Find[ImageProcessor]() returned %d plugins, want 1", len(processors))
	}
}
