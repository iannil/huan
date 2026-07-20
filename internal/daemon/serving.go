package daemon

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

// ServingOptions configures the Serving layer.
type ServingOptions struct {
	OutputDir      string
	Bind           string
	Port           string
	TLSCert        string
	TLSKey         string
	AdminHandler   http.Handler
	JITCache       *cache.JITCache
	Builder        *Builder
	Bus            eventbus.EventBus
	Logf           func(format string, args ...any)
	Health         *HealthChecker
	Metrics        *MetricsCollector
}

// Serving manages the HTTP server, static file serving, JIT rendering, and admin API.
type Serving struct {
	opts    ServingOptions
	httpSrv *http.Server
}

// NewServing creates a new Serving instance.
func NewServing(opts ServingOptions) *Serving {
	return &Serving{opts: opts}
}

// Start begins the HTTP server and blocks.
func (s *Serving) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	if s.opts.Health != nil {
		mux.Handle("/health", s.opts.Health.Handler())
	} else {
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","uptime":"0s"}`))
		})
	}

	// Metrics
	if s.opts.Metrics != nil {
		mux.Handle("/metrics", s.opts.Metrics.Handler())
	}

	// Admin
	if s.opts.AdminHandler != nil {
		mux.Handle("/admin/", s.opts.AdminHandler)
	}

	// Static file server with JIT fallback
	fileServer := http.FileServer(http.Dir(s.opts.OutputDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if pathResolvesToFile(s.opts.OutputDir, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		// JIT fallback: check cache then render
		s.jitFallback(w, r)
	})

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", s.opts.Bind, s.opts.Port),
		Handler: mux,
	}

	s.opts.Logf("serving: listening on %s", s.httpSrv.Addr)

	if s.opts.TLSCert != "" && s.opts.TLSKey != "" {
		return s.httpSrv.ListenAndServeTLS(s.opts.TLSCert, s.opts.TLSKey)
	}
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Serving) Shutdown(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// HandleCacheUpdated is called when the cache is updated.
func (s *Serving) HandleCacheUpdated(ctx context.Context, event eventbus.Event) error {
	s.opts.Logf("serving: cache updated")
	return nil
}

// jitFallback handles JIT rendering for paths not found in the static output.
func (s *Serving) jitFallback(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Check JIT cache
	if entry := s.opts.JITCache.Get(path); entry != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Huan-Cache", "jit")
		_, _ = w.Write(entry.HTML)
		return
	}

	// JIT render via Builder
	if s.opts.Builder != nil {
		html, err := s.opts.Builder.RenderPageJIT(r.Context(), path)
		if err != nil {
			s.opts.Logf("serving: JIT render failed for %s: %v", path, err)
			http.NotFound(w, r)
			return
		}

		// Cache the result
		s.opts.JITCache.Set(path, &cache.JITEntry{
			Path:       path,
			HTML:       []byte(html),
			RenderedAt: time.Now(),
			TTL:        5 * time.Minute,
		})

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Huan-Cache", "jit-hit")
		_, _ = w.Write([]byte(html))
		return
	}

	http.NotFound(w, r)
}

// pathResolvesToFile reports whether a request for urlPath under outputDir
// would be served as a real file. Copied from internal/dev/server.go.
func pathResolvesToFile(outputDir, urlPath string) bool {
	if !strings.HasPrefix(urlPath, "/") {
		return false
	}
	clean := path.Clean(urlPath)
	if clean == "/" {
		clean = "."
	} else {
		clean = strings.TrimPrefix(clean, "/")
	}
	if strings.HasPrefix(clean, "..") || clean == ".." {
		return false
	}
	fs := http.Dir(outputDir)
	f, err := fs.Open(clean)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return true
	}
	idx, err := fs.Open(path.Join(clean, "index.html"))
	if err != nil {
		return false
	}
	idx.Close()
	return true
}