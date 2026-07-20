package daemon

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

// ServingOptions configures the Serving layer.
type ServingOptions struct {
	OutputDir    string
	Bind         string
	Port         string
	AdminHandler http.Handler
	JITCache     *cache.JITCache
	Builder      *Builder
	Bus          eventbus.EventBus
	Logf         func(format string, args ...any)
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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","uptime":"0s"}`))
	})

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

	// TODO: Phase 2 — real JIT rendering via build.RenderPage()
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