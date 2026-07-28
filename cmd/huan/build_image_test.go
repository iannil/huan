package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/plugin"
)

// mockImageProcessor is a test ImageProcessor that records Process calls.
type mockImageProcessor struct {
	name       string
	called     bool
	gotOutput  string
	gotSource  string
	returnErr  error
}

func (m *mockImageProcessor) Name() string { return m.name }
func (m *mockImageProcessor) Process(outputDir, sourceDir string) error {
	m.called = true
	m.gotOutput = outputDir
	m.gotSource = sourceDir
	return m.returnErr
}

// TestRunImagePipeline_NoProcessor verifies runImagePipeline is a no-op when no
// ImageProcessor is registered (selection is by capability, not by name).
func TestRunImagePipeline_NoProcessor(t *testing.T) {
	if err := runImagePipeline(t.TempDir(), t.TempDir(), plugin.NewRegistry()); err != nil {
		t.Errorf("expected nil when no ImageProcessor registered, got: %v", err)
	}
}

// TestRunImagePipeline_RunsRegisteredProcessor verifies a registered
// ImageProcessor is discovered by capability and executed with the right dirs.
func TestRunImagePipeline_RunsRegisteredProcessor(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "source")
	outputDir := filepath.Join(t.TempDir(), "output")

	reg := plugin.NewRegistry()
	mock := &mockImageProcessor{name: "mock_image"}
	if err := reg.Register(mock); err != nil {
		t.Fatal(err)
	}

	if err := runImagePipeline(sourceDir, outputDir, reg); err != nil {
		t.Fatalf("runImagePipeline: %v", err)
	}
	if !mock.called {
		t.Fatal("expected Process to be called")
	}
	if mock.gotOutput != outputDir || mock.gotSource != sourceDir {
		t.Errorf("Process got (out=%q, src=%q), want (out=%q, src=%q)",
			mock.gotOutput, mock.gotSource, outputDir, sourceDir)
	}
}

// TestRunImagePipeline_PropagatesError verifies a processor error is wrapped and
// returned.
func TestRunImagePipeline_PropagatesError(t *testing.T) {
	reg := plugin.NewRegistry()
	if err := reg.Register(&mockImageProcessor{name: "mock_image", returnErr: errors.New("boom")}); err != nil {
		t.Fatal(err)
	}

	err := runImagePipeline(t.TempDir(), t.TempDir(), reg)
	if err == nil {
		t.Fatal("expected error from processor")
	}
}
