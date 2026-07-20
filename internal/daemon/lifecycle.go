package daemon

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// SystemdNotifier integrates with systemd's sd_notify protocol.
// Phase 1: basic support. Phase 2: full sd_notify implementation.
type SystemdNotifier struct {
	enabled bool
	socket  string
}

// NewSystemdNotifier creates a SystemdNotifier.
// If enabled is true and NOTIFY_SOCKET env var is set, notifications are sent.
func NewSystemdNotifier(enabled bool) *SystemdNotifier {
	socket := os.Getenv("NOTIFY_SOCKET")
	return &SystemdNotifier{
		enabled: enabled && socket != "",
		socket:  socket,
	}
}

// Ready sends READY=1 to systemd.
func (n *SystemdNotifier) Ready() {
	if !n.enabled {
		return
	}
	n.send("READY=1")
}

// Stopping sends STOPPING=1 to systemd.
func (n *SystemdNotifier) Stopping() {
	if !n.enabled {
		return
	}
	n.send("STOPPING=1")
}

// Status sends a status message to systemd.
func (n *SystemdNotifier) Status(msg string) {
	if !n.enabled {
		return
	}
	n.send(fmt.Sprintf("STATUS=%s", msg))
}

// send sends a message via Unix datagram socket to systemd.
func (n *SystemdNotifier) send(msg string) {
	if n.socket == "" {
		return
	}
	addr := &net.UnixAddr{Name: n.socket, Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(msg))
}

// WaitForShutdown returns a channel that fires on SIGINT/SIGTERM.
func WaitForShutdown() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}
