package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMarkdownDiskRoundTripAndIsolation(t *testing.T) {
	requireMarkdownDiskWrites(t)
	dir := t.TempDir()
	c, err := NewPersistentMarkdownCache(4096, dir, "engine-a", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	key := markdownKey{Raw: [32]byte{1}}
	want := markdownValue{HTML: "<p>hello\xff</p>", Plain: "hello", Summary: "<p>hello</p>", WordCount: 1}
	c.put(key, want)
	// Reopen without any in-memory state, like a new CLI process.
	reopened, err := NewPersistentMarkdownCache(4096, dir, "engine-a", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.get(key); !ok || got != want {
		t.Fatalf("round trip=%+v %v", got, ok)
	}
	if reopened.Stats().DiskHits != 1 {
		t.Fatal("expected disk hit")
	}
	other, err := NewPersistentMarkdownCache(4096, dir, "engine-b", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := other.get(key); ok {
		t.Fatal("engine namespace collision")
	}
}

func TestMarkdownDiskCorruptionFallsBack(t *testing.T) {
	requireMarkdownDiskWrites(t)
	dir := t.TempDir()
	c, err := NewPersistentMarkdownCache(4096, dir, "engine", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	key := markdownKey{Raw: [32]byte{1}}
	c.put(key, markdownValue{HTML: "valid"})
	paths, err := filepath.Glob(filepath.Join(dir, "*.mdcache"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("paths %v %v", paths, err)
	}
	if err := os.WriteFile(paths[0], []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Clear()
	if _, ok := c.get(key); ok {
		t.Fatal("corrupt result accepted")
	}
	c.put(key, markdownValue{HTML: "repaired"})
	c.Clear()
	if got, ok := c.get(key); !ok || got.HTML != "repaired" {
		t.Fatal("entry did not self repair")
	}
}

func TestMarkdownDiskBudgetAndOversizedEntry(t *testing.T) {
	requireMarkdownDiskWrites(t)
	dir := t.TempDir()
	c, err := NewPersistentMarkdownCache(0, dir, "engine", 1024)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	for n := 0; n < 8; n++ {
		c.put(markdownKey{Raw: [32]byte{byte(n)}}, markdownValue{HTML: strings.Repeat("a", 200)})
		if used := markdownDiskUsage(t, dir); used > 1024 {
			t.Fatalf("store exceeded budget before PruneDisk: %d", used)
		}
	}
	c.put(markdownKey{Raw: [32]byte{99}}, markdownValue{HTML: strings.Repeat("x", 2048)})
	if _, ok := c.get(markdownKey{Raw: [32]byte{99}}); ok {
		t.Fatal("oversized disk entry stored")
	}
	if err := c.PruneDisk(); err != nil {
		t.Fatal(err)
	}
	var total int64
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if total > 1024 {
		t.Fatalf("disk usage %d", total)
	}
}

func markdownDiskUsage(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}

func newTestMarkdownDisk(t *testing.T, dir string, budget int64) *MarkdownCache {
	t.Helper()
	requireMarkdownDiskWrites(t)
	c, err := NewPersistentMarkdownCache(0, dir, "engine", budget)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c
}

func requireMarkdownDiskWrites(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("disk writes unsupported on this platform")
	}
}

func TestMarkdownDiskIndexesAndBoundsExistingRecords(t *testing.T) {
	dir := t.TempDir()
	c := newTestMarkdownDisk(t, dir, 1<<20)
	for n := 0; n < 6; n++ {
		key := markdownKey{Raw: [32]byte{byte(n)}}
		c.put(key, markdownValue{HTML: strings.Repeat("x", 200)})
		stamp := time.Unix(int64(n+1000), 0)
		if err := os.Chtimes(c.disk.path(c.disk.id(key)), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newTestMarkdownDisk(t, dir, 1024)
	if used := markdownDiskUsage(t, dir); used > 1024 {
		t.Fatalf("existing cache not bounded: %d", used)
	}
	if _, ok := reopened.get(markdownKey{Raw: [32]byte{0}}); ok {
		t.Fatal("oldest record not evicted at startup")
	}
	if _, ok := reopened.get(markdownKey{Raw: [32]byte{5}}); !ok {
		t.Fatal("newest record was not retained")
	}
	reader := newTestMarkdownDisk(t, dir, 512)
	if err := reader.PruneDisk(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.get(markdownKey{Raw: [32]byte{4}}); !ok {
		t.Fatal("readonly instance pruned writer's records")
	}
}

func TestMarkdownDiskWriterLease(t *testing.T) {
	dir := t.TempDir()
	a := newTestMarkdownDisk(t, dir, 4096)
	b := newTestMarkdownDisk(t, dir, 4096)
	key := markdownKey{Raw: [32]byte{1}}
	a.put(key, markdownValue{HTML: "first"})
	b.put(key, markdownValue{HTML: "second"})
	if got, ok := a.disk.load(key); !ok || got.HTML != "first" {
		t.Fatalf("second writer overwrote record: %+v %v", got, ok)
	}
	if got, ok := b.disk.load(key); !ok || got.HTML != "first" {
		t.Fatal("reader cannot read writer records")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	c := newTestMarkdownDisk(t, dir, 4096)
	c.put(key, markdownValue{HTML: "third"})
	if got, ok := c.disk.load(key); !ok || got.HTML != "third" {
		t.Fatal("lease was not released by Close")
	}
}

func TestMarkdownDiskCleansCrashTempsOnlyAfterLease(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, ".markdown-crashed")
	if err := os.WriteFile(old, []byte("orphan"), 0600); err != nil {
		t.Fatal(err)
	}
	c := newTestMarkdownDisk(t, dir, 4096)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("orphan retained: %v", err)
	}
	live := filepath.Join(dir, ".markdown-live")
	if err := os.WriteFile(live, []byte("in progress"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := newTestMarkdownDisk(t, dir, 1)
	if err := reader.PruneDisk(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("reader deleted writer temp: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	newTestMarkdownDisk(t, dir, 4096)
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatalf("next writer retained orphan: %v", err)
	}
}

func TestMarkdownDiskRejectsCopiedRecord(t *testing.T) {
	c := newTestMarkdownDisk(t, t.TempDir(), 4096)
	a, b := markdownKey{Raw: [32]byte{1}}, markdownKey{Raw: [32]byte{2}}
	c.put(a, markdownValue{HTML: "bytes\xff\xfe"})
	data, err := os.ReadFile(c.disk.path(c.disk.id(a)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.disk.path(c.disk.id(b)), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.get(b); ok {
		t.Fatal("copied record accepted under wrong key")
	}
}

func TestMarkdownDiskFailedPublishCleansTempAndReservation(t *testing.T) {
	dir := t.TempDir()
	c := newTestMarkdownDisk(t, dir, 512)
	key := markdownKey{Raw: [32]byte{7}}
	destination := c.disk.path(c.disk.id(key))
	// A directory occupying the destination forces rename to fail after the
	// temporary file has been fully written and closed.
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	c.put(key, markdownValue{HTML: "first"})
	temps, err := filepath.Glob(filepath.Join(dir, ".markdown-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("failed publish leaked temporary files: %v %v", temps, err)
	}
	if c.disk.reserved != 0 || c.disk.used != 0 || len(c.disk.inflight) != 0 {
		t.Fatalf("failed publish leaked capacity: used=%d reserved=%d inflight=%d", c.disk.used, c.disk.reserved, len(c.disk.inflight))
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	want := markdownValue{HTML: "retry"}
	c.put(key, want)
	if got, ok := c.get(key); !ok || got != want {
		t.Fatalf("retry after failed publish=%+v %v", got, ok)
	}
}

func TestMarkdownDiskConcurrentStoreBudget(t *testing.T) {
	dir := t.TempDir()
	const budget = 128 << 10
	c := newTestMarkdownDisk(t, dir, budget)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				key := markdownKey{Raw: [32]byte{byte(n), byte(i % 5)}}
				want := markdownValue{HTML: strings.Repeat("x", 16<<10), Plain: "\xff", Summary: "\xfe", WordCount: 1}
				c.put(key, want)
				if got, ok := c.get(key); ok && got != want {
					t.Error("partial or corrupt concurrent read")
				}
				c.disk.mu.Lock()
				if c.disk.used+c.disk.reserved > budget {
					t.Error("reservations exceed disk budget")
				}
				// Hold the accounting lock: commits cannot swap an entry between
				// enumeration and stat, so observed files cannot be double-counted.
				if used := markdownDiskUsage(t, dir); used > budget {
					t.Errorf("physical bytes exceed budget: %d", used)
				}
				c.disk.mu.Unlock()
			}
		}(n)
	}
	wg.Wait()
}
