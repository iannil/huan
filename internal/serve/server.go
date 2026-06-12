package serve

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	mux.Handle("/", http.FileServer(http.Dir(s.opts.OutputDir)))

	httpSrv := &http.Server{Handler: mux}

	if s.addrCh != nil {
		s.addrCh <- listener.Addr().String()
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
