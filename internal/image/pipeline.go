package image

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// RunProcessors executes any registered image-processing plugin after a
// build. Plugins are selected by the ImageProcessor capability interface —
// no plugin name is hardcoded here.
//
// Registration model: build-time image plugins are loaded via the static/mixed
// plugin registry (like seo_injector/sitemap_enhancer); in daemon mode they are
// loaded by the LifecycleManager. Either way they appear in `registry`, so this
// function just discovers them by capability. If none is registered there is
// nothing to do.
func RunProcessors(outputDir, sourceDir string, registry *plugin.Registry) error {
	for _, p := range plugin.Find[ImageProcessor](registry) {
		if err := p.Process(outputDir, sourceDir); err != nil {
			return fmt.Errorf("image pipeline: process: %w", err)
		}
	}
	return nil
}
