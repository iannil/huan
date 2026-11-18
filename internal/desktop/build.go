package desktop

import (
	"fmt"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
)

// BuildSiteTo runs the production build for sourceDir into outputDir —
// the same pipeline as `huan build` defaults (full build + image pipeline,
// no dev-only caches since a one-shot build has no incremental reuse).
// onPluginWarnings receives plugins that failed the static registry scan
// (empty slices are never delivered); nil drops them. onConfigWarnings
// receives config-level plugin warnings (yaml-declared plugins with no .so)
// the same way. Both fire synchronously before the build starts, on the
// caller's goroutine.
func BuildSiteTo(sourceDir, outputDir string, logf func(format string, a ...any), onPluginWarnings func([]plugin.LoadWarning), onConfigWarnings func([]string)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	reg, err := plugin.NewRegistryFromConfig(cfg, sourceDir, "")
	if err != nil {
		return fmt.Errorf("plugin registry: %w", err)
	}
	if len(reg.Warnings) > 0 && onPluginWarnings != nil {
		onPluginWarnings(reg.Warnings)
	}
	if len(reg.ConfigWarnings) > 0 && onConfigWarnings != nil {
		onConfigWarnings(reg.ConfigWarnings)
	}
	themeMgr := theme.NewManager(reg)
	if cfg.Theme != "" {
		if err := themeMgr.Activate(cfg.Theme); err != nil {
			logf("huan: theme activate %q: %v", cfg.Theme, err)
		}
	}
	opts := build.Options{
		Timings:        build.NewTimings(),
		SourceDir:      sourceDir,
		OutputDir:      outputDir,
		Logf:           logf,
		PluginRegistry: reg,
		ThemeManager:   themeMgr,
	}
	if cfg.IsMultiLanguage() {
		if _, err := build.BuildMultiSite(opts); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	} else {
		if _, err := build.BuildSite(opts); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	}
	return image.RunProcessors(outputDir, sourceDir, reg)
}
