package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/contentindex"
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

	// ContentIndex is reloaded from <OutputDir>/api/*.json after each build
	// so the /api/v1/* query API serves fresh data. Optional: when nil, the
	// reload hook is skipped.
	ContentIndex *contentindex.ContentIndex
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

	// Invalidate JIT cache — full rebuild means all cached HTML is stale.
	if b.opts.JITCache != nil {
		b.opts.JITCache.Clear()
	}

	// Reload the content query index from <OutputDir>/api/*.json so the
	// /api/v1/* handler serves fresh data after a full build.
	if b.opts.ContentIndex != nil {
		if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
			b.opts.Logf("builder: content index reload: %v", err)
		}
	}

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
	//
	// Path normalization: the file watcher (fsnotify) emits absolute paths,
	// while DAG.sources is keyed by page.RelPath — a path relative to the
	// content/ directory (e.g. "posts/hello.md"). Without normalization,
	// AffectedBy never matches and incremental builds silently no-op. Accept
	// both absolute and site-relative paths; strip the SourceDir prefix and
	// any leading content/ segment so the result matches the RelPath form.
	normalized := normalizeChangedFiles(changedFiles, b.opts.SourceDir)
	affected := b.opts.DAG.AffectedBy(normalized)
	if len(affected) == 0 {
		b.opts.Logf("builder: no pages affected by %d file changes", len(changedFiles))
		return nil
	}

	// 4. Order by dependency (leaf first, parents last).
	ordered := b.opts.DAG.OrderByDependency(affected)
	b.opts.Logf("builder: incremental build: %d pages affected by %d file changes",
		len(ordered), len(changedFiles))

	// 4a. Invalidate ContentCache entries for changed files so the
	// incremental render loads fresh content from disk.
	for _, cf := range normalized {
		b.opts.PipelineCache.ContentCache.Invalidate(cf)
	}

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

	// Invalidate JIT cache for affected pages — their content changed.
	if b.opts.JITCache != nil {
		for _, url := range ordered {
			b.opts.JITCache.Remove(url)
		}
	}

	// Reload the content query index from <OutputDir>/api/*.json so the
	// /api/v1/* handler serves fresh data after an incremental build.
	if b.opts.ContentIndex != nil {
		if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
			b.opts.Logf("builder: content index reload: %v", err)
		}
	}

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

// normalizeChangedFiles converts each changed file path to the DAG's
// RelPath form (relative to the content/ directory, e.g. "posts/hello.md").
//
// Accepts absolute paths (as emitted by the fsnotify watcher), site-relative
// paths (e.g. "content/posts/hello.md"), and already-normalized RelPaths
// (e.g. "posts/hello.md"). Paths outside SourceDir are passed through
// unchanged so HasTemplateChanges-style lookups still work for layouts, etc.
func normalizeChangedFiles(files []string, sourceDir string) []string {
	if len(files) == 0 {
		return files
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, normalizeOneChangedFile(f, sourceDir))
	}
	return out
}

// normalizeOneChangedFile normalizes a single path; see normalizeChangedFiles.
func normalizeOneChangedFile(f, sourceDir string) string {
	if f == "" {
		return f
	}
	// Convert absolute paths to site-relative first.
	rel := f
	if filepath.IsAbs(f) && sourceDir != "" {
		if r, err := filepath.Rel(sourceDir, f); err == nil && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		}
	} else {
		rel = filepath.ToSlash(rel)
	}
	// Strip the leading "content/" segment so the result matches page.RelPath.
	if strings.HasPrefix(rel, "content/") {
		return strings.TrimPrefix(rel, "content/")
	}
	return rel
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
// Reuses the cached pipeline state for speed. Returns the HTML (not written
// to disk); the caller (serving.jitFallback) caches it in JITCache.
//
// Tries the fast path (JITRenderFast) first for regular pages: loads only the
// target page, renders only its markdown, and builds only its context. Falls
// back to the full RenderPageWithCache for list pages or when the cache is
// not yet populated.
//
// Returns an error (→ 404 in serving layer) when the source file cannot
// be resolved or does not exist on disk.
func (b *Builder) RenderPageJIT(ctx context.Context, pageURL string) (string, error) {
	cache := b.opts.PipelineCache

	// Fast path: JITRenderFast for regular pages.
	if cache != nil && cache.ContentCache != nil && cache.Templates != nil && cache.MDRenderer != nil {
		if !isListPage(pageURL) {
			html, err := build.JITRenderFast(build.Options{
				SourceDir:     b.opts.SourceDir,
				OutputDir:     b.opts.OutputDir,
				IncludeDrafts: true,
				Logf:          b.opts.Logf,
				PipelineCache: cache,
			}, cache, pageURL)
			if err == nil && html != "" {
				return html, nil
			}
			// On error or empty, fall through to legacy path.
			if err != nil {
				b.opts.Logf("JIT fast path failed for %s: %v, falling back\n", pageURL, err)
			}
		}
	}

	// Legacy path: RenderPageWithCache.
	// 1. pageURL → source file (relative to content/).
	sourceRel, ok := resolveSourceFile(b.opts.DAG, pageURL)
	if !ok {
		return "", fmt.Errorf("JIT: cannot resolve source for %s", pageURL)
	}

	// 2. Verify the source file exists on disk.
	sourcePath := filepath.Join(b.opts.SourceDir, "content", sourceRel)
	if _, err := os.Stat(sourcePath); err != nil {
		return "", fmt.Errorf("JIT: source not found: %s", sourcePath)
	}

	// 3. Render with cached pipeline (forces IncludeDrafts=true).
	html, err := build.RenderPageWithCache(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: true,
		Logf:          b.opts.Logf,
		PipelineCache: cache,
	}, cache, pageURL)
	if err != nil {
		return "", fmt.Errorf("JIT render %s: %w", pageURL, err)
	}

	return html, nil
}

// isListPage returns true for section, home, tag, taxonomy pages.
func isListPage(pageURL string) bool {
	clean := strings.Trim(pageURL, "/")
	if clean == "" {
		return true // home
	}
	if strings.HasPrefix(clean, "tags/") || strings.HasPrefix(clean, "categories/") {
		return true
	}
	// Single segment = section index
	parts := strings.Split(clean, "/")
	return len(parts) == 1 && !strings.HasSuffix(parts[0], ".html")
}

// resolveSourceFile maps a page URL to its source file path (relative to
// content/). Tries the DAG first (pages present in the last full build),
// then falls back to URL-based derivation for pages not yet built (drafts,
// new pages).
func resolveSourceFile(dg *dag.DependencyGraph, pageURL string) (string, bool) {
	// 1. DAG reverse lookup (exact, for built pages).
	if dg != nil {
		if src, ok := dg.SourceFromPagePath(pageURL); ok {
			return src, true
		}
	}
	// 2. URL derivation (fallback, for unbuilt pages).
	if src := build.ResolveSourceFromURL(pageURL); src != "" {
		return src, true
	}
	return "", false
}