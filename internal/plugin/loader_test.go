package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_LoadPlugin_RealSO(t *testing.T) {
	// This test requires the .so to be built with the exact same build ID as the test binary.
	// Due to Go plugin versioning constraints, this test is skipped by default.
	// To run manually:
	//   go test -c -o /tmp/plugin_test ./internal/plugin
	//   go build -buildmode=plugin -o internal/plugin/testdata/simple_plugin/simple_plugin.so ./internal/plugin/testdata/simple_plugin
	//   /tmp/plugin_test -test.run TestLoader_LoadPlugin_RealSO -test.v
	t.Skip("skipping real .so test - requires manual build due to Go plugin versioning")
}

func TestLoader_ScanAndLoad_WithRealSO(t *testing.T) {
	// This test requires the .so to be built with the exact same build ID as the test binary.
	// Due to Go plugin versioning constraints, this test is skipped by default.
	// To run manually:
	//   go test -c -o /tmp/plugin_test ./internal/plugin
	//   go build -buildmode=plugin -o internal/plugin/testdata/simple_plugin/simple_plugin.so ./internal/plugin/testdata/simple_plugin
	//   /tmp/plugin_test -test.run TestLoader_ScanAndLoad_WithRealSO -test.v
	t.Skip("skipping real .so test - requires manual build due to Go plugin versioning")
}

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
	// plugin.Open rejects the file before symbol lookup, so the error
	// mentions "plugin.Open" or "dlopen" rather than "InitPlugin".
	if !strings.Contains(err.Error(), "plugin.Open") {
		t.Errorf("error = %q, want mention plugin.Open", err.Error())
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