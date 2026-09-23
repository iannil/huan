package dev

import "sync"

// RebuildQueue serializes rebuilds and merges changes received during a build.
// The callback handles its own build errors and must return normally.
type RebuildQueue struct {
	mu      sync.Mutex
	idle    *sync.Cond
	rebuild func([]string)
	running bool
	closed  bool
	full    bool
	pending []string
	seen    map[string]bool
}

// NewRebuildQueue creates a queue without starting any goroutines.
func NewRebuildQueue(rebuild func([]string)) *RebuildQueue {
	q := &RebuildQueue{rebuild: rebuild}
	q.idle = sync.NewCond(&q.mu)
	return q
}

// Submit accepts a change batch. nil requests a full rebuild and takes
// precedence over paths in the same pending batch; an empty non-nil slice does
// nothing. The idle caller runs builds synchronously until the queue is empty.
// While a build runs, submissions copy and merge paths, then return immediately.
// The callback may call Submit. Submissions after Close are ignored.
func (q *RebuildQueue) Submit(paths []string) {
	q.mu.Lock()
	if q.closed || (paths != nil && len(paths) == 0) {
		q.mu.Unlock()
		return
	}
	if paths == nil {
		q.full = true
		q.pending, q.seen = nil, nil
	} else if !q.full {
		if q.seen == nil {
			q.seen = make(map[string]bool)
		}
		for _, path := range paths {
			if !q.seen[path] {
				q.seen[path] = true
				q.pending = append(q.pending, path)
			}
		}
	}
	if q.running {
		q.mu.Unlock()
		return
	}
	q.running = true
	for {
		batch := q.pending
		q.full = false
		q.pending, q.seen = nil, nil
		q.mu.Unlock()
		q.rebuild(batch)
		q.mu.Lock()
		if !q.full && len(q.pending) == 0 {
			// The empty check and ownership release share the submission lock,
			// so a change cannot be stranded between the two operations.
			q.running = false
			q.idle.Broadcast()
			q.mu.Unlock()
			return
		}
	}
}

// Close rejects new submissions while allowing accepted work to finish.
// It does not block and may be called from the rebuild callback.
func (q *RebuildQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
}

// Wait blocks until the queue is idle. Call Close first when shutting down so
// no later submission can start another build. Do not call Wait in the callback.
func (q *RebuildQueue) Wait() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.running {
		q.idle.Wait()
	}
}
