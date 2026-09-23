package main

import (
	"bytes"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandMarkdownCacheFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addBuildCacheFlags(cmd)
	t.Setenv("HUAN_HOME", t.TempDir())
	c := commandMarkdownCache(cmd, nil)
	if c == nil {
		t.Fatal("default cache missing")
	}
	t.Cleanup(func() { _ = c.Close() })
	if c.Stats().MaxBytes != devMarkdownCacheBytes {
		t.Fatal("unexpected memory budget")
	}
	if err := cmd.Flags().Set("noCache", "true"); err != nil {
		t.Fatal(err)
	}
	if commandMarkdownCache(cmd, nil) != nil {
		t.Fatal("noCache ignored")
	}
}

func TestCommandMarkdownCacheCustomDirectory(t *testing.T) {
	cmd := &cobra.Command{}
	addBuildCacheFlags(cmd)
	path := filepath.Join(t.TempDir(), "missing-parent", "cache")
	if err := cmd.Flags().Set("cacheDir", path); err != nil {
		t.Fatal(err)
	}
	c := commandMarkdownCache(cmd, nil)
	if c == nil {
		t.Fatal("custom cache failed")
	}
	t.Cleanup(func() { _ = c.Close() })
}

func TestCommandCacheDirInsideSourceUsesDedicatedSubdirectory(t *testing.T) {
	cmd := &cobra.Command{}
	addBuildCacheFlags(cmd)
	source := t.TempDir()
	if err := cmd.Flags().Set("cacheDir", source); err != nil {
		t.Fatal(err)
	}
	if got := commandCacheDir(cmd); got != filepath.Join(source, "markdown") {
		t.Fatalf("cache dir %q", got)
	}
}

func TestCommandMarkdownCacheUnavailableFallsBack(t *testing.T) {
	cmd := &cobra.Command{}
	addBuildCacheFlags(cmd)
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("cacheDir", path); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	cmd.SetErr(&log)
	c := commandMarkdownCache(cmd, nil)
	if c == nil {
		t.Fatal("memory fallback missing")
	}
	t.Cleanup(func() { _ = c.Close() })
	if !strings.Contains(log.String(), "using memory") {
		t.Fatalf("missing fallback notice: %s", log.String())
	}
}
