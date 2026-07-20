package eventbus

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// handlerTimeout is the default max duration for a single handler to run.
const handlerTimeout = 30 * time.Second

// EventBus defines the interface for publishing and subscribing to events.
type EventBus interface {
	// Publish dispatches an event to all subscribed handlers asynchronously.
	Publish(ctx context.Context, event Event) error
	// Subscribe registers a handler for the given event type. Returns a handler
	// ID that can be used to unsubscribe.
	Subscribe(eventType EventType, handler Handler) string
	// Unsubscribe removes a handler registered with the given ID.
	Unsubscribe(eventType EventType, handlerID string)
	// Close shuts down the bus and releases resources.
	Close() error
}

// handlerEntry wraps a Handler with its ID for unsubscribe support.
type handlerEntry struct {
	id      string
	handler Handler
}

// ChannelBus is the default EventBus implementation using Go channels.
// Each event type fans out to its subscribers in separate goroutines.
type ChannelBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]handlerEntry
	closed   bool
	seq      uint64
	logf     func(format string, args ...any)
}

// NewChannelBus creates a new ChannelBus.
func NewChannelBus() *ChannelBus {
	return &ChannelBus{
		handlers: make(map[EventType][]handlerEntry),
		logf:     log.Printf,
	}
}

// Publish dispatches event to all subscribers of its type.
func (b *ChannelBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return fmt.Errorf("eventbus: closed")
	}
	entries := b.handlers[event.Type]
	for _, entry := range entries {
		h := entry.handler // capture for closure
		go func() {
			hctx, cancel := context.WithTimeout(ctx, handlerTimeout)
			defer cancel()
			if err := h(hctx, event); err != nil {
				b.logf("eventbus: handler %s error: %v", entry.id, err)
			}
		}()
	}
	return nil
}

// Subscribe registers a handler for eventType. Returns a unique handler ID.
func (b *ChannelBus) Subscribe(eventType EventType, handler Handler) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := fmt.Sprintf("h%d", b.seq)
	b.handlers[eventType] = append(b.handlers[eventType], handlerEntry{id: id, handler: handler})
	return id
}

// Unsubscribe removes a handler by ID.
func (b *ChannelBus) Unsubscribe(eventType EventType, handlerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries := b.handlers[eventType]
	for i, e := range entries {
		if e.id == handlerID {
			b.handlers[eventType] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// Close marks the bus as closed. Future Publish calls return an error.
func (b *ChannelBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.handlers = nil
	return nil
}