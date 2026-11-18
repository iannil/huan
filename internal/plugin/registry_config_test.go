package plugin

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestNewRegistryFromConfigCollectsConfigWarnings(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Declared in yaml but no .so anywhere — the canonical config warning.
	cfg := &config.Config{Plugins: map[string]config.PluginConfig{
		"ghost": {Category: CategoryStatic},
	}}
	// Capture stderr: config warnings must be collected, not printed inline.
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	reg, err := NewRegistryFromConfig(cfg, dir, pluginsDir)
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("NewRegistryFromConfig: %v", err)
	}
	if reg == nil {
		t.Fatal("registry is nil")
	}
	if strings.Contains(string(out), "huan: plugin config warning:") {
		t.Errorf("config warning printed inline to stderr; want collected only, got: %q", string(out))
	}
	want := `plugin "ghost": declared in yaml but no .so found in $HUAN_HOME or plugins/`
	if len(reg.ConfigWarnings) != 1 || reg.ConfigWarnings[0] != want {
		t.Errorf("ConfigWarnings = %#v, want [%q]", reg.ConfigWarnings, want)
	}
}

func TestPrintConfigWarningsFormat(t *testing.T) {
	var buf bytes.Buffer
	PrintConfigWarnings(&buf, []string{`plugin "ghost": declared in yaml but no .so found in $HUAN_HOME or plugins/`})
	want := "huan: plugin config warning: plugin \"ghost\": declared in yaml but no .so found in $HUAN_HOME or plugins/\n"
	if got := buf.String(); got != want {
		t.Errorf("PrintConfigWarnings output:\n got %q\nwant %q", got, want)
	}
}
