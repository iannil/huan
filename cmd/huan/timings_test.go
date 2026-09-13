package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunBuildFailureTimingsOptional(t *testing.T) {
	old := sourceDir
	defer func() { sourceDir = old }()
	sourceDir = t.TempDir()
	for name, data := range map[string]string{"huan.yaml": "title: fixture\nbaseURL: https://example.org/\n", "content/broken.md": "---\ntitle: [\n---\nBroken"} {
		path := filepath.Join(sourceDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, enabled := range []bool{false, true} {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("timings", enabled, "")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		if err := runBuild(cmd, nil); err == nil {
			t.Fatal("expected invalid content error")
		}
		got := buf.String()
		if strings.Contains(got, "[timings]") != enabled {
			t.Fatalf("enabled=%v output=%s", enabled, got)
		}
		if enabled && (!strings.Contains(got, "load content") || !strings.Contains(got, "build total") || !strings.Contains(got, "failed")) {
			t.Fatalf("missing failure timings: %s", got)
		}
	}
}
