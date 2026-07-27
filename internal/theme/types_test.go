package theme_test

import (
	"context"
	"html/template"
	"io/fs"
	"testing"

	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/shortcode"
	"github.com/iannil/huan/internal/theme"
)

// mockThemePlugin implements theme.ThemePlugin for testing.
type mockThemePlugin struct {
	plugin.Plugin
	info      theme.ThemeInfo
	templates []theme.TemplateEntry
	funcMap   template.FuncMap
	assets    fs.FS
}

func (m *mockThemePlugin) Name() string                     { return m.info.Name }
func (m *mockThemePlugin) Info() theme.ThemeInfo             { return m.info }
func (m *mockThemePlugin) Templates() []theme.TemplateEntry  { return m.templates }
func (m *mockThemePlugin) FuncMap() template.FuncMap         { return m.funcMap }
func (m *mockThemePlugin) Assets() fs.FS                     { return m.assets }

// mockThemePluginShortcodes implements both ThemePlugin and ShortcodeProvider.
type mockThemePluginShortcodes struct {
	plugin.Plugin
	info      theme.ThemeInfo
	templates []theme.TemplateEntry
	funcMap   template.FuncMap
	assets    fs.FS
}

func (m *mockThemePluginShortcodes) Name() string                    { return m.info.Name }
func (m *mockThemePluginShortcodes) Info() theme.ThemeInfo            { return m.info }
func (m *mockThemePluginShortcodes) Templates() []theme.TemplateEntry { return m.templates }
func (m *mockThemePluginShortcodes) FuncMap() template.FuncMap        { return m.funcMap }
func (m *mockThemePluginShortcodes) Assets() fs.FS                    { return m.assets }
func (m *mockThemePluginShortcodes) Shortcodes() map[string]shortcode.Handler {
	return map[string]shortcode.Handler{
		"audio": func(ctx *shortcode.Context) (string, error) {
			return "<audio>overridden</audio>", nil
		},
	}
}

// mockThemePluginWithHooks implements both ThemePlugin and ThemeHooks.
type mockThemePluginWithHooks struct {
	mockThemePlugin
	beforeCalled bool
	afterCalled  bool
}

func (m *mockThemePluginWithHooks) BeforeRender(ctx context.Context) error {
	m.beforeCalled = true
	return nil
}
func (m *mockThemePluginWithHooks) AfterRender(ctx context.Context) error {
	m.afterCalled = true
	return nil
}

func TestThemePluginInterface(t *testing.T) {
	// Verify that a mockThemePlugin satisfies the ThemePlugin interface
	var tp theme.ThemePlugin = &mockThemePlugin{
		info: theme.ThemeInfo{
			Name:    "test_theme",
			Version: "1.0.0",
			Author:  "test",
		},
	}
	if tp.Name() != "test_theme" {
		t.Errorf("expected test_theme, got %s", tp.Name())
	}
}

func TestThemeHooksInterface(t *testing.T) {
	// Verify that a mockThemePluginWithHooks satisfies both interfaces
	var tp theme.ThemePlugin = &mockThemePluginWithHooks{
		mockThemePlugin: mockThemePlugin{
			info: theme.ThemeInfo{Name: "hook_test"},
		},
	}
	hooks, ok := tp.(theme.ThemeHooks)
	if !ok {
		t.Fatal("expected ThemePlugin to implement ThemeHooks")
	}
	if err := hooks.BeforeRender(context.Background()); err != nil {
		t.Errorf("BeforeRender: %v", err)
	}
	if err := hooks.AfterRender(context.Background()); err != nil {
		t.Errorf("AfterRender: %v", err)
	}
}

func TestThemeInfoFields(t *testing.T) {
	info := theme.ThemeInfo{
		Name:        "zhurongshuo",
		Version:     "0.1.0",
		Author:      "iannil",
		Description: "祝融说官方主题",
		Tags:        []string{"blog", "chinese"},
		MinHuanVer:  "v0.7.0",
	}
	if info.Name != "zhurongshuo" {
		t.Errorf("unexpected name: %s", info.Name)
	}
}

func TestTemplateEntry(t *testing.T) {
	entry := theme.TemplateEntry{
		Path:    "index.html",
		Content: "<html>{{ .Title }}</html>",
	}
	if entry.Path != "index.html" {
		t.Errorf("unexpected path: %s", entry.Path)
	}
}

func TestShortcodeProviderInterface(t *testing.T) {
	// Verify that a mockThemePluginShortcodes satisfies both ThemePlugin and ShortcodeProvider
	var tp theme.ThemePlugin = &mockThemePluginShortcodes{
		info: theme.ThemeInfo{Name: "shortcode_test"},
	}
	provider, ok := tp.(theme.ShortcodeProvider)
	if !ok {
		t.Fatal("expected ThemePlugin to implement ShortcodeProvider")
	}
	shortcodes := provider.Shortcodes()
	if len(shortcodes) != 1 {
		t.Fatalf("expected 1 shortcode, got %d", len(shortcodes))
	}
	handler, ok := shortcodes["audio"]
	if !ok {
		t.Fatal("expected 'audio' shortcode handler")
	}
	out, err := handler(&shortcode.Context{})
	if err != nil {
		t.Errorf("handler returned error: %v", err)
	}
	if out != "<audio>overridden</audio>" {
		t.Errorf("unexpected output: %s", out)
	}
}