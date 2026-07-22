package sse

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleSubscribe_SSEHeaders(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	// Run handler in background; it blocks until client disconnect.
	done := make(chan struct{})
	go func() {
		h.HandleSubscribe(rec, req)
		close(done)
	}()

	// Wait for registration: poll ClientCount so we don't race the handler.
	waitForClientCount(t, h, 1, time.Second)

	// Force handler exit by closing all client channels (simulates shutdown).
	// Once the handler returns, reading rec.Header() is safe.
	h.mu.Lock()
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
	h.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit after channel close")
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestHandleSubscribe_EventFormat(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.HandleSubscribe(rec, req)
		close(done)
	}()

	// Wait for registration, then broadcast.
	waitForClientCount(t, h, 1, time.Second)
	h.Broadcast(Event{Type: "build_completed", Data: map[string]int{"pages": 3}})

	// Close all client channels so the handler's read returns and it exits.
	// Then reading rec.Body is safe — no concurrent writer.
	h.mu.Lock()
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
	h.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit after channel close")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: build_completed") {
		t.Errorf("body missing event line:\n%s", body)
	}
	if !strings.Contains(body, `"pages":3`) {
		t.Errorf("body missing data:\n%s", body)
	}
}

// waitForClientCount polls the hub until the client count matches n or the
// deadline elapses. Avoids time.Sleep-based races between handler start and
// the test asserting on shared state.
func waitForClientCount(t *testing.T, h *SSEHub, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.ClientCount() == n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("client count = %d, want %d", h.ClientCount(), n)
}

func TestHandleSubscribe_MaxClientsRejected(t *testing.T) {
	h := NewSSEHub(testLogf(t))
	// Artificially fill to maxClients.
	for i := 0; i < maxClients; i++ {
		h.registerClient()
	}

	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Code = %d, want %d (max clients)", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleSubscribe_ReadsStream(t *testing.T) {
	// End-to-end: real httptest.Server, read SSE stream via bufio.Scanner.
	// ResponseRecorder doesn't support streaming flush reliably, so we use a
	// real server which honors http.Flusher.
	h := NewSSEHub(testLogf(t))
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Start(ctx)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Broadcast an event.
	time.Sleep(50 * time.Millisecond) // let client register
	h.Broadcast(Event{Type: "content_changed", Data: "hello"})

	scanner := bufio.NewScanner(resp.Body)
	got := false
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for SSE event")
		default:
		}
		if !scanner.Scan() {
			break
		}
		if strings.Contains(scanner.Text(), "content_changed") {
			got = true
		}
		if got && scanner.Text() == "" {
			break // end of event block (blank line)
		}
	}
	if !got {
		t.Error("did not see content_changed in stream")
	}
}
