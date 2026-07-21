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
	_, err := l.LoadPlugin(emptyPath)
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
	_, err := l.LoadPlugin("/nonexistent/path/plugin.so")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoader_ScanAndLoad_DirNotExist(t *testing.T) {
	l := NewLoader("/nonexistent/plugin/dir")
	plugins, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad on nonexistent dir: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("got %d plugins, want 0", len(plugins))
	}
}

func TestLoader_ScanAndLoad_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLoader(tmpDir)
	plugins, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad on empty dir: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("got %d plugins, want 0", len(plugins))
	}
}

func TestLoader_ScanAndLoad_SkipsNonSOFiles(t *testing.T) {
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
	plugins, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("got %d plugins, want 0 (non-.so files should be skipped)", len(plugins))
	}
}

// Note: TestLoader_LoadPlugin_RealSO and TestLoader_ScanAndLoad_RealSO
// require a valid .so plugin built with the exact same Go version and
// module state as the test runner. Due to Go plugin system constraints,
// these tests are integration tests that must be run manually after
// building the test fixture with:
//   make -C internal/plugin/testdata/simple_plugin
// The error handling tests above verify the Loader logic comprehensively.
