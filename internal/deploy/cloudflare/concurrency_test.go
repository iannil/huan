package cloudflare

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiter_AcquireRelease_Normal(t *testing.T) {
	l := NewLimiter()
	ctx := context.Background()

	if err := l.Acquire(ctx); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if l.IsDegraded() {
		t.Error("Limiter should not be degraded")
	}
	l.Release()
}

func TestLimiter_Acquire_BlocksWhenFull(t *testing.T) {
	l := NewLimiter()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Fill all 3 normal slots.
	for i := 0; i < HTTPMaxParallel; i++ {
		if err := l.Acquire(ctx); err != nil {
			t.Fatalf("fill Acquire %d: %v", i, err)
		}
	}

	// Next Acquire should block until ctx timeout.
	err := l.Acquire(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Acquire when full = %v, want DeadlineExceeded", err)
	}
}

func TestLimiter_DegradeLowersCapToOne(t *testing.T) {
	l := NewLimiter()

	// Pre-degrade: 3 concurrent acquires succeed.
	for i := 0; i < HTTPMaxParallel; i++ {
		if err := l.Acquire(context.Background()); err != nil {
			t.Fatalf("pre-degrade Acquire %d: %v", i, err)
		}
	}
	for i := 0; i < HTTPMaxParallel; i++ {
		l.Release()
	}

	// Degrade.
	l.Degrade()
	if !l.IsDegraded() {
		t.Error("IsDegraded = false after Degrade")
	}

	// Post-degrade: only 1 concurrent acquire succeeds.
	if err := l.Acquire(context.Background()); err != nil {
		t.Fatalf("post-degrade Acquire 1: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := l.Acquire(ctx); err != context.DeadlineExceeded {
		t.Errorf("post-degrade Acquire 2 = %v, want DeadlineExceeded", err)
	}
}

func TestLimiter_Degrade_IsIdempotent(t *testing.T) {
	l := NewLimiter()
	l.Degrade()
	l.Degrade()
	l.Degrade()
	if !l.IsDegraded() {
		t.Error("Degrade should be idempotent")
	}
}

func TestLimiter_Acquire_RespectsContextCancellation(t *testing.T) {
	l := NewLimiter()
	// Fill all 3 normal slots.
	for i := 0; i < HTTPMaxParallel; i++ {
		_ = l.Acquire(context.Background())
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := l.Acquire(ctx)
	if err != context.Canceled {
		t.Errorf("Acquire = %v, want context.Canceled", err)
	}
}

// TestLimiter_ConcurrentAcquires_NeverExceedCap is the load-bearing test:
// verifies that at most HTTPMaxParallel goroutines are ever inside the
// critical section simultaneously. A regression that bumps the cap to 8
// would fail here.
func TestLimiter_ConcurrentAcquires_NeverExceedCap(t *testing.T) {
	l := NewLimiter()
	const N = 50
	var inFlight, peak int32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := l.Acquire(ctx); err != nil {
				return
			}
			defer l.Release()

			cur := atomic.AddInt32(&inFlight, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
		}()
	}
	wg.Wait()

	if peak > int32(HTTPMaxParallel) {
		t.Errorf("peak concurrency = %d, want <= %d", peak, HTTPMaxParallel)
	}
}

func TestLimiter_Degrade_TracksPeakAfterDegraded(t *testing.T) {
	l := NewLimiter()
	l.Degrade()

	var inFlight, peak int32
	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			if err := l.Acquire(context.Background()); err != nil {
				return
			}
			defer l.Release()
			cur := atomic.AddInt32(&inFlight, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
		}()
	}
	wg.Wait()

	if peak > int32(HTTPDegradedParallel) {
		t.Errorf("peak after degrade = %d, want <= %d", peak, HTTPDegradedParallel)
	}
}
