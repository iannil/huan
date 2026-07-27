package theme_test

import (
	"html/template"
	"io/fs"
	"testing"

	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
)

type simpleTheme struct {
	name string
}

func (s *simpleTheme) Name() string                    { return s.name }
func (s *simpleTheme) Info() theme.ThemeInfo            { return theme.ThemeInfo{Name: s.name} }
func (s *simpleTheme) Templates() []theme.TemplateEntry  { return nil }
func (s *simpleTheme) FuncMap() template.FuncMap         { return nil }
func (s *simpleTheme) Assets() fs.FS                     { return nil }

func TestManagerActivateDeactivate(t *testing.T) {
	reg := plugin.NewRegistry()
	_ = reg.Register(&simpleTheme{name: "test_theme"})

	mgr := theme.NewManager(reg)

	// Initially no active theme
	if mgr.Active() != nil {
		t.Error("expected no active theme initially")
	}
	if mgr.ActiveName() != "" {
		t.Errorf("expected empty active name, got %s", mgr.ActiveName())
	}

	// Activate
	if err := mgr.Activate("test_theme"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if mgr.ActiveName() != "test_theme" {
		t.Errorf("expected test_theme, got %s", mgr.ActiveName())
	}
	if mgr.Active() == nil {
		t.Fatal("expected non-nil active theme")
	}

	// Deactivate
	mgr.Deactivate()
	if mgr.Active() != nil {
		t.Error("expected no active theme after deactivation")
	}
}

func TestManagerActivateNotFound(t *testing.T) {
	reg := plugin.NewRegistry()
	mgr := theme.NewManager(reg)

	err := mgr.Activate("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestManagerActivateNonThemePlugin(t *testing.T) {
	reg := plugin.NewRegistry()
	// Register a non-theme plugin (just Name, no ThemePlugin)
	_ = reg.Register(&simplePlugin{name: "cloudflare"})

	mgr := theme.NewManager(reg)
	err := mgr.Activate("cloudflare")
	if err == nil {
		t.Fatal("expected error for non-theme plugin")
	}
}

type simplePlugin struct{ name string }

func (s *simplePlugin) Name() string { return s.name }

func TestManagerListAvailable(t *testing.T) {
	reg := plugin.NewRegistry()
	_ = reg.Register(&simpleTheme{name: "theme_a"})
	_ = reg.Register(&simpleTheme{name: "theme_b"})
	_ = reg.Register(&simplePlugin{name: "not_a_theme"})

	mgr := theme.NewManager(reg)
	available := mgr.ListAvailable()
	if len(available) != 2 {
		t.Errorf("expected 2 available themes, got %d", len(available))
	}
}