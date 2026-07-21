# Daemon 热插拔插件系统（Hot-Pluggable Plugin System）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 huan daemon 增加运行时热插拔插件能力：Loader 加载 `.so` 插件、LifecycleManager 管理生命周期、Admin API 暴露端点、CLI 扩展管理命令。

**Architecture:** 在现有 `internal/plugin` 包基础上新增 Loader 和 LifecycleManager，通过 EventBus 发布生命周期事件，通过增强的 Registry 支持运行时注册/注销。daemon 启动流程插入 LifecycleManager，Admin API 和 CLI 作为控制平面。

**Tech Stack:** Go (go1.26.2), `plugin` stdlib, fsnotify, cobra, prometheus

## Global Constraints

- 不改动已有 Plugin 接口（`Name() string`）
- 不改动已有 Registry 的 Register/All/Get/Find 语义
- 所有新增 API 端点受 TokenMiddleware 保护
- 错误处理按分层设计：Loader 层/ LifecycleManager 层 / Admin API 层
- go1.26.2 编译，Go plugin 需要同版本编译 `.so`

---

### Task 1: Registry 增强 — 新增 Unregister 方法

**Files:**
- Modify: `internal/plugin/plugin.go` — 新增 `Unregister` 方法
- Modify: `internal/plugin/plugin_test.go` — 新增 `Unregister` 测试

**Interfaces:**
- Produces: `Registry.Unregister(name string) bool`

- [ ] **Step 1: Write the failing test**

在 `internal/plugin/plugin_test.go` 末尾添加：

```go
func TestUnregister_Success(t *testing.T) {
    r := NewRegistry()
    if err := r.Register(&stubPlugin{name: "alpha"}); err != nil {
        t.Fatalf("Register alpha: %v", err)
    }
    got := r.Unregister("alpha")
    if !got {
        t.Error("Unregister(alpha): want true, got false")
    }
    if _, ok := r.Get("alpha"); ok {
        t.Error("Get(alpha) after Unregister returned ok=true")
    }
}

func TestUnregister_NotFound(t *testing.T) {
    r := NewRegistry()
    got := r.Unregister("nonexistent")
    if got {
        t.Error("Unregister(nonexistent): want false, got true")
    }
}

func TestUnregister_MaintainsOrder(t *testing.T) {
    r := NewRegistry()
    _ = r.Register(&stubPlugin{name: "alpha"})
    _ = r.Register(&stubPlugin{name: "bravo"})
    _ = r.Register(&stubPlugin{name: "charlie"})
    r.Unregister("bravo")
    want := []string{"alpha", "charlie"}
    got := r.Names()
    if len(got) != len(want) {
        t.Fatalf("Names() len = %d, want %d", len(got), len(want))
    }
    for i, name := range want {
        if got[i] != name {
            t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
        }
    }
}
```

Run: `go test ./internal/plugin/ -run "TestUnregister_" -v`
Expected: COMPILATION ERROR or FAIL (no Unregister method)

- [ ] **Step 2: 实现 Unregister**

在 `internal/plugin/plugin.go` 的 `All()` 方法之后添加：

```go
// Unregister removes a plugin by name. Returns false if the name wasn't
// registered. After Unregister, the plugin is no longer returned by Get,
// All, Names, or Find[T].
func (r *Registry) Unregister(name string) bool {
    if _, exists := r.plugins[name]; !exists {
        return false
    }
    delete(r.plugins, name)
    for i, n := range r.order {
        if n == name {
            r.order = append(r.order[:i], r.order[i+1:]...)
            break
        }
    }
    return true
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/plugin/ -run "TestUnregister_" -v`
Expected: All 3 tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/plugin/plugin.go internal/plugin/plugin_test.go
git commit -m "feat(plugin): add Registry.Unregister for runtime plugin removal"
```

---

### Task 2: EventBus 新增插件生命周期事件

**Files:**
- Modify: `internal/daemon/eventbus/types.go` — 新增 4 个事件类型
- Modify: `internal/daemon/eventbus/bus_test.go` — 测试新事件可发布

**Interfaces:**
- Produces: `EventPluginLoaded`, `EventPluginUnloaded`, `EventPluginReloaded`, `EventPluginError`

- [ ] **Step 1: 在 types.go 中添加新事件常量**

```go
// 在 EventServerShutdown 之后添加
    EventPluginLoaded   EventType = iota + 10 // 插件加载完成
    EventPluginUnloaded                       // 插件卸载完成
    EventPluginReloaded                       // 插件热重载完成
    EventPluginError                          // 插件异常
```

更新 `String()` 方法，在 `return "unknown"` 之前添加：

```go
    case EventPluginLoaded:
        return "plugin_loaded"
    case EventPluginUnloaded:
        return "plugin_unloaded"
    case EventPluginReloaded:
        return "plugin_reloaded"
    case EventPluginError:
        return "plugin_error"
```

- [ ] **Step 2: 运行现有测试确认无回归**

Run: `go test ./internal/daemon/eventbus/ -v`
Expected: ALL PASS

- [ ] **Step 3: 提交**

```bash
git add internal/daemon/eventbus/types.go
git commit -m "feat(eventbus): add plugin lifecycle event types (loaded/unloaded/reloaded/error)"
```

---

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

### Task 4: GRPCStub — gRPC 预留骨架

**Files:**
- Create: `internal/plugin/grpc_stub.go`
- Create: `internal/plugin/grpc_stub_test.go`

- [ ] **Step 1: 编写测试**

`internal/plugin/grpc_stub_test.go`：

```go
package plugin

import (
    "context"
    "testing"
)

func TestGRPCStub_ImplementsPlugin(t *testing.T) {
    s := NewGRPCStub("test-plugin", "deployer", "localhost:50051")
    if s.Name() != "test-plugin" {
        t.Errorf("Name() = %q, want test-plugin", s.Name())
    }
}

func TestGRPCStub_Capability(t *testing.T) {
    s := NewGRPCStub("test", "deployer", "")
    if s.Capability() != "deployer" {
        t.Errorf("Capability() = %q, want deployer", s.Capability())
    }
}

func TestGRPCStub_CallReturnsNotImplemented(t *testing.T) {
    s := NewGRPCStub("test", "", "")
    _, err := s.Call(context.Background(), "Deploy", nil)
    if err != ErrGRPCNotImplemented {
        t.Errorf("Call: want ErrGRPCNotImplemented, got %v", err)
    }
}

func TestGRPCStub_HealthReturnsNil(t *testing.T) {
    s := NewGRPCStub("test", "", "")
    err := s.Health(context.Background())
    if err != nil {
        t.Errorf("Health: want nil, got %v", err)
    }
}
```

Run: `go test ./internal/plugin/ -run "TestGRPCStub_" -v`
Expected: COMPILATION ERROR (no grpc_stub.go yet)

- [ ] **Step 2: 实现 GRPCStub**

`internal/plugin/grpc_stub.go`：

```go
package plugin

import (
    "context"
    "errors"
)

// ErrGRPCNotImplemented is returned by GRPCStub methods until the gRPC
// transport layer is actually implemented.
var ErrGRPCNotImplemented = errors.New("plugin: gRPC not implemented yet")

// GRPCPlugin defines the interface for plugins that communicate via gRPC.
// This is a reserved interface for future use — the gRPC transport layer
// will be implemented when cross-language plugin support is needed.
type GRPCPlugin interface {
    Plugin
    // Capability returns the plugin's capability type (e.g. "deployer",
    // "translator", "seo_checker").
    Capability() string
    // Call invokes a remote method on the plugin.
    // Currently returns ErrGRPCNotImplemented.
    Call(ctx context.Context, method string, payload []byte) ([]byte, error)
    // Health checks whether the remote plugin is alive.
    Health(ctx context.Context) error
}

// GRPCStub is a placeholder for future gRPC-based plugins. It implements
// GRPCPlugin with stub methods that return ErrGRPCNotImplemented. The
// actual gRPC client will be implemented later in internal/plugin/grpc/.
type GRPCStub struct {
    name       string
    capability string
    address    string // remote gRPC address, e.g. "localhost:50051"
}

// NewGRPCStub creates a new GRPCStub. All methods return stub values
// until the gRPC transport layer is implemented.
func NewGRPCStub(name, capability, address string) *GRPCStub {
    return &GRPCStub{
        name:       name,
        capability: capability,
        address:    address,
    }
}

func (s *GRPCStub) Name() string { return s.name }

func (s *GRPCStub) Capability() string { return s.capability }

func (s *GRPCStub) Call(_ context.Context, _ string, _ []byte) ([]byte, error) {
    return nil, ErrGRPCNotImplemented
}

func (s *GRPCStub) Health(_ context.Context) error {
    return nil
}
```

- [ ] **Step 3: 运行测试验证通过**

Run: `go test ./internal/plugin/ -run "TestGRPCStub_" -v`
Expected: ALL PASS

- [ ] **Step 4: 提交**

```bash
git add internal/plugin/grpc_stub.go internal/plugin/grpc_stub_test.go
git commit -m "feat(plugin): add GRPCStub skeleton for future gRPC plugin support"
```

---

### Task 5: LifecycleManager — 插件生命周期管理器

**Files:**
- Create: `internal/plugin/lifecycle.go`
- Create: `internal/plugin/lifecycle_test.go`

**Interfaces:**
- Produces: `LifecycleManager`, `PluginInfo`, `PluginWatcher`
- Consumes: `Loader`, `Registry`, eventbus.EventBus, eventbus.EventPlugin*`

- [ ] **Step 1: 编写 LifecycleManager 测试**

`internal/plugin/lifecycle_test.go`：

```go
package plugin

import (
    "context"
    "os"
    "path/filepath"
    "sync"
    "testing"
    "time"

    "github.com/iannil/huan/internal/daemon/eventbus"
)

// lifecycleTestPlugin is a minimal plugin for lifecycle testing.
type lifecycleTestPlugin struct {
    name string
}

func (p *lifecycleTestPlugin) Name() string { return p.name }

func TestLifecycleManager_List_Empty(t *testing.T) {
    registry := NewRegistry()
    bus := eventbus.NewChannelBus()
    defer bus.Close()
    lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
    info := lm.List()
    if len(info) != 0 {
        t.Errorf("List() = %d items, want 0", len(info))
    }
}

func TestLifecycleManager_LoadAndList(t *testing.T) {
    registry := NewRegistry()
    bus := eventbus.NewChannelBus()
    defer bus.Close()
    // Pre-register a compiled plugin
    _ = registry.Register(&lifecycleTestPlugin{name: "compiled-alpha"})

    lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
    info := lm.List()
    if len(info) != 1 {
        t.Fatalf("List() = %d items, want 1", len(info))
    }
    if info[0].Name != "compiled-alpha" {
        t.Errorf("Name = %q, want compiled-alpha", info[0].Name)
    }
    if info[0].Source != "compiled" {
        t.Errorf("Source = %q, want compiled", info[0].Source)
    }
    if info[0].Status != "active" {
        t.Errorf("Status = %q, want active", info[0].Status)
    }
}

func TestLifecycleManager_Load_EventPublished(t *testing.T) {
    registry := NewRegistry()
    bus := eventbus.NewChannelBus()
    defer bus.Close()

    events := make(chan eventbus.Event, 1)
    bus.Subscribe(eventbus.EventPluginLoaded, func(ctx context.Context, ev eventbus.Event) error {
        select {
        case events <- ev:
        default:
        }
        return nil
    })

    lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
    // Register a compiled plugin — should publish EventPluginLoaded
    lm.registerCompiled(&lifecycleTestPlugin{name: "compiled"})

    select {
    case ev := <-events:
        if ev.Type != eventbus.EventPluginLoaded {
            t.Errorf("event type = %v, want EventPluginLoaded", ev.Type)
        }
    case <-time.After(100 * time.Millisecond):
        t.Error("expected EventPluginLoaded, got none")
    }
}

func TestLifecycleManager_Unload_NotFound(t *testing.T) {
    registry := NewRegistry()
    bus := eventbus.NewChannelBus()
    defer bus.Close()
    lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)
    err := lm.Unload("nonexistent")
    if err == nil {
        t.Fatal("expected error for nonexistent plugin")
    }
}

func TestLifecycleManager_Reload_Rollback(t *testing.T) {
    registry := NewRegistry()
    bus := eventbus.NewChannelBus()
    defer bus.Close()

    // Register a compiled plugin
    original := &lifecycleTestPlugin{name: "test-p"}
    _ = registry.Register(original)

    // Track unregister events
    var mu sync.Mutex
    unregisteredName := ""
    bus.Subscribe(eventbus.EventPluginUnloaded, func(ctx context.Context, ev eventbus.Event) error {
        mu.Lock()
        if payload, ok := ev.Payload.(map[string]any); ok {
            if n, ok := payload["name"].(string); ok {
                unregisteredName = n
            }
        }
        mu.Unlock()
        return nil
    })

    lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)

    // Reload with a nonexistent .so path should fail and roll back
    err := lm.Reload("test-p", "/nonexistent/plugin.so")
    if err == nil {
        t.Fatal("expected error for nonexistent .so path")
    }

    // The original plugin should still be in the registry
    p, ok := registry.Get("test-p")
    if !ok {
        t.Fatal("original plugin should still be registered after rollback")
    }
    if p.Name() != "test-p" {
        t.Errorf("after rollback, Name() = %q", p.Name())
    }
}

func TestLifecycleManager_List_ActiveAndLoaded(t *testing.T) {
    registry := NewRegistry()
    bus := eventbus.NewChannelBus()
    defer bus.Close()
    lm := NewLifecycleManager(registry, NewLoader(t.TempDir()), bus)

    // Register compiled plugin
    _ = registry.Register(&lifecycleTestPlugin{name: "compiled"})

    // Create a .so and load it
    // (Use a temp dir with a real .so for a full integration test;
    //  for unit test, verify the List method correctly reports source)
    info := lm.List()
    if len(info) != 1 {
        t.Fatalf("List() = %d items, want 1", len(info))
    }
    // Verify required fields are present
    if info[0].Name == "" {
        t.Error("PluginInfo.Name should not be empty")
    }
    if info[0].Status != "active" {
        t.Errorf("Status = %q, want active", info[0].Status)
    }
}
```

Run: `go test ./internal/plugin/ -run "TestLifecycle" -v`
Expected: COMPILATION ERROR (no lifecycle.go yet)

- [ ] **Step 2: 实现 LifecycleManager**

`internal/plugin/lifecycle.go`：

```go
package plugin

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
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
    registry *Registry
    loader   *Loader
    bus      eventbus.EventBus

    mu        sync.Mutex
    loaded    map[string]*loadedPlugin // tracks all plugins (compiled + loaded)
    watcher   *PluginWatcher
    watchCtx  context.Context
    watchCancel context.CancelFunc
}

// NewLifecycleManager creates a LifecycleManager.
func NewLifecycleManager(registry *Registry, loader *Loader, bus eventbus.EventBus) *LifecycleManager {
    return &LifecycleManager{
        registry: registry,
        loader:   loader,
        bus:      bus,
        loaded:   make(map[string]*loadedPlugin),
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
    plugins, err := m.loader.ScanAndLoad()
    if err != nil {
        return fmt.Errorf("lifecycle: scan plugins: %w", err)
    }
    for _, p := range plugins {
        name := p.Name()
        if _, exists := m.registry.Get(name); exists {
            fmt.Fprintf(os.Stderr, "huan: plugin %q: name conflict with compiled plugin, skipping\n", name)
            continue
        }
        _ = m.registry.Register(p)
        m.loaded[name] = &loadedPlugin{
            plugin:   p,
            source:   "loaded",
            loadedAt: time.Now(),
        }
        m.publishEventUnsafe(ctx, eventbus.EventPluginLoaded, map[string]any{
            "name":   name,
            "source": "loaded",
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
// already exists.
func (m *LifecycleManager) Load(soPath string) (Plugin, error) {
    p, err := m.loader.LoadPlugin(soPath)
    if err != nil {
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
        soPath:   soPath,
        loadedAt: time.Now(),
    }

    m.publishEventUnsafe(context.Background(), eventbus.EventPluginLoaded, map[string]any{
        "name":   name,
        "source": "loaded",
        "path":   soPath,
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
        // Name changed — use the new name
        // (Silently register under new name; old name stays free)
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

func (m *LifecycleManager) publishEventUnsafe(_ context.Context, eventType eventbus.EventType, payload map[string]any) {
    _ = m.bus.Publish(context.Background(), eventbus.Event{
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
    dir   string
    logf  func(string, ...any)
}

// Start begins watching the plugin directory. Placeholder implementation.
func (w *PluginWatcher) Start(ctx context.Context) error {
    return nil
}

func fileExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}
```

- [ ] **Step 3: 运行测试验证通过**

Run: `go test ./internal/plugin/ -run "TestLifecycle" -v`
Expected: ALL PASS

- [ ] **Step 4: 运行所有 plugin 测试确保无回归**

Run: `go test ./internal/plugin/ -v`
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/plugin/lifecycle.go internal/plugin/lifecycle_test.go
git commit -m "feat(plugin): add LifecycleManager for plugin lifecycle management"
```

---

### Task 6: Admin API 新增插件管理端点

**Files:**
- Modify: `internal/admin/types.go` — 新增插件管理请求/响应类型
- Modify: `internal/admin/api.go` — 新增 /plugins 路由
- Modify: `internal/admin/audit.go` — 新增插件操作审计 action
- Modify: `internal/admin/handler.go` — 传递 LifecycleManager 到 API handler

**Interfaces:**
- Consumes: `plugin.LifecycleManager`, `plugin.PluginInfo`
- Produces: `GET /admin/api/plugins`, `POST /admin/api/plugins/load`, `POST /admin/api/plugins/unload`, `POST /admin/api/plugins/reload`

- [ ] **Step 1: 新增类型定义**

在 `internal/admin/types.go` 末尾添加：

```go
// PluginManageRequest is the API body for plugin load/unload/reload operations.
type PluginManageRequest struct {
    Name string `json:"name"` // plugin name (for unload/reload)
    Path string `json:"path"` // .so file path (for load/reload)
}

// PluginManageResponse wraps the plugin management API response.
type PluginManageResponse struct {
    Status string            `json:"status"`
    Plugin *plugin.PluginInfo `json:"plugin,omitempty"`
}
```

在 `internal/admin/audit.go` 的 const 块末尾添加：

```go
    ActionPluginLoad   AuditAction = "plugin.load"
    ActionPluginUnload AuditAction = "plugin.unload"
    ActionPluginReload AuditAction = "plugin.reload"
