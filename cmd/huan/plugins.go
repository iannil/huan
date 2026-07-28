package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
	"github.com/iannil/huan/internal/translate"
)

// newPluginRegistry is the composition root for the unified plugin system
// (ADR 0003 §7). It loads all .so plugins from the plugins/ directory that
// are declared in huan.yaml with category "static" or "mixed", registers
// them with a fresh Registry, and validates their configs against schemas.
//
// Plugins with category "dynamic" are excluded — they are loaded at runtime
// by the daemon's LifecycleManager.
//
// Unknown plugins declared in yaml (not found as .so files) are silently
// skipped at compile time.
func newPluginRegistry(cfg *config.Config, sourceDir string) (*plugin.Registry, error) {
	r := plugin.NewRegistry()
	pluginDir := pluginDirFromSource(sourceDir)
	loader := plugin.NewLoader(pluginDir)

	// Scan and load all .so files, filter by category
	results, err := loader.ScanAndLoadByCategory(cfg.Plugins, plugin.CategoryStatic, plugin.CategoryMixed)
	if err != nil {
		return nil, fmt.Errorf("plugin: scan static plugins: %w", err)
	}
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

	// Validate configs against schemas
	if errs, warns := plugin.ValidateRawConfigs(r, cfg.Plugins); len(errs) > 0 || len(warns) > 0 {
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "huan: plugin config warning: %s\n", w)
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("plugin config errors:\n  - %s", strings.Join(errs, "\n  - "))
		}
	}
	return r, nil
}

// pluginDirFromSource returns the plugins directory path relative to sourceDir.
func pluginDirFromSource(sourceDir string) string {
	return sourceDir + "/plugins"
}

// capabilityLabels returns the capability interface names a plugin implements.
// Used by `huan plugin list` to show what each plugin can do.
func capabilityLabels(p plugin.Plugin) []string {
	var labels []string
	if _, ok := p.(deploy.Deployer); ok {
		labels = append(labels, "deploy")
	}
	if _, ok := p.(translate.Translator); ok {
		labels = append(labels, "translate")
	}
	if _, ok := p.(image.ImageProcessor); ok {
		labels = append(labels, "image")
	}
	if _, ok := p.(theme.ThemePlugin); ok {
		labels = append(labels, "theme")
	}
	if _, ok := p.(build.Hook); ok {
		labels = append(labels, "hook")
	}
	return labels
}
