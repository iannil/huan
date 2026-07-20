package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/dag"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

// BuilderOptions configures the Builder.
type BuilderOptions struct {
	SourceDir   string
	OutputDir   string
	Bus         eventbus.EventBus
	DAG         *dag.DependencyGraph
	JITCache    *cache.JITCache
	BuildDrafts bool
	Logf        func(format string, args ...any)
}

// Builder manages the full build and incremental update pipeline.
type Builder struct {
	opts    BuilderOptions
	busy    bool
	pending bool
}

// NewBuilder creates a new Builder.
func NewBuilder(opts BuilderOptions) *Builder {
	return &Builder{opts: opts}
}

// FullBuild runs the complete 8-stage build pipeline.
func (b *Builder) FullBuild(ctx context.Context) error {
	b.busy = true
	defer func() { b.busy = false }()

	_ = b.opts.Bus.Publish(ctx, eventbus.Event{
		Type:      eventbus.EventBuildStarted,
		Timestamp: time.Now(),
	})

	// Ensure output directory exists
	_ = os.MkdirAll(b.opts.OutputDir, 0755)

	result, err := build.BuildSite(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: b.opts.BuildDrafts,
		Logf:          b.opts.Logf,
	})
	if err != nil {
		_ = b.opts.Bus.Publish(ctx, eventbus.Event{
			Type:      eventbus.EventBuildFailed,
			Timestamp: time.Now(),
			Payload:   err.Error(),
		})
		return fmt.Errorf("full build: %w", err)
	}

	// Build DAG from site (for incremental updates)
	// Note: This builds DAG from the pipeline's site state.
	// For now, we skip DAG building in Phase 1 — it will be added
	// in Phase 2 when the pipeline exposes the site.

	b.opts.Logf("builder: full build complete: %d pages, %d files, %d bytes",
		result.PagesRendered, result.FilesWritten, result.BytesWritten)

	_ = b.opts.Bus.Publish(ctx, eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   result,
	})
	return nil
}

// HandleContentChanged is called when content changes are detected.
func (b *Builder) HandleContentChanged(ctx context.Context, event eventbus.Event) error {
	b.opts.Logf("builder: content changed, starting rebuild...")
	return b.FullBuild(ctx)
}

// TriggerRebuild is an external trigger (from Admin API) to rebuild.
func (b *Builder) TriggerRebuild() {
	_ = b.opts.Bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventContentChanged,
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"trigger": "admin"},
	})
}