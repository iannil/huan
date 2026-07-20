package daemon

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// HealthChecker provides health check endpoint.
type HealthChecker struct {
	startTime time.Time
	ready     atomic.Bool
}

// NewHealthChecker creates a HealthChecker.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		startTime: time.Now(),
	}
}

// SetReady marks the daemon as ready to serve traffic.
func (h *HealthChecker) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Handler returns the HTTP handler for health checks.
func (h *HealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ready.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"uptime":  time.Since(h.startTime).String(),
			"version": "0.6.0",
		})
	}
}