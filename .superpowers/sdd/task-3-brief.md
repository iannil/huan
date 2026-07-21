### Task 3: Loader — .so 插件加载器

**Files:**
- Create: `internal/plugin/loader.go`
- Create: `internal/plugin/testdata/simple_plugin/main.go` — 测试用 .so 插件
- Create: `internal/plugin/testdata/simple_plugin/Makefile` — 编译脚本
- Create: `internal/plugin/loader_test.go`

**Interfaces:**
- Produces: `Loader`, `Loader.LoadPlugin(path) (Plugin, error)`, `Loader.ScanAndLoad() ([]Plugin, error)`, `PluginInitFunc`, `ErrMissingInitSymbol`, `ErrPluginNameConflict`

- [ ] **Step 1: 创建测试用 .so 插件**

`internal/plugin/testdata/simple_plugin/main.go`：

```go
package main

import "github.com/iannil/huan/internal/plugin"

type simplePlugin struct {
    name    string
    version string
}

func (p *simplePlugin) Name() string { return p.name }

// InitPlugin 是 Loader 查找的导出符号
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    name := "simple-test"
    if v, ok := cfg["name"].(string); ok && v != "" {
        name = v
    }
    return &simplePlugin{name: name, version: "1.0.0"}, nil
}
```

`internal/plugin/testdata/simple_plugin/Makefile`：

```makefile
.PHONY: all clean

GO ?= go

all: simple_plugin.so

simple_plugin.so: main.go
	$(GO) build -buildmode=plugin -o $@ .

clean:
	rm -f *.so
```

- [ ] **Step 2: 编写 Loader 失败测试**

`internal/plugin/loader_test.go`：

```go
package plugin

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestLoader_LoadPlugin_MissingSymbol(t *testing.T) {
    tmpDir := t.TempDir()
    // Create an empty .so (no InitPlugin symbol)
    emptyPath := filepath.Join(tmpDir, "empty.so")
    if err := os.WriteFile(emptyPath, []byte("not a real .so"), 0644); err != nil {
        t.Fatal(err)
    }
    l := NewLoader(tmpDir)
    _, err := l.LoadPlugin(emptyPath)
    if err == nil {
        t.Fatal("expected error for invalid .so")
    }
    // Should mention "missing" or "InitPlugin"
    if !strings.Contains(err.Error(), "InitPlugin") {
        t.Errorf("error = %q, want mention InitPlugin", err.Error())
    }
}

func TestLoader_LoadPlugin_FileNotExist(t *testing.T) {
    l := NewLoader(t.TempDir())
    _, err := l.LoadPlugin("/nonexistent/path/plugin.so")
    if err == nil {
        t.Fatal("expected error for nonexistent file")
    }
}

func TestLoader_ScanAndLoad_DirNotExist(t *testing.T) {
    l := NewLoader("/nonexistent/plugin/dir")
    plugins, err := l.ScanAndLoad()
    if err != nil {
        t.Fatalf("ScanAndLoad on nonexistent dir: %v", err)
    }
    if len(plugins) != 0 {
        t.Errorf("got %d plugins, want 0", len(plugins))
    }
}

func TestLoader_ScanAndLoad_EmptyDir(t *testing.T) {
    tmpDir := t.TempDir()
    l := NewLoader(tmpDir)
    plugins, err := l.ScanAndLoad()
    if err != nil {
        t.Fatalf("ScanAndLoad on empty dir: %v", err)
    }
    if len(plugins) != 0 {
        t.Errorf("got %d plugins, want 0", len(plugins))
    }
}
```

Run: `go test ./internal/plugin/ -run "TestLoader_" -v`
Expected: COMPILATION ERROR (no loader.go yet)

- [ ] **Step 3: 实现 Loader**

`internal/plugin/loader.go`：

```go
package plugin

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "plugin"
)

var (
    ErrMissingInitSymbol = errors.New("plugin: missing InitPlugin symbol")
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

// ScanAndLoad scans the pluginDir for all .so files, loads each one, and
// returns the successfully loaded plugins. Files that fail to load are
// skipped with a warning (logged to stderr). Returns an error only if the
// pluginDir cannot be read.
func (l *Loader) ScanAndLoad() ([]Plugin, error) {
    entries, err := os.ReadDir(l.pluginDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil // directory doesn't exist, no plugins to load
        }
        return nil, fmt.Errorf("plugin: scan dir %s: %w", l.pluginDir, err)
    }

    var plugins []Plugin
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
        plugins = append(plugins, p)
    }
    return plugins, nil
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/plugin/ -run "TestLoader_" -v`
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/plugin/loader.go internal/plugin/loader_test.go internal/plugin/testdata/
git commit -m "feat(plugin): add Loader for .so plugin loading"
```

---

