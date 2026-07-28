package main

import (
	"context"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/translate"
)

// stubDeployer is a minimal deploy.Deployer implementation for testing
// capabilityLabels without pulling in the full cloudflare package.
type stubDeployer struct{ name string }

func (s *stubDeployer) Name() string { return s.name }
func (s *stubDeployer) Deploy(_ context.Context, _ deploy.Options) (*deploy.Report, error) {
	return nil, nil
}

// stubTranslator is a minimal translate.Translator for testing.
type stubTranslator struct{ name string }

func (s *stubTranslator) Name() string { return s.name }
func (s *stubTranslator) Translate(_ context.Context, _ translate.Request) (*translate.Response, error) {
	return nil, nil
}

// stubImageProcessor is a minimal image.ImageProcessor for testing.
type stubImageProcessor struct{ name string }

func (s *stubImageProcessor) Name() string { return s.name }
func (s *stubImageProcessor) Process(_, _ string) error { return nil }

// stubPlainPlugin is a plugin that does NOT implement any capability.
type stubPlainPlugin struct{ name string }

func (s *stubPlainPlugin) Name() string { return s.name }

// Compile-time interface satisfaction checks.
var _ plugin.Plugin = (*stubPlainPlugin)(nil)
var _ deploy.Deployer = (*stubDeployer)(nil)
var _ image.ImageProcessor = (*stubImageProcessor)(nil)

func TestNewPluginRegistry_UnknownPluginSilentlySkipped(t *testing.T) {
	cfg := &config.Config{
		Plugins: map[string]config.PluginConfig{
			"unknown_thing": {Config: map[string]any{"foo": "bar"}},
		},
	}
	r, err := newPluginRegistry(cfg, "")
	if err != nil {
		t.Fatalf("unknown plugin should be silently skipped, got error: %v", err)
	}
	if len(r.All()) != 0 {
		t.Errorf("got %d plugins, want 0 (unknown should be skipped)", len(r.All()))
	}
}

func TestNewPluginRegistry_EmptyPluginsMap(t *testing.T) {
	cfg := &config.Config{}
	r, err := newPluginRegistry(cfg, "")
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

func TestCapabilityLabels_TranslatorPluginReturnsTranslateLabel(t *testing.T) {
	tr := &stubTranslator{name: "x"}
	labels := capabilityLabels(tr)
	if len(labels) != 1 || labels[0] != "translate" {
		t.Errorf("labels = %v, want [translate]", labels)
	}
}

func TestCapabilityLabels_ImageProcessorReturnsImageLabel(t *testing.T) {
	ip := &stubImageProcessor{name: "x"}
	labels := capabilityLabels(ip)
	if len(labels) != 1 || labels[0] != "image" {
		t.Errorf("labels = %v, want [image]", labels)
	}
}

func TestCapabilityLabels_NonDeployerReturnsEmpty(t *testing.T) {
	p := &stubPlainPlugin{name: "x"}
	labels := capabilityLabels(p)
	if len(labels) != 0 {
		t.Errorf("labels = %v, want empty for non-deployer", labels)
	}
}

func TestCapabilityLabels_MultiCapability(t *testing.T) {
	// A plugin that implements both deploy.Deployer and image.ImageProcessor
	// using a single Name method to avoid diamond embedding ambiguity.
	labels := capabilityLabels(&multiCapPlugin{
		stubDeployer:       stubDeployer{name: "multi"},
		stubImageProcessor: stubImageProcessor{name: "multi"},
	})
	if len(labels) != 2 {
		t.Errorf("labels = %v, want 2 capabilities", labels)
	}
}

// multiCapPlugin embeds both deploy and image stubs (both have Name()).
// Use a custom Name() to resolve the diamond ambiguity.
type multiCapPlugin struct {
	stubDeployer
	stubImageProcessor
}

func (m *multiCapPlugin) Name() string { return "multi" }
