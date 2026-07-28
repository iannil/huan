// Package plugin provides canonical type definitions for the huan plugin system.
package plugin

import "context"

// PostBuildHook is the .so-facing build hook. Only OnOutputWritten crosses a
// .so boundary usefully: page-mutating hooks would receive an opaque
// interface{} the plugin cannot inspect (content.Page is not importable from
// pkg/ or .so plugin modules). Rich page-level hooks stay in internal/build.Hook,
// which is compiled-in only.
type PostBuildHook interface {
	Plugin

	// OnOutputWritten is called after all output files are written, before the
	// build result is finalized. Receives the output directory for
	// post-processing (e.g. inject SEO meta, enhance sitemap, inject HTML).
	// Collection-not-interruption: a returned error logs a warning and does
	// not abort the build.
	OnOutputWritten(ctx context.Context, outputDir string) error
}
