package build

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCleanBeforeBuildRemovesStaleOutput verifies that a full build wipes
// stale files left in the publish dir when cleanPublishDir is on (default),
// and keeps them when explicitly disabled.
func TestCleanBeforeBuildRemovesStaleOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-ish")
	}
	root := t.TempDir()
	for _, d := range []string{"content/posts", "layouts", "static"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "huan.yaml"), []byte("baseURL: http://x/\n"), 0644)
	os.WriteFile(filepath.Join(root, "content/posts/a.md"), []byte("---\ntitle: A\ndate: 2026-01-01\n---\nhi\n"), 0644)

	out := filepath.Join(root, "public")
	os.MkdirAll(out, 0755)
	stale := filepath.Join(out, "stale-file.txt")
	os.WriteFile(stale, []byte("old"), 0644)

	if _, err := BuildSite(Options{SourceDir: root, OutputDir: out}); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale file should be removed by default cleanPublishDir=true")
	}
}
