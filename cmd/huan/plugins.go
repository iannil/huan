package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/seo/htmlinjector"
	"github.com/iannil/huan/internal/seo/injector"
	"github.com/iannil/huan/internal/seo/sitemap"
	"github.com/iannil/huan/internal/translate"
)

// newPluginRegistry is the composition root for the unified plugin system
// (ADR 0003 §7). It instantiates each plugin declared in cfg.Plugins via its
// typed constructor and registers it with a fresh Registry.
//
// Adding a compiled-in plugin = add a case to this switch + import the
// plugin package. This file is the only place that knows about all available
// compiled-in plugins.
//
// Unknown plugins declared in yaml are silently skipped at compile time — they
// will be loaded at runtime by the LifecycleManager's .so scanner. Warnings
// are printed to stderr for unknown plugins and config validation issues.
// Validation errors (missing required fields, type mismatches) fail fast.
func newPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
	r := plugin.NewRegistry()
	for name := range cfg.Plugins {
		switch name {
		// ### Compiled-in plugins ###
		case "seo_injector":
			raw := cfg.Plugins[name]
			pluginCfg, err := injector.ParseConfig(raw)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
			if err := r.Register(injector.New(pluginCfg)); err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
		// Add `case "name":` here for plugins compiled into the binary.
		// Example:
		//   case "cloudflare":
		//       cfCfg, err := cloudflare.ParseConfig(raw)
		//       if err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
		//       if err := r.Register(cloudflare.New(cfCfg)); err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
		case "html_injector":
			raw := cfg.Plugins[name]
			pluginCfg, err := htmlinjector.ParseConfig(raw)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
			if err := r.Register(htmlinjector.New(pluginCfg)); err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
		case "sitemap_enhancer":
			raw := cfg.Plugins[name]
			pluginCfg, err := sitemap.ParseConfig(raw)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
			if err := r.Register(sitemap.New(pluginCfg)); err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}

		// ### .so plugins (handled at runtime by LifecycleManager) ###
		default:
			// .so plugin — will be loaded from the plugins/ directory at
			// runtime. Silently skip at compile time.
		}
	}

	// Validate configs against schemas for compiled-in plugins.
	// Unknown plugins (declared in yaml but not registered) produce warnings.
	// Validation errors (missing required, type mismatch) fail fast.
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
	return labels
}