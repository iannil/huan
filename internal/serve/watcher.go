package serve

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
	Logf      func(format string, args ...any)
}

// Watcher recursively watches SourceDir for changes and invokes OnChange
// after a debounce delay.
type Watcher struct {
	opts   WatcherOptions
	fsw    *fsnotify.Watcher
	mu     sync.Mutex
	timer  *time.Timer
	logf   func(string, ...any)
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
	w := &Watcher{opts: opts, fsw: fsw, logf: opts.Logf}
	if err := w.addRecursive(opts.SourceDir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	return w, nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if w.isIgnored(path) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

func (w *Watcher) isIgnored(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	return false
}

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
			w.schedule()
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			w.logf("watcher error: %v\n", err)
		}
	}
}

func (w *Watcher) schedule() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.opts.Debounce, func() {
		if w.opts.OnChange != nil {
			w.opts.OnChange()
		}
	})
}
