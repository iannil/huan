package serve

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherFiresOnChange(t *testing.T) {
	dir, err := os.MkdirTemp("", "huan-watch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: dir,
		Debounce:  50 * time.Millisecond,
		OnChange: func() {
			atomic.AddInt32(&calls, 1)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	// Give watcher time to install hooks
	time.Sleep(100 * time.Millisecond)

	// Write a file
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing
	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got < 1 {
		t.Errorf("OnChange called %d times, want >= 1", got)
	}
}

func TestWatcherDebouncesBursts(t *testing.T) {
	dir, err := os.MkdirTemp("", "huan-watch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: dir,
		Debounce:  100 * time.Millisecond,
		OnChange: func() {
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

	// 5 rapid writes to the same file
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(filepath.Join(dir, "burst.txt"), []byte("x"), 0o644)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait past debounce
	time.Sleep(400 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("OnChange called %d times, want exactly 1 (after debounce)", got)
	}
}

func TestWatcherIgnoresHiddenFiles(t *testing.T) {
	dir, err := os.MkdirTemp("", "huan-watch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: dir,
		Debounce:  50 * time.Millisecond,
		OnChange: func() {
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

	// Write a hidden file — should be ignored
	_ = os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644)

	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("OnChange called %d times for hidden file, want 0", got)
	}
}

func TestWatcherAddsNewSubdirectories(t *testing.T) {
	dir, err := os.MkdirTemp("", "huan-watch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: dir,
		Debounce:  50 * time.Millisecond,
		OnChange: func() {
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

	// Create a new subdirectory then write a file inside it
	subdir := filepath.Join(dir, "newdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give watcher time to detect the new dir and add it
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(subdir, "inside.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got < 1 {
		t.Errorf("OnChange called %d times, want >= 1 (should detect file in new subdir)", got)
	}
}
