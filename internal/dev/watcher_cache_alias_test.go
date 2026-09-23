package dev

import (
	"os"
	"path/filepath"
	"testing"
)

// A source directory and a custom cache directory may identify the same tree
// through different parent-directory aliases (including /tmp vs /private/tmp).
func TestWatcherCacheIgnoreThroughParentSymlink(t *testing.T) {
	parent := t.TempDir()
	realParent := filepath.Join(parent, "real")
	cache := filepath.Join(realParent, "site", "cache", "markdown")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(realParent, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	source := filepath.Join(alias, "site")
	w, err := NewWatcher(WatcherOptions{SourceDir: source, IgnoreDirs: []string{cache}})
	if err != nil {
		t.Fatal(err)
	}
	defer w.fsw.Close()
	eventPath := filepath.Join(source, "cache", "markdown", "entry.mdcache")
	if !w.isIgnoredDir(eventPath) {
		t.Fatalf("cache writes through source alias trigger rebuilds: %s (ignored real directory %s)", eventPath, cache)
	}
}

func TestWatcherCacheIgnoreMissingDirectoryThroughAlias(t *testing.T) {
	parent := t.TempDir()
	realParent := filepath.Join(parent, "real")
	source := filepath.Join(realParent, "site")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(realParent, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// The custom cache does not exist when the watcher is constructed.
	ignored := filepath.Join(alias, "site", "future", "cache", "markdown")
	w, err := NewWatcher(WatcherOptions{SourceDir: source, IgnoreDirs: []string{ignored}})
	if err != nil {
		t.Fatal(err)
	}
	defer w.fsw.Close()
	actual := filepath.Join(source, "future", "cache", "markdown")
	if !w.isIgnoredDir(filepath.Join(actual, "entry.mdcache")) {
		t.Fatal("future cache alias not ignored")
	}
	if w.isIgnoredDir(filepath.Join(source, "content", "post.md")) {
		t.Fatal("normal content ignored")
	}
	if err := os.MkdirAll(actual, 0755); err != nil {
		t.Fatal(err)
	}
	if err := w.addRecursive(filepath.Join(source, "future")); err != nil {
		t.Fatal(err)
	}
	for _, watched := range w.fsw.WatchList() {
		if watched == actual {
			t.Fatal("created cache directory was watched")
		}
	}
}

func TestWatcherIgnoreAncestorThroughAlias(t *testing.T) {
	parent := t.TempDir()
	realParent := filepath.Join(parent, "real")
	source := filepath.Join(realParent, "site")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(realParent, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	w, err := NewWatcher(WatcherOptions{SourceDir: source, IgnoreDirs: []string{alias}})
	if err != nil {
		t.Fatal(err)
	}
	defer w.fsw.Close()
	if !w.isIgnoredDir(filepath.Join(source, "entry.mdcache")) {
		t.Fatal("alias of ignored ancestor not recognized")
	}
}
