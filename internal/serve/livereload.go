package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// perClientSendBufferSize bounds how many queued messages a single browser
// client can hold. Beyond this, broadcasts drop (slow-client protection).
const perClientSendBufferSize = 16

// writeTimeout is the per-write deadline. A slow client that blocks longer
// than this is disconnected.
const writeTimeout = 100 * time.Millisecond

// LiveReloadHub manages WebSocket connections from browser clients running
// livereload.js. It broadcasts reload and alert messages to all clients.
//
// Each client has its own writer goroutine that serializes writes — required
// because coder/websocket forbids concurrent calls to Conn.Write per conn.
// Broadcasts are fan-out to per-client send channels, so slow clients don't
// block other clients or the broadcaster.
type LiveReloadHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]chan []byte
	logf    func(format string, args ...any)
}

func NewHub() *LiveReloadHub {
	return &LiveReloadHub{
		clients: map[*websocket.Conn]chan []byte{},
		logf:    func(string, ...any) {},
	}
}

// HandleConn runs the WebSocket lifecycle for one client: starts a writer
// goroutine, sends the hello message, then runs the read loop until the
// client disconnects or ctx is cancelled. Blocks until disconnect.
func (h *LiveReloadHub) HandleConn(ctx context.Context, c *websocket.Conn) {
	send := make(chan []byte, perClientSendBufferSize)
	h.add(c, send)
	defer h.remove(c)

	hello, _ := json.Marshal(map[string]any{
		"command":    "hello",
		"protocols":  []string{"http://livereload.com/protocols/official-7"},
		"serverName": "huan",
	})

	// Writer goroutine: drains `send` and writes serially to the conn.
	// coder/websocket requires one writer at a time per conn; this goroutine
	// is the only thing that calls c.Write.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		// Send hello first (priority).
		wctx, cancel := context.WithTimeout(ctx, writeTimeout)
		err := c.Write(wctx, websocket.MessageText, hello)
		cancel()
		if err != nil {
			return
		}
		for msg := range send {
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}()

	// Read loop: detect disconnect. We don't care about incoming content
	// (livereload.js sends hello/info but we ignore it).
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			break
		}
	}
	close(send)     // signal writer to drain and exit
	<-writerDone    // wait for writer to finish (or have already exited)
}

// BroadcastReload notifies all clients to refresh the page.
func (h *LiveReloadHub) BroadcastReload() {
	h.broadcast(map[string]any{
		"command": "reload",
		"path":    "/",
		"liveCSS": true,
	})
}

// BroadcastAlert shows an alert popup in connected browsers (used for build errors).
func (h *LiveReloadHub) BroadcastAlert(message string) {
	h.broadcast(map[string]any{
		"command": "alert",
		"message": message,
	})
}

// ClientCount returns the current number of connected clients (useful for tests/logging).
func (h *LiveReloadHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// broadcast fans msg out to all clients' send channels. Non-blocking per
// client: a full queue means the client is behind, so we drop the message
// rather than block the broadcaster.
func (h *LiveReloadHub) broadcast(msg map[string]any) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	chans := make([]chan []byte, 0, len(h.clients))
	for _, ch := range h.clients {
		chans = append(chans, ch)
	}
	h.mu.RUnlock()

	for _, ch := range chans {
		select {
		case ch <- data:
		default:
			// Queue full — drop. The client will see the next reload instead.
		}
	}
}

func (h *LiveReloadHub) add(c *websocket.Conn, send chan []byte) {
	h.mu.Lock()
	h.clients[c] = send
	h.mu.Unlock()
}

func (h *LiveReloadHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	send, ok := h.clients[c]
	if ok {
		delete(h.clients, c)
	}
	h.mu.Unlock()
	if ok {
		// Closing send tells the writer goroutine to drain and exit. If the
		// writer already exited (write error), this is a no-op on a closed
		// channel — guarded by recover in case of a race.
		closeSafe(send)
	}
	_ = c.CloseNow()
}

// closeSafe closes a channel, recovering from the rare race where another
// goroutine already closed it. We don't expect this in normal flow, but
// defensive code is cheap here.
func closeSafe(ch chan []byte) {
	defer func() { _ = recover() }()
	close(ch)
}

// AcceptHTTP upgrades an HTTP request to a WebSocket and hands it to the hub.
// Convenience for wiring into a stdlib mux.
func (h *LiveReloadHub) AcceptHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow ws:// and any origin (dev server only)
	})
	if err != nil {
		return
	}
	defer c.CloseNow()
	h.HandleConn(r.Context(), c)
}
