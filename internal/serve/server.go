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
	Logf      func(format string, args ...any)
}

// Server serves the built site over HTTP.
type Server struct {
	opts   ServerOptions
	logf   func(string, ...any)
	addrCh chan string // optional: tests read actual listen addr from here
}

func New(opts ServerOptions) *Server {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Server{opts: opts, logf: opts.Logf}
}

// Run blocks until ctx is cancelled or a SIGINT/SIGTERM arrives.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(s.opts.Bind, s.opts.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()
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
