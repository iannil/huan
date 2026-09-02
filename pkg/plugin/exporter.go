package plugin

import "context"

// Exporter is the capability interface for plugins that transform site
// content into offline document formats (epub/pdf/docx). It mirrors the
// cross-.so sharing pattern of deploy.Deployer: the contract lives under
// pkg/ so out-of-tree .so plugins import the SAME types as the host binary,
// making exporters discoverable via plugin.Find[Exporter].
type Exporter interface {
	Plugin

	// Export runs the ebook generation batch. Implementations should:
	//   - Honor ctx for cancellation.
	//   - Return a Result with per-item Succeeded/Failed/Skipped lists even on
	//     partial failure (collection-not-interruption).
	//   - Return a non-nil error only when the export cannot proceed at all
	//     (invalid request, missing content root, missing CJK font for pdf).
	Export(ctx context.Context, req ExportRequest) (ExportResult, error)
}

// ExportRequest carries invocation-time parameters from the CLI.
type ExportRequest struct {
	// SourceDir is the project root containing huan.yaml and content/.
	SourceDir string

	// Type filters content: "books", "practices", "all".
	Type string

	// Format filters output: "epub", "pdf", "docx", "all".
	Format string

	// Level filters granularity: "individual", "volumes", "complete", "all".
	// ("volumes" and "seasons" are aliases; normalized here to "volumes".)
	Level string

	// Slug restricts to a single book/practice slug (optional).
	Slug string

	// Volume restricts to one volume/season number, 1-based (optional; 0 = all).
	Volume int

	// Force regenerates even when the manifest hash matches.
	Force bool

	// Jobs is the parallelism for per-book generation. 0 = runtime.NumCPU()-1.
	Jobs int
}

// ExportItem describes one generated artifact.
type ExportItem struct {
	// Path is the output file path relative to the project root.
	Path string
	// Lang is "zh" or "en".
	Lang string
	// Format is "epub", "pdf", or "docx".
	Format string
	// Slug of the source book/practice (empty for complete collections).
	Slug string
}

// ExportFailure pairs a failed unit with its error.
type ExportFailure struct {
	Item ExportItem
	Err  string
}

// ExportResult reports the batch outcome.
type ExportResult struct {
	Succeeded []ExportItem
	Failed    []ExportFailure
	// Skipped lists items skipped by the incremental manifest (hash match).
	Skipped []ExportItem
	// Warnings carries non-fatal notes (e.g. missing .en.md sidecars).
	Warnings []string
}
