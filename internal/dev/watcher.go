package dev

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatcherOptions configures a Watcher.
type WatcherOptions struct {
	SourceDir string
	Debounce  time.Duration
	OnChange  func()
	// OnChanges receives all unique changed paths and takes precedence over OnChange.
	OnChanges func([]string)
	Logf      func(format string, args ...any)
	// IgnoreDirs lists directories (relative to SourceDir, or absolute) that
	// are neither watched nor able to trigger rebuilds — e.g. the publish
	// output dir, which a concurrent `huan build` may rewrite.
	IgnoreDirs []string
}

// Watcher recursively watches SourceDir for changes and invokes OnChange
// after a debounce delay.
type Watcher struct {
	opts       WatcherOptions
	fsw        *fsnotify.Watcher
	mu         sync.Mutex
	timer      *time.Timer
	logf       func(string, ...any)
	ignoreAbs  map[string]bool
	pending    []string
	seen       map[string]bool
	generation uint64
	stopped    bool
	ctx        context.Context
	callbacks  sync.WaitGroup
}

func NewWatcher(opts WatcherOptions) (*Watcher, error) {
	if opts.Debounce == 0 {
		opts.Debounce = 400 * time.Millisecond
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{opts: opts, fsw: fsw, logf: opts.Logf, ignoreAbs: map[string]bool{}}
	rootAbs, err := filepath.Abs(opts.SourceDir)
	if err != nil {
		_ = fsw.Close()
		return nil, err
	}
	rootReal := resolveWatcherDirectory(rootAbs)
	for _, d := range opts.IgnoreDirs {
		abs := d
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(rootAbs, abs)
		}
		w.ignoreAbs[filepath.Clean(abs)] = true
		real := resolveWatcherDirectory(abs)
		w.ignoreAbs[real] = true
		// fsnotify reports paths using the registered source spelling. Map
		// canonical exclusions back onto that spelling once, not per event.
		if rel, ok := watcherRelativeWithin(rootReal, real); ok {
			w.ignoreAbs[filepath.Join(rootAbs, rel)] = true
		} else if _, ok := watcherRelativeWithin(real, rootReal); ok {
			w.ignoreAbs[rootAbs] = true
		}
	}
	if err := w.addRecursive(opts.SourceDir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	return w, nil
}

func watcherRelativeWithin(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveWatcherDirectory resolves existing parent aliases even when a cache
// directory has not been created yet. It only runs during watcher setup.
func resolveWatcherDirectory(path string) string {
	original := filepath.Clean(path)
	current, tail := original, ""
	for {
		if real, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(real, tail)
		} else if !os.IsNotExist(err) {
			return original
		}
		parent := filepath.Dir(current)
		if parent == current {
			return original
		}
		tail = filepath.Join(filepath.Base(current), tail)
		current = parent
	}
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		// The walk root itself is always watched: `huan dev`'s default
		// --source is ".", whose base name starts with a dot and would
		// otherwise SkipDir the whole tree before anything is added.
		if path != root && (w.isIgnored(path) || w.isIgnoredDir(path)) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

// isIgnored returns true for editor artifacts and dotfiles that should not
// trigger a rebuild. Covers vim swap/backup, emacs lock/auto-save, merge
// leftovers, and vim's "4913" write-test probe.
func (w *Watcher) isIgnored(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	switch base {
	case "4913": // vim's write-permission probe
		return true
	}
	switch {
	case strings.HasSuffix(base, ".swp"), // vim swap
		strings.HasSuffix(base, ".swo"),  // vim swap (overflow)
		strings.HasSuffix(base, ".swn"),  // vim swap (overflow)
		strings.HasSuffix(base, "~"),     // vim/emacs backup
		strings.HasSuffix(base, ".orig"), // merge backup
		strings.HasSuffix(base, ".rej"),  // merge reject
		strings.HasSuffix(base, ".bak"):  // generic backup
		return true
	case strings.HasPrefix(base, "#") && strings.HasSuffix(base, "#"): // emacs auto-save
		return true
	case strings.HasPrefix(base, ".#"): // emacs lock
		return true
	}
	return false
}

// isIgnoredDir reports whether path is inside one of the configured
// IgnoreDirs. Events under an ignored directory never schedule a rebuild.
func (w *Watcher) isIgnoredDir(path string) bool {
	if len(w.ignoreAbs) == 0 {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for d := filepath.Clean(abs); ; {
		if w.ignoreAbs[d] {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	defer w.fsw.Close()
	w.mu.Lock()
	w.ctx = ctx
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.stopped = true
		if w.timer != nil {
			w.timer.Stop()
		}
		w.pending, w.seen = nil, nil
		w.mu.Unlock()
		w.callbacks.Wait()
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			// Metadata-only events (e.g. atime updates fired as Chmod when
			// the build reads files back) never change build output.
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if w.isIgnored(ev.Name) || w.isIgnoredDir(ev.Name) {
				continue
			}
			// If a new dir was created, watch it too
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(ev.Name)
				}
			}
			w.schedule(ev.Name)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			w.logf("watcher error: %v\n", err)
		}
	}
}

// Only logs are abbreviated; callbacks always receive the complete batch.
const maxLoggedPaths = 5

func (w *Watcher) schedule(changedPath string) {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	if w.seen == nil {
		w.seen = make(map[string]bool)
	}
	if changedPath != "" && !w.seen[changedPath] {
		w.seen[changedPath] = true
		w.pending = append(w.pending, changedPath)
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.generation++
	generation := w.generation
	w.timer = time.AfterFunc(w.opts.Debounce, func() {
		w.mu.Lock()
		if w.stopped || generation != w.generation || (w.ctx != nil && w.ctx.Err() != nil) {
			w.mu.Unlock()
			return
		}
		paths := w.pending
		w.pending, w.seen = nil, nil
		w.callbacks.Add(1)
		w.mu.Unlock()
		defer w.callbacks.Done()
		if len(paths) > 0 {
			logged := paths
			if len(logged) > maxLoggedPaths {
				logged = logged[:maxLoggedPaths]
			}
			w.logf("[watch] changed (%d): %s\n", len(paths), strings.Join(logged, ", "))
		}
		if w.opts.OnChanges != nil {
			w.opts.OnChanges(paths)
		} else if w.opts.OnChange != nil {
			w.opts.OnChange()
		}
	})
	w.mu.Unlock()
}
