// Package image defines the ImageProcessor capability contract for build-time
// image plugins. It lives in pkg/ (not internal/) so .so plugins can import
// the exact host type — a contract defined in internal/ causes silent
// cross-.so interface mismatch (see the deploy/translate bugs of 2026-07-28).
package image

import pkgplugin "github.com/iannil/huan/pkg/plugin"

// ImageProcessor is the capability interface for plugins that process images
// during the build pipeline (compress, resize, format conversion). It embeds
// the base Plugin and adds Process.
type ImageProcessor interface {
	pkgplugin.Plugin

	// Process compresses, converts, and resizes images in the output directory.
	// outputDir is the build output directory (publishDir).
	// sourceDir is the project root (for config resolution).
	Process(outputDir, sourceDir string) error
}
