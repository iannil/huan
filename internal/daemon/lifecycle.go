package daemon

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// SystemdNotifier integrates with systemd's sd_notify protocol.
// Phase 1: basic support. Phase 2: full sd_notify implementation.
type SystemdNotifier struct {
	enabled bool
}

// NewSystemdNotifier creates a SystemdNotifier.
func NewSystemdNotifier(enabled bool) *SystemdNotifier {
	return &SystemdNotifier{enabled: enabled}
}

// Notify sends a notification to systemd.
func (n *SystemdNotifier) Notify(msg string) {
	if !n.enabled {
		return
	}
	// TODO: Phase 2 — implement sd_notify via socket
	log.Printf("systemd: %s", msg)
}

// WaitForShutdown returns a channel that fires on SIGINT/SIGTERM.
func WaitForShutdown() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	return ch
}