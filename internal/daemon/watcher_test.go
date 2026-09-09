package daemon

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// TestWatcherRelativeSourceDir reproduces the daemon's default invocation,
// where the source dir is ".". The walk root itself must be watched even
// though its base name starts with a dot.
func TestWatcherRelativeSourceDir(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: ".",
		Debounce:  50 * time.Millisecond,
		OnChange: func(files []string) {
			atomic.AddInt32(&calls, 1)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile("a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got < 1 {
		t.Errorf("OnChange called %d times with relative source dir, want >= 1", got)
	}
}
