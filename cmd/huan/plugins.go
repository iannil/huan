package main

import (
	"fmt"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/deploy/cloudflare"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/translate"
	"github.com/iannil/huan/internal/translate/qwen3"
)

// newPluginRegistry is the composition root for the unified plugin system
// (ADR 0003 §7). It instantiates each plugin declared in cfg.Plugins via its
// typed constructor and registers it with a fresh Registry.
//
// Adding a new plugin = add a case to this switch + import the plugin package.
// This file is the only place that knows about all available plugins.
//
// Unknown plugins declared in yaml fail fast at startup (returns error) rather
// than silently passing through to a nil pointer dereference later.
func newPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
	r := plugin.NewRegistry()
	for name, raw := range cfg.Plugins {
		switch name {
		case "cloudflare":
			cfCfg, err := cloudflare.ParseConfig(raw)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
			if err := r.Register(cloudflare.New(cfCfg)); err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
		case "qwen3_translate":
			qCfg, err := qwen3.ParseConfig(raw)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
			p, err := qwen3.New(qCfg, sourceDir)
			if err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
			if err := r.Register(p); err != nil {
				return nil, fmt.Errorf("plugin %s: %w", name, err)
			}
		default:
			return nil, fmt.Errorf("plugin %q: unknown (not compiled in)", name)
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
	// future: payment.PaymentProvider -> "payment"; etc.
	return labels
}
