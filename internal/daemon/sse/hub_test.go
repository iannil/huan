package sse

import (
	"context"
	"testing"
	"time"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

func TestBroadcast_DeliversToClient(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()

	h.Broadcast(Event{Type: "build_completed", Data: map[string]int{"pages": 10}})

	select {
	case ev := <-ch:
		if ev.Type != "build_completed" {
			t.Errorf("Type = %q, want build_completed", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not receive event")
	}
}

func TestBroadcast_MultipleClients(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch1 := h.registerClient()
	ch2 := h.registerClient()

	h.Broadcast(Event{Type: "content_changed", Data: nil})

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("client %d did not receive event", i)
		}
	}
}

func TestBroadcast_SlowClientDrops(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()
	// Fill the buffer (clientBufferSize events).
	for i := 0; i < clientBufferSize; i++ {
		h.Broadcast(Event{Type: "build_completed"})
	}
	// One more should be dropped (non-blocking) — client never reads.
	h.Broadcast(Event{Type: "build_completed"})
	// Drain to verify buffer not exceeded.
	count := 0
	draining := true
	for draining {
		select {
		case <-ch:
			count++
		default:
			draining = false
		}
	}
	if count != clientBufferSize {
		t.Errorf("received %d, want %d (overflow dropped)", count, clientBufferSize)
	}
}

func TestClientCount(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	if h.ClientCount() != 0 {
		t.Fatalf("initial count = %d, want 0", h.ClientCount())
	}
	ch := h.registerClient()
	if h.ClientCount() != 1 {
		t.Errorf("after register = %d, want 1", h.ClientCount())
	}
	h.unregisterClient(ch)
	if h.ClientCount() != 0 {
		t.Errorf("after unregister = %d, want 0", h.ClientCount())
	}
}

func TestBroadcastRaw_HeartbeatComment(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()
	h.broadcastRaw(":heartbeat\n\n")
	select {
	case ev := <-ch:
		// Raw broadcast is delivered as an Event with empty Type and the raw line in Data.
		if ev.Type != "" {
			t.Errorf("raw event Type = %q, want empty", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not receive raw broadcast")
	}
}

func TestEncodeEvent_StructuredWith_Type(t *testing.T) {
	out, err := encodeEvent(Event{Type: "build_completed", Data: map[string]int{"pages": 10}})
	if err != nil {
		t.Fatalf("encodeEvent error: %v", err)
	}
	want := "event: build_completed\ndata: {\"pages\":10}\n\n"
	if out != want {
		t.Errorf("encodeEvent = %q, want %q", out, want)
	}
}

func TestEncodeEvent_StructuredWithout_Type(t *testing.T) {
	out, err := encodeEvent(Event{Data: "hello"})
	if err != nil {
		t.Fatalf("encodeEvent error: %v", err)
	}
	want := "data: \"hello\"\n\n"
	if out != want {
		t.Errorf("encodeEvent = %q, want %q", out, want)
	}
}

func TestEncodeEvent_RawVerbatim(t *testing.T) {
	raw := ":heartbeat\n\n"
	out, err := encodeEvent(Event{Data: raw, raw: true})
	if err != nil {
		t.Fatalf("encodeEvent error: %v", err)
	}
	if out != raw {
		t.Errorf("encodeEvent raw = %q, want %q", out, raw)
	}
}

func TestStartHeartbeat_Broadcasts(t *testing.T) {
	// Use a hub with a tiny heartbeat by validating the public contract:
	// a registered client receives a raw heartbeat while startHeartbeat runs.
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.startHeartbeat(ctx)

	// Wait up to heartbeatInterval + slack for the first tick.
	select {
	case ev := <-ch:
		if ev.Type != "" {
			t.Errorf("heartbeat Type = %q, want empty", ev.Type)
		}
		s, _ := ev.Data.(string)
		if s != ":heartbeat\n\n" {
			t.Errorf("heartbeat Data = %q, want %q", s, ":heartbeat\n\n")
		}
	case <-time.After(heartbeatInterval + 5*time.Second):
		t.Fatal("did not receive heartbeat within interval")
	}
}

func TestNewSSEHub_NilLogf_NoPanic(t *testing.T) {
	h := NewSSEHub(nil)
	ch := h.registerClient()
	// Should not panic on Broadcast drop-path even with nil-derived logf.
	for i := 0; i < clientBufferSize+1; i++ {
		h.Broadcast(Event{Type: "build_completed"})
	}
	h.unregisterClient(ch)
	if h.ClientCount() != 0 {
		t.Errorf("after unregister = %d, want 0", h.ClientCount())
	}
}

func TestSubscribeBus_Bridges(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	ch := h.registerClient()
	bus := eventbus.NewChannelBus()
	defer bus.Close()

	h.SubscribeBus(bus)

	// Publish a build_completed event.
	_ = bus.Publish(context.Background(), eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload:   map[string]int{"pages": 5},
	})

	select {
	case ev := <-ch:
		if ev.Type != "build_completed" {
			t.Errorf("Type = %q, want build_completed", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive bridged event")
	}
}

func testLogf(t *testing.T) func(string, ...any) {
	t.Helper()
	return func(format string, args ...any) { t.Logf(format, args...) }
}
