package desktop

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iannil/huan/internal/admin"
	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/daemon/eventbus"
	"github.com/iannil/huan/internal/dev"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/observability"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
)

// devMarkdownCacheBytes bounds retained Markdown data independently of the
// site size (same budget as `huan dev`).
const devMarkdownCacheBytes = 128 << 20

// ServeOptions configures an in-process huan dev server.
type ServeOptions struct {
	SourceDir string
	Logf      func(format string, a ...any)
}

// ServeRuntime owns one in-process serve session: initial build, file
// watching, incremental rebuilds, and the dev HTTP server (site + /admin +
// LiveReload). Start/Stop are serialized internally: a Start while already
// running returns the existing admin URL; overlapping Start calls during the
// startup window report an error; Stop is idempotent and safe to call when
// not running.
type ServeRuntime struct {
	opts ServeOptions
	log  *observability.Logger
	logf func(format string, a ...any)

	mu       sync.Mutex
	starting bool // guards concurrent Start calls
	cancel   context.CancelFunc
	done     chan struct{} // closed when the serve goroutine exits
	tmpDir   string
	cfg      *config.Config
	adminURL string
	siteURL  string
	running  bool

	// OnServeExited fires when the serve goroutine exits for any reason
	// (Stop also triggers it with a nil/canceled error). Set before Start;
	// invoked outside the runtime mutex.
	OnServeExited func(err error)
	// OnRebuildFailed fires when a watched-change rebuild fails. Set before
	// Start; invoked from the rebuild worker goroutine.
	OnRebuildFailed func(err error)
}

// NewServeRuntime creates a runtime for the given site source directory.
func NewServeRuntime(opts ServeOptions) *ServeRuntime {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &ServeRuntime{opts: opts, logf: opts.Logf, log: observability.NewLogger("")}
}

// Config returns the loaded site config; nil before a successful Start.
func (r *ServeRuntime) Config() *config.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

// AdminURL returns the admin panel base URL; empty before Start.
func (r *ServeRuntime) AdminURL() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adminURL
}

// SiteURL returns the site home URL; empty before Start.
func (r *ServeRuntime) SiteURL() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.siteURL
}

// SourceDir returns the site source directory.
func (r *ServeRuntime) SourceDir() string { return r.opts.SourceDir }

// Running reports whether the server is currently serving.
func (r *ServeRuntime) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// pickFreePort asks the OS for a free TCP port on 127.0.0.1 and releases it.
// The tiny reuse race is acceptable for a local desktop app; the dev server
// surfaces EADDRINUSE as a Start error, which the app reports.
func pickFreePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("pick free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return fmt.Sprintf("%d", port), nil
}

