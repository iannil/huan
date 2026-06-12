package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// LiveReloadHub manages WebSocket connections from browser clients running livereload.js.
// It broadcasts reload and alert messages to all connected clients.
type LiveReloadHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
	logf    func(format string, args ...any)
}

func NewHub() *LiveReloadHub {
	return &LiveReloadHub{
		clients: map[*websocket.Conn]struct{}{},
		logf:    func(string, ...any) {},
	}
}

// HandleConn runs the WebSocket read loop for one client. Blocks until
// the client disconnects or ctx is cancelled. Sends the hello message
// immediately on connect.
func (h *LiveReloadHub) HandleConn(ctx context.Context, c *websocket.Conn) {
	h.add(c)
	defer h.remove(c)

	hello := map[string]any{
		"command":    "hello",
		"protocols":  []string{"http://livereload.com/protocols/official-7"},
		"serverName": "huan",
	}
	_ = h.writeJSON(ctx, c, hello)

	// Read loop: client may send hello back or info messages. We don't
	// care about the content; we just need to detect disconnect.
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			return
		}
	}
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

func (h *LiveReloadHub) broadcast(msg map[string]any) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_ = c.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}

func (h *LiveReloadHub) writeJSON(ctx context.Context, c *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, data)
}

func (h *LiveReloadHub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *LiveReloadHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = c.CloseNow()
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
