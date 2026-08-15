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
	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/daemon/cache"
	"github.com/iannil/huan/internal/daemon/contentindex"
	"github.com/iannil/huan/internal/daemon/dag"
	"github.com/iannil/huan/internal/daemon/eventbus"
	"github.com/iannil/huan/internal/daemon/sse"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
	"github.com/iannil/huan/internal/translate"
)

// Options configures the daemon.
type Options struct {
	SourceDir      string
	ConfigPath     string // daemon.yaml path, optional
	Port           string
	Bind           string
	TLSCert        string
	TLSKey         string
	Systemd        bool
	BuildDrafts    bool
	DisableWatch   bool             // disable file watching (default false)
	BuildInterval  time.Duration    // periodic full rebuild interval (0 = disabled)
	PluginDir      string           // plugin directory (default: <sourceDir>/plugins)
	DisablePlugin  bool             // disable plugin loading
	PluginRegistry *plugin.Registry // compiled plugins registry (optional)
	ThemeManager   *theme.Manager   // theme manager (optional)
}

// Daemon holds the long-running server state.
type Daemon struct {
	opts          Options
	cfg           *config.Config
	bus           eventbus.EventBus
	builder       *Builder
	serving       *Serving
	dag           *dag.DependencyGraph
	jitCache      *cache.JITCache
	health        *HealthChecker
	metrics       *MetricsCollector
	pluginManager *plugin.LifecycleManager
	themeManager  *theme.Manager
	tmpDir        string
	httpSrv       *http.Server
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

	// 6.5 Init ContentIndex + /api/v1/* query API (when ai.contentAPI is on).
	// The index is reloaded by the Builder after every successful build so
	// the query API always reflects the latest output.
	var contentAPI http.Handler
	var contentIdx *contentindex.ContentIndex
	if cfg.AI.ContentAPI {
		contentIdx = contentindex.NewContentIndex(cfg.BaseURL)
		// Initial load is best-effort — tmpDir may be empty before the
		// first build (LoadFromDir treats a missing api/ dir as "empty
		// index", not an error). The Builder reload hook will refresh it.
		if err := contentIdx.LoadFromDir(tmpDir); err != nil {
			log.Printf("daemon: content index load: %v", err)
		}
		contentAPI = contentindex.NewHandler(contentIdx)
		log.Println("daemon: content query API enabled (/api/v1/*)")
	} else {
		log.Println("daemon: content query API disabled (ai.contentAPI not set)")
	}

	// 6.75 Init ThemeManager from config
	d.themeManager = opts.ThemeManager
	if d.themeManager == nil {
		// Create a ThemeManager even without a plugin registry — Activate
		// will fail gracefully if the theme plugin is not found.
		registry := opts.PluginRegistry
		if registry == nil {
			registry = plugin.NewRegistry()
		}
		d.themeManager = theme.NewManager(registry)
	}
	if cfg.Theme != "" {
		if err := d.themeManager.Activate(cfg.Theme); err != nil {
			log.Printf("daemon: theme activate %q: %v", cfg.Theme, err)
		} else {
			log.Printf("daemon: theme %q activated", cfg.Theme)
		}
	}

	// 7. Initialize Builder
	// Create the PipelineCache up-front so BuildSite can populate it during
	// the initial full build. Subsequent incremental builds reuse it.
	pipelineCache := build.NewPipelineCache()
	d.builder = NewBuilder(BuilderOptions{
		SourceDir:     opts.SourceDir,
		OutputDir:     tmpDir,
		Bus:           d.bus,
		DAG:           d.dag,
		JITCache:      d.jitCache,
		Metrics:       d.metrics,
		BuildDrafts:   opts.BuildDrafts,
		Logf:          log.Printf,
		PipelineCache: pipelineCache,
		ContentIndex:  contentIdx,
		ThemeManager:  d.themeManager,
		OnAfterBuild: func(r *build.Result) error {
			// PipelineCache is populated by BuildSite via build.Options.PipelineCache.
			return nil
		},
	})

	// 7.5 Init Plugin Lifecycle Manager
	if !opts.DisablePlugin {
		pluginDir := opts.PluginDir
		if pluginDir == "" {
			pluginDir = filepath.Join(opts.SourceDir, "plugins")
		}

		pluginLoader := plugin.NewLoader(pluginDir)

		// Use the compiled plugin registry if provided, otherwise create a fresh one
		plugRegistry := opts.PluginRegistry
		if plugRegistry == nil {
			plugRegistry = plugin.NewRegistry()
		}

		d.pluginManager = plugin.NewLifecycleManager(
			plugRegistry,
			pluginLoader,
			d.bus,
		)

		// Register capability detector for Admin API plugin listing
		d.pluginManager.SetCapabilityDetector(func(p plugin.Plugin) string {
			var caps []string
			if _, ok := p.(deploy.Deployer); ok {
				caps = append(caps, "deploy")
			}
			if _, ok := p.(translate.Translator); ok {
				caps = append(caps, "translate")
			}
			if _, ok := p.(image.ImageProcessor); ok {
				caps = append(caps, "image")
			}
			if _, ok := p.(theme.ThemePlugin); ok {
				caps = append(caps, "theme")
			}
			if len(caps) == 0 {
				return ""
			}
			out := caps[0]
			for _, c := range caps[1:] {
				out += "," + c
			}
			return out
		})

		if err := d.pluginManager.Start(context.Background()); err != nil {
			log.Printf("daemon: plugin manager start warning: %v", err)
		}
		log.Printf("daemon: plugin manager started (dir: %s)", pluginDir)
	}

	// 8. Initialize Serving
	adminToken, _, err := resolveAdminToken(opts.Bind)
	if err != nil {
		return err
	}
	adminHandler := admin.NewHandler(admin.HandlerOptions{
		Cfg:           cfg,
		SourceDir:     opts.SourceDir,
		Rebuild:       d.builder.TriggerRebuild,
		ServeURL:      fmt.Sprintf("http://%s:%s/", opts.Bind, opts.Port),
		BindAddr:      opts.Bind,
		Token:         adminToken,
		MemoryDir:     filepath.Join(opts.SourceDir, "memory", "daily"),
		PluginManager: d.pluginManager,
		ThemeManager:  d.themeManager,
		})
	sseHub := sse.NewSSEHub(log.Printf)
	sseHub.SubscribeBus(d.bus)
	sseWatchCtx, sseWatchCancel := context.WithCancel(context.Background())
	defer sseWatchCancel()
	sseHub.Start(sseWatchCtx)
	log.Println("daemon: SSE push enabled (/api/v1/events)")

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
		ContentAPI:   contentAPI,
		SSEHub:       sseHub,
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

	// 11. Schedule periodic full rebuild (if configured)
	if opts.BuildInterval > 0 {
		go d.periodicRebuild(opts.BuildInterval)
	}

	// 12. Mark as ready after initial build
	d.health.SetReady(true)

	// 13. Notify systemd that we're ready
	notifier := NewSystemdNotifier(opts.Systemd)
	notifier.Ready()

	// 13. Start file watcher (if not disabled)
	// Build a dedicated watcher context so the watcher can use it before the
	// HTTP server ctx is created. The watcher context will be cancelled when
	// Run() returns via defer.
	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()

	if !opts.DisableWatch {
		watcher, err := NewWatcher(WatcherOptions{
			SourceDir: opts.SourceDir,
			OnChange: func(changedFiles []string) {
				_ = d.bus.Publish(context.Background(), eventbus.Event{
					Type:      eventbus.EventContentChanged,
					Timestamp: time.Now(),
					Payload:   map[string]interface{}{"changed_files": changedFiles},
				})
			},
			Logf: log.Printf,
		})
		if err != nil {
			log.Printf("daemon: watcher unavailable: %v", err)
		} else {
			go func() {
				if err := watcher.Run(watchCtx); err != nil && err != context.Canceled {
					log.Printf("daemon: watcher error: %v", err)
				}
			}()
			log.Println("daemon: file watcher started")
		}
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

	// Stop plugin manager before shutting down HTTP server
	if d.pluginManager != nil {
		d.pluginManager.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	_ = d.serving.Shutdown(shutdownCtx)
	_ = d.bus.Close()
	log.Println("daemon: stopped")
	return nil
}

// resolveAdminToken mirrors dev.go's token flow for the daemon: env token
// wins; loopback binds auto-generate (printed to stderr by the caller);
// non-loopback binds without env fail fast (ADR 0011 L1).
func resolveAdminToken(bind string) (string, bool, error) {
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety(bind, token); err != nil {
		return "", false, err
	}
	if !fromEnv {
		var err error
		if token, err = admin.GenerateToken(); err != nil {
			return "", false, fmt.Errorf("generate admin token: %w", err)
		}
		admin.MustPrintAutoGeneratedToken(token, true)
	}
	return token, fromEnv, nil
}

// periodicRebuild runs a full rebuild on the given interval.
func (d *Daemon) periodicRebuild(interval time.Duration) {
	log.Printf("daemon: periodic rebuild every %v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Printf("daemon: periodic rebuild triggered")
			if err := d.builder.FullBuild(context.Background()); err != nil {
				log.Printf("daemon: periodic rebuild failed: %v", err)
			}
		}
	}
}