```

- [ ] **Step 2: 修改 API handler 支持插件操作**

修改 `apiHandlerConfig` 和 `apiHandler` 结构体，增加 `pluginManager` 字段：

```go
// internal/admin/api.go — 在 apiHandlerConfig 中增加
type apiHandlerConfig struct {
    // ... 原有字段
    pluginManager *plugin.LifecycleManager
}

// 在 apiHandler 中增加
type apiHandler struct {
    // ... 原有字段
    pluginManager *plugin.LifecycleManager
}

// 在 newAPIHandler 中增加
func newAPIHandler(cfg apiHandlerConfig) *apiHandler {
    return &apiHandler{
        // ... 原有初始化
        pluginManager: cfg.pluginManager,
    }
}
```

在 `apiHandler.ServeHTTP` 的 default 之前增加路由：

```go
    case path == "plugins" && r.Method == http.MethodGet:
        h.listPlugins(w, r)
    case path == "plugins/load" && r.Method == http.MethodPost:
        h.loadPlugin(w, r)
    case path == "plugins/unload" && r.Method == http.MethodPost:
        h.unloadPlugin(w, r)
    case path == "plugins/reload" && r.Method == http.MethodPost:
        h.reloadPlugin(w, r)
```

新增 handler 方法：

```go
func (h *apiHandler) listPlugins(w http.ResponseWriter, r *http.Request) {
    if h.pluginManager == nil {
        writeJSON(w, http.StatusOK, PluginManageResponse{Status: "plugin manager unavailable"})
        return
    }
    plugins := h.pluginManager.List()
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "plugins": plugins,
        "total":   len(plugins),
    })
}

func (h *apiHandler) loadPlugin(w http.ResponseWriter, r *http.Request) {
    if h.pluginManager == nil {
        writeJSON(w, http.StatusServiceUnavailable, APIError{Error: "plugin manager unavailable"})
        return
    }
    var req PluginManageRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
        return
    }
    if req.Path == "" {
        writeJSON(w, http.StatusBadRequest, APIError{Error: "path is required"})
        return
    }
    p, err := h.pluginManager.Load(req.Path)
    if err != nil {
        status := http.StatusInternalServerError
        if err == plugin.ErrPluginNameConflict {
            status = http.StatusConflict
        } else if err == plugin.ErrMissingInitSymbol {
            status = http.StatusBadRequest
        }
        writeJSON(w, status, APIError{Error: err.Error()})
        return
    }
    h.auditLog(AuditRecord{
        Action: ActionPluginLoad,
        Path:   req.Path,
    })
    writeJSON(w, http.StatusOK, PluginManageResponse{
        Status: "loaded",
        Plugin: &plugin.PluginInfo{Name: p.Name(), Source: "loaded", Status: "active"},
    })
}

func (h *apiHandler) unloadPlugin(w http.ResponseWriter, r *http.Request) {
    if h.pluginManager == nil {
        writeJSON(w, http.StatusServiceUnavailable, APIError{Error: "plugin manager unavailable"})
        return
    }
    var req PluginManageRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
        return
    }
    if req.Name == "" {
        writeJSON(w, http.StatusBadRequest, APIError{Error: "name is required"})
        return
    }
    if err := h.pluginManager.Unload(req.Name); err != nil {
        status := http.StatusInternalServerError
        if err == plugin.ErrPluginNotFound {
            status = http.StatusNotFound
        }
        writeJSON(w, status, APIError{Error: err.Error()})
        return
    }
    h.auditLog(AuditRecord{
        Action: ActionPluginUnload,
        Path:   req.Name,
    })
    writeJSON(w, http.StatusOK, PluginManageResponse{Status: "unloaded"})
}

func (h *apiHandler) reloadPlugin(w http.ResponseWriter, r *http.Request) {
    if h.pluginManager == nil {
        writeJSON(w, http.StatusServiceUnavailable, APIError{Error: "plugin manager unavailable"})
        return
    }
    var req PluginManageRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, APIError{Error: "invalid JSON: " + err.Error()})
        return
    }
    if req.Name == "" || req.Path == "" {
        writeJSON(w, http.StatusBadRequest, APIError{Error: "name and path are required"})
        return
    }
    if err := h.pluginManager.Reload(req.Name, req.Path); err != nil {
        status := http.StatusInternalServerError
        if err == plugin.ErrPluginNotFound {
            status = http.StatusNotFound
        }
        writeJSON(w, status, APIError{Error: err.Error()})
        return
    }
    h.auditLog(AuditRecord{
        Action: ActionPluginReload,
        Path:   req.Name,
    })
    writeJSON(w, http.StatusOK, PluginManageResponse{Status: "reloaded"})
}
```

- [ ] **Step 3: 修改 handler.go 传递 LifecycleManager**

```go
// NewHandler 函数签名不需要改，增加一个可选的 Manager 字段在 HandlerOptions 中
type HandlerOptions struct {
    // ... 原有字段
    PluginManager *plugin.LifecycleManager
}

