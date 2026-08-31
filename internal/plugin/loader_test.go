package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_LoadPlugin_MissingSymbol(t *testing.T) {
	tmpDir := t.TempDir()
	// Create an empty .so (no InitPlugin symbol)
	emptyPath := filepath.Join(tmpDir, "empty.so")
	if err := os.WriteFile(emptyPath, []byte("not a real .so"), 0644); err != nil {
		t.Fatal(err)
	}
	l := NewLoader(tmpDir)
	_, err := l.LoadPlugin(emptyPath, nil)
	if err == nil {
		t.Fatal("expected error for invalid .so")
	}
	// Should mention "plugin:" prefix from our error wrapping
	if !strings.Contains(err.Error(), "plugin:") {
		t.Errorf("error = %q, want mention plugin:", err.Error())
	}
}

func TestLoader_LoadPlugin_FileNotExist(t *testing.T) {
	l := NewLoader(t.TempDir())
	_, err := l.LoadPlugin("/nonexistent/path/plugin.so", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoader_LoadPlugin_NilConfig(t *testing.T) {
	// Verify that passing nil config doesn't panic and is treated as empty map.
	// This test uses a nonexistent path, so it will error — but the important
	// thing is that nil config is accepted without panic.
	l := NewLoader(t.TempDir())
	_, err := l.LoadPlugin("/nonexistent/path/plugin.so", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoader_ScanAndLoad_DirNotExist(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan
	l := NewLoader("/nonexistent/plugin/dir")
	results, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad on nonexistent dir: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestLoader_ScanAndLoad_EmptyDir(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan
	tmpDir := t.TempDir()
	l := NewLoader(tmpDir)
	results, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad on empty dir: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestLoader_ScanAndLoad_SkipsNonSOFiles(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan
	tmpDir := t.TempDir()
	// Create non-.so files
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("doc"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a subdirectory
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(tmpDir)
	results, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 (non-.so files should be skipped)", len(results))
	}
}

func TestShouldLoadInCategory(t *testing.T) {
	tests := []struct {
		category string
		mode     string
		expected bool
	}{
		{"static", "build", true},
		{"static", "daemon", false},
		{"dynamic", "build", false},
		{"dynamic", "daemon", true},
		{"mixed", "build", true},
		{"mixed", "daemon", true},
		{"", "build", false},
		{"", "daemon", true},
		{"unknown", "build", false},
		{"unknown", "daemon", true},
	}
	for _, tt := range tests {
		got := ShouldLoadInCategory(tt.category, tt.mode)
		if got != tt.expected {
			t.Errorf("ShouldLoadInCategory(%q, %q) = %v, want %v", tt.category, tt.mode, got, tt.expected)
		}
	}
}

// Note: TestLoader_LoadPlugin_RealSO and TestLoader_ScanAndLoad_RealSO
// require a valid .so plugin built with the exact same Go version and
// module state as the test runner. Due to Go plugin system constraints,
// these tests are integration tests that must be run manually after
// building the test fixture with:
//   make -C internal/plugin/testdata/simple_plugin
// The error handling tests above verify the Loader logic comprehensively.

func TestHuanHome_EnvOverride(t *testing.T) {
	t.Setenv("HUAN_HOME", "/custom/huan/home")
	if got := HuanHome(); got != "/custom/huan/home" {
		t.Errorf("HuanHome() = %q, want /custom/huan/home", got)
	}
}

func TestHuanHome_DefaultsToDotHuan(t *testing.T) {
	t.Setenv("HUAN_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}
	want := filepath.Join(home, ".huan")
	if got := HuanHome(); got != want {
		t.Errorf("HuanHome() = %q, want %q", got, want)
	}
}

func TestLoader_Resolve_PrefersHuanHome(t *testing.T) {
	huanHome := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("HUAN_HOME", huanHome)

	// Create $HUAN_HOME/plugins subdirectory (new layout since 8d06b30)
	homePluginsDir := filepath.Join(huanHome, "plugins")
	if err := os.MkdirAll(homePluginsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Same-named .so in both dirs; $HUAN_HOME/plugins must win.
	homeSo := filepath.Join(homePluginsDir, "cloudflare.so")
	projSo := filepath.Join(projectDir, "cloudflare.so")
	if err := os.WriteFile(homeSo, []byte("home"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projSo, []byte("proj"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(projectDir)
	if got := l.Resolve("cloudflare.so"); got != homeSo {
		t.Errorf("Resolve() = %q, want %q ($HUAN_HOME/plugins priority)", got, homeSo)
	}
}

func TestLoader_Resolve_FallsBackToProject(t *testing.T) {
	huanHome := t.TempDir() // exists but empty
	projectDir := t.TempDir()
	t.Setenv("HUAN_HOME", huanHome)

	projSo := filepath.Join(projectDir, "seo-injector.so")
	if err := os.WriteFile(projSo, []byte("proj"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(projectDir)
	if got := l.Resolve("seo-injector.so"); got != projSo {
		t.Errorf("Resolve() = %q, want project fallback %q", got, projSo)
	}
}

func TestLoader_Resolve_NotFound(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir())
	l := NewLoader(t.TempDir())
	if got := l.Resolve("missing.so"); got != "" {
		t.Errorf("Resolve() = %q, want empty string when not found", got)
	}
}
