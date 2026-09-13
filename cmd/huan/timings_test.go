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

func TestRunBuildDefaultLogsStayOnStdout(t *testing.T) {
	oldSource, oldStdout, oldStderr := sourceDir, os.Stdout, os.Stderr
	defer func() { sourceDir = oldSource; os.Stdout = oldStdout; os.Stderr = oldStderr }()
	sourceDir = t.TempDir()
	for name, data := range map[string]string{"huan.yaml": "title: stdout fixture\nbaseURL: https://example.org/\n", "content/post.md": "---\ntitle: Post\n---\nHello", "layouts/_default/single.html": "{{ .Content }}"} {
		path := filepath.Join(sourceDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	os.Stdout, os.Stderr = stdout, stderr
	cmd := &cobra.Command{}
	cmd.Flags().Bool("timings", false, "")
	if err := runBuild(cmd, nil); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	errs, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Building site: stdout fixture") || !strings.Contains(string(out), "Build complete.") {
		t.Fatalf("normal logs missing from stdout: stdout=%q stderr=%q", out, errs)
	}
	if strings.Contains(string(errs), "Building site:") || strings.Contains(string(errs), "Build complete.") {
		t.Fatalf("build logs reached stderr: %q", errs)
	}
	if strings.Contains(string(out)+string(errs), "[timings]") {
		t.Fatal("timings enabled by default")
	}
}
