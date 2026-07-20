package daemon

import (
	"net/http"
	"time"
)

// MetricsCollector tracks build and request metrics.
// Phase 1: basic counters. Phase 2: full Prometheus integration.
type MetricsCollector struct {
	buildCount    int64
	buildDuration time.Duration
	requestCount  int64
}

// NewMetricsCollector creates a MetricsCollector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

// Handler returns a basic metrics endpoint (JSON for now).
// Phase 2: replace with prometheus/client_golang.
func (m *MetricsCollector) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// TODO: Phase 2 — return real Prometheus metrics
		_, _ = w.Write([]byte(`{}`))
	}
}