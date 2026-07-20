package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatcherOptions configures the daemon's file watcher.
type WatcherOptions struct {
	SourceDir string
	Debounce  time.Duration // default 400ms
	OnChange  func(changedFiles []string) // called with list of changed files after debounce
	Logf      func(format string, args ...any)
}

// Watcher watches content/ and data/ for changes, then triggers rebuild.
type Watcher struct {
	opts   WatcherOptions
	fsw    *fsnotify.Watcher
	mu     sync.Mutex
	timer  *time.Timer
	pending map[string]struct{}
	logf   func(string, ...any)
}

// NewWatcher creates a new Watcher that recursively watches SourceDir.
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
	w := &Watcher{
		opts:    opts,
		fsw:     fsw,
		pending: make(map[string]struct{}),
		logf:    opts.Logf,
	}
	if err := w.addRecursive(opts.SourceDir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	return w, nil
}

// addRecursive walks root and adds all non-skipped directories to the watcher.
func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if w.isSkippedDir(path) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

// isSkippedDir returns true for directories that should not be watched.
// Skips dotfiles, common non-content directories, and build artifacts.
func (w *Watcher) isSkippedDir(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	switch base {
	case "layouts", "static", "themes", "docs", "node_modules", ".git",
		"public", "resources", "assets":
		return true
	}
	return false
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
		strings.HasSuffix(base, ".swo"), // vim swap (overflow)
		strings.HasSuffix(base, ".swn"), // vim swap (overflow)
		strings.HasSuffix(base, "~"),    // vim/emacs backup
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

// Run starts the watcher event loop. Blocks until ctx is cancelled or watcher
// encounters a fatal error.
func (w *Watcher) Run(ctx context.Context) error {
	defer w.fsw.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if w.isIgnored(ev.Name) {
				continue
			}
			// If a new dir was created, watch it too
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(ev.Name)
				}
			}
			// Accumulate the changed file and schedule debounce
			w.mu.Lock()
			w.pending[ev.Name] = struct{}{}
			w.mu.Unlock()
			w.schedule()
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			w.logf("watcher error: %v", err)
		}
	}
}

// schedule resets the debounce timer. When it fires, pending changes are
// collected and passed to OnChange.
func (w *Watcher) schedule() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.opts.Debounce, func() {
		w.mu.Lock()
		files := make([]string, 0, len(w.pending))
		for f := range w.pending {
			files = append(files, f)
		}
		w.pending = make(map[string]struct{})
		w.mu.Unlock()
		if w.opts.OnChange != nil && len(files) > 0 {
			w.opts.OnChange(files)
		}
	})
}