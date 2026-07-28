package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/iannil/huan/internal/config"
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
type PluginInitFunc func(cfg map[string]any) (interface{}, error)

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

	// InitPlugin returns interface{} because .so plugins use self-contained
	// type copies whose concrete types are different named types than huan's.
	// Interface satisfaction in Go is structural, so the returned value will
	// satisfy huan's Plugin (and all capability interfaces) as long as the
	// method signatures match.
	if initFn, ok := sym.(func(map[string]any) (interface{}, error)); ok {
		if pluginCfg == nil {
			pluginCfg = make(map[string]any)
		}
		raw, err := initFn(pluginCfg)
		if err != nil {
			return nil, fmt.Errorf("plugin: %s init: %w", path, err)
		}
		if raw == nil {
			return nil, fmt.Errorf("plugin: %s: InitPlugin returned nil", path)
		}
		inst, ok := raw.(Plugin)
		if !ok {
			return nil, fmt.Errorf("plugin: %s: InitPlugin returned %T which does not implement Plugin", path, raw)
		}
		return inst, nil
	}

	return nil, fmt.Errorf("plugin: %s: InitPlugin has wrong signature (got %T)", path, sym)
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

// ScanAndLoadByCategory scans all .so files in the plugin directory,
// loads each one, and returns only those whose category (from config)
// matches one of the given categories.
func (l *Loader) ScanAndLoadByCategory(pluginConfigs map[string]config.PluginConfig, categories ...string) ([]ScanAndLoadResult, error) {
	if l.pluginDir == "" {
		return nil, nil
	}
	// Check if directory exists
	if _, err := os.Stat(l.pluginDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		return nil, fmt.Errorf("plugin: scan dir %s: %w", l.pluginDir, err)
	}

	var results []ScanAndLoadResult
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".so" {
			continue
		}

		// Derive plugin name from filename: "seo-injector.so" -> "seo_injector"
		// (yaml uses underscores, so files use hyphens)
		fileName := strings.TrimSuffix(entry.Name(), ".so")
		pluginName := strings.ReplaceAll(fileName, "-", "_")

		// Check category
		pc, exists := pluginConfigs[pluginName]
		if !exists {
			// Plugin not in config — skip (default dynamic behavior)
			continue
		}
		if !matchCategory(pc.Category, categories) {
			continue
		}

		fullPath := filepath.Join(l.pluginDir, entry.Name())
		p, err := l.LoadPlugin(fullPath, pc.Config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "huan: plugin load warning: %s: %v\n", entry.Name(), err)
			continue
		}
		results = append(results, ScanAndLoadResult{Plugin: p, Path: fullPath})
	}
	return results, nil
}

// matchCategory checks if a plugin's category matches any of the wanted categories.
func matchCategory(pluginCategory string, wantedCategories []string) bool {
	if pluginCategory == "" {
		pluginCategory = CategoryDynamic
	}
	for _, wc := range wantedCategories {
		if pluginCategory == wc {
			return true
		}
	}
	return false
}
