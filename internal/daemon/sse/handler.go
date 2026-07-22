package sse

import (
	"context"
	"fmt"
	"net/http"
)

// Start launches the heartbeat goroutine. Call once at daemon startup
// (calling more than once is safe but redundant — each call spawns an
// additional heartbeat loop). Cancelling ctx stops the goroutine.
func (h *SSEHub) Start(ctx context.Context) {
	go h.startHeartbeat(ctx)
}

// ServeHTTP makes *SSEHub an http.Handler, routing to HandleSubscribe.
// This lets callers pass the hub directly to http.NewServeMux or
// httptest.NewServer without an adapter.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.HandleSubscribe(w, r)
}

// HandleSubscribe is the HTTP handler for GET /api/v1/events.
//
// It validates the request, registers a new client with the hub, then loops
// reading events from the client's channel and writing them as SSE wire text
// to the response. The loop exits when the client disconnects (r.Context is
// cancelled), the channel is closed (shutdown), or a write fails.
//
// registerClient returns nil when maxClients has been reached; in that case
// the connection is refused with 503.
func (h *SSEHub) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Register the client first so we know whether maxClients is exhausted.
	// registerClient returns nil when the connection cap is reached.
	ch := h.registerClient()
	if ch == nil {
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	defer h.unregisterClient(ch)

	// SSE response headers. These must be set before WriteHeader/first write.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			return
		case ev, ok := <-ch:
			if !ok {
				// Channel closed (e.g. shutdown).
				return
			}
			line, err := encodeEvent(ev)
			if err != nil {
				h.logf("sse: encode error: %v", err)
				continue
			}
			if _, err := fmt.Fprint(w, line); err != nil {
				// Write failed (client gone).
				return
			}
			flusher.Flush()
		}
	}
}
