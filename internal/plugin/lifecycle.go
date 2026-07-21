package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iannil/huan/internal/daemon/eventbus"
)

var ErrPluginNotFound = fmt.Errorf("plugin: not found")

// PluginInfo is the metadata returned by LifecycleManager.List() and used
// by the Admin API and CLI for display.
type PluginInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Source     string `json:"source"`               // "compiled" | "loaded" | "grpc"
	Capability string `json:"capability,omitempty"`
	Status     string `json:"status"`               // "active" | "inactive" | "error"
	LoadedAt   string `json:"loadedAt,omitempty"`
	Error      string `json:"error,omitempty"`
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
}

// NewLifecycleManager creates a LifecycleManager.
func NewLifecycleManager(registry *Registry, loader *Loader, bus eventbus.EventBus) *LifecycleManager {
	return &LifecycleManager{
		registry:  registry,
		loader:    loader,
		bus:       bus,
		pluginDir: loader.PluginDir(),
		loaded:    make(map[string]*loadedPlugin),
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

	return nil
}

// Stop unloads all runtime-loaded plugins. Does not remove compiled plugins.
func (m *LifecycleManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watchCancel != nil {
		m.watchCancel()
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
func (m *LifecycleManager) Load(soPath string) (Plugin, error) {
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

	p, err := m.loader.LoadPlugin(cleanPath)
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
func (m *LifecycleManager) Reload(name string, newSO string) error {
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
	newPlugin, err := m.loader.LoadPlugin(newSO)
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
		// Attempt to detect capability
		if caps := detectCapability(lp.plugin); caps != "" {
			info.Capability = caps
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

// detectCapability attempts to determine a plugin's capability via type assertion.
func detectCapability(p Plugin) string {
	// Check registered capability interfaces.
	// New capabilities should be added here as they are introduced.
	return "" // placeholder — will be populated as capability interfaces grow
}

// PluginWatcher monitors the plugin directory for new, modified, or deleted
// .so files and triggers hot-reload. Currently a placeholder — will be
// implemented with fsnotify in a future phase.
type PluginWatcher struct {
	dir  string
	logf func(string, ...any)
}

// Start begins watching the plugin directory. Placeholder implementation.
func (w *PluginWatcher) Start(ctx context.Context) error {
	return nil
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
