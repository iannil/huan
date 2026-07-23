package main

import (
	"context"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/plugin"
)

// stubDeployer is a minimal deploy.Deployer implementation for testing
// capabilityLabels without pulling in the full cloudflare package.
type stubDeployer struct{ name string }

func (s *stubDeployer) Name() string { return s.name }
func (s *stubDeployer) Deploy(_ context.Context, _ deploy.Options) (*deploy.Report, error) {
	return nil, nil
}

// stubPlainPlugin is a plugin that does NOT implement Deployer.
type stubPlainPlugin struct{ name string }

func (s *stubPlainPlugin) Name() string { return s.name }

// Compile-time interface satisfaction checks.
var _ plugin.Plugin = (*stubPlainPlugin)(nil)
var _ deploy.Deployer = (*stubDeployer)(nil)

func TestNewPluginRegistry_UnknownPluginSilentlySkipped(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]map[string]any{
			"unknown_thing": {"foo": "bar"},
		},
	}
	r, err := newPluginRegistry(cfg)
	if err != nil {
		t.Fatalf("unknown plugin should be silently skipped, got error: %v", err)
	}
	if len(r.All()) != 0 {
		t.Errorf("got %d plugins, want 0 (unknown should be skipped)", len(r.All()))
	}
}

func TestNewPluginRegistry_EmptyPluginsMap(t *testing.T) {
	cfg := &config.Config{}
	r, err := newPluginRegistry(cfg)
	if err != nil {
		t.Fatalf("empty plugins map: %v", err)
	}
	if len(r.All()) != 0 {
		t.Errorf("got %d plugins, want 0", len(r.All()))
	}
}

func TestCapabilityLabels_DeployerPluginReturnsDeployLabel(t *testing.T) {
	d := &stubDeployer{name: "x"}
	labels := capabilityLabels(d)
	if len(labels) != 1 || labels[0] != "deploy" {
		t.Errorf("labels = %v, want [deploy]", labels)
	}
}

func TestCapabilityLabels_NonDeployerReturnsEmpty(t *testing.T) {
	p := &stubPlainPlugin{name: "x"}
	labels := capabilityLabels(p)
	if len(labels) != 0 {
		t.Errorf("labels = %v, want empty for non-deployer", labels)
	}
}
