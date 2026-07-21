// Package image defines the ImageProcessor capability interface for image
// pipeline plugins. Plugins that process images during build (compress,
// resize, format conversion) implement ImageProcessor.
package image

import (
	"github.com/iannil/huan/internal/plugin"
)

// ImageProcessor is the capability interface for plugins that process images
// during the build pipeline. It embeds plugin.Plugin and adds Process.
type ImageProcessor interface {
	plugin.Plugin

	// Process compresses, converts, and resizes images in the output directory.
	// outputDir is the build output directory (publishDir).
	// sourceDir is the project root (for config resolution).
	// Returns an error if processing cannot proceed.
	Process(outputDir, sourceDir string) error
}