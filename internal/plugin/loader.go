package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
)

var (
	ErrMissingInitSymbol  = errors.New("plugin: missing InitPlugin symbol")
	ErrPluginNameConflict = errors.New("plugin: name already registered")
)

// PluginInitFunc is the exported symbol every .so plugin must define.
// The function receives the plugin's raw config and returns a Plugin instance.
type PluginInitFunc func(cfg map[string]any) (Plugin, error)

// Loader discovers and loads .so plugin files from a directory.
type Loader struct {
	pluginDir string
}

// NewLoader creates a Loader that scans pluginDir for .so files.
func NewLoader(pluginDir string) *Loader {
	return &Loader{pluginDir: pluginDir}
}

// PluginDir returns the plugin directory path.
func (l *Loader) PluginDir() string {
	return l.pluginDir
}

// LoadPlugin opens a .so file, finds the InitPlugin symbol, and calls it.
// Returns the Plugin instance or an error.
func (l *Loader) LoadPlugin(path string) (Plugin, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: open %s: %w", path, err)
	}

	sym, err := p.Lookup("InitPlugin")
	if err != nil {
		return nil, fmt.Errorf("plugin: %s: %w", path, ErrMissingInitSymbol)
	}

	initFn, ok := sym.(func(map[string]any) (Plugin, error))
	if !ok {
		return nil, fmt.Errorf("plugin: %s: InitPlugin has wrong signature", path)
	}

	// Pass an empty config map — the plugin can ignore it or use it for
	// optional configuration. Full config integration is a future enhancement.
	instance, err := initFn(make(map[string]any))
	if err != nil {
		return nil, fmt.Errorf("plugin: %s init: %w", path, err)
	}

	if instance == nil {
		return nil, fmt.Errorf("plugin: %s: InitPlugin returned nil", path)
	}

	return instance, nil
}

// ScanAndLoadResult pairs a loaded plugin with its .so filesystem path.
type ScanAndLoadResult struct {
	Plugin Plugin
	Path   string
}

// ScanAndLoad scans the pluginDir for all .so files, loads each one, and
// returns the successfully loaded plugins with their paths. Files that
// fail to load are skipped with a warning (logged to stderr). Returns an
// error only if the pluginDir cannot be read.
func (l *Loader) ScanAndLoad() ([]ScanAndLoadResult, error) {
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // directory doesn't exist, no plugins to load
		}
		return nil, fmt.Errorf("plugin: scan dir %s: %w", l.pluginDir, err)
	}

	var results []ScanAndLoadResult
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".so" {
			continue
		}
		fullPath := filepath.Join(l.pluginDir, entry.Name())
		p, err := l.LoadPlugin(fullPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "huan: plugin load warning: %s: %v\n", entry.Name(), err)
			continue
		}
		results = append(results, ScanAndLoadResult{Plugin: p, Path: fullPath})
	}
	return results, nil
}