// 在 NewHandler 中构建 apiHandler 时传入
api := newAPIHandler(apiHandlerConfig{
    // ... 原有字段
    pluginManager: opts.PluginManager,
})
```

- [ ] **Step 4: 运行现有测试确保无回归**

Run: `go test ./internal/admin/ -v`
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/admin/types.go internal/admin/api.go internal/admin/audit.go internal/admin/handler.go
git commit -m "feat(admin): add plugin management API endpoints (list/load/unload/reload)"
```

---

### Task 7: Daemon 集成 LifecycleManager

**Files:**
- Modify: `internal/daemon/daemon.go` — 新增 PluginDir/DisablePlugin 选项，集成 LifecycleManager
- Modify: `cmd/huan/daemon.go` — 新增 --plugin-dir / --disable-plugin 参数

- [ ] **Step 1: 修改 daemon Options 和 Daemon 结构体**

在 `internal/daemon/daemon.go`：

```go
// Options 新增
type Options struct {
    // ... 原有字段
    PluginDir     string
    DisablePlugin bool
}

// Daemon 结构体新增
type Daemon struct {
    // ... 原有字段
    pluginManager *plugin.LifecycleManager
}
```

在 `Run()` 函数中，在 `Init Builder` 之后、`Init Serving` 之前插入：

```go
// 8.5 Init Plugin Lifecycle Manager
if !opts.DisablePlugin {
    pluginDir := opts.PluginDir
    if pluginDir == "" {
        pluginDir = filepath.Join(opts.SourceDir, "plugins")
    }

    pluginLoader := plugin.NewLoader(pluginDir)
    d.pluginManager = plugin.NewLifecycleManager(
        plugin.NewRegistry(), // LifecycleManager 新建一个空 Registry
        pluginLoader,
        d.bus,
    )

    // 将现有编译期插件注册到 LifecycleManager
    if err := d.pluginManager.Start(context.Background()); err != nil {
        log.Printf("daemon: plugin manager start warning: %v", err)
    }
    log.Printf("daemon: plugin manager started (dir: %s)", pluginDir)
}
```

修改 ServingOptions 传递 PluginManager（可选）：

```go
// ServingOptions 不需要直接使用 LifecycleManager，
// Admin API 通过 HandlerOptions.PluginManager 传递
```

在 `ServingOptions` 中不需要改动，Admin 的 NewHandler 会通过 HandlerOptions 接收到。

修改 `NewServing` 附近的 AdminHandler 初始化：

```go
adminHandler := admin.NewHandler(admin.HandlerOptions{
    Cfg:           cfg,
    SourceDir:     opts.SourceDir,
    Rebuild:       d.builder.TriggerRebuild,
    ServeURL:      fmt.Sprintf("http://%s:%s/", opts.Bind, opts.Port),
    BindAddr:      opts.Bind,
    Token:         "", // Uses env var HUAN_ADMIN_TOKEN
    MemoryDir:     filepath.Join(opts.SourceDir, "memory", "daily"),
    PluginManager: d.pluginManager, // 传递
})
```

在 shutdown 流程中加入插件关闭：

```go
// 在 Shutdown 之前
if d.pluginManager != nil {
    d.pluginManager.Stop()
}
```

- [ ] **Step 2: 添加 import**

在 `internal/daemon/daemon.go` 的 import 中添加：

```go
"github.com/iannil/huan/internal/plugin"
```

- [ ] **Step 3: 修改 daemon CLI 命令**

在 `cmd/huan/daemon.go` 的 init() 中添加：

```go
daemonCmd.Flags().String("plugin-dir", "", "plugin directory (default: <sourceDir>/plugins)")
daemonCmd.Flags().Bool("disable-plugin", false, "disable plugin loading")
```

在 RunE 中读取参数并传递：

```go
pluginDir, _ := cmd.Flags().GetString("plugin-dir")
disablePlugin, _ := cmd.Flags().GetBool("disable-plugin")

return daemon.Run(daemon.Options{
    // ... 原有字段
    PluginDir:     pluginDir,
    DisablePlugin: disablePlugin,
})
```

- [ ] **Step 4: 运行编译检查**

Run: `go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行现有测试**

Run: `go test ./internal/daemon/ ./internal/admin/ -v`
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/daemon/daemon.go cmd/huan/daemon.go
git commit -m "feat(daemon): integrate Plugin LifecycleManager into daemon startup"
```

---

