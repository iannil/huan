package serve

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func mustListenFreePort(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func httpServeWS(ctx context.Context, ln net.Listener, hub *LiveReloadHub) {
	mux := http.NewServeMux()
	mux.HandleFunc("/livereload", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // dev server: allow ws:// (not wss://) and any origin
		})
		if err != nil {
			return
		}
		defer c.CloseNow()
		hub.HandleConn(ctx, c)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	<-ctx.Done()
	srv.Shutdown(context.Background()) //nolint:errcheck
}

func TestLiveReloadHubHandshakeAndReload(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln := mustListenFreePort(t)
	go httpServeWS(ctx, ln, hub)

	// Connect a client
	c, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	c.SetReadLimit(1 << 16)

	// Expect hello from server first
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !strings.Contains(string(msg), `"hello"`) {
		t.Errorf("first msg = %s, want contains 'hello'", string(msg))
	}
	if !strings.Contains(string(msg), "official-7") {
		t.Errorf("hello missing protocol 'official-7': %s", string(msg))
	}

	// Trigger a reload
	hub.BroadcastReload()

	// Expect reload msg
	_, msg, err = c.Read(ctx)
	if err != nil {
		t.Fatalf("read reload: %v", err)
	}
	if !strings.Contains(string(msg), `"reload"`) {
		t.Errorf("second msg = %s, want 'reload'", string(msg))
	}
}

func TestLiveReloadHubAlert(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln := mustListenFreePort(t)
	go httpServeWS(ctx, ln, hub)

	c, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	c.SetReadLimit(1 << 16)

	// Read hello first
	_, _, _ = c.Read(ctx)

	// Trigger an alert
	hub.BroadcastAlert("something went wrong")

	// Expect alert msg
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read alert: %v", err)
	}
	if !strings.Contains(string(msg), `"alert"`) {
		t.Errorf("msg = %s, want 'alert'", string(msg))
	}
	if !strings.Contains(string(msg), "something went wrong") {
		t.Errorf("alert missing message: %s", string(msg))
	}
}

func TestLiveReloadHubHandlesClientDisconnect(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln := mustListenFreePort(t)
	go httpServeWS(ctx, ln, hub)

	c, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	c.SetReadLimit(1 << 16)
	_, _, _ = c.Read(ctx) // read hello

	// Verify hub has 1 client
	if got := hub.ClientCount(); got != 1 {
		t.Errorf("after connect, client count = %d, want 1", got)
	}

	// Client disconnects
	c.Close(websocket.StatusNormalClosure, "bye")

	// Give server time to detect disconnect
	time.Sleep(100 * time.Millisecond)

	if got := hub.ClientCount(); got != 0 {
		t.Errorf("after disconnect, client count = %d, want 0", got)
	}
}

// TestLiveReloadHubParallelBroadcasts verifies that rapid-fire broadcasts
// don't race on a single conn (coder/websocket requires one writer at a time
// per conn). The per-client write goroutine serializes writes, so all 10
// reloads must arrive cleanly.
func TestLiveReloadHubParallelBroadcasts(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln := mustListenFreePort(t)
	go httpServeWS(ctx, ln, hub)

	c, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	c.SetReadLimit(1 << 16)
	_, _, _ = c.Read(ctx) // hello

	// Fire 10 reloads back-to-back.
	for i := 0; i < 10; i++ {
		hub.BroadcastReload()
	}

	// Expect all 10 to arrive within a reasonable window.
	for i := 0; i < 10; i++ {
		readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
		_, _, err := c.Read(readCtx)
		readCancel()
		if err != nil {
			t.Fatalf("read %d/10 failed: %v", i+1, err)
		}
	}
}

// TestLiveReloadHubMultiClientBroadcastParallel verifies that broadcasts
// fan out to N clients concurrently (worst-case latency ≈ per-client timeout,
// not N × per-client timeout).
func TestLiveReloadHubMultiClientBroadcastParallel(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln := mustListenFreePort(t)
	go httpServeWS(ctx, ln, hub)

	const numClients = 5
	var clients []*websocket.Conn
	for i := 0; i < numClients; i++ {
		c, _, err := websocket.Dial(ctx, "ws://"+ln.Addr().String()+"/livereload", nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		c.SetReadLimit(1 << 16)
		_, _, _ = c.Read(ctx) // hello
		clients = append(clients, c)
	}
	defer func() {
		for _, c := range clients {
			c.CloseNow()
		}
	}()

	if got := hub.ClientCount(); got != numClients {
		t.Fatalf("hub has %d clients, want %d", got, numClients)
	}

	// Broadcast and measure how long until all clients receive it.
	start := time.Now()
	hub.BroadcastReload()
	for _, c := range clients {
		readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
		_, _, err := c.Read(readCtx)
		readCancel()
		if err != nil {
			t.Fatalf("client read: %v", err)
		}
	}
	elapsed := time.Since(start)

	// Sequential write with 100ms timeout per client would worst-case at
	// ~500ms for 5 clients. Parallel should be well under that. Use 300ms
	// as the bound to avoid CI flakiness.
	if elapsed > 300*time.Millisecond {
		t.Errorf("broadcast to %d clients took %v, want < 300ms (parallel)", numClients, elapsed)
	}
}
