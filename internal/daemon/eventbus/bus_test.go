package eventbus

import (
	"context"
	"sync"
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

func TestEventType_PluginLifecycleEvents(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		want      string
	}{
		{"EventPluginLoaded", EventPluginLoaded, "plugin_loaded"},
		{"EventPluginUnloaded", EventPluginUnloaded, "plugin_unloaded"},
		{"EventPluginReloaded", EventPluginReloaded, "plugin_reloaded"},
		{"EventPluginError", EventPluginError, "plugin_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.eventType.String(); got != tt.want {
				t.Errorf("%s.String() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestEventBus_PluginLifecycleEventsCanBePublished(t *testing.T) {
	pluginEvents := []EventType{EventPluginLoaded, EventPluginUnloaded, EventPluginReloaded, EventPluginError}

	for _, et := range pluginEvents {
		t.Run(et.String(), func(t *testing.T) {
			bus := NewChannelBus()
			defer bus.Close()

			var received atomic.Int32
			bus.Subscribe(et, func(ctx context.Context, event Event) error {
				received.Add(1)
				return nil
			})

			err := bus.Publish(context.Background(), Event{Type: et, Timestamp: time.Now()})
			if err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			time.Sleep(50 * time.Millisecond)
			if got := received.Load(); got != 1 {
				t.Errorf("expected 1 handler call, got %d", got)
			}
		})
	}
}

func TestPublish_HandlerPanic(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		panic("handler panic")
	})

	// This should not crash the test
	err := bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Give goroutine time to panic
	time.Sleep(50 * time.Millisecond)
}

func TestUnsubscribeNonExistent(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	// Should not panic
	bus.Unsubscribe(EventContentChanged, "nonexistent-id")
	bus.Unsubscribe(EventBuildCompleted, "another-nonexistent")
}

func TestSubscribeAfterClose(t *testing.T) {
	bus := NewChannelBus()
	bus.Close()

	// Should not panic (current behavior allows it)
	bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
		return nil
	})
}

func TestManySubscribers(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	const count = 1000
	var totalReceived atomic.Int32
	for i := 0; i < count; i++ {
		bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
			totalReceived.Add(1)
			return nil
		})
	}

	err := bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if got := totalReceived.Load(); got != int32(count) {
		t.Errorf("expected %d handler calls, got %d", count, got)
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	err := bus.Publish(context.Background(), Event{Type: EventBuildCompleted, Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	// Should not panic or leak goroutines
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	bus := NewChannelBus()
	defer bus.Close()

	const goroutines = 10
	const eventsPerGoroutine = 100
	totalExpected := int32(goroutines * goroutines * eventsPerGoroutine)

	// Use a WaitGroup to wait for all handler goroutines to complete
	var handlerWg sync.WaitGroup
	handlerWg.Add(int(totalExpected))

	var totalReceived atomic.Int32
	for i := 0; i < goroutines; i++ {
		bus.Subscribe(EventContentChanged, func(ctx context.Context, event Event) error {
			totalReceived.Add(1)
			handlerWg.Done()
			return nil
		})
	}

	// Concurrently publish
	var publishWg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		publishWg.Add(1)
		go func() {
			defer publishWg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				_ = bus.Publish(context.Background(), Event{Type: EventContentChanged, Timestamp: time.Now()})
			}
		}()
	}
	publishWg.Wait()

	// Wait for all async handlers to complete instead of time.Sleep
	handlerWg.Wait()

	if got := totalReceived.Load(); got != totalExpected {
		t.Errorf("expected %d handler calls, got %d", totalExpected, got)
	}
}