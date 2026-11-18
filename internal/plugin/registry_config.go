package plugin

import (
	"fmt"
	"os"
	"strings"

	"github.com/iannil/huan/internal/config"
)

// NewRegistryFromConfig is the composition root for the unified plugin system
// (ADR 0003 §7). It loads all .so plugins from the plugins/ directory that
// are declared in huan.yaml with category "static" or "mixed", registers
// them with a fresh Registry, and validates their configs against schemas.
//
// Plugins with category "dynamic" are excluded — they are loaded at runtime
// by the daemon's LifecycleManager.
//
// Unknown plugins declared in yaml (not found as .so files) are silently
// skipped at compile time.
func NewRegistryFromConfig(cfg *config.Config, sourceDir string, pluginDirOverride string) (*Registry, error) {
	r := NewRegistry()
	pluginDir := DirFromSource(sourceDir)
	if pluginDirOverride != "" {
		pluginDir = pluginDirOverride
	}
	loader := NewLoader(pluginDir)

	// Scan and load all .so files, filter by category
	results, err := loader.ScanAndLoadByCategory(cfg.Plugins, CategoryStatic, CategoryMixed)
	if err != nil {
		return nil, fmt.Errorf("plugin: scan static plugins: %w", err)
	}
	// Transfer scan warnings onto the registry; printing is the caller's job.
	r.Warnings = append(r.Warnings, loader.Warnings...)
	for _, result := range results {
		name := result.Plugin.Name()
		if _, exists := r.Get(name); exists {
			fmt.Fprintf(os.Stderr, "huan: plugin %q: name conflict, skipping\n", name)
			continue
		}
		if err := r.Register(result.Plugin); err != nil {
			fmt.Fprintf(os.Stderr, "huan: plugin %q: register error: %v\n", name, err)
		}
	}

	// Validate configs against schemas. A plugin declared in yaml but not in
	// this build-stage registry (dynamic plugins) is fine as long as its .so
	// is resolvable — it will be loaded at runtime — so only warn on a truly
	// missing .so.
	soExists := func(name string) bool { return loader.Resolve(SoFileName(name)) != "" }
	if errs, warns := ValidateRawConfigs(r, cfg.Plugins, soExists); len(errs) > 0 || len(warns) > 0 {
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "huan: plugin config warning: %s\n", w)
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("plugin config errors:\n  - %s", strings.Join(errs, "\n  - "))
		}
	}
	return r, nil
}

// DirFromSource returns the plugins directory path relative to sourceDir.
func DirFromSource(sourceDir string) string {
	return sourceDir + "/plugins"
}
