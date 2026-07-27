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

const (
	CategoryStatic  = "static"
	CategoryDynamic = "dynamic"
	CategoryMixed   = "mixed"
)

// ShouldLoadInCategory 判断给定 category 的插件是否应在当前 mode 下加载。
// mode 为 "build" 或 "daemon"。
func ShouldLoadInCategory(pluginCategory, mode string) bool {
	if pluginCategory == "" {
		pluginCategory = CategoryDynamic // 默认 dynamic
	}
	switch pluginCategory {
	case CategoryStatic:
		return mode == "build"
	case CategoryDynamic:
		return mode == "daemon"
	case CategoryMixed:
		return true
	default:
		return mode == "daemon" // 未知 category 默认 dynamic
	}
}

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
// Returns the Plugin instance or an error. The pluginCfg map is passed to
// the plugin's InitPlugin function, allowing configuration from huan.yaml.
func (l *Loader) LoadPlugin(path string, pluginCfg map[string]any) (Plugin, error) {
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

	// Pass the provided config map to the plugin. If nil, pass an empty map.
	if pluginCfg == nil {
		pluginCfg = make(map[string]any)
	}
	instance, err := initFn(pluginCfg)
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
		p, err := l.LoadPlugin(fullPath, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "huan: plugin load warning: %s: %v\n", entry.Name(), err)
			continue
		}
		results = append(results, ScanAndLoadResult{Plugin: p, Path: fullPath})
	}
	return results, nil
}