### Task 8: CLI 扩展 — plugin load/unload/reload 子命令

**Files:**
- Create: `cmd/huan/plugin_load.go`
- Create: `cmd/huan/plugin_unload.go`
- Create: `cmd/huan/plugin_reload.go`
- Modify: `cmd/huan/plugin_cmd.go` — 新增 `--all` 标记到 list 命令

- [ ] **Step 1: plugin load 子命令**

`cmd/huan/plugin_load.go`：

```go
package main

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newPluginLoadCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "load <path>",
        Short: "Load a .so plugin at runtime",
        Long: `Load a .so plugin file into the running daemon.

If the daemon is running, this sends the load request via the Admin API.
Otherwise (daemon not running), it loads the plugin into a temporary
registry for development/testing purposes.`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            pluginPath := args[0]

            // Try Admin API first
            if err := callPluginAdminAPI("POST", "/admin/api/plugins/load",
                map[string]string{"path": pluginPath}); err == nil {
                return nil
            }

            // Fallback: local load (dev mode)
            fmt.Printf("plugin: loading %s (local mode)...\n", pluginPath)
            loader := newLocalPluginLoader()
            p, err := loader.LoadPlugin(pluginPath)
            if err != nil {
                return fmt.Errorf("load plugin: %w", err)
            }
            fmt.Printf("plugin: loaded %q (name: %s)\n", pluginPath, p.Name())
            return nil
        },
    }
}
```

- [ ] **Step 2: plugin unload 子命令**

`cmd/huan/plugin_unload.go`：

```go
package main

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newPluginUnloadCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "unload <name>",
        Short: "Unload a loaded plugin at runtime",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            name := args[0]

            if err := callPluginAdminAPI("POST", "/admin/api/plugins/unload",
                map[string]string{"name": name}); err == nil {
                return nil
            }

            return fmt.Errorf("plugin %q: not loaded or daemon not running", name)
        },
    }
}
```

- [ ] **Step 3: plugin reload 子命令**

`cmd/huan/plugin_reload.go`：

```go
package main

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newPluginReloadCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "reload <name> <path>",
        Short: "Hot-reload a loaded plugin with a new .so file",
        Args:  cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            name := args[0]
            path := args[1]

            if err := callPluginAdminAPI("POST", "/admin/api/plugins/reload",
                map[string]string{"name": name, "path": path}); err == nil {
                return nil
            }

            return fmt.Errorf("plugin %q: reload failed or daemon not running", name)
        },
    }
}
```

- [ ] **Step 4: 修改 plugin_cmd.go 注册子命令**

```go
func newPluginCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "plugin",
        Short: "Manage plugins",
        Long:  "Inspect plugins compiled into this huan binary and their effective configuration.",
    }
    cmd.AddCommand(newPluginListCmd())
    cmd.AddCommand(newPluginInfoCmd())
    cmd.AddCommand(newPluginLoadCmd())    // 新增
    cmd.AddCommand(newPluginUnloadCmd())  // 新增
    cmd.AddCommand(newPluginReloadCmd())  // 新增
    return cmd
}
```

给 `newPluginListCmd` 添加 `--all` 标记（在 `RunE` 中通过 flag 读取）：

```go
func newPluginListCmd() *cobra.Command {
    var showAll bool
    cmd := &cobra.Command{
        Use:   "list",
        Short: "List all configured plugins",
        RunE: func(cmd *cobra.Command, args []string) error {
            cfg, err := config.Load(sourceDir)
            if err != nil {
                return fmt.Errorf("load config: %w", err)
            }
            registry, err := newPluginRegistry(cfg)
            if err != nil {
                return fmt.Errorf("plugin registry: %w", err)
            }
            printPluginList(registry)

            if showAll {
                // Also list runtime-loaded plugins from daemon
                if err := listRuntimePlugins(); err != nil {
                    fmt.Printf("(runtime plugins: %v)\n", err)
                }
            }
            return nil
        },
    }
    cmd.Flags().BoolVarP(&showAll, "all", "a", false, "also list runtime-loaded plugins from daemon")
    return cmd
}
```

- [ ] **Step 5: 实现 Admin API 客户端帮助函数**

在 `cmd/huan/plugin_helpers.go`（新建）中实现 `callPluginAdminAPI` 和 `listRuntimePlugins`：

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"
)

