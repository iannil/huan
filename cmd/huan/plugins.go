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
func newPluginRegistry(cfg *config.Config, sourceDir string, pluginDirOverride string) (*plugin.Registry, error) {
	reg, err := plugin.NewRegistryFromConfig(cfg, sourceDir, pluginDirOverride)
	if err != nil {
		return nil, err
	}
	plugin.PrintWarnings(os.Stderr, reg.Warnings)
	return reg, nil
}

// pluginDirFromSource returns the plugins directory path relative to sourceDir.
func pluginDirFromSource(sourceDir string) string {
	return plugin.DirFromSource(sourceDir)
}

// loadConfiguredPlugins loads every huan.yaml-declared plugin that is not
// already present in the registry, resolving each .so by naming convention
// ($HUAN_HOME first, then pluginDir — see plugin.Loader.Resolve). Commands
// select the plugin they need by capability interface (deploy.Deployer,
// translate.Translator, ...), so NO plugin name is hardcoded in Go — plugin
// names live only in huan.yaml.
//
// Plugins whose .so is missing or fails to initialize are skipped silently:
// selection is by capability, so an unrelated plugin that can't load must not
// abort the command.
func loadConfiguredPlugins(registry *plugin.Registry, pluginDir, sourceDir string, plugins map[string]config.PluginConfig) {
	loader := plugin.NewLoader(pluginDir)
	for name, pc := range plugins {
		if _, exists := registry.Get(name); exists {
			continue
		}
		soPath := loader.Resolve(plugin.SoFileName(name))
		if soPath == "" {
			continue
		}
		// Copy the plugin config and inject the project root so plugins that
		// resolve project-relative paths (prompt/glossary files, etc.) work
		// without the command knowing which plugin needs it.
		cfgMap := make(map[string]any, len(pc.Config)+1)
		for k, v := range pc.Config {
			cfgMap[k] = v
		}
		cfgMap["_project_root"] = sourceDir

		p, err := loader.LoadPlugin(soPath, cfgMap)
		if err != nil {
			continue
		}
		if _, exists := registry.Get(p.Name()); exists {
			continue
		}
		_ = registry.Register(p)
	}
}

// diagnoseCapabilityGap explains why a capability lookup came up empty. When no
// plugin is loaded it returns "" (a genuine "not configured" case the caller
// should report plainly). When plugins ARE loaded but none satisfy the wanted
// capability, it lists each plugin with the capabilities it actually satisfies,
// so an operator can spot the plugin that should provide `capability` but shows
// [none] — the signature of a .so/host contract mismatch (see 2026-07-28).
func diagnoseCapabilityGap(registry *plugin.Registry, capability string) string {
	plugins := registry.All()
	if len(plugins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(plugins))
	for _, p := range plugins {
		labels := capabilityLabels(p)
		if len(labels) == 0 {
			labels = []string{"none"}
		}
		parts = append(parts, fmt.Sprintf("%s[%s]", p.Name(), strings.Join(labels, ",")))
	}
	return fmt.Sprintf(
		"%d plugin(s) loaded, none satisfy %s: %s; a plugin that should provide it but shows [none] indicates a .so/host contract mismatch — rebuild plugins against current pkg/ contracts (scripts/build-plugins.sh && cp release/plugins/*.so \"$HUAN_HOME\")",
		len(plugins), capability, strings.Join(parts, ", "),
	)
}

// capabilityLabels returns the capability interface names a plugin implements.
// Used by `huan plugin list` to show what each plugin can do.
func capabilityLabels(p plugin.Plugin) []string {
	var labels []string
	if _, ok := p.(deploy.Deployer); ok {
		labels = append(labels, "deploy")
	}
	if _, ok := p.(image.ImageProcessor); ok {
		labels = append(labels, "image")
	}
	if _, ok := p.(theme.ThemePlugin); ok {
		labels = append(labels, "theme")
	}
	if _, ok := p.(build.Hook); ok {
		labels = append(labels, "hook")
	} else if _, ok := p.(plugin.PostBuildHook); ok {
		labels = append(labels, "hook")
	}
	return labels
}
