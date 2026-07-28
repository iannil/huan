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

// PluginDir returns the project-level plugin directory path. This is the
// directory the daemon watches for hot-reload and validates reload paths
// against; it does NOT include $HUAN_HOME (see searchDirs).
func (l *Loader) PluginDir() string {
	return l.pluginDir
}

// HuanHome returns the global huan home directory used for centrally-installed
// plugins. It honors the $HUAN_HOME environment variable, falling back to
// ~/.huan. Returns "" only when neither $HUAN_HOME nor a home directory can be
// determined.
func HuanHome() string {
	if h := strings.TrimSpace(os.Getenv("HUAN_HOME")); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".huan")
}

// SoFileName returns the conventional .so file name for a plugin whose config
// key / Name() is `name`. huan.yaml plugin keys use underscores
// (e.g. qwen3_translate); the .so files on disk use hyphens
// (e.g. qwen3-translate.so). Callers derive filenames from config keys via this
// helper instead of hardcoding plugin file names.
func SoFileName(name string) string {
	return strings.ReplaceAll(name, "_", "-") + ".so"
}

// searchDirs returns the plugin directories to scan, in priority order:
// $HUAN_HOME (or ~/.huan) first, then the project plugin dir. Empty and
// duplicate entries are removed. A plugin found in an earlier directory takes
// precedence over the same-named plugin in a later one.
func (l *Loader) searchDirs() []string {
	var dirs []string
	seen := make(map[string]bool)
	add := func(d string) {
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		dirs = append(dirs, d)
	}
	add(HuanHome())
	add(l.pluginDir)
	return dirs
}

// Resolve returns the filesystem path to a plugin .so identified by its base
// file name (e.g. "cloudflare.so"), searching $HUAN_HOME first then the
// project plugin dir. It returns "" when the file exists in none of them.
func (l *Loader) Resolve(soFile string) string {
	for _, d := range l.searchDirs() {
		p := filepath.Join(d, soFile)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
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
	var results []ScanAndLoadResult
	loaded := make(map[string]bool) // .so file name -> already loaded from a higher-priority dir

	for _, dir := range l.searchDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // directory doesn't exist, try the next one
			}
			return nil, fmt.Errorf("plugin: scan dir %s: %w", dir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".so" {
				continue
			}
			if loaded[entry.Name()] {
				continue // same .so already loaded from $HUAN_HOME (higher priority)
			}
			fullPath := filepath.Join(dir, entry.Name())
			p, err := l.LoadPlugin(fullPath, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "huan: plugin load warning: %s: %v\n", entry.Name(), err)
				continue
			}
			loaded[entry.Name()] = true
			results = append(results, ScanAndLoadResult{Plugin: p, Path: fullPath})
		}
	}
	return results, nil
}

// ScanAndLoadByCategory scans all .so files in the plugin directory,
// loads each one, and returns only those whose category (from config)
// matches one of the given categories.
func (l *Loader) ScanAndLoadByCategory(pluginConfigs map[string]config.PluginConfig, categories ...string) ([]ScanAndLoadResult, error) {
	var results []ScanAndLoadResult
	loadedNames := make(map[string]bool) // plugin name -> loaded from a higher-priority dir

	for _, dir := range l.searchDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // directory doesn't exist, try the next one
			}
			return nil, fmt.Errorf("plugin: scan dir %s: %w", dir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".so" {
				continue
			}

			// Derive plugin name from filename: "seo-injector.so" -> "seo_injector"
			// (yaml uses underscores, so files use hyphens)
			fileName := strings.TrimSuffix(entry.Name(), ".so")
			pluginName := strings.ReplaceAll(fileName, "-", "_")

			// A plugin found in $HUAN_HOME (higher priority) wins over the same
			// plugin in the project dir — skip the lower-priority duplicate.
			if loadedNames[pluginName] {
				continue
			}

			// Check category
			pc, exists := pluginConfigs[pluginName]
			if !exists {
				// Plugin not in config — skip (default dynamic behavior)
				continue
			}
			if !matchCategory(pc.Category, categories) {
				continue
			}

			fullPath := filepath.Join(dir, entry.Name())
			p, err := l.LoadPlugin(fullPath, pc.Config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "huan: plugin load warning: %s: %v\n", entry.Name(), err)
				continue
			}
			loadedNames[pluginName] = true
			results = append(results, ScanAndLoadResult{Plugin: p, Path: fullPath})
		}
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
