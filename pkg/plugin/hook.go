// Package plugin provides canonical type definitions for the huan plugin system.
package plugin

import "context"

// Hook is the capability interface for plugins that participate in the build
// pipeline. Each method maps to a BuildSite stage.
//
// Every method is optional — a plugin that only needs to run after writing
// output implements OnOutputWritten and returns nil for the others.
//
// Hook methods use interface{} for page references to avoid importing the
// content package, which is not importable from pkg/plugin/ or from .so
// plugin modules.
type Hook interface {
	Plugin

	// OnContentLoaded is called after all content files are loaded and parsed.
	// The plugin receives the full page list and may return a modified list.
	// Returning nil (or the same slice) is a no-op.
	OnContentLoaded(ctx context.Context, pages []interface{}) ([]interface{}, error)

	// OnPageRendered is called after each page is rendered to HTML.
	OnPageRendered(ctx context.Context, page interface{}) error

	// OnOutputWritten is called after all output files are written but before
	// the build result is finalized. Receives the output directory path for
	// post-processing.
	OnOutputWritten(ctx context.Context, outputDir string) error
}
