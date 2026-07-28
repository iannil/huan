package image_test

import (
	"testing"

	pkgimage "github.com/iannil/huan/pkg/image"
	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// mockProc satisfies both the base Plugin and the ImageProcessor capability.
type mockProc struct{}

func (mockProc) Name() string                              { return "mock" }
func (mockProc) Process(outputDir, sourceDir string) error { return nil }

func TestImageProcessorSatisfiedByMock(t *testing.T) {
	var _ pkgplugin.Plugin = mockProc{}
	var _ pkgimage.ImageProcessor = mockProc{}
}
