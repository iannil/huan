package template

import (
	"context"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
)

func TestLoader_New(t *testing.T) {
	l := NewLoader("/tmp", "mytheme", FuncMap(""))
	if l == nil {
		t.Fatal("expected non-nil loader")
	}
}

func TestLoader_LoadAll(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a minimal template file
	layoutsDir := filepath.Join(tmpDir, "layouts", "_default")
	os.MkdirAll(layoutsDir, 0755)
	os.WriteFile(filepath.Join(layoutsDir, "single.html"), []byte("{{ .Content }}"), 0644)

	l := NewLoader(tmpDir, "", FuncMap(""))
	tmpl, err := l.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
	if tpl := tmpl.Lookup("_default/single.html"); tpl == nil {
		t.Error("expected _default/single.html to be loaded")
	}
}

func TestLoader_LoadAll_WithTheme(t *testing.T) {
	tmpDir := t.TempDir()

	// Create theme template
	themeDir := filepath.Join(tmpDir, "themes", "mytheme", "layouts", "_default")
	os.MkdirAll(themeDir, 0755)
	os.WriteFile(filepath.Join(themeDir, "baseof.html"), []byte("<html>{{ block \"main\" . }}{{ end }}</html>"), 0644)

	// Create local override
	layoutsDir := filepath.Join(tmpDir, "layouts", "_default")
	os.MkdirAll(layoutsDir, 0755)
	os.WriteFile(filepath.Join(layoutsDir, "single.html"), []byte("{{ define \"main\" }}content{{ end }}"), 0644)

	l := NewLoader(tmpDir, "mytheme", FuncMap(""))
	tmpl, err := l.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll with theme: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
}

func TestLoader_LoadAll_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	// No layouts directory

	l := NewLoader(tmpDir, "", FuncMap(""))
	tmpl, err := l.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on empty dir: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template even with no files")
	}
}

func TestLoadAllTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	layoutsDir := filepath.Join(tmpDir, "layouts", "_default")
	os.MkdirAll(layoutsDir, 0755)
	os.WriteFile(filepath.Join(layoutsDir, "single.html"), []byte("{{ .Content }}"), 0644)

	tmpl, err := LoadAllTemplates(tmpDir, "https://example.com/")
	if err != nil {
		t.Fatalf("LoadAllTemplates: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
}

func TestLoadAllTemplates_WithThemeManager(t *testing.T) {
	tmpDir := t.TempDir()
	layoutsDir := filepath.Join(tmpDir, "layouts", "_default")
	os.MkdirAll(layoutsDir, 0755)
	os.WriteFile(filepath.Join(layoutsDir, "single.html"), []byte("layouts single: {{ .Title }}"), 0644)

	// Set up a theme plugin manager with a mock theme
	themeReg := plugin.NewRegistry()
	themeMgr := theme.NewManager(themeReg)

	mockTheme := &mockThemePlugin{
		name: "test_theme",
		info: theme.ThemeInfo{Name: "test_theme"},
		templates: []theme.TemplateEntry{
			{Path: "index.html", Content: "theme index: {{ .Title }}"},
			{Path: "_default/single.html", Content: "theme single: {{ .Title }}"},
		},
		funcMap: template.FuncMap{
			"themeFunc": func() string { return "from_theme" },
		},
	}
	_ = themeReg.Register(mockTheme)
	_ = themeMgr.Activate("test_theme")

	// Load via LoadAllTemplates with theme manager
	tmpl, err := LoadAllTemplates(tmpDir, "https://example.com/", themeMgr)
	if err != nil {
		t.Fatalf("LoadAllTemplates with theme manager: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}

	// Verify theme plugin templates are loaded
	if tpl := tmpl.Lookup("index.html"); tpl == nil {
		t.Error("expected index.html from theme plugin to be loaded")
	}

	// Verify layouts/ overrides theme plugin templates
	if tpl := tmpl.Lookup("_default/single.html"); tpl != nil {
		// Render to verify it's the layouts version, not the theme plugin version
		var buf strings.Builder
		ctx := &Context{Title: "test"}
		if err := tpl.Execute(&buf, ctx); err != nil {
			t.Fatalf("execute _default/single.html: %v", err)
		}
		expected := "layouts single: test"
		if buf.String() != expected {
			t.Errorf("expected %q, got %q", expected, buf.String())
		}
	} else {
		t.Error("expected _default/single.html to be loaded")
	}

	// Verify theme function is available
	if tpl := tmpl.Lookup("index.html"); tpl != nil {
		var buf strings.Builder
		ctx := &Context{Title: "test"}
		if err := tpl.Execute(&buf, ctx); err != nil {
			t.Fatalf("execute index.html: %v", err)
		}
		expected := "theme index: test"
		if buf.String() != expected {
			t.Errorf("expected %q, got %q", expected, buf.String())
		}
	}
}

func TestNewLoaderWithTheme(t *testing.T) {
	themeReg := plugin.NewRegistry()
	themeMgr := theme.NewManager(themeReg)

	mockTheme := &mockThemePlugin{
		name: "test_theme",
		info: theme.ThemeInfo{Name: "test_theme"},
		templates: []theme.TemplateEntry{
			{Path: "index.html", Content: "theme index: {{ .Title }}"},
		},
		funcMap: template.FuncMap{
			"themeFunc": func() string { return "from_theme" },
		},
	}
	_ = themeReg.Register(mockTheme)
	_ = themeMgr.Activate("test_theme")

	fm := template.FuncMap{}
	loader := NewLoaderWithTheme("/tmp/test", "/tmp/test/layouts", fm, themeMgr)
	if loader == nil {
		t.Fatal("expected non-nil loader")
	}
}

// mockThemePlugin is a test double for theme.ThemePlugin.
type mockThemePlugin struct {
	name      string
	info      theme.ThemeInfo
	templates []theme.TemplateEntry
	funcMap   template.FuncMap
	assets    map[string]string
}

func (m *mockThemePlugin) Name() string                                     { return m.name }
func (m *mockThemePlugin) Info() theme.ThemeInfo                            { return m.info }
func (m *mockThemePlugin) Templates() []theme.TemplateEntry                 { return m.templates }
func (m *mockThemePlugin) FuncMap() template.FuncMap                        { return m.funcMap }
func (m *mockThemePlugin) Assets() fs.FS                                    { return nil }
func (m *mockThemePlugin) Start(ctx context.Context) error                  { return nil }
func (m *mockThemePlugin) Stop(ctx context.Context) error                   { return nil }

func TestNewScratch(t *testing.T) {
	s := NewScratch()
	if s == nil {
		t.Fatal("expected non-nil scratch")
	}

	// Set and Get
	s.Set("key", "value")
	if v := s.Get("key"); v != "value" {
		t.Errorf("Scratch.Get: got %v, want value", v)
	}

	// Add for counter
	s.Add("counter", 1)
	s.Add("counter", 2)
	if v := s.Get("counter"); v != 3 {
		t.Errorf("Scratch.Add counter: got %v, want 3", v)
	}

	// Add for slice
	s.Add("items", []interface{}{"a"})
	s.Add("items", []interface{}{"b"})
	items := s.Get("items")
	slice := toSlice(items)
	if len(slice) != 2 {
		t.Errorf("Scratch.Add slice: got %d items, want 2", len(slice))
	}

	// Add for PageSlice
	s.Add("pages", PageSlice{&Context{Title: "p1"}})
	s.Add("pages", PageSlice{&Context{Title: "p2"}})
	pagesItems := s.Get("pages")
	pagesSlice := toSlice(pagesItems)
	if len(pagesSlice) != 2 {
		t.Errorf("Scratch.Add PageSlice: got %d items, want 2", len(pagesSlice))
	}

	// Add for string (concatenation)
	s.Add("str", "hello")
	s.Add("str", " world")
	if v := s.Get("str"); v != "hello world" {
		t.Errorf("Scratch.Add string: got %v, want 'hello world'", v)
	}

	// Add for float
	s.Add("float", 1.5)
	s.Add("float", 2.5)
	if v := s.Get("float"); v != 4.0 {
		t.Errorf("Scratch.Add float: got %v, want 4.0", v)
	}
}

func TestSetActiveTemplate(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("hello"))
	SetActiveTemplate(tmpl)
	// Just verify it doesn't panic
	// Verify tmplRef is set
	if tmplRef == nil {
		t.Error("tmplRef should not be nil after SetActiveTemplate")
	}
}

func TestToURLize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello-world"},
		{"Hello World", "hello-world"},
		{"书稿", "%E4%B9%A6%E7%A8%BF"},
		{"hello-world", "hello-world"},
		{"test_file", "test_file"},
		{"path/to/file", "path/to/file"},
		{"Hello World Test", "hello-world-test"},
		{"(parentheses)", "%28parentheses%29"},
	}

	for _, tt := range tests {
		result := ToURLize(tt.input)
		if result != tt.expected {
			t.Errorf("ToURLize(%q): got %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestReplaceDottedFuncs(t *testing.T) {
	input := `{{ crypto.MD5 "test" }} {{ strings.RuneCount "hello" }}`
	result := replaceDottedFuncs(input)
	if contains(result, "crypto.MD5") {
		t.Error("replaceDottedFuncs should replace crypto.MD5")
	}
	if contains(result, "strings.RuneCount") {
		t.Error("replaceDottedFuncs should replace strings.RuneCount")
	}
	if !contains(result, "crypto_MD5") {
		t.Error("replaceDottedFuncs should have crypto_MD5")
	}
}

func TestPercentEncode(t *testing.T) {
	// Test uppercase hex encoding
	result := percentEncode(0xE4) // UTF-8 byte for '书'
	if result != "%E4" {
		t.Errorf("percentEncode(0xE4): got %s, want %%E4", result)
	}
}
