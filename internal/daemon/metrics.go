package daemon

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsCollector tracks build and request metrics via Prometheus.
type MetricsCollector struct {
	buildCount      prometheus.Counter
	buildDuration   prometheus.Histogram
	buildFailures   prometheus.Counter
	requestCount    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	cacheHits       prometheus.Counter
	cacheMisses     prometheus.Counter
	reg             *prometheus.Registry
}

// NewMetricsCollector creates a MetricsCollector with registered Prometheus metrics.
func NewMetricsCollector() *MetricsCollector {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	m := &MetricsCollector{
		buildCount: factory.NewCounter(prometheus.CounterOpts{
			Name: "huan_build_total",
			Help: "Total number of builds.",
		}),
		buildDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "huan_build_duration_seconds",
			Help:    "Build duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		buildFailures: factory.NewCounter(prometheus.CounterOpts{
			Name: "huan_build_failures_total",
			Help: "Total number of failed builds.",
		}),
		requestCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "huan_http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "path"}),
		requestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "huan_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
		cacheHits: factory.NewCounter(prometheus.CounterOpts{
			Name: "huan_cache_hits_total",
			Help: "Total number of JIT cache hits.",
		}),
		cacheMisses: factory.NewCounter(prometheus.CounterOpts{
			Name: "huan_cache_misses_total",
			Help: "Total number of JIT cache misses.",
		}),
		reg: reg,
	}
	return m
}

// RecordBuild records a successful build.
func (m *MetricsCollector) RecordBuild(duration time.Duration) {
	m.buildCount.Inc()
	m.buildDuration.Observe(duration.Seconds())
}

// RecordBuildFailure records a failed build.
func (m *MetricsCollector) RecordBuildFailure() {
	m.buildFailures.Inc()
}

// RecordRequest records an HTTP request.
func (m *MetricsCollector) RecordRequest(method, path string, duration time.Duration) {
	m.requestCount.WithLabelValues(method, path).Inc()
	m.requestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordCacheHit records a JIT cache hit.
func (m *MetricsCollector) RecordCacheHit() {
	m.cacheHits.Inc()
}

// RecordCacheMiss records a JIT cache miss.
func (m *MetricsCollector) RecordCacheMiss() {
	m.cacheMisses.Inc()
}

// Handler returns the Prometheus metrics endpoint.
func (m *MetricsCollector) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}