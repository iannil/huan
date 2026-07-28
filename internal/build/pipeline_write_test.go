package build

import (
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/iannil/huan/internal/output"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
)

// mockThemePlugin is a ThemePlugin whose assets come from an in-memory FS,
// standing in for a real .so theme plugin's embedded assets.
type mockThemePlugin struct {
	name   string
	assets fs.FS
}

func (m *mockThemePlugin) Name() string                   { return m.name }
func (m *mockThemePlugin) Info() map[string]any           { return map[string]any{"Name": m.name} }
func (m *mockThemePlugin) Templates() []map[string]string { return nil }
func (m *mockThemePlugin) FuncMap() template.FuncMap      { return nil }
func (m *mockThemePlugin) Assets() fs.FS                  { return m.assets }

func TestWriteThemePluginAssets(t *testing.T) {
	dir := t.TempDir()

	reg := plugin.NewRegistry()
	if err := reg.Register(&mockThemePlugin{
		name: "zhurongshuo",
		assets: fstest.MapFS{
			"css/zozo.css":        {Data: []byte("body{}")},
			"js/zozo.js":          {Data: []byte("console.log(1)")},
			"images/favicon.ico":  {Data: []byte("ico")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	mgr := theme.NewManager(reg)
	if err := mgr.Activate("zhurongshuo"); err != nil {
		t.Fatalf("activate: %v", err)
	}

	p := &pipeline{
		writer:       output.NewWriter(dir),
		themeManager: mgr,
		logf:         func(string, ...any) {},
	}
	p.writeThemePluginAssets()

	// Assets must land under theme/<name>/ to match "/theme/<name>/..." URLs.
	want := []string{
		"theme/zhurongshuo/css/zozo.css",
		"theme/zhurongshuo/js/zozo.js",
		"theme/zhurongshuo/images/favicon.ico",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected asset written at %s: %v", rel, err)
		}
	}
}

// A nil theme manager or no active theme must be a no-op, not a panic.
func TestWriteThemePluginAssets_NoTheme(t *testing.T) {
	dir := t.TempDir()
	p := &pipeline{
		writer:       output.NewWriter(dir),
		themeManager: nil,
		logf:         func(string, ...any) {},
	}
	p.writeThemePluginAssets() // must not panic
}
