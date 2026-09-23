package build

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestSwapBuildDirReplacesContents verifies the happy path:
// liveDir's old contents are replaced by nextDir's contents,
// and the old contents are removed.
func TestSwapBuildDirReplacesContents(t *testing.T) {
	parent, err := os.MkdirTemp("", "huan-swap-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(parent)

	liveDir := filepath.Join(parent, "live")
	nextDir := filepath.Join(parent, "next")

	// Old live dir has "stale.txt"
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Next dir has "fresh.txt"
	if err := os.MkdirAll(nextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nextDir, "fresh.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SwapBuildDir(liveDir, nextDir); err != nil {
		t.Fatalf("SwapBuildDir: %v", err)
	}

	// liveDir now has fresh.txt, NOT stale.txt
	if _, err := os.Stat(filepath.Join(liveDir, "fresh.txt")); err != nil {
		t.Errorf("fresh.txt missing from liveDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(liveDir, "stale.txt")); err == nil {
		t.Error("stale.txt should have been removed from liveDir")
	}

	// Old staging dir is gone
	if _, err := os.Stat(liveDir + ".old"); err == nil {
		t.Error("liveDir.old should have been cleaned up")
	}
	// Next dir is gone (renamed into live)
	if _, err := os.Stat(nextDir); err == nil {
		t.Error("nextDir should have been renamed into liveDir")
	}
}

// TestSwapBuildDirPreservesLiveOnRenameFailure simulates next→live failure
// and verifies liveDir is restored to its original contents.
func TestSwapBuildDirRollsBackOnError(t *testing.T) {
	parent, err := os.MkdirTemp("", "huan-swap-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(parent)

	liveDir := filepath.Join(parent, "live")
	nextDir := filepath.Join(parent, "next")

	// Populate live
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "original.txt"), []byte("preserved"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Missing nextDir makes the second rename fail after live was moved.
	if err := SwapBuildDir(liveDir, nextDir); err == nil {
		t.Fatal("expected rename failure")
	}
	if data, err := os.ReadFile(filepath.Join(liveDir, "original.txt")); err != nil || string(data) != "preserved" {
		t.Fatalf("rollback lost live content: %q, %v", data, err)
	}
}

func writeSwapVersion(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "version"), []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSwapVersion(t *testing.T, dir, version string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "version"))
	if err != nil || string(data) != version {
		t.Fatalf("%s version = %q, %v; want %q", dir, data, err, version)
	}
}

func TestBuildDirSwapperPublishesBeforeCleanupAndWaits(t *testing.T) {
	live, next := filepath.Join(t.TempDir(), "live"), filepath.Join(t.TempDir(), "next")
	writeSwapVersion(t, live, "old")
	writeSwapVersion(t, next, "new")
	started, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	s := &BuildDirSwapper{cleanup: func(path string) { close(started); <-release; _ = os.RemoveAll(path) }}
	defer func() { unblock.Do(func() { close(release) }); s.Wait() }()
	swapped := make(chan error, 1)
	go func() { swapped <- s.Swap(live, next) }()
	select {
	case err := <-swapped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Swap blocked on cleanup")
	}
	<-started
	assertSwapVersion(t, live, "new")
	assertSwapVersion(t, live+".old", "old")
	waited := make(chan struct{})
	go func() { s.Wait(); close(waited) }()
	select {
	case <-waited:
		t.Fatal("Wait returned before cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	unblock.Do(func() { close(release) })
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not finish")
	}
	if _, err := os.Stat(live + ".old"); !os.IsNotExist(err) {
		t.Fatalf("old directory remains: %v", err)
	}
	assertSwapVersion(t, live, "new")
}

func TestBuildDirSwapperWaitsBeforeReusingOldDirectory(t *testing.T) {
	root := t.TempDir()
	live, next := filepath.Join(root, "live"), filepath.Join(root, "next")
	writeSwapVersion(t, live, "v1")
	writeSwapVersion(t, next, "v2")
	started, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	var calls int
	s := &BuildDirSwapper{cleanup: func(path string) {
		calls++
		if calls == 1 {
			close(started)
			<-release
		}
		_ = os.RemoveAll(path)
	}}
	defer func() { unblock.Do(func() { close(release) }); s.Wait() }()
	if err := s.Swap(live, next); err != nil {
		t.Fatal(err)
	}
	<-started
	writeSwapVersion(t, next, "v3")
	swapped := make(chan error, 1)
	go func() { swapped <- s.Swap(live, next) }()
	select {
	case err := <-swapped:
		t.Fatalf("second swap overtook cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	assertSwapVersion(t, live, "v2")
	assertSwapVersion(t, live+".old", "v1")
	unblock.Do(func() { close(release) })
	select {
	case err := <-swapped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("second swap did not finish")
	}
	s.Wait()
	assertSwapVersion(t, live, "v3")
	if calls != 2 {
		t.Fatalf("cleanup called %d times", calls)
	}
}

func TestBuildDirSwapperFailurePreservesLiveWithoutCleanup(t *testing.T) {
	root := t.TempDir()
	live, next := filepath.Join(root, "live"), filepath.Join(root, "missing")
	writeSwapVersion(t, live, "preserved")
	var calls int
	s := &BuildDirSwapper{cleanup: func(path string) { calls++; _ = os.RemoveAll(path) }}
	if err := s.Swap(live, next); err == nil {
		t.Fatal("expected missing next directory failure")
	}
	s.Wait()
	assertSwapVersion(t, live, "preserved")
	if calls != 0 {
		t.Fatalf("failed swap scheduled %d cleanups", calls)
	}
}

func TestBuildDirSwapperZeroValueCleansOldOutput(t *testing.T) {
	root := t.TempDir()
	live, next := filepath.Join(root, "live"), filepath.Join(root, "next")
	writeSwapVersion(t, live, "old")
	writeSwapVersion(t, next, "new")
	var s BuildDirSwapper
	if err := s.Swap(live, next); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	assertSwapVersion(t, live, "new")
	if _, err := os.Stat(live + ".old"); !os.IsNotExist(err) {
		t.Fatalf("old directory remains: %v", err)
	}
}
