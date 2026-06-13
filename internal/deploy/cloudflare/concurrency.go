package cloudflare

import (
	"context"
	"sync/atomic"
)

// HTTPMaxParallel is the hard cap on concurrent in-flight HTTP POST requests
// to Cloudflare (Pages assets/upload + R2 PutObject). Per ADR 0002 §14.3 this
// is not user-tunable — exceeding 3 typically trips CF gateway 5xx.
const HTTPMaxParallel = 3

// HTTPDegradedParallel is the cap after a gateway 5xx triggers auto-degrade
// per ADR 0002 §9. Degrade is one-way (this constant never grows back during
// a single deploy invocation).
const HTTPDegradedParallel = 1

// Limiter caps concurrent HTTP requests per ADR §14.3 (3 normal, 1 degraded).
//
// Behavior:
//   - Initially allows up to HTTPMaxParallel concurrent Acquire calls.
//   - Degrade() switches the cap to HTTPDegradedParallel permanently (atomic
//     store; not reversible). In-flight Acquire holders continue; future
//     Acquires use the smaller semaphore.
//   - Acquire respects ctx.Done() — goroutine can be cancelled while waiting.
//   - Release is the caller's responsibility; pair with defer.
//
// Limiter is safe for concurrent use.
type Limiter struct {
	normalSem   chan struct{}
	degradedSem chan struct{}
	degraded    atomic.Bool
}

// NewLimiter returns a fresh Limiter in non-degraded state.
func NewLimiter() *Limiter {
	return &Limiter{
		normalSem:   make(chan struct{}, HTTPMaxParallel),
		degradedSem: make(chan struct{}, HTTPDegradedParallel),
	}
}

// Acquire blocks until a slot is available or ctx is cancelled. Callers MUST
// call Release when done with the slot.
func (l *Limiter) Acquire(ctx context.Context) error {
	sem := l.normalSem
	if l.degraded.Load() {
		sem = l.degradedSem
	}
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees one slot. Must be called exactly once per successful Acquire.
// Releases into whichever semaphore is current (post-Degrade, tokens go back
// to degradedSem even if originally acquired from normalSem — the slot is
// simply freed; over-counting in normalSem is benign because future acquires
// use degradedSem).
func (l *Limiter) Release() {
	if l.degraded.Load() {
		select {
		case <-l.degradedSem:
			return
		default:
		}
	}
	// Try the (currently) active semaphore first; fall back to the other to
	// avoid leaking a token if Degrade happened between Acquire and Release.
	select {
	case <-l.normalSem:
		return
	default:
	}
	select {
	case <-l.degradedSem:
		return
	default:
	}
}

// Degrade permanently lowers the concurrency cap to HTTPDegradedParallel.
// Subsequent Acquire calls use the smaller semaphore. Idempotent — calling
// multiple times is a no-op after the first.
//
// Per ADR §9 this fires on any 5xx response (gateway / app error); the
// conservative over-reaction cost is "deploy gets slower", the under-reaction
// cost is "continuous 5xx storms waste upload attempts".
func (l *Limiter) Degrade() {
	l.degraded.Store(true)
}

// IsDegraded returns whether Degrade has been called. Useful for tests.
func (l *Limiter) IsDegraded() bool {
	return l.degraded.Load()
}
