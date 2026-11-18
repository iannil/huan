package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/config"
)

func TestNewRegistryFromConfig_EmptyConfigYieldsEmptyRegistry(t *testing.T) {
	cfg := &config.Config{}
	reg, err := NewRegistryFromConfig(cfg, t.TempDir(), "")
	if err != nil {
		t.Fatalf("NewRegistryFromConfig: %v", err)
	}
	if len(reg.All()) != 0 {
		t.Errorf("registry not empty: %d plugins", len(reg.All()))
	}
}

func TestNewRegistryFromConfigCollectsWarnings(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "broken.so"), []byte("not a plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Plugins: map[string]config.PluginConfig{
		"broken": {Category: CategoryStatic},
	}}
	reg, err := NewRegistryFromConfig(cfg, dir, pluginsDir)
	if err != nil {
		t.Fatalf("NewRegistryFromConfig: %v", err)
	}
	if len(reg.Warnings) != 1 {
		t.Fatalf("want 1 registry warning, got %d: %+v", len(reg.Warnings), reg.Warnings)
	}
	if reg.Warnings[0].Name != "broken.so" {
		t.Errorf("warning name = %q, want broken.so", reg.Warnings[0].Name)
	}
}

func TestNewRegistryFromConfig_UnknownPluginSilentlySkipped(t *testing.T) {
	dir := t.TempDir()
	// huan.yaml declares a plugin whose .so does not exist — must not error.
	// Mirror of cmd/huan TestNewPluginRegistry_UnknownPluginSilentlySkipped.
	cfg := &config.Config{Plugins: map[string]config.PluginConfig{
		"no_such_plugin": {Config: map[string]any{"enabled": true}},
	}}
	reg, err := NewRegistryFromConfig(cfg, dir, filepath.Join(dir, "plugins"))
	if err != nil {
		t.Fatalf("unknown plugin should be silently skipped, got error: %v", err)
	}
	if _, exists := reg.Get("no_such_plugin"); exists {
		t.Errorf("plugin %q should not be registered", "no_such_plugin")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("source dir untouched check failed: %v", err)
	}
}
