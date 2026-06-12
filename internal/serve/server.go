package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

// ServerOptions configures a Server.
type ServerOptions struct {
	OutputDir string
	Bind      string
	Port      string // ":0" or "0" makes the OS pick a free port
	Hub       *LiveReloadHub // optional; if nil, /livereload route returns 404
	Logf      func(format string, args ...any)
}

// Server serves the built site over HTTP.
type Server struct {
	opts   ServerOptions
	logf   func(string, ...any)
	addrCh chan string // optional: tests read actual listen addr from here
	hub    *LiveReloadHub
}

func New(opts ServerOptions) *Server {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Server{opts: opts, logf: opts.Logf, hub: opts.Hub}
}

// isPortInUseError returns true if err looks like a "bind: address already in use" error.
// Unfortunately Go's net package doesn't expose this as a typed error, so we check the
// underlying syscall error. On POSIX systems this is EADDRINUSE.
func isPortInUseError(err error) bool {
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		return sysErr.Err == syscall.EADDRINUSE
	}
	return false
}

// Run blocks until ctx is cancelled or a SIGINT/SIGTERM arrives.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(s.opts.Bind, s.opts.Port))
	if err != nil {
		if isPortInUseError(err) {
			return fmt.Errorf("port %s already in use on %s (try --port <other>): %w",
				s.opts.Port, s.opts.Bind, err)
		}
		return fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/livereload.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(livereloadJS)
	})
	if s.hub != nil {
		hub := s.hub // capture for closure
		mux.HandleFunc("/livereload", hub.AcceptHTTP)
	}
	// Static file server with custom 404.html fallback: if the requested
	// path doesn't resolve to a file or directory index, and 404.html
	// exists in OutputDir, serve that with status 404. Mirrors Hugo's
	// behavior.
	fileServer := http.FileServer(http.Dir(s.opts.OutputDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if pathResolvesToFile(s.opts.OutputDir, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveNotFoundHTML(w, r, s.opts.OutputDir)
	})

	httpSrv := &http.Server{Handler: mux}

	if s.addrCh != nil {
		// Non-blocking send: if the test consumer isn't reading (or the channel
		// is unbuffered), drop the address instead of deadlocking Run. Real
		// callers don't use addrCh at all — it's a test hook only.
		select {
		case s.addrCh <- listener.Addr().String():
		default:
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s.logf("Serving at: http://%s/\n", listener.Addr())
		if err := httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logf("serve error: %v\n", err)
		}
	}()

	select {
	case <-ctx.Done():
	case <-sigCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

// pathResolvesToFile reports whether a request for urlPath under outputDir
// would be served as a real file (including directory index resolution).
// Returns false for missing files, missing directory indices, or paths
// that escape outputDir.
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
	// Reject anything that escapes the root (path.Clean already normalizes
	// "..", but be defensive).
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
	// Directory — check for index.html (matches http.FileServer behavior).
	idx, err := fs.Open(path.Join(clean, "index.html"))
	if err != nil {
		return false
	}
	idx.Close()
	return true
}

// serveNotFoundHTML writes a 404 response: if 404.html exists in outputDir,
// serve it; otherwise fall back to Go's default "404 page not found".
func serveNotFoundHTML(w http.ResponseWriter, r *http.Request, outputDir string) {
	fs := http.Dir(outputDir)
	f, err := fs.Open("404.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.Copy(w, rs)
}
