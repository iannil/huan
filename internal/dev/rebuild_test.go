package dev

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRebuildQueueMergesConcurrentSubmissions(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var active atomic.Int32
	var batches [][]string
	q := NewRebuildQueue(func(paths []string) {
		if active.Add(1) != 1 {
			t.Error("concurrent rebuilds")
		}
		defer active.Add(-1)
		batches = append(batches, paths)
		if len(batches) == 1 {
			close(started)
			<-release
		}
	})
	go func() { q.Submit([]string{"initial.md"}); close(done) }()
	<-started
	var submitters sync.WaitGroup
	for i := 0; i < 100; i++ {
		submitters.Add(1)
		go func(i int) {
			defer submitters.Done()
			q.Submit([]string{fmt.Sprintf("%d.md", i), "shared.md"})
		}(i)
	}
	submitters.Wait()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queue did not drain")
	}
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(batches))
	}
	seen := make(map[string]bool)
	for _, p := range batches[1] {
		if seen[p] {
			t.Errorf("duplicate path %s", p)
		}
		seen[p] = true
	}
	if len(seen) != 101 {
		t.Fatalf("got %d paths, want 101", len(seen))
	}
	for i := 0; i < 100; i++ {
		if !seen[fmt.Sprintf("%d.md", i)] {
			t.Errorf("missing path %d.md", i)
		}
	}
}

func TestRebuildQueueFullRebuildDominatesPendingPaths(t *testing.T) {
	var batches [][]string
	var q *RebuildQueue
	q = NewRebuildQueue(func(paths []string) {
		batches = append(batches, paths)
		if len(batches) == 1 {
			q.Submit([]string{"before.md"})
			q.Submit(nil)
			q.Submit([]string{"after.md"})
		}
	})
	q.Submit([]string{"initial.md"})
	if !reflect.DeepEqual(batches, [][]string{{"initial.md"}, nil}) {
		t.Fatalf("batches = %#v", batches)
	}
}

func TestRebuildQueueCallbackCanSubmitAcrossBatchHandoffs(t *testing.T) {
	var q *RebuildQueue
	var calls int
	var active bool
	q = NewRebuildQueue(func(paths []string) {
		if active {
			t.Error("callback is reentrant")
		}
		active = true
		defer func() { active = false }()
		calls++
		if calls < 1000 {
			q.Submit([]string{fmt.Sprintf("%d.md", calls)})
		}
		// A build reporting an error simply returns; queued work must still run.
	})
	q.Submit(nil)
	if calls != 1000 {
		t.Fatalf("got %d rebuilds, want 1000", calls)
	}
	q.Submit([]string{"idle.md"})
	if calls != 1001 {
		t.Fatalf("submission after idle lost: %d", calls)
	}
}

func TestRebuildQueueCloseDrainsAcceptedWork(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var batches [][]string
	q := NewRebuildQueue(func(paths []string) {
		batches = append(batches, paths)
		if len(batches) == 1 {
			close(started)
			<-release
		}
	})
	go q.Submit([]string{"initial.md"})
	<-started
	paths := []string{"accepted.md"}
	q.Submit(paths)
	paths[0] = "mutated.md"
	q.Close()
	q.Close()
	q.Submit([]string{"rejected.md"})
	go func() { q.Wait(); close(done) }()
	select {
	case <-done:
		t.Fatal("Wait returned during rebuild")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return")
	}
	if !reflect.DeepEqual(batches, [][]string{{"initial.md"}, {"accepted.md"}}) {
		t.Fatalf("batches = %#v", batches)
	}
}
