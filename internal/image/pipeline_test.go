package image

import (
	"testing"

	"github.com/iannil/huan/internal/plugin"
)

// stubImageProcessor is a minimal ImageProcessor for pipeline tests.
type stubImageProcessor struct{ name string }

func (s *stubImageProcessor) Name() string              { return s.name }
func (s *stubImageProcessor) Process(_, _ string) error { return nil }

var _ ImageProcessor = (*stubImageProcessor)(nil)
var _ plugin.Plugin = (*stubImageProcessor)(nil)

func TestRunProcessors_EmptyRegistryIsNoop(t *testing.T) {
	if err := RunProcessors(t.TempDir(), t.TempDir(), plugin.NewRegistry()); err != nil {
		t.Fatalf("RunProcessors with empty registry: %v", err)
	}
}
