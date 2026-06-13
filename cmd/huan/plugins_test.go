package main

import (
	"context"
	"strings"
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

func TestNewPluginRegistry_UnknownPluginReturnsError(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]map[string]any{
			"unknown_thing": {"foo": "bar"},
		},
	}
	_, err := newPluginRegistry(cfg)
	if err == nil {
		t.Fatal("want error for unknown plugin")
	}
	if !strings.Contains(err.Error(), "not compiled in") {
		t.Errorf("err = %q, want contains 'not compiled in'", err.Error())
	}
	if !strings.Contains(err.Error(), "unknown_thing") {
		t.Errorf("err = %q, want mention plugin name", err.Error())
	}
}

func TestNewPluginRegistry_MultipleUnknownPlugins_FailsOnFirst(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]map[string]any{
			"alpha_unknown": {},
			"beta_unknown":  {},
		},
	}
	_, err := newPluginRegistry(cfg)
	if err == nil {
		t.Fatal("want error")
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

func TestNewPluginRegistry_ValidCloudflare(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]map[string]any{
			"cloudflare": {
				"accountId": "acc-1",
				"apiToken":  "tok-1",
				"pages": map[string]any{
					"project": "proj",
					"branch":  "main",
				},
			},
		},
	}
	r, err := newPluginRegistry(cfg)
	if err != nil {
		t.Fatalf("newPluginRegistry: %v", err)
	}
	p, ok := r.Get("cloudflare")
	if !ok {
		t.Fatal("cloudflare plugin not registered")
	}
	if p.Name() != "cloudflare" {
		t.Errorf("Name = %q", p.Name())
	}
}

func TestNewPluginRegistry_CloudflareMissingFieldsReturnsError(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "missing accountId",
			raw:  map[string]any{"apiToken": "tok", "pages": map[string]any{"project": "p", "branch": "b"}},
			want: "accountId",
		},
		{
			name: "missing apiToken",
			raw:  map[string]any{"accountId": "acc", "pages": map[string]any{"project": "p", "branch": "b"}},
			want: "apiToken",
		},
		{
			name: "missing project",
			raw:  map[string]any{"accountId": "acc", "apiToken": "tok", "pages": map[string]any{"branch": "b"}},
			want: "project",
		},
		{
			name: "missing branch",
			raw:  map[string]any{"accountId": "acc", "apiToken": "tok", "pages": map[string]any{"project": "p"}},
			want: "branch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Plugins: map[string]map[string]any{"cloudflare": tc.raw},
			}
			_, err := newPluginRegistry(cfg)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
			if !strings.Contains(err.Error(), "plugin cloudflare") {
				t.Errorf("err = %q, want mention 'plugin cloudflare'", err.Error())
			}
		})
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