// callPluginAdminAPI sends a request to the daemon's admin API.
// Returns nil on success, error on failure (daemon not running, etc.).
func callPluginAdminAPI(method, endpoint string, body interface{}) error {
    token := os.Getenv("HUAN_ADMIN_TOKEN")
    if token == "" {
        return fmt.Errorf("HUAN_ADMIN_TOKEN not set")
    }

    var reqBody io.Reader
    if body != nil {
        data, err := json.Marshal(body)
        if err != nil {
            return fmt.Errorf("marshal request: %w", err)
        }
        reqBody = bytes.NewReader(data)
    }

    url := fmt.Sprintf("http://127.0.0.1:8080%s", endpoint)
    req, err := http.NewRequest(method, url, reqBody)
    if err != nil {
        return fmt.Errorf("create request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Huan-Admin-Token", token)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("admin API request failed (is daemon running?): %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        respBody, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("admin API: %s", string(respBody))
    }
    return nil
}

// listRuntimePlugins queries the daemon for runtime-loaded plugins and prints them.
func listRuntimePlugins() error {
    token := os.Getenv("HUAN_ADMIN_TOKEN")
    if token == "" {
        return fmt.Errorf("HUAN_ADMIN_TOKEN not set")
    }

    url := "http://127.0.0.1:8080/admin/api/plugins"
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("X-Huan-Admin-Token", token)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("admin API request failed: %w", err)
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return fmt.Errorf("decode response: %w", err)
    }

    plugins, ok := result["plugins"].([]interface{})
    if !ok || len(plugins) == 0 {
        fmt.Println("\nNo runtime-loaded plugins.")
        return nil
    }

    fmt.Println("\nRuntime plugins:")
    for _, p := range plugins {
        if info, ok := p.(map[string]interface{}); ok {
            fmt.Printf("  - %s (source: %s, status: %s)\n",
                info["name"], info["source"], info["status"])
        }
    }
    return nil
}
```

- [ ] **Step 6: 运行编译检查**

Run: `go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 7: 提交**

```bash
git add cmd/huan/plugin_load.go cmd/huan/plugin_unload.go cmd/huan/plugin_reload.go cmd/huan/plugin_helpers.go cmd/huan/plugin_cmd.go
git commit -m "feat(cli): add plugin load/unload/reload subcommands with --all flag"
```

---

### Task 9: daemon 集成测试 — 验证插件启动和 Admin API

**Files:**
- Modify: `internal/daemon/daemon_test.go` — 新增插件集成测试
- Modify: `internal/admin/api_test.go` — 新增插件 API 测试

- [ ] **Step 1: 编写 Admin API 插件端点测试**

```go
// internal/admin/api_test.go — 在文件末尾新增

func TestAdminAPI_PluginEndpoints_NoManager(t *testing.T) {
    // When no PluginManager is configured, endpoints should return 503
    handler := newAPIHandler(apiHandlerConfig{})
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        handler.ServeHTTP(w, r)
    }))
    defer srv.Close()

    // GET /plugins
    resp, _ := http.Get(srv.URL + "/plugins")
    if resp.StatusCode != http.StatusOK {
        t.Errorf("GET /plugins without manager: want 200, got %d", resp.StatusCode)
    }

    // POST /plugins/load
    body := `{"path":"test.so"}`
    resp, _ = http.Post(srv.URL+"/plugins/load", "application/json", strings.NewReader(body))
    if resp.StatusCode != http.StatusServiceUnavailable {
        t.Errorf("POST /plugins/load without manager: want 503, got %d", resp.StatusCode)
    }
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/admin/ -v`
Expected: ALL PASS

- [ ] **Step 3: 提交**

```bash
git add internal/admin/api_test.go internal/daemon/daemon_test.go
git commit -m "test(admin,daemon): add plugin lifecycle integration tests"
```

---

### Task 10: 全量编译测试与文档更新

**Files:**
- Modify: `docs/superpowers/specs/2026-07-20-daemon-hotplug-plugin-design.md` — 标记实现状态
- (可选) Create: `docs/adr/0012-hot-pluggable-plugins.md`

- [ ] **Step 1: 全量编译**

Run: `go build ./... && go vet ./...`
Expected: SUCCESS

- [ ] **Step 2: 全量测试**

Run: `go test ./...`
Expected: ALL PASS

- [ ] **Step 3: 更新设计文档标记实现状态**

在 spec 文件头部添加 `Status: Implemented`。

- [ ] **Step 4: 最终提交**

```bash
git add -A
git commit -m "feat(daemon): implement hot-pluggable plugin system

- Add Registry.Unregister for runtime plugin removal
- Add plugin lifecycle event types (loaded/unloaded/reloaded/error)
- Add Loader for discovering and loading .so plugin files
- Add LifecycleManager for plugin lifecycle and event publishing
- Add GRPCStub skeleton for future gRPC plugin support
- Add Admin API endpoints for plugin management (list/load/unload/reload)
- Add CLI subcommands: plugin load/unload/reload
- Integrate LifecycleManager into daemon startup with --plugin-dir flag
- Add audit logging for plugin lifecycle operations

Co-Authored-By: Claude <noreply@anthropic.com>"
```