// Start assembles and launches the in-process server. It blocks until the
// first build completes and the HTTP listener answers, then returns the
// admin URL.
func (r *ServeRuntime) Start() (string, error) {
	r.mu.Lock()
	if r.running {
		url := r.adminURL
		r.mu.Unlock()
		return url, nil
	}
	if r.starting {
		r.mu.Unlock()
		return "", fmt.Errorf("start already in progress")
	}
	r.starting = true
	defer func() {
		r.mu.Lock()
		r.starting = false
		r.mu.Unlock()
	}()
	startLog := r.log.LogFunctionStart("serve-start", map[string]any{"source": r.opts.SourceDir})
	startTime := time.Now()
	r.mu.Unlock()

	cfg, err := config.Load(r.opts.SourceDir)
	if err != nil {
		r.log.LogError(startLog, err, nil)
		return "", fmt.Errorf("load config: %w", err)
	}

	// Admin token: generate silently (no stdout printing in a GUI app).
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety("127.0.0.1", token); err != nil {
		return "", err
	}
	if !fromEnv {
		if token, err = admin.GenerateToken(); err != nil {
			return "", fmt.Errorf("generate admin token: %w", err)
		}
	}

	// Resolve a free port up front so LiveReload URLs can be embedded in the
	// initial build output (same construction as `huan dev`).
	port, err := pickFreePort()
	if err != nil {
		return "", err
	}
	browserHost := "127.0.0.1"
	devBaseURL := "http://" + browserHost + ":" + port + "/"
	lrURL := "ws://" + browserHost + ":" + port + "/livereload"

	tmpDir, err := os.MkdirTemp("", "huan-desktop-*")
	if err != nil {
		return "", fmt.Errorf("mkdtemp: %w", err)
	}

	buildOpts := build.Options{
		Timings:          build.NewTimings(),
		MarkdownCache:    build.NewMarkdownCache(devMarkdownCacheBytes),
		ParseCache:       content.NewParseCache(devMarkdownCacheBytes),
		SourceDir:        r.opts.SourceDir,
		OutputDir:        tmpDir,
		IncludeDrafts:    false,
		InjectLiveReload: true,
		LiveReloadURL:    lrURL,
		BaseURLOverride:  devBaseURL,
		Logf:             r.logf,
	}
	defer buildOpts.MarkdownCache.Close()

	reg, err := plugin.NewRegistryFromConfig(cfg, r.opts.SourceDir, "")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("plugin registry: %w", err)
	}
	buildOpts.PluginRegistry = reg
	themeMgr := theme.NewManager(reg)
	if cfg.Theme != "" {
		if err := themeMgr.Activate(cfg.Theme); err != nil {
			r.logf("huan: theme activate %q: %v", cfg.Theme, err)
		}
	}
	buildOpts.ThemeManager = themeMgr

	runBuild := func(opts build.Options) error {
		if cfg.IsMultiLanguage() {
			if _, err := build.BuildMultiSite(opts); err != nil {
				return err
			}
		} else {
			if _, err := build.BuildSite(opts); err != nil {
				return err
			}
		}
		// Run the image pipeline after the build regardless of site mode
		// (same ordering as `huan dev`).
		return image.RunProcessors(opts.OutputDir, r.opts.SourceDir, reg)
	}

	if err := runBuild(buildOpts); err != nil {
		_ = os.RemoveAll(tmpDir)
		r.log.LogError(startLog, err, map[string]any{"phase": "initial-build"})
		return "", fmt.Errorf("initial build: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var swapper build.BuildDirSwapper
	nextDir := tmpDir + ".next"
	queue := dev.NewRebuildQueue(func(paths []string) {
		if !dev.MarkdownOnlyChanges(r.opts.SourceDir, paths) {
			buildOpts.MarkdownCache.Clear()
		}
		r.logf("[watch] change detected, rebuilding...")
		_ = os.RemoveAll(nextDir)
		opts := buildOpts
		opts.OutputDir = nextDir
		opts.Timings = build.NewTimings()
		rebuildErr := func() error {
			if err := runBuild(opts); err != nil {
				return err
			}
			return swapper.Swap(tmpDir, nextDir)
		}()
		if rebuildErr != nil {
			_ = os.RemoveAll(nextDir)
			r.logf("[watch] rebuild error: %v", rebuildErr)
			if r.OnRebuildFailed != nil {
				r.OnRebuildFailed(rebuildErr)
			}
			return
		}
		r.logf("[watch] rebuild complete")
	})

	watcherOpts := dev.WatcherOptions{
		SourceDir: r.opts.SourceDir,
		Debounce:  400 * time.Millisecond,
		OnChanges: queue.Submit,
		Logf:      r.logf,
	}
	// The publish dir is build output, not input: exclude it so a concurrent
	// build rewriting it cannot loop dev rebuilds (same as `huan dev`).
	if dir := filepath.Clean(cfg.PublishDir); dir != "" && dir != "." && !filepath.IsAbs(dir) {
		watcherOpts.IgnoreDirs = append(watcherOpts.IgnoreDirs, dir)
	}
	watcher, err := dev.NewWatcher(watcherOpts)
	if err != nil {
		cancel()
		queue.Close()
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("watcher: %w", err)
	}
	wg.Add(1)
	go func() { defer wg.Done(); _ = watcher.Run(ctx) }()

	// Admin handler: same wiring as `huan dev` (rebuild via queue submit).
	pluginLoader := plugin.NewLoader(plugin.DirFromSource(r.opts.SourceDir))
	adminPluginMgr := plugin.NewLifecycleManager(reg, pluginLoader, eventbus.NewChannelBus())
	adminHandler := admin.NewHandler(admin.HandlerOptions{
		Cfg:           cfg,
		SourceDir:     r.opts.SourceDir,
		Rebuild:       func() { queue.Submit(nil) },
		ServeURL:      devBaseURL,
		BindAddr:      "127.0.0.1",
		Token:         token,
		MemoryDir:     filepath.Join(r.opts.SourceDir, "memory", "daily"),
		PluginManager: adminPluginMgr,
		ThemeManager:  themeMgr,
	})

	hub := dev.NewHub()
	srv := dev.New(dev.ServerOptions{
		OutputDir:    tmpDir,
		Bind:         "127.0.0.1",
		Port:         port,
		Hub:          hub,
		AdminHandler: adminHandler,
		Logf:         r.logf,
	})

	done := make(chan struct{})
	r.mu.Lock()
	r.cancel = cancel
	r.done = done
	r.cfg = cfg
	r.adminURL = devBaseURL + "admin/"
	r.siteURL = devBaseURL
	adminURL := r.adminURL
	r.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			if rec := recover(); rec != nil {
				r.log.LogError("serve-goroutine", fmt.Errorf("panic: %v", rec), nil)
				r.mu.Lock()
				r.running = false
				r.mu.Unlock()
				if r.OnServeExited != nil {
					r.OnServeExited(fmt.Errorf("serve panic: %v", rec))
				}
				cancel()
			}
		}()
		err := srv.Run(ctx)
		wg.Wait()
		queue.Close()
		queue.Wait()
		swapper.Wait()
		_ = os.RemoveAll(tmpDir)
		_ = os.RemoveAll(nextDir)
		r.mu.Lock()
		wasRunning := r.running
		r.running = false
		r.mu.Unlock()
		if wasRunning && r.OnServeExited != nil {
			r.OnServeExited(err)
		}
	}()

	// Wait for the listener to answer before reporting ready.
	siteURL := devBaseURL
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(siteURL)
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			r.log.LogError(startLog, err, map[string]any{"phase": "listen-wait"})
			return "", fmt.Errorf("server did not become ready: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	r.mu.Lock()
	r.running = true
	r.mu.Unlock()
	r.log.LogFunctionEnd(startLog, time.Since(startTime), map[string]any{"admin_url": adminURL})
	return adminURL, nil
}

// Stop shuts the server down gracefully; safe to call when not running.
func (r *ServeRuntime) Stop(timeout time.Duration) error {
	r.mu.Lock()
	cancel, done, running := r.cancel, r.done, r.running
	r.mu.Unlock()
	if !running {
		return nil
	}
	stopLog := r.log.LogFunctionStart("serve-stop", nil)
	stopStart := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(timeout):
		err := fmt.Errorf("serve stop timed out after %v", timeout)
		r.log.LogError(stopLog, err, nil)
		return err
	}
	r.mu.Lock()
	r.cancel, r.done, r.cfg, r.adminURL, r.siteURL = nil, nil, nil, "", ""
	r.mu.Unlock()
	r.log.LogFunctionEnd(stopLog, time.Since(stopStart), nil)
	return nil
}
