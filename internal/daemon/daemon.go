package daemon

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/iannil/huan/internal/admin"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/dag"
	"github.com/iannil/huan/internal/daemon/eventbus"
)

// Options configures the daemon.
type Options struct {
	SourceDir    string
	ConfigPath   string // daemon.yaml path, optional
	Port         string
	Bind         string
	TLSCert      string
	TLSKey       string
	Systemd      bool
	BuildDrafts  bool
	DisableWatch bool // disable file watching (default false)
}

// Daemon holds the long-running server state.
type Daemon struct {
	opts     Options
	cfg      *config.Config
	bus      eventbus.EventBus
	builder  *Builder
	serving  *Serving
	dag      *dag.DependencyGraph
	jitCache *cache.JITCache
	health   *HealthChecker
	metrics  *MetricsCollector
	tmpDir   string
	httpSrv  *http.Server
}

// Run starts the daemon and blocks until shutdown.
func Run(opts Options) error {
	d := &Daemon{
		opts: opts,
	}

	// 1. Load config
	cfg, err := config.Load(opts.SourceDir)
	if err != nil {
		return fmt.Errorf("daemon: load config: %w", err)
	}
	d.cfg = cfg

	// 2. Initialize EventBus
	d.bus = eventbus.NewChannelBus()

	// 3. Initialize cache
	d.jitCache = cache.NewJITCache(1000, 5*time.Minute)

	// 4. Initialize Health + Metrics
	d.health = NewHealthChecker()
	d.metrics = NewMetricsCollector()

	// 5. Create temp dir for rendered output
	tmpDir, err := os.MkdirTemp("", "huan-daemon-*")
	if err != nil {
		return fmt.Errorf("daemon: mkdtemp: %w", err)
	}
	d.tmpDir = tmpDir
	defer os.RemoveAll(tmpDir)

	// 6. Initialize DAG (loaded from disk if exists)
	d.dag = dag.NewDependencyGraph()

	// 7. Initialize Builder
	d.builder = NewBuilder(BuilderOptions{
		SourceDir:   opts.SourceDir,
		OutputDir:   tmpDir,
		Bus:         d.bus,
		DAG:         d.dag,
		JITCache:    d.jitCache,
		Metrics:     d.metrics,
		BuildDrafts: opts.BuildDrafts,
		Logf:        log.Printf,
	})

	// 8. Initialize Serving
	adminHandler := admin.NewHandler(admin.HandlerOptions{
		Cfg:       cfg,
		SourceDir: opts.SourceDir,
		Rebuild:   d.builder.TriggerRebuild,
		ServeURL:  fmt.Sprintf("http://%s:%s/", opts.Bind, opts.Port),
		BindAddr:  opts.Bind,
		Token:     "", // Uses env var HUAN_ADMIN_TOKEN
		MemoryDir: filepath.Join(opts.SourceDir, "memory", "daily"),
	})

	d.serving = NewServing(ServingOptions{
		OutputDir:    tmpDir,
		Bind:         opts.Bind,
		Port:         opts.Port,
		TLSCert:      opts.TLSCert,
		TLSKey:       opts.TLSKey,
		AdminHandler: adminHandler,
		JITCache:     d.jitCache,
		Builder:      d.builder,
		Bus:          d.bus,
		Health:       d.health,
		Metrics:      d.metrics,
		Logf:         log.Printf,
	})

	// 9. Subscribe event handlers
	d.bus.Subscribe(eventbus.EventContentChanged, d.builder.HandleContentChanged)
	d.bus.Subscribe(eventbus.EventCacheUpdated, d.serving.HandleCacheUpdated)
	d.bus.Subscribe(eventbus.EventBuildStarted, func(ctx context.Context, event eventbus.Event) error {
		// Health check should return 503 during build
		return nil
	})

	// 10. Initial full build
	log.Println("daemon: initial full build...")
	start := time.Now()
	if err := d.builder.FullBuild(context.Background()); err != nil {
		return fmt.Errorf("daemon: initial build failed: %w", err)
	}
	log.Printf("daemon: initial build complete in %v", time.Since(start))

	// 11. Mark as ready after initial build
	d.health.SetReady(true)

	// 12. Notify systemd that we're ready
	notifier := NewSystemdNotifier(opts.Systemd)
	notifier.Ready()

	// 13. Start file watcher (if not disabled — ctx must be created before HTTP server)
	if !opts.DisableWatch {
		// startWatcher uses dev.Watcher from huan dev, but for now
		// watcher integration is deferred — daemon rebuilds via EventBus.
		// fsnotify integration will be added in a follow-up.
	}

	// 14. Start HTTP server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := d.serving.Start(ctx); err != nil && err != http.ErrServerClosed {
			log.Printf("daemon: serving error: %v", err)
		}
	}()

	// 15. Wait for shutdown signal
	sigCh := WaitForShutdown()
	<-sigCh

	notifier.Stopping()
	log.Println("daemon: shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	_ = d.serving.Shutdown(shutdownCtx)
	_ = d.bus.Close()
	log.Println("daemon: stopped")
	return nil
}
