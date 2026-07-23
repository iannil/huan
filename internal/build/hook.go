// Package build defines the Hook capability interface for plugins that
// participate in the build pipeline. Each hook point maps to a BuildSite stage.
//
// This is a skeleton interface — no concrete Hook implementations ship with
// huan v1.0. The interface exists so that future plugins (e.g. SEO injector,
// custom output transformer, external API notifier) can register without
// requiring changes to the build pipeline.
//
// See docs/adr/0003-unified-plugin-system.md for the capability interface pattern.
package build

import (
	"context"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/plugin"
)

// Hook is the capability interface for plugins that participate in the build
// pipeline. Each method maps to a BuildSite stage.
//
// Every method is optional — a plugin that only needs to run after writing
// output implements OnOutputWritten and returns nil for the others.
//
// Hook methods are called with collection-not-interruption semantics:
// a failing hook logs a warning but does not abort the build.
type Hook interface {
	plugin.Plugin

	// OnContentLoaded is called after all content files are loaded and parsed
	// (Stage 2 of BuildSite). The plugin receives the full page list and may
	// return a modified list. Returning nil (or the same slice) is a no-op.
	OnContentLoaded(ctx context.Context, pages []*content.Page) ([]*content.Page, error)

	// OnPageRendered is called after each page is rendered to HTML (Stage 5).
	// The plugin may modify the page's Content, Summary, or other fields.
	OnPageRendered(ctx context.Context, page *content.Page) error

	// OnOutputWritten is called after all output files are written but before
	// the build result is finalized (end of Stage 8). Receives the output
	// directory path for post-processing (e.g. copying extra files, generating
	// a manifest, notifying an external service).
	OnOutputWritten(ctx context.Context, outputDir string) error
}