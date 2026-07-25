package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

var ErrPluginNotFound = fmt.Errorf("plugin: not found")

// PluginInfo is the metadata returned by LifecycleManager.List() and used
// by the Admin API and CLI for display.
type PluginInfo struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Source     string   `json:"source"`               // "compiled" | "loaded" | "grpc"
	Capability string   `json:"capability,omitempty"`
	Status     string   `json:"status"`               // "active" | "inactive" | "error"
	LoadedAt   string   `json:"loadedAt,omitempty"`
	Error      string   `json:"error,omitempty"`
	// 新增元数据字段
	Author     string   `json:"author,omitempty"`
	RepoURL    string   `json:"repoURL,omitempty"`
	License    string   `json:"license,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// loadedPlugin tracks metadata about a runtime-loaded plugin.
type loadedPlugin struct {
	plugin   Plugin
	source   string // "compiled" | "loaded" | "grpc"
	soPath   string // filesystem path of the .so (empty for compiled/grpc)
	loadedAt time.Time
}

// LifecycleManager manages the complete lifecycle of plugins: discovery,
// loading, unloading, reloading, and event publishing.
type LifecycleManager struct {
	registry  *Registry
	loader    *Loader
	bus       eventbus.EventBus
	pluginDir string // plugin directory for path validation

	mu          sync.Mutex
	loaded      map[string]*loadedPlugin // tracks all plugins (compiled + loaded)
	watcher     *PluginWatcher
	watchCtx    context.Context
	watchCancel context.CancelFunc

	// detectCapability is an optional function that returns capability labels
	// for a given plugin. When nil, List() returns an empty Capability string.
	// The composition root (cmd/huan) should set this via SetCapabilityDetector
	// to enable per-capability detection without importing domain packages here.
	detectCapabilityFn func(Plugin) string

	// subscriptionIDs tracks eventbus subscriptions per plugin name.
	// Key: plugin name, Value: list of subscription entries for unsubscription.
	subscriptionIDs map[string][]subscriptionEntry
}

// subscriptionEntry holds the event type and handler ID for a plugin subscription.
type subscriptionEntry struct {
	eventType eventbus.EventType
	handlerID string
}

// SetCapabilityDetector registers a function that returns capability labels
// for a given plugin. The composition root (cmd/huan) should set this to
// enable Admin API and CLI plugin list to show capability info.
//
// The detector receives the full plugin.Plugin interface and can use type
// assertions against domain capability interfaces (deploy.Deployer, etc.).
// It is called from List() and from the Admin API.
func (m *LifecycleManager) SetCapabilityDetector(fn func(Plugin) string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detectCapabilityFn = fn
}

// NewLifecycleManager creates a LifecycleManager.
func NewLifecycleManager(registry *Registry, loader *Loader, bus eventbus.EventBus) *LifecycleManager {
	return &LifecycleManager{
		registry:  registry,
		loader:    loader,
		bus:       bus,
		pluginDir: loader.PluginDir(),
		loaded:    make(map[string]*loadedPlugin),
		subscriptionIDs: make(map[string][]subscriptionEntry),
	}
}

// Start discovers and loads all .so plugins from the plugin directory, then
// starts the file watcher for hot-reload. Already-registered compiled plugins
// are tracked but not re-loaded.
func (m *LifecycleManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track existing compiled plugins
	for _, p := range m.registry.All() {
		name := p.Name()
		if _, exists := m.loaded[name]; !exists {
			m.loaded[name] = &loadedPlugin{
				plugin:   p,
				source:   "compiled",
				loadedAt: time.Now(),
			}
			m.publishEventUnsafe(ctx, eventbus.EventPluginLoaded, map[string]any{
				"name":   name,
				"source": "compiled",
			})
		}
	}

	// Scan and load .so plugins
	results, err := m.loader.ScanAndLoad()
	if err != nil {
		m.publishEventUnsafe(ctx, eventbus.EventPluginError, map[string]any{
			"error": err.Error(),
		})
		return fmt.Errorf("lifecycle: scan plugins: %w", err)
	}
	for _, result := range results {
		name := result.Plugin.Name()
		if _, exists := m.registry.Get(name); exists {
			fmt.Fprintf(os.Stderr, "huan: plugin %q: name conflict with compiled plugin, skipping\n", name)
			m.publishEventUnsafe(ctx, eventbus.EventPluginError, map[string]any{
				"name":  name,
				"error": "name conflict with compiled plugin",
			})
			continue
		}
		_ = m.registry.Register(result.Plugin)
		m.loaded[name] = &loadedPlugin{
			plugin:   result.Plugin,
			source:   "loaded",
			soPath:   result.Path,
			loadedAt: time.Now(),
		}
		m.publishEventUnsafe(ctx, eventbus.EventPluginLoaded, map[string]any{
			"name":   name,
			"source": "loaded",
			"path":   result.Path,
		})
	}

	// Start file watcher for hot-reload
	m.watchCtx, m.watchCancel = context.WithCancel(ctx)
	m.watcher = NewPluginWatcher(m.pluginDir, m, func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, format, a...)
	})
	go func() {
		if err := m.watcher.Start(m.watchCtx); err != nil {
			fmt.Fprintf(os.Stderr, "huan: plugin watcher: %v\n", err)
		}
	}()

	// Subscribe compiled plugins to system events
	for _, p := range m.registry.All() {
		m.subscribePluginEvents(p)
	}

	return nil
}

// Stop unloads all runtime-loaded plugins. Does not remove compiled plugins.
func (m *LifecycleManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watchCancel != nil {
		m.watchCancel()
	}

	// Unsubscribe all plugin event handlers
	for name := range m.subscriptionIDs {
		m.unsubscribePluginEvents(name)
	}

	for name, lp := range m.loaded {
		if lp.source != "compiled" {
			m.registry.Unregister(name)
			delete(m.loaded, name)
		}
	}
}

// Load loads a .so plugin from the given path, registers it, and publishes
// an event. Returns ErrPluginNameConflict if a plugin with the same name
// already exists. Returns an error if the path is outside the plugin directory.
// The pluginCfg map is passed to the plugin's InitPlugin function.
func (m *LifecycleManager) Load(soPath string, pluginCfg map[string]any) (Plugin, error) {
	// Validate path is within plugin directory
	cleanPath := filepath.Clean(soPath)
	cleanPluginDir := filepath.Clean(m.pluginDir)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(cleanPluginDir, cleanPath)
	}
	// Ensure the resolved path is within the plugin directory
	if !isPathWithinDir(cleanPath, cleanPluginDir) {
		m.publishEvent(context.Background(), eventbus.EventPluginError, map[string]any{
			"path":  soPath,
			"error": "path is outside plugin directory",
		})
		return nil, fmt.Errorf("plugin: path %q is outside plugin directory %q", soPath, m.pluginDir)
	}

	p, err := m.loader.LoadPlugin(cleanPath, pluginCfg)
	if err != nil {
		m.publishEvent(context.Background(), eventbus.EventPluginError, map[string]any{
			"path":  soPath,
			"error": err.Error(),
		})
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	name := p.Name()
	if _, exists := m.registry.Get(name); exists {
		return nil, ErrPluginNameConflict
	}

	if err := m.registry.Register(p); err != nil {
		return nil, err
	}

	m.loaded[name] = &loadedPlugin{
		plugin:   p,
		source:   "loaded",
		soPath:   cleanPath,
		loadedAt: time.Now(),
	}

	m.publishEventUnsafe(context.Background(), eventbus.EventPluginLoaded, map[string]any{
		"name":   name,
		"source": "loaded",
		"path":   cleanPath,
	})

	// Subscribe to system events if the plugin implements EventSubscriber
	m.subscribePluginEvents(p)

	return p, nil
}

// Unload removes a plugin by name. Returns ErrPluginNotFound if the plugin
// is not registered. Does NOT remove compiled plugins.
func (m *LifecycleManager) Unload(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lp, exists := m.loaded[name]
	if !exists {
		return ErrPluginNotFound
	}
	if lp.source == "compiled" {
		return fmt.Errorf("plugin %q: cannot unload compiled plugin", name)
	}

	// Unsubscribe from system events
	m.unsubscribePluginEvents(name)

	m.registry.Unregister(name)
	delete(m.loaded, name)

	m.publishEventUnsafe(context.Background(), eventbus.EventPluginUnloaded, map[string]any{
		"name": name,
	})

	return nil
}

// Reload replaces a loaded plugin's implementation by loading a new .so.
// If the new .so fails to load, the original plugin is preserved (rollback).
// Returns ErrPluginNotFound if the plugin is not registered.
// The pluginCfg map is passed to the plugin's InitPlugin function.
func (m *LifecycleManager) Reload(name string, newSO string, pluginCfg map[string]any) error {
	m.mu.Lock()

	lp, exists := m.loaded[name]
	if !exists {
		m.mu.Unlock()
		return ErrPluginNotFound
	}
	if lp.source == "compiled" {
		m.mu.Unlock()
		return fmt.Errorf("plugin %q: cannot reload compiled plugin", name)
	}

	// Save old state for rollback
	oldPlugin := lp.plugin
	oldSO := lp.soPath

	// Unregister old
	m.registry.Unregister(name)
	m.mu.Unlock()

	// Load new .so (outside the lock — plugin.Open can block)
	newPlugin, err := m.loader.LoadPlugin(newSO, pluginCfg)
	if err != nil {
		// Rollback: re-register old
		m.mu.Lock()
		_ = m.registry.Register(oldPlugin)
		m.loaded[name] = &loadedPlugin{
			plugin:   oldPlugin,
			source:   "loaded",
			soPath:   oldSO,
			loadedAt: time.Now(),
		}
		m.mu.Unlock()
		return fmt.Errorf("reload: %w (rolled back)", err)
	}

	// Register new
	m.mu.Lock()
	newName := newPlugin.Name()
	if newName != name {
		// Name changed during reload — this creates inconsistent state.
		// The user should Unload + Load instead.
		_ = m.registry.Register(newPlugin)
		delete(m.loaded, name)
		m.loaded[newName] = &loadedPlugin{
			plugin:   newPlugin,
			source:   "loaded",
			soPath:   newSO,
			loadedAt: time.Now(),
		}
		m.mu.Unlock()
		return fmt.Errorf("reload: plugin name changed from %q to %q (use Unload+Load instead)", name, newName)
	}
	_ = m.registry.Register(newPlugin)
	delete(m.loaded, name)
	m.loaded[newName] = &loadedPlugin{
		plugin:   newPlugin,
		source:   "loaded",
		soPath:   newSO,
		loadedAt: time.Now(),
	}
	m.mu.Unlock()

	m.publishEvent(context.Background(), eventbus.EventPluginReloaded, map[string]any{
		"old_name": name,
		"new_name": newName,
		"path":     newSO,
	})

	return nil
}

// List returns metadata about all registered plugins (compiled + loaded).
func (m *LifecycleManager) List() []PluginInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure all registry plugins are tracked in m.loaded
	for _, p := range m.registry.All() {
		name := p.Name()
		if _, exists := m.loaded[name]; !exists {
			m.loaded[name] = &loadedPlugin{
				plugin:   p,
				source:   "compiled",
				loadedAt: time.Now(),
			}
		}
	}

	out := make([]PluginInfo, 0, len(m.loaded))
	for name, lp := range m.loaded {
		info := PluginInfo{
			Name:     name,
			Source:   lp.source,
			Status:   "active",
			LoadedAt: lp.loadedAt.Format(time.RFC3339),
		}
		// Attempt to detect capability via the optional detector function
		if m.detectCapabilityFn != nil {
			if caps := m.detectCapabilityFn(lp.plugin); caps != "" {
				info.Capability = caps
			}
		}
		// Detect metadata via optional MetadataProvider interface
		if mp, ok := lp.plugin.(MetadataProvider); ok {
			meta := mp.PluginMetadata()
			info.Version = meta.Version
			info.Author = meta.Author
			info.RepoURL = meta.RepoURL
			info.License = meta.License
			info.Tags = meta.Tags
		}
		out = append(out, info)
	}
	return out
}

// registerCompiled registers a compiled plugin and publishes an event.
// Used internally for integration with daemon startup.
func (m *LifecycleManager) registerCompiled(p Plugin) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := p.Name()
	if _, exists := m.loaded[name]; exists {
		return
	}

	_ = m.registry.Register(p)
	m.loaded[name] = &loadedPlugin{
		plugin:   p,
		source:   "compiled",
		loadedAt: time.Now(),
	}
	m.publishEventUnsafe(context.Background(), eventbus.EventPluginLoaded, map[string]any{
		"name":   name,
		"source": "compiled",
	})
}

// --- subscription helpers ---

// subscribePluginEvents checks if a plugin implements EventSubscriber and
// subscribes to its declared events. Must be called with m.mu held.
func (m *LifecycleManager) subscribePluginEvents(p Plugin) {
	es, ok := p.(EventSubscriber)
	if !ok {
		return
	}
	events := es.SubscribedEvents()
	if len(events) == 0 {
		return
	}

	name := p.Name()
	var entries []subscriptionEntry
	for _, evtType := range events {
		// Capture the handler for the closure
		h := es.HandleEvent
		handlerID := m.bus.Subscribe(evtType, func(ctx context.Context, ev eventbus.Event) error {
			return h(ctx, ev)
		})
		entries = append(entries, subscriptionEntry{
			eventType: evtType,
			handlerID: handlerID,
		})
	}
	m.subscriptionIDs[name] = entries
}

// unsubscribePluginEvents removes all event subscriptions for a plugin.
// Must be called with m.mu held.
func (m *LifecycleManager) unsubscribePluginEvents(name string) {
	entries, ok := m.subscriptionIDs[name]
	if !ok {
		return
	}
	for _, entry := range entries {
		m.bus.Unsubscribe(entry.eventType, entry.handlerID)
	}
	delete(m.subscriptionIDs, name)
}

// --- helpers ---

func (m *LifecycleManager) publishEvent(ctx context.Context, eventType eventbus.EventType, payload map[string]any) {
	m.mu.Lock()
	m.publishEventUnsafe(ctx, eventType, payload)
	m.mu.Unlock()
}

func (m *LifecycleManager) publishEventUnsafe(ctx context.Context, eventType eventbus.EventType, payload map[string]any) {
	_ = m.bus.Publish(ctx, eventbus.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// capabilityLabels is a helper that calls the registered detector function.
// Kept for backward compatibility — the real capability detection is now
// configured via LifecycleManager.SetCapabilityDetector.

// PluginWatcher monitors the plugin directory for new, modified, or deleted
// .so files and triggers hot-reload through the LifecycleManager.
type PluginWatcher struct {
	dir     string
	logf    func(string, ...any)
	manager *LifecycleManager
}

// NewPluginWatcher creates a PluginWatcher that notifies the given manager
// of filesystem changes in dir.
func NewPluginWatcher(dir string, manager *LifecycleManager, logf func(string, ...any)) *PluginWatcher {
	if logf == nil {
		logf = func(format string, a ...any) { fmt.Fprintf(os.Stderr, format, a...) }
	}
	return &PluginWatcher{
		dir:     dir,
		logf:    logf,
		manager: manager,
	}
}

// Start begins watching the plugin directory for .so file changes. It uses
// fsnotify to trigger automatic Load/Unload/Reload operations. A debounce of
// 500ms prevents duplicate events. Returns nil when the context is cancelled.
func (w *PluginWatcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("plugin watcher: create fsnotify: %w", err)
	}
	defer watcher.Close()

	// Watch the plugin directory if it exists
	if fi, err := os.Stat(w.dir); err == nil && fi.IsDir() {
		if err := watcher.Add(w.dir); err != nil {
			return fmt.Errorf("plugin watcher: watch %s: %w", w.dir, err)
		}
		w.logf("plugin watcher: watching %s\n", w.dir)
	} else {
		w.logf("plugin watcher: directory %s does not exist, watching skipped\n", w.dir)
	}

	// debounceTimer resets on each event; fires after 500ms of quiet
	var debounceTimer *time.Timer
	var pendingEvent string // tracks the event type for debounce

	// flushPending applies the pending debounced event
	flushPending := func() {
		switch pendingEvent {
		case "create", "write":
			w.handleCreateOrModify()
		case "remove":
			w.handleRemove()
		}
		pendingEvent = ""
	}

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Only care about .so files
			if filepath.Ext(event.Name) != ".so" {
				continue
			}

			// Map event to our pending type
			var evType string
			if event.Has(fsnotify.Create) {
				evType = "create"
			} else if event.Has(fsnotify.Write) {
				evType = "write"
			} else if event.Has(fsnotify.Remove) {
				evType = "remove"
			} else {
				continue // Rename, Chmod — ignore
			}

			pendingEvent = evType

			// Reset or start debounce timer
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(500*time.Millisecond, flushPending)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.logf("plugin watcher: error: %v\n", err)
		}
	}
}

// handleCreateOrModify loads or reloads a plugin when a .so file is created or modified.
func (w *PluginWatcher) handleCreateOrModify() {
	w.manager.mu.Lock()
	defer w.manager.mu.Unlock()

	// Scan all .so files in the directory
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		w.logf("plugin watcher: read dir: %v\n", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".so" {
			continue
		}
		soPath := filepath.Join(w.dir, entry.Name())

		// Derive plugin name from file name: "image_pipeline.so" -> "image_pipeline"
		pluginName := strings.TrimSuffix(entry.Name(), ".so")

		// Check if already loaded
		if _, exists := w.manager.loaded[pluginName]; exists {
			continue // already loaded, ignore
		}

		// Load the plugin
		soCopy := soPath // capture for closure
		w.manager.mu.Unlock()
		p, err := w.manager.loader.LoadPlugin(soCopy, nil)
		w.manager.mu.Lock()
		if err != nil {
			w.logf("plugin watcher: load %s: %v\n", soCopy, err)
			continue
		}

		name := p.Name()
		if _, exists := w.manager.registry.Get(name); exists {
			w.logf("plugin watcher: %q: name conflict, skipping\n", name)
			continue
		}

		_ = w.manager.registry.Register(p)
		w.manager.loaded[name] = &loadedPlugin{
			plugin:   p,
			source:   "loaded",
			soPath:   soCopy,
			loadedAt: time.Now(),
		}
		w.manager.publishEventUnsafe(context.Background(), eventbus.EventPluginLoaded, map[string]any{
			"name":   name,
			"source": "loaded",
			"path":   soCopy,
		})
		w.logf("plugin watcher: loaded %q from %s\n", name, soCopy)
	}
}

// handleRemove removes a plugin when its .so file is deleted.
func (w *PluginWatcher) handleRemove() {
	w.manager.mu.Lock()
	defer w.manager.mu.Unlock()

	// Check which loaded .so plugins still have their files on disk
	for name, lp := range w.manager.loaded {
		if lp.source != "loaded" {
			continue
		}
		if !fileExists(lp.soPath) {
			w.manager.registry.Unregister(name)
			delete(w.manager.loaded, name)
			w.manager.publishEventUnsafe(context.Background(), eventbus.EventPluginUnloaded, map[string]any{
				"name": name,
			})
			w.logf("plugin watcher: unloaded %q (%s removed)\n", name, lp.soPath)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isPathWithinDir checks if a given absolute path is within the specified directory.
func isPathWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	// If the relative path starts with "..", it's outside the directory
	return !filepath.IsAbs(rel) && !strings.HasPrefix(rel, "..")
}
