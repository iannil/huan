// Package sse provides Server-Sent Events (SSE) push for the daemon.
// SSEHub subscribes to daemon EventBus events and broadcasts them to all
// connected browser clients via the /api/v1/events endpoint.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

const (
	// clientBufferSize is the per-client event buffer. A slow client that
	// falls behind this many events has new events dropped (non-blocking).
	clientBufferSize = 16
	// maxClients caps concurrent SSE connections to prevent resource exhaustion.
	maxClients = 1000
	// heartbeatInterval is how often a keep-alive comment line is sent.
	heartbeatInterval = 15 * time.Second
)

// Event is a single SSE message sent to clients.
type Event struct {
	// Type is the SSE event name (e.g. "build_completed"). Empty for raw
	// comment lines (heartbeats) — see broadcastRaw.
	Type string
	// Data is the event payload (JSON-marshaled on the wire). For raw
	// broadcasts this holds the literal wire text.
	Data any
	// raw, when true, means Data is already-formatted wire text written
	// verbatim (used by heartbeats).
	raw bool
}

// SSEHub manages SSE client connections and broadcasts daemon events.
// Thread-safe. Slow clients do not block fast clients.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
	logf    func(format string, args ...any)
}

// NewSSEHub creates an empty hub. If logf is nil, a no-op logger is used.
func NewSSEHub(logf func(string, ...any)) *SSEHub {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &SSEHub{
		clients: map[chan Event]struct{}{},
		logf:    logf,
	}
}

// registerClient adds a new buffered client channel and returns it.
// The caller must eventually call unregisterClient to avoid leaks.
func (h *SSEHub) registerClient() chan Event {
	ch := make(chan Event, clientBufferSize)
	h.mu.Lock()
	// Enforce the connection cap: if we are at maxClients, refuse the new
	// connection by returning a nil channel. Callers must check for nil.
	if len(h.clients) >= maxClients {
		h.mu.Unlock()
		h.logf("sse: maxClients %d reached, refusing new client", maxClients)
		return nil
	}
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// unregisterClient removes a client channel and drains it.
func (h *SSEHub) unregisterClient(ch chan Event) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// ClientCount returns the number of connected clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends an event to all clients. A client whose buffer is full
// (slow consumer) has the event dropped — it never blocks other clients.
func (h *SSEHub) Broadcast(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
			h.logf("sse: client buffer full, dropping event %q", event.Type)
		}
	}
}

// broadcastRaw sends already-formatted wire text (e.g. a heartbeat comment
// line) to all clients as a raw Event.
func (h *SSEHub) broadcastRaw(line string) {
	h.Broadcast(Event{Data: line, raw: true})
}

// startHeartbeat periodically broadcasts an SSE comment line to keep
// connections alive through proxies. Runs until ctx is cancelled.
func (h *SSEHub) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.broadcastRaw(":heartbeat\n\n")
		}
	}
}

// encodeEvent formats an Event as SSE wire text. Structured events become
// "event: <type>\ndata: <json>\n\n"; raw events are written verbatim. Events
// with an empty Type (and not raw) become "data: <json>\n\n".
func encodeEvent(ev Event) (string, error) {
	if ev.raw {
		if s, ok := ev.Data.(string); ok {
			return s, nil
		}
		// Non-string raw payloads are a programming error; fall through and
		// marshal them so callers still get deterministic output.
	}
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return "", fmt.Errorf("sse: marshal event data: %w", err)
	}
	if ev.Type == "" {
		return fmt.Sprintf("data: %s\n\n", data), nil
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", ev.Type, data), nil
}

// SubscribeBus wires the hub to the daemon EventBus: every build/content/
// plugin event is forwarded to all SSE clients. Call once at daemon startup.
func (h *SSEHub) SubscribeBus(bus eventbus.EventBus) {
	for _, eventType := range []eventbus.EventType{
		eventbus.EventBuildCompleted,
		eventbus.EventBuildFailed,
		eventbus.EventContentChanged,
		eventbus.EventPluginLoaded,
		eventbus.EventPluginUnloaded,
	} {
		bus.Subscribe(eventType, func(_ context.Context, ev eventbus.Event) error {
			h.Broadcast(Event{Type: ev.Type.String(), Data: ev.Payload})
			return nil
		})
	}
}
