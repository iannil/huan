package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/content"
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
		AfterBuildSite: b.buildAndPersistDAG,
	})
	if err != nil {
		_ = b.opts.Bus.Publish(ctx, eventbus.Event{
			Type:      eventbus.EventBuildFailed,
			Timestamp: time.Now(),
			Payload:   err.Error(),
		})
		return fmt.Errorf("full build: %w", err)
	}

	b.opts.Logf("builder: full build complete: %d pages, %d files, %d bytes",
		result.PagesRendered, result.FilesWritten, result.BytesWritten)

	_ = b.opts.Bus.Publish(ctx, eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   result,
	})
	return nil
}

// buildAndPersistDAG builds the DAG from the site and persists it to disk.
// This is called as the AfterBuildSite callback after a successful full build.
func (b *Builder) buildAndPersistDAG(site *content.Site) error {
	b.opts.DAG.BuildFromSite(site)
	dagPath := filepath.Join(b.opts.OutputDir, ".dag.json")
	if data, err := b.opts.DAG.Serialize(); err == nil {
		_ = os.WriteFile(dagPath, data, 0644)
		b.opts.Logf("builder: DAG persisted to %s (%d nodes)", dagPath, b.opts.DAG.NodeCount())
	}
	return nil
}

// HandleContentChanged is called when content changes are detected.
func (b *Builder) HandleContentChanged(ctx context.Context, event eventbus.Event) error {
	changedFiles := []string{}
	if payload, ok := event.Payload.(map[string]interface{}); ok {
		if files, ok := payload["changed_files"].([]string); ok {
			changedFiles = files
		}
	}

	// Phase 1: Incremental build only works for modified files.
	// New/deleted files fall back to full rebuild.
	if len(changedFiles) > 0 && b.opts.DAG.NodeCount() > 0 {
		return b.IncrementalBuild(ctx, changedFiles)
	}

	b.opts.Logf("builder: content changed, starting full rebuild...")
	return b.FullBuild(ctx)
}

// IncrementalBuild rebuilds only the pages affected by the given file changes.
// It uses the DAG to determine which pages need to be rebuilt.
func (b *Builder) IncrementalBuild(ctx context.Context, changedFiles []string) error {
	affected := b.opts.DAG.AffectedBy(changedFiles)
	if len(affected) == 0 {
		b.opts.Logf("builder: no pages affected by changes")
		return nil
	}

	b.opts.Logf("builder: incremental build: %d pages affected by %d file changes",
		len(affected), len(changedFiles))

	// Phase 1 limitation: Single-page rendering (build.RenderPage) is not
	// implemented yet. For now, fall back to full build when incremental
	// is triggered. This will be properly implemented in Phase 2.
	b.opts.Logf("builder: incremental build not yet implemented, falling back to full build")
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