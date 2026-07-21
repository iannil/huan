package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
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
	Metrics     *MetricsCollector
	BuildDrafts bool
	Logf        func(format string, args ...any)

	// OnAfterBuild is called after a successful full build completes.
	// This is separate from the build's AfterBuild which captures the RenderPageFunc.
	OnAfterBuild func(*build.Result) error

	// PipelineCache holds reusable build state for incremental builds.
	// Populated after the first full build (via build.Options.PipelineCache).
	// When nil, IncrementalBuild falls back to a full build.
	PipelineCache *build.PipelineCache
}

// Builder manages the full build and incremental update pipeline.
type Builder struct {
	opts         BuilderOptions
	busy         atomic.Bool
	pending      atomic.Bool
	renderPageFn build.RenderPageFunc // captured from AfterBuild for JIT use
}

// NewBuilder creates a new Builder.
func NewBuilder(opts BuilderOptions) *Builder {
	return &Builder{opts: opts}
}

// FullBuild runs the complete 8-stage build pipeline.
// Uses the serialized build queue to prevent concurrent builds.
func (b *Builder) FullBuild(ctx context.Context) error {
	return b.QueueBuild(ctx, func() error {
		return b.executeFullBuild(ctx)
	})
}

// executeFullBuild is the inner build logic, called by QueueBuild.
func (b *Builder) executeFullBuild(ctx context.Context) error {
	_ = b.opts.Bus.Publish(ctx, eventbus.Event{
		Type:      eventbus.EventBuildStarted,
		Timestamp: time.Now(),
	})

	// Ensure output directory exists
	_ = os.MkdirAll(b.opts.OutputDir, 0755)

	var renderPageFn build.RenderPageFunc
	start := time.Now()
	result, err := build.BuildSite(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: b.opts.BuildDrafts,
		Logf:          b.opts.Logf,
		PipelineCache: b.opts.PipelineCache,
		AfterBuild: func(r *build.Result, fn build.RenderPageFunc) error {
			renderPageFn = fn // capture for later JIT use
			if b.opts.OnAfterBuild != nil {
				return b.opts.OnAfterBuild(r)
			}
			return nil
		},
		AfterBuildSite: b.buildAndPersistDAG,
	})
	if err != nil {
		_ = b.opts.Bus.Publish(ctx, eventbus.Event{
			Type:      eventbus.EventBuildFailed,
			Timestamp: time.Now(),
			Payload:   err.Error(),
		})
		if b.opts.Metrics != nil {
			b.opts.Metrics.RecordBuildFailure()
		}
		return fmt.Errorf("full build: %w", err)
	}

	// Store the RenderPageFunc for JIT rendering.
	b.renderPageFn = renderPageFn

	// Record metrics
	if b.opts.Metrics != nil {
		b.opts.Metrics.RecordBuild(time.Since(start))
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

// QueueBuild ensures only one build runs at a time. If a build is in progress,
// it marks pending and returns (the running build will trigger a trailing rebuild).
// This coalesces multiple concurrent change events into a single rebuild.
func (b *Builder) QueueBuild(ctx context.Context, buildFn func() error) error {
	// Try to acquire the build lock
	if !b.busy.CompareAndSwap(false, true) {
		// Build in progress, mark pending and return
		b.pending.Store(true)
		b.opts.Logf("builder: build in progress, queuing pending rebuild")
		return nil
	}

	// Execute the build, then check for pending requests
	for {
		err := buildFn()
		if err != nil {
			b.busy.Store(false)
			return err
		}

		// Check if another rebuild was queued while we were building
		if !b.pending.CompareAndSwap(true, false) {
			break // no pending rebuild, exit loop
		}
		b.opts.Logf("builder: pending rebuild detected, running again")
	}

	b.busy.Store(false)
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

	// If no DAG yet, do a full build
	if b.opts.DAG.NodeCount() == 0 {
		b.opts.Logf("builder: no DAG, doing full build")
		return b.FullBuild(ctx)
	}

	// Use the serialized build queue
	return b.QueueBuild(ctx, func() error {
		return b.IncrementalBuild(ctx, changedFiles)
	})
}

// IncrementalBuild rebuilds only the pages affected by the given file changes.
// It uses the DAG to determine affected pages and build.IncrementalRender
// to re-render only those pages, reusing the cached pipeline state.
//
// Falls back to a full build when:
//   - a template/i18n/config/theme file changed (cache invalid)
//   - no PipelineCache is available
//   - the DAG is empty (no prior full build)
func (b *Builder) IncrementalBuild(ctx context.Context, changedFiles []string) error {
	// 1. Template/config/i18n change → full rebuild.
	if build.HasTemplateChanges(changedFiles, b.opts.SourceDir) {
		b.opts.Logf("builder: template/config change detected, doing full build")
		return b.executeFullBuild(ctx)
	}

	// 2. No cache or empty DAG → full build.
	if b.opts.PipelineCache == nil || b.opts.DAG.NodeCount() == 0 {
		b.opts.Logf("builder: no pipeline cache or empty DAG, doing full build")
		return b.executeFullBuild(ctx)
	}

	// 3. Compute affected pages via DAG.
	affected := b.opts.DAG.AffectedBy(changedFiles)
	if len(affected) == 0 {
		b.opts.Logf("builder: no pages affected by %d file changes", len(changedFiles))
		return nil
	}

	// 4. Order by dependency (leaf first, parents last).
	ordered := b.opts.DAG.OrderByDependency(affected)
	b.opts.Logf("builder: incremental build: %d pages affected by %d file changes",
		len(ordered), len(changedFiles))

	// 5. Re-render only affected pages, reusing the cached pipeline state.
	start := time.Now()
	err := build.IncrementalRender(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: b.opts.BuildDrafts,
		Logf:          b.opts.Logf,
		PipelineCache: b.opts.PipelineCache,
	}, b.opts.PipelineCache, ordered)

	if err != nil {
		b.opts.Logf("builder: incremental render failed: %v", err)
		return fmt.Errorf("incremental build: %w", err)
	}

	if b.opts.Metrics != nil {
		b.opts.Metrics.RecordBuild(time.Since(start))
	}

	b.opts.Logf("builder: incremental build complete in %v", time.Since(start))

	// 6. Publish build-completed event with incremental marker.
	_ = b.opts.Bus.Publish(ctx, eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"incremental": true,
			"pages":       len(ordered),
		},
	})
	return nil
}

// TriggerRebuild is an external trigger (from Admin API) to rebuild.
func (b *Builder) TriggerRebuild() {
	_ = b.opts.Bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventContentChanged,
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"trigger": "admin"},
	})
}

// RenderPageJIT renders a single page on demand for JIT fallback.
// Uses the RenderPageFunc captured from AfterBuild during the last full build.
// If no full build has completed yet, returns an error.
//
// Phase 1 limitation: The page lookup from DAG is not yet fully implemented.
// This method requires additional work to properly load page content from
// the source file path stored in the DAG.
func (b *Builder) RenderPageJIT(ctx context.Context, pagePath string) (string, error) {
	if b.renderPageFn == nil {
		return "", fmt.Errorf("JIT rendering: no RenderPageFunc available (full build not yet completed)")
	}
	// Phase 1 limitation: Page lookup from DAG requires loading content files
	// and finding the corresponding *content.Page. The current DAG stores
	// source paths but does not maintain loaded Page objects.
	// This is a Phase 2 enhancement.
	return "", fmt.Errorf("JIT rendering: page lookup from DAG not yet implemented — use full build for now")
}