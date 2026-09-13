package build

import (
	"sync"
	"time"
)

// TimingEntry reports wall time for a stage. Child stages overlap their parents.
type TimingEntry struct {
	Scope, Stage string
	Duration     time.Duration
	Failed       bool
}

// Timings collects one build's measurements and is safe for concurrent hooks.
type Timings struct {
	mu      sync.Mutex
	entries []TimingEntry
}

func NewTimings() *Timings { return &Timings{} }
func (c *Timings) Measure(scope, stage string, fn func() error) error {
	if c == nil {
		return fn()
	}
	start := time.Now()
	err := fn()
	c.Record(scope, stage, time.Since(start), err != nil)
	return err
}
func (c *Timings) Record(scope, stage string, d time.Duration, failed bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, TimingEntry{scope, stage, d, failed})
}
func (c *Timings) Entries() []TimingEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TimingEntry(nil), c.entries...)
}
func (c *Timings) Report(logf func(string, ...any)) {
	for _, e := range c.Entries() {
		suffix := ""
		if e.Failed {
			suffix = " failed"
		}
		logf("[timings] %s/%s %s%s\n", e.Scope, e.Stage, e.Duration, suffix)
	}
}
