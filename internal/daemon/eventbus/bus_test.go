package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		received.Add(1)
		return nil
	})

	err := bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Give async handler time to run
	time.Sleep(50 * time.Millisecond)
	if got := received.Load(); got != 1 {
		t.Errorf("expected 1 handler call, got %d", got)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	var received atomic.Int32
	id := bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		received.Add(1)
		return nil
	})
	bus.Unsubscribe(EventContentChanged, id)

	_ = bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	if got := received.Load(); got != 0 {
		t.Errorf("expected 0 handler calls after unsubscribe, got %d", got)
	}
}

func TestEventBus_CloseBlockPublish(t *testing.T) {
	bus := NewChannelBus()
	bus.Close()
	err := bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	if err == nil {
		t.Error("expected error publishing on closed bus")
	}
}

func TestEventBus_HandlerTimeout(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	done := make(chan struct{})
	bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		// Simulate a handler that never completes — timeout should fire
		<-ctx.Done()
		close(done)
		return ctx.Err()
	})

	_ = bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	select {
	case <-done:
		// handler timed out as expected
	case <-time.After(handlerTimeout + 2*time.Second):
		t.Fatal("handler did not timeout within expected window")
	}
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	var count1, count2 atomic.Int32
	bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		count1.Add(1)
		return nil
	})
	bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		count2.Add(1)
		return nil
	})

	_ = bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	if count1.Load() != 1 || count2.Load() != 1 {
		t.Errorf("expected both handlers to fire: %d, %d", count1.Load(), count2.Load())
	}
}