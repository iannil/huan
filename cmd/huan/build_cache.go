package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/plugin"
	"github.com/spf13/cobra"
)

const markdownDiskCacheBytes = 512 << 20

func addBuildCacheFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("noCache", false, "disable reusable build caches")
	cmd.Flags().String("cacheDir", "", "build cache root (default: $HUAN_HOME/cache)")
}

var executableNamespace = sync.OnceValues(func() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return "", err
	}
	// Includes renderer code, dependencies and toolchain. Rebuilding a changed
	// executable cannot reuse results from an older renderer implementation.
	return fmt.Sprintf("markdown-v1-%x", hash.Sum(nil)), nil
})

func commandMarkdownCache(cmd *cobra.Command, timings *build.Timings) *build.MarkdownCache {
	disabled, _ := cmd.Flags().GetBool("noCache")
	if disabled {
		return nil
	}
	cache := build.NewMarkdownCache(devMarkdownCacheBytes)
	_ = timings.Measure("cli", "markdown cache preparation", func() error {
		dir := commandCacheDir(cmd)
		if dir == "" {
			return nil
		}

		namespace, err := executableNamespace()
		if err == nil {
			cache, err = build.NewPersistentMarkdownCache(devMarkdownCacheBytes, dir, namespace, markdownDiskCacheBytes)
		}
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "huan: cache unavailable, using memory: %v\n", err)
			cache = build.NewMarkdownCache(devMarkdownCacheBytes)
		}
		return nil
	})
	return cache
}

func pruneCommandCache(cmd *cobra.Command, cache *build.MarkdownCache, timings *build.Timings) {
	_ = timings.Measure("cli", "markdown cache maintenance", func() error {
		if err := cache.PruneDisk(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "huan: cache maintenance: %v\n", err)
		}
		return nil
	})
}

// commandCacheDir is also used by the watcher, so cache writes can never become
// build input events when a custom directory or HUAN_HOME lives inside a site.
func commandCacheDir(cmd *cobra.Command) string {
	dir, _ := cmd.Flags().GetString("cacheDir")
	if dir == "" {
		home := plugin.HuanHome()
		if home == "" {
			return ""
		}
		dir = filepath.Join(home, "cache")
	}
	dir = filepath.Join(dir, "markdown")
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}
