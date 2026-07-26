package build

import (
	"html/template"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/build/cache"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/i18n"
	"github.com/iannil/huan/internal/markdown"
	"github.com/iannil/huan/internal/output"
	"github.com/iannil/huan/internal/shortcode"
)

// PipelineCache holds reusable build state across incremental builds.
// Populated after a full build completes. Incremental builds reuse this
// state to avoid re-parsing templates, reloading i18n bundles, and
// re-initializing the writer.
//
// DESIGN NOTE: This cache intentionally does NOT hold per-page content or
// template contexts. Those reference page pointers that change on every
// content edit, so caching them would produce stale output for list pages.
// Instead, incremental builds reload content + rebuild contexts (cheap)
// and reuse only the rendering infrastructure here (expensive to rebuild).
//
// ContentCache is the exception: it caches page content by file path with
// mtime-based invalidation, so incremental builds and JIT can skip re-reading
// unchanged files. It is populated lazily (first access loads the file) and
// is safe to share across builds as long as the source directory is stable.
type PipelineCache struct {
	// Rendering infrastructure (valid until template/i18n/config changes)
	Templates  *template.Template
	I18nBundle *i18n.Bundle
	SCRegistry *shortcode.Registry
	MDRenderer *markdown.Renderer

	// Site config (valid until huan.yaml changes)
	SiteCfg *config.Config

	// Output writer (valid across builds; writes to the same OutputDir)
	Writer *output.Writer

	// BuiltAt records when this cache was populated (the last full build time).
	BuiltAt time.Time

	// ContentCache caches loaded page content by content-relative path.
	// Populated lazily on first access; invalidated by file mtime changes.
	ContentCache *cache.ContentCache
}

// NewPipelineCache returns an empty PipelineCache with BuiltAt set to now.
func NewPipelineCache() *PipelineCache {
	return &PipelineCache{
		BuiltAt:      time.Now(),
		ContentCache: cache.NewContentCache(5000),
	}
}

// HasTemplateChanges reports whether any changed file invalidates the
// pipeline cache. Files under layouts/, i18n/, themes/, or the huan.yaml
// config itself require a full rebuild because they affect the cached
// templates/i18n/config.
//
// Returns false for content/ and static/ changes (those are handled
// incrementally).
func HasTemplateChanges(changedFiles []string, sourceDir string) bool {
	for _, f := range changedFiles {
		rel, err := filepath.Rel(sourceDir, f)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		// If the file is outside sourceDir, Rel returns a "../" path.
		// Those are not template changes relative to this site.
		if strings.HasPrefix(rel, "../") {
			continue
		}
		switch {
		case strings.HasPrefix(rel, "layouts/"):
			return true
		case strings.HasPrefix(rel, "i18n/"):
			return true
		case rel == "huan.yaml":
			return true
		case strings.HasPrefix(rel, "themes/"):
			return true
		}
	}
	return false
}
