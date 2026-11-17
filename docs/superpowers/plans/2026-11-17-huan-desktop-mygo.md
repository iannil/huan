# huan 桌面端（MyGo）Phase 1 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 MyGo 框架构建 huan 桌面端 Phase 1「管理后台壳」：macOS .app，webview 承载现有 admin SPA，托盘/菜单提供启停 serve、一键 build、打开站点目录、切换站点。

**Architecture:** 同仓同模块 + 进程内嵌入。`cmd/huan-desktop` 入口装配 `internal/desktop`（MyGo 生命周期 + 站点状态机 + serve/build 运行时），webview 通过 `Page().LoadURL` 加载进程内 dev server 的 `/admin`，SPA 零改动。serve 装配复用 `internal/dev` + `internal/admin` + `internal/plugin` 的既有件，唯一重构是把 `newPluginRegistry`/`runImagePipeline` 从 `package main` 下沉到 internal 包供双入口共用。

**Tech Stack:** Go 1.26、`github.com/egoist/mygo` v0.9.2（纯 Go 无 cgo）、cobra（CLI 侧不动）、goldmark 构建管线（不动）。

**Spec:** `docs/superpowers/specs/2026-11-17-huan-desktop-mygo-design.md`（已确认）

## Global Constraints

- 模块 `github.com/iannil/huan`，Go 1.26，构建保持 `CGO_ENABLED=0` 可用（MyGo 自身无 cgo）
- MyGo 固定 `v0.9.2`（pre-1.0，不自动升级）；所有 MyGo 调用只允许出现在 `cmd/huan-desktop` 与 `internal/desktop`
- `web/admin` SPA 零改动；`cmd/huan` CLI 行为零变化（重构仅「下沉 + 委托」）
- 文档中文、代码与注释英文；时间戳 UTC；结构化 JSON 日志走 `internal/observability.Logger`
- 发布物归档 `release/`；平台目标 `darwin/arm64`
- **环境前置**：`internal/admin` 以 `//go:embed dist/*` 嵌入 admin SPA——任何 `go build` 前必须存在 `internal/admin/dist`（缺失时先 `cd web/admin && npm ci && npm run build`）
- **MyGo pre-1.0 核对程序**：每个用到 MyGo 符号的任务，第一步运行 `go doc github.com/egoist/mygo <Symbol>`（或 `go doc github.com/egoist/mygo <Type>.<Field>`）核对签名；若与计划代码有出入，以 go doc 实际输出为准微调字段/方法名，语义保持计划所述——这是核对步骤，不是占位符
- 批量改动前按 CLAUDE.md 约定备份原文件到 `backup/`（确认已被 .gitignore 覆盖，未提交）
- 每个任务的进展随时追加到 `docs/progress/2026-11-17-desktop-phase1.md`（Task 1 创建），带实际时间

---

### Task 1: 环境准备 + MyGo 骨架 spike

**Files:**
- Create: `mygo.json`
- Create: `cmd/huan-desktop/main.go`
- Create: `internal/desktop/app.go`（spike 版）
- Create: `internal/desktop/assets/.gitkeep`
- Create: `docs/progress/2026-11-17-desktop-phase1.md`

**Interfaces:**
- Consumes: 无（首个任务）
- Produces: `internal/desktop.Run() error`（spike 版：打开窗口加载 example.com，验证 webview + LoadURL + 生命周期通路）；`mygo.json` 打包配置

- [ ] **Step 1: 环境前置检查**

```bash
# admin SPA dist 必须存在（embed 硬依赖）
ls internal/admin/dist/index.html || (cd web/admin && npm ci && npm run build)
# 安装固定版本的 mygo CLI
go install github.com/egoist/mygo/cmd/mygo@v0.9.2
mygo --version
# 引入框架依赖
go get github.com/egoist/mygo@v0.9.2
```

- [ ] **Step 2: 核对 MyGo 应用生命周期 API**

```bash
go doc github.com/egoist/mygo App        # 确认 WhenReady / Run / Quit
go doc github.com/egoist/mygo NewWindow  # 确认签名
go doc github.com/egoist/mygo WindowOptions
go doc github.com/egoist/mygo Window.Page
go doc github.com/egoist/mygo Page.LoadURL
```

记录实际签名；后续代码按实际输出微调（语义不变：WhenReady 里建窗口、LoadURL 加载、App.Run 阻塞运行）。

- [ ] **Step 3: 写 mygo.json**

```json
{
  "name": "huan",
  "slug": "huan",
  "identifier": "io.github.iannil.huan.desktop",
  "version": "0.13.0",
  "main": "cmd/huan-desktop",
  "out": "build/desktop"
}
```

- [ ] **Step 4: 写 spike 版 app.go**

```go
// Package desktop implements the huan desktop app shell (MyGo-based).
// Phase 1: a webview hosting the embedded admin SPA plus native tray/menu.
package desktop

import (
	"fmt"

	"github.com/egoist/mygo"
)

// Run starts the huan desktop app and blocks until the app quits.
func Run() error {
	ready := make(chan struct{})
	mygo.App.WhenReady(func() {
		defer close(ready)
		win, err := mygo.NewWindow(mygo.WindowOptions{
			Title:  "huan",
			Width:  1280,
			Height: 800,
		})
		if err != nil {
			mygo.Dialog.Error("huan", fmt.Sprintf("create window: %v", err))
			return
		}
		// Spike: prove the webview can load a running HTTP origin.
		if err := win.Page().LoadURL("https://example.com"); err != nil {
			mygo.Dialog.Error("huan", fmt.Sprintf("load url: %v", err))
		}
	})
	return mygo.App.Run()
}
```

- [ ] **Step 5: 写入口 main.go**

```go
// Command huan-desktop is the huan desktop app (MyGo shell, Phase 1).
package main

import (
	"fmt"
	"os"

	"github.com/iannil/huan/internal/desktop"
)

func main() {
	if err := desktop.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "huan-desktop:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: 编译并手动验证**

```bash
mkdir -p internal/desktop/assets && touch internal/desktop/assets/.gitkeep
go build ./cmd/huan-desktop && CGO_ENABLED=0 go build ./cmd/huan-desktop && go vet ./internal/desktop/ ./cmd/huan-desktop/
go run ./cmd/huan-desktop   # 预期：弹出 1280x800 窗口，加载 example.com；手动关闭
```

Expected: 编译通过（含 CGO_ENABLED=0），窗口正常显示 example.com。若 go doc 核对发现 API 偏差，按实际签名修正后重跑。

- [ ] **Step 7: 创建进度文档并提交**

`docs/progress/2026-11-17-desktop-phase1.md` 初始内容：标题、spec 链接、本计划链接、`## 20:xx Task 1 完成：MyGo spike 通过（窗口 + LoadURL 验证）` 与实际时间。

```bash
git add mygo.json cmd/huan-desktop internal/desktop docs/progress/2026-11-17-desktop-phase1.md go.mod go.sum
git commit -m "feat(desktop): scaffold MyGo desktop shell with webview spike"
```

---

### Task 2: 下沉插件注册与图片管线到 internal 包

**Files:**
- Create: `internal/plugin/registry_config.go`
- Create: `internal/plugin/registry_config_test.go`
- Create: `internal/image/pipeline.go`
- Create: `internal/image/pipeline_test.go`
- Modify: `cmd/huan/plugins.go`（`newPluginRegistry`/`pluginDirFromSource` 改为委托）
- Modify: `cmd/huan/build_image.go`（`runImagePipeline` 改为委托）
- Modify: `internal/dev/server.go` 新增 `MarkdownOnlyChanges`；`cmd/huan/dev.go:289-308` 的 `devMarkdownOnlyChanges` 改为委托
- Backup: `backup/cmd-huan-plugins.go`, `backup/cmd-huan-build_image.go`, `backup/cmd-huan-dev.go`

**Interfaces:**
- Consumes: `plugin.NewRegistry()`、`plugin.NewLoader(dir)`、`loader.ScanAndLoadByCategory(...)`、`plugin.ValidateRawConfigs(...)`（均为 internal/plugin 既有 API，原样搬移）
- Produces（后续任务依赖的精确签名）:
  - `plugin.NewRegistryFromConfig(cfg *config.Config, sourceDir, pluginDirOverride string) (*plugin.Registry, error)`
  - `plugin.DirFromSource(sourceDir string) string`
  - `image.RunProcessors(outputDir, sourceDir string, registry *plugin.Registry) error`
  - `dev.MarkdownOnlyChanges(source string, paths []string) bool`

- [ ] **Step 1: 备份并将三个函数体原样搬入 internal 包**

```bash
mkdir -p backup && grep -q "^backup/" .gitignore || echo "backup/" >> .gitignore
cp cmd/huan/plugins.go backup/cmd-huan-plugins.go
cp cmd/huan/build_image.go backup/cmd-huan-build_image.go
cp cmd/huan/dev.go backup/cmd-huan-dev.go
```

`internal/plugin/registry_config.go`：把 `cmd/huan/plugins.go:26-64` `newPluginRegistry` 的完整函数体（含注释）搬入，重命名为 `NewRegistryFromConfig`；`pluginDirFromSource` 搬入为 `DirFromSource`。包内引用不变（`plugin.NewRegistry` 等去掉包限定）。

```go
package plugin

import (
	"fmt"
	"os"
	"strings"

	"github.com/iannil/huan/internal/config"
)

// NewRegistryFromConfig is the composition root for the unified plugin system
// (ADR 0003 §7). It loads all .so plugins from the plugins/ directory that
// are declared in huan.yaml with category "static" or "mixed", registers
// them with a fresh Registry, and validates their configs against schemas.
//
// Plugins with category "dynamic" are excluded — they are loaded at runtime
// by the daemon's LifecycleManager.
//
// Unknown plugins declared in yaml (not found as .so files) are silently
// skipped at compile time.
func NewRegistryFromConfig(cfg *config.Config, sourceDir string, pluginDirOverride string) (*Registry, error) {
	r := NewRegistry()
	pluginDir := DirFromSource(sourceDir)
	if pluginDirOverride != "" {
		pluginDir = pluginDirOverride
	}
	loader := NewLoader(pluginDir)

	// Scan and load all .so files, filter by category
	results, err := loader.ScanAndLoadByCategory(cfg.Plugins, CategoryStatic, CategoryMixed)
	if err != nil {
		return nil, fmt.Errorf("plugin: scan static plugins: %w", err)
	}
	for _, result := range results {
		name := result.Plugin.Name()
		if _, exists := r.Get(name); exists {
			fmt.Fprintf(os.Stderr, "huan: plugin %q: name conflict, skipping\n", name)
			continue
		}
		if err := r.Register(result.Plugin); err != nil {
			fmt.Fprintf(os.Stderr, "huan: plugin %q: register error: %v\n", name, err)
		}
	}

	// Validate configs against schemas. A plugin declared in yaml but not in
	// this build-stage registry (dynamic plugins) is fine as long as its .so
	// is resolvable — it will be loaded at runtime — so only warn on a truly
	// missing .so.
	soExists := func(name string) bool { return loader.Resolve(SoFileName(name)) != "" }
	if errs, warns := ValidateRawConfigs(r, cfg.Plugins, soExists); len(errs) > 0 || len(warns) > 0 {
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "huan: plugin config warning: %s\n", w)
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("plugin config errors:\n  - %s", strings.Join(errs, "\n  - "))
		}
	}
	return r, nil
}

// DirFromSource returns the plugins directory path relative to sourceDir.
func DirFromSource(sourceDir string) string {
	return sourceDir + "/plugins"
}
```

`internal/image/pipeline.go`：把 `cmd/huan/build_image.go` 的 `runImagePipeline` 函数体（含注释）搬入：

```go
package image

import (
	"fmt"

	"github.com/iannil/huan/internal/plugin"
)

// RunProcessors executes any registered image-processing plugin after a
// build. Plugins are selected by the ImageProcessor capability interface —
// no plugin name is hardcoded here.
//
// Registration model: build-time image plugins are loaded via the static/mixed
// plugin registry (like seo_injector/sitemap_enhancer); in daemon mode they are
// loaded by the LifecycleManager. Either way they appear in `registry`, so this
// function just discovers them by capability. If none is registered there is
// nothing to do.
func RunProcessors(outputDir, sourceDir string, registry *plugin.Registry) error {
	for _, p := range plugin.Find[ImageProcessor](registry) {
		if err := p.Process(outputDir, sourceDir); err != nil {
			return fmt.Errorf("image pipeline: process: %w", err)
		}
	}
	return nil
}
```

`internal/dev/server.go` 追加（从 `cmd/huan/dev.go:289-308` 搬入，含注释）：

```go
// MarkdownOnlyChanges keeps pure Markdown results for content edits. Unknown
// inputs conservatively invalidate them; no filesystem stat is needed, so
// deletions and editor atomic-save renames are classified the same as writes.
func MarkdownOnlyChanges(source string, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	root, err := filepath.Abs(filepath.Join(source, "content"))
	if err != nil {
		return false
	}
	for _, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Ext(rel) != ".md" {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: cmd/huan 三个原函数改为委托**

```go
// cmd/huan/plugins.go — 替换函数体，签名与注释保留：
func newPluginRegistry(cfg *config.Config, sourceDir string, pluginDirOverride string) (*plugin.Registry, error) {
	return plugin.NewRegistryFromConfig(cfg, sourceDir, pluginDirOverride)
}

func pluginDirFromSource(sourceDir string) string {
	return plugin.DirFromSource(sourceDir)
}

// cmd/huan/build_image.go：
func runImagePipeline(sourceDir, outputDir string, registry *plugin.Registry) error {
	return image.RunProcessors(outputDir, sourceDir, registry)
}

// cmd/huan/dev.go：
func devMarkdownOnlyChanges(source string, paths []string) bool {
	return dev.MarkdownOnlyChanges(source, paths)
}
```

清理三个文件因搬移而不再使用的 import。

- [ ] **Step 3: 写 internal 包测试**

`internal/plugin/registry_config_test.go`：

```go
package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/config"
)

func TestNewRegistryFromConfig_EmptyConfigYieldsEmptyRegistry(t *testing.T) {
	cfg := &config.Config{}
	reg, err := NewRegistryFromConfig(cfg, t.TempDir(), "")
	if err != nil {
		t.Fatalf("NewRegistryFromConfig: %v", err)
	}
	if len(reg.All()) != 0 {
		t.Errorf("registry not empty: %d plugins", len(reg.All()))
	}
}

func TestNewRegistryFromConfig_UnknownPluginSilentlySkipped(t *testing.T) {
	dir := t.TempDir()
	// huan.yaml declares a plugin whose .so does not exist — must not error.
	// Mirror of cmd/huan TestNewPluginRegistry_UnknownPluginSilentlySkipped.
	cfg := &config.Config{Plugins: map[string]map[string]any{
		"no_such_plugin": {"enabled": true},
	}}
	reg, err := NewRegistryFromConfig(cfg, dir, filepath.Join(dir, "plugins"))
	if err != nil {
		t.Fatalf("unknown plugin should be silently skipped, got error: %v", err)
	}
	if _, exists := reg.Get("no_such_plugin"); exists {
		t.Errorf("plugin %q should not be registered", "no_such_plugin")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("source dir untouched check failed: %v", err)
	}
}
```

`internal/image/pipeline_test.go`：

```go
package image

import (
	"testing"

	"github.com/iannil/huan/internal/plugin"
)

// stubImageProcessor is a minimal ImageProcessor for pipeline tests.
type stubImageProcessor struct{ name string }

func (s *stubImageProcessor) Name() string                  { return s.name }
func (s *stubImageProcessor) Process(_, _ string) error     { return nil }

var _ ImageProcessor = (*stubImageProcessor)(nil)
var _ plugin.Plugin = (*stubImageProcessor)(nil)

func TestRunProcessors_EmptyRegistryIsNoop(t *testing.T) {
	if err := RunProcessors(t.TempDir(), t.TempDir(), plugin.NewRegistry()); err != nil {
		t.Fatalf("RunProcessors with empty registry: %v", err)
	}
}
```

- [ ] **Step 4: 运行测试与全量回归**

```bash
go test ./internal/plugin/ ./internal/image/ ./internal/dev/ ./cmd/...
go build ./... && go vet ./...
```

Expected: 全部 PASS（cmd 既有测试经由委托继续通过）。`stubImageProcessor` 若与 internal/image 已有测试桩重名，改用包内已有桩或加唯一后缀。

- [ ] **Step 5: 提交**

```bash
git add internal/plugin internal/image internal/dev cmd/huan .gitignore
git commit -m "refactor: move plugin registry, image pipeline, markdown-change helpers into internal packages"
```

---

### Task 3: 桌面配置持久化（internal/desktop/config.go）

**Files:**
- Create: `internal/desktop/config.go`
- Create: `internal/desktop/config_test.go`

**Interfaces:**
- Consumes: 标准库 `os`/`encoding/json`
- Produces:
  - `type DesktopConfig struct { RecentSites []string \`json:"recentSites"\` }`
  - `func DefaultConfigDir() (string, error)`（`os.UserConfigDir()/huan-desktop`）
  - `func ConfigPath() (string, error)`（`DefaultConfigDir()/config.json`）
  - `func Load(path string) (*DesktopConfig, error)`（文件缺失 → 默认值，无错误）
  - `func LoadOrDefault(path string) (*DesktopConfig, error)`（损坏 → 备份 `.bak` 后返回默认值）
  - `func Save(path string, cfg *DesktopConfig) error`
  - `func (c *DesktopConfig) AddRecentSite(dir string)`（abs+clean、去重置顶、上限 10）

- [ ] **Step 1: 写失败测试**

```go
package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if len(cfg.RecentSites) != 0 {
		t.Errorf("expected empty recent sites, got %v", cfg.RecentSites)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &DesktopConfig{}
	cfg.AddRecentSite("/tmp/site-a")
	cfg.AddRecentSite("/tmp/site-b")
	cfg.AddRecentSite("/tmp/site-a") // dup moves to front
	if cfg.RecentSites[0] != "/tmp/site-a" || len(cfg.RecentSites) != 2 {
		t.Fatalf("recent order wrong: %v", cfg.RecentSites)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.RecentSites[0] != "/tmp/site-a" || len(got.RecentSites) != 2 {
		t.Errorf("round trip mismatch: %v", got.RecentSites)
	}
}

func TestLoadOrDefault_CorruptFileBackedUpAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	cfg, err := LoadOrDefault(path)
	if err != nil {
		t.Fatalf("LoadOrDefault should recover, got: %v", err)
	}
	if len(cfg.RecentSites) != 0 {
		t.Errorf("expected default config, got %v", cfg.RecentSites)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("corrupt file not backed up: %v", err)
	}
}

func TestAddRecentSite_CapTenDedupToFront(t *testing.T) {
	cfg := &DesktopConfig{}
	for i := 0; i < 12; i++ {
		cfg.AddRecentSite(string(rune('a'+i))) // 12 distinct
	}
	if len(cfg.RecentSites) != 10 {
		t.Fatalf("cap failed: %d", len(cfg.RecentSites))
	}
	oldest := cfg.RecentSites[len(cfg.RecentSites)-1]
	cfg.AddRecentSite(oldest)
	if cfg.RecentSites[0] != oldest || len(cfg.RecentSites) != 10 {
		t.Errorf("re-add should move to front without growth: %v", cfg.RecentSites)
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.json")
	if err := Save(path, &DesktopConfig{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var cfg DesktopConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/desktop/ -run TestLoad -run TestSave -run TestAddRecent -v
```

Expected: FAIL（`config.go` 不存在，编译错误）。

- [ ] **Step 3: 实现 config.go**

```go
package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// appName is the directory name used under the user config dir.
const appName = "huan-desktop"

// maxRecentSites caps the recent-sites list.
const maxRecentSites = 10

// DesktopConfig is the persisted desktop app configuration.
type DesktopConfig struct {
	RecentSites []string `json:"recentSites"`
}

// DefaultConfigDir returns the per-user config directory for the desktop app.
func DefaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, appName), nil
}

// ConfigPath returns the config file path.
func ConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file; a missing file yields the default config.
func Load(path string) (*DesktopConfig, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &DesktopConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg DesktopConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// LoadOrDefault reads the config, recovering from a corrupt file by backing
// it up and returning defaults. Startup must never be blocked by config.
func LoadOrDefault(path string) (*DesktopConfig, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	_ = os.Rename(path, path+".bak")
	return &DesktopConfig{}, nil
}

// Save atomically writes the config, creating parent directories.
func Save(path string, cfg *DesktopConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return os.Rename(tmp, path)
}

// AddRecentSite records a site directory as most-recent, deduplicated,
// capped at maxRecentSites.
func (c *DesktopConfig) AddRecentSite(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	out := make([]string, 0, maxRecentSites)
	out = append(out, abs)
	for _, s := range c.RecentSites {
		if s != abs && len(out) < maxRecentSites {
			out = append(out, s)
		}
	}
	c.RecentSites = out
}
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./internal/desktop/ -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/desktop/config.go internal/desktop/config_test.go
git commit -m "feat(desktop): persist desktop config with recent-sites list"
```

---

### Task 4: 站点状态机与校验（internal/desktop/state.go）

**Files:**
- Create: `internal/desktop/state.go`
- Create: `internal/desktop/state_test.go`

**Interfaces:**
- Consumes: `config.Load(sourceDir)`（`internal/config` 既有）
- Produces:
  - `type State int32`；常量 `StateIdle, StateServing, StateBuilding, StateError`；`func (s State) String() string`
  - `type Machine struct`；`func NewMachine() *Machine`；`func (m *Machine) Current() State`；`func (m *Machine) Transition(from, to State) bool`（原子 CAS，并发下只有一个赢家）
  - `func ValidateSite(sourceDir string) error`（`huan.yaml` 存在 + `config.Load` 通过）

- [ ] **Step 1: 写失败测试**

```go
package desktop

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestTransition_ValidAndInvalid(t *testing.T) {
	m := NewMachine()
	if m.Current() != StateIdle {
		t.Fatalf("initial state = %v, want idle", m.Current())
	}
	if !m.Transition(StateIdle, StateServing) {
		t.Fatal("idle→serving should succeed")
	}
	if m.Transition(StateIdle, StateBuilding) {
		t.Fatal("serving→building must fail when from=idle")
	}
	if !m.Transition(StateServing, StateBuilding) {
		t.Fatal("serving→building should succeed")
	}
	if !m.Transition(StateBuilding, StateServing) {
		t.Fatal("building→serving should succeed")
	}
	if !m.Transition(StateServing, StateIdle) {
		t.Fatal("serving→idle should succeed")
	}
	if !m.Transition(StateIdle, StateError) {
		t.Fatal("idle→error should succeed")
	}
	if !m.Transition(StateError, StateIdle) {
		t.Fatal("error→idle (reset) should succeed")
	}
}

func TestTransition_ConcurrentOnlyOneWinner(t *testing.T) {
	m := NewMachine()
	var wg sync.WaitGroup
	wins := 0
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if m.Transition(StateIdle, StateServing) {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("expected exactly 1 winner, got %d", wins)
	}
	if m.Current() != StateServing {
		t.Errorf("state = %v, want serving", m.Current())
	}
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateIdle: "idle", StateServing: "serving",
		StateBuilding: "building", StateError: "error",
		State(99): "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestValidateSite(t *testing.T) {
	if err := ValidateSite(t.TempDir()); err == nil {
		t.Fatal("empty dir should fail validation")
	}
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "huan.yaml"),
		[]byte("baseURL: https://example.com/\ntitle: T\nlanguageCode: en\n"), 0o644)
	if err := ValidateSite(src); err != nil {
		t.Fatalf("valid site rejected: %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/desktop/ -run "TestTransition|TestState|TestValidateSite" -v
```

Expected: FAIL（编译错误，state.go 不存在）。

- [ ] **Step 3: 实现 state.go**

```go
package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/iannil/huan/internal/config"
)

// State enumerates the desktop site runtime states.
type State int32

const (
	StateIdle     State = iota // no server running
	StateServing               // dev server running
	StateBuilding              // production build in progress
	StateError                 // last operation failed; recover via idle
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateServing:
		return "serving"
	case StateBuilding:
		return "building"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Machine is a tiny CAS-based state machine; concurrent transitions are
// idempotent — exactly one caller wins.
type Machine struct {
	cur atomic.Int32
}

// NewMachine returns a machine in StateIdle.
func NewMachine() *Machine {
	m := &Machine{}
	m.cur.Store(int32(StateIdle))
	return m
}

// Current reports the current state.
func (m *Machine) Current() State { return State(m.cur.Load()) }

// Transition moves from → to atomically; reports whether it won.
func (m *Machine) Transition(from, to State) bool {
	return m.cur.CompareAndSwap(int32(from), int32(to))
}

// ValidateSite checks that sourceDir is a usable huan site: huan.yaml exists
// and config loads. The explicit file check gives a clearer message than a
// bare config error when the user picked a non-site directory.
func ValidateSite(sourceDir string) error {
	if _, err := os.Stat(filepath.Join(sourceDir, "huan.yaml")); err != nil {
		return fmt.Errorf("%s is not a huan site: huan.yaml not found", sourceDir)
	}
	if _, err := config.Load(sourceDir); err != nil {
		return fmt.Errorf("load site config: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./internal/desktop/ -v
```

Expected: PASS（含 Task 3 的测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/desktop/state.go internal/desktop/state_test.go
git commit -m "feat(desktop): site state machine and site validation"
```

---

### Task 5: 进程内 serve 运行时（internal/desktop/server.go）

**Files:**
- Create: `internal/desktop/server.go`
- Create: `internal/desktop/server_test.go`

**Interfaces:**
- Consumes: `config.Load`、`admin.ResolveToken/GenerateToken/CheckBindSafety/NewHandler`、`dev.New/NewHub/NewWatcher/NewRebuildQueue/MarkdownOnlyChanges/WatcherOptions`、`build.Options/NewSite/BuildMultiSite/NewTimings/NewMarkdownCache`、`content.NewParseCache`、`plugin.NewRegistryFromConfig/DirFromSource/NewLoader/NewLifecycleManager`、`theme.NewManager`、`eventbus.NewChannelBus`、`image.RunProcessors`、`observability.NewLoggerWithWriter`（Task 2 的 Produces）
- Produces:
  - `type ServeOptions struct { SourceDir string; Logf func(format string, a ...any) }`
  - `func NewServeRuntime(opts ServeOptions) *ServeRuntime`
  - `func (r *ServeRuntime) Start() (adminURL string, err error)`（装配置/插件/主题 → 初次构建 → watcher + rebuild queue → dev server 随机端口；就绪后返回 `http://127.0.0.1:<port>/admin/`）
  - `func (r *ServeRuntime) Stop(timeout time.Duration) error`（幂等；优雅停 HTTP + watcher + queue + 清理 tmpDir）
  - `func (r *ServeRuntime) Running() bool`；`AdminURL() string`；`SiteURL() string`；`SourceDir() string`；`Config() *config.Config`
  - 回调字段：`OnServeExited func(err error)`（serve goroutine 异常退出，含 panic recover）、`OnRebuildFailed func(err error)`

- [ ] **Step 1: 写失败测试（夹具照搬 `internal/build/incremental_test.go` 的 writeSite 模式）**

```go
package desktop

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSite creates a minimal but complete site: huan.yaml + two layouts +
// two content pages (same shape as internal/build/incremental_test.go).
func writeSite(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("huan.yaml", "baseURL: https://example.com/\ntitle: Desktop Test\nlanguageCode: en\n")
	write(filepath.Join("layouts", "_default", "single.html"),
		"<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>\n<body><article><h1>{{ .Title }}</h1><div class=\"body\">{{ .Content }}</div></article></body></html>\n")
	write(filepath.Join("layouts", "_default", "list.html"),
		"<!DOCTYPE html><html><head><title>{{ .Title }}</title></head>\n<body><section><h1>{{ .Title }}</h1>{{ range .Data.Pages }}<a href=\"{{ .Permalink }}\">{{ .Title }}</a>{{ end }}</section></body></html>\n")
	write(filepath.Join("content", "posts", "alpha.md"),
		"---\ntitle: \"Alpha\"\ndate: 2026-07-01T00:00:00Z\n---\nOriginal alpha body.\n")
	write(filepath.Join("content", "posts", "beta.md"),
		"---\ntitle: \"Beta\"\ndate: 2026-07-02T00:00:00Z\n---\nOriginal beta body.\n")
	return src
}

func TestServeRuntime_StartServeStop(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "admin", "dist", "index.html")); err != nil {
		t.Skip("internal/admin/dist missing — build admin SPA first (cd web/admin && npm ci && npm run build)")
	}
	src := writeSite(t)
	exited := make(chan error, 1)
	rt := NewServeRuntime(ServeOptions{SourceDir: src})
	rt.OnServeExited = func(err error) { exited <- err }

	adminURL, err := rt.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rt.Stop(5 * time.Second)
	if !rt.Running() {
		t.Fatal("runtime should be running after Start")
	}
	if !strings.HasPrefix(adminURL, "http://127.0.0.1:") || !strings.HasSuffix(adminURL, "/admin/") {
		t.Fatalf("adminURL malformed: %q", adminURL)
	}
	if rt.SiteURL() == "" || rt.Config() == nil {
		t.Fatal("SiteURL/Config must be populated after Start")
	}

	// Site home renders the list page with content.
	resp, err := http.Get(rt.SiteURL())
	if err != nil {
		t.Fatalf("GET site: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Alpha") {
		t.Errorf("site home: status=%d contains Alpha=%v", resp.StatusCode, strings.Contains(string(body), "Alpha"))
	}

	// Admin SPA is served at the admin URL.
	resp2, err := http.Get(adminURL)
	if err != nil {
		t.Fatalf("GET admin: %v", err)
	}
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("admin status = %d, want 200", resp2.StatusCode)
	}

	if err := rt.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if rt.Running() {
		t.Fatal("runtime should not be running after Stop")
	}
	if _, err := http.Get(rt.SiteURL()); err == nil {
		t.Error("site should refuse connections after Stop")
	}
	select {
	case err := <-exited:
		if err != nil && err != context.Canceled {
			t.Errorf("serve exited with unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("OnServeExited not fired within 2s of Stop")
	}
}

func TestServeRuntime_StopWithoutStartIsNoop(t *testing.T) {
	rt := NewServeRuntime(ServeOptions{SourceDir: t.TempDir()})
	if err := rt.Stop(time.Second); err != nil {
		t.Fatalf("Stop without Start: %v", err)
	}
}

func TestServeRuntime_StartRejectsInvalidSite(t *testing.T) {
	rt := NewServeRuntime(ServeOptions{SourceDir: t.TempDir()})
	if _, err := rt.Start(); err == nil {
		t.Fatal("Start on non-site dir should fail")
	}
	if rt.Running() {
		t.Fatal("runtime must not be running after failed Start")
	}
}
```

（测试文件顶部 import 需含 `context`。）

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/desktop/ -run TestServeRuntime -v
```

Expected: FAIL（server.go 不存在，编译错误）。

- [ ] **Step 3: 实现 server.go**

```go
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
// LiveReload). Start/Stop are serialized internally; concurrent Start calls
// are idempotent (the second caller gets the existing URL once ready).
type ServeRuntime struct {
	opts ServeOptions
	log  *observability.Logger
	logf func(format string, a ...any)

	mu       sync.Mutex
	starting bool          // guards concurrent Start calls
	cancel   context.CancelFunc
	done     chan struct{} // closed when the serve goroutine exits
	tmpDir   string
	cfg      *config.Config
	adminURL string
	siteURL  string
	running  bool

	// OnServeExited fires when the serve goroutine exits for any reason
	// (Stop also triggers it with a nil/canceled error).
	OnServeExited func(err error)
	// OnRebuildFailed fires when a watched-change rebuild fails.
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
			_, err := build.BuildMultiSite(opts)
			return err
		}
		_, err := build.BuildSite(opts)
		if err != nil {
			return err
		}
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
	r.log.LogFunctionEnd(startLog, map[string]any{"admin_url": r.adminURL})
	return r.adminURL, nil
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
	cancel()
	select {
	case <-done:
	case <-time.After(timeout):
		r.log.LogError(stopLog, fmt.Errorf("stop timed out after %v", timeout), nil)
		return fmt.Errorf("serve stop timed out after %v", timeout)
	}
	r.mu.Lock()
	r.cancel, r.done, r.cfg, r.adminURL, r.siteURL = nil, nil, nil, "", ""
	r.mu.Unlock()
	r.log.LogFunctionEnd(stopLog, nil)
	return nil
}
```

（若 `r.log.LogFunctionEnd` 的签名与 `internal/observability` 实际不同——核对 `internal/observability/logging.go:120` 附近的定义后按实际签名调用。）

- [ ] **Step 4: 运行确认通过**

```bash
go test ./internal/desktop/ -run TestServeRuntime -v
go test ./internal/desktop/ -v   # 全包回归
```

Expected: PASS。若初次构建因缺 `internal/admin/dist` skip，先执行环境前置再跑。

- [ ] **Step 5: 提交**

```bash
git add internal/desktop/server.go internal/desktop/server_test.go
git commit -m "feat(desktop): in-process serve runtime with watch, rebuild, and admin"
```

---

### Task 6: 生产构建运行时（internal/desktop/build.go）

**Files:**
- Create: `internal/desktop/build.go`
- Create: `internal/desktop/build_test.go`

**Interfaces:**
- Consumes: Task 2 的 `plugin.NewRegistryFromConfig`、`image.RunProcessors`；`build.BuildSite/BuildMultiSite/NewTimings`；`theme.NewManager`；`config.Load`
- Produces:
  - `func BuildSiteTo(sourceDir, outputDir string, logf func(format string, a ...any)) error`（等价 `huan build` 默认路径：全量构建 + 图片管线；差异仅无构建缓存——单次构建无增量收益）

- [ ] **Step 1: 写失败测试**

```go
package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSiteTo_ProducesOutput(t *testing.T) {
	src := writeSite(t)
	out := filepath.Join(t.TempDir(), "public")
	if err := BuildSiteTo(src, out, t.Logf); err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	for _, rel := range []string{"index.html", filepath.Join("posts", "alpha", "index.html")} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("output missing %s: %v", rel, err)
		}
	}
}

func TestBuildSiteTo_InvalidSiteFails(t *testing.T) {
	if err := BuildSiteTo(t.TempDir(), filepath.Join(t.TempDir(), "out"), t.Logf); err == nil {
		t.Fatal("BuildSiteTo on non-site dir should fail")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
go test ./internal/desktop/ -run TestBuildSiteTo -v
```

Expected: FAIL（build.go 不存在）。

- [ ] **Step 3: 实现 build.go**

```go
package desktop

import (
	"fmt"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
	"github.com/iannil/huan/internal/theme"
)

// BuildSiteTo runs the production build for sourceDir into outputDir —
// the same pipeline as `huan build` defaults (full build + image pipeline,
// no dev-only caches since a one-shot build has no incremental reuse).
func BuildSiteTo(sourceDir, outputDir string, logf func(format string, a ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	reg, err := plugin.NewRegistryFromConfig(cfg, sourceDir, "")
	if err != nil {
		return fmt.Errorf("plugin registry: %w", err)
	}
	themeMgr := theme.NewManager(reg)
	if cfg.Theme != "" {
		if err := themeMgr.Activate(cfg.Theme); err != nil {
			logf("huan: theme activate %q: %v", cfg.Theme, err)
		}
	}
	opts := build.Options{
		Timings:       build.NewTimings(),
		SourceDir:     sourceDir,
		OutputDir:     outputDir,
		Logf:          logf,
		PluginRegistry: reg,
		ThemeManager:  themeMgr,
	}
	if cfg.IsMultiLanguage() {
		if _, err := build.BuildMultiSite(opts); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	} else {
		if _, err := build.BuildSite(opts); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	}
	return image.RunProcessors(outputDir, sourceDir, reg)
}
```

- [ ] **Step 4: 运行确认通过**

```bash
go test ./internal/desktop/ -v
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/desktop/build.go internal/desktop/build_test.go
git commit -m "feat(desktop): production build runtime for one-click builds"
```

---

### Task 7: 图标资源 + 托盘/菜单（internal/desktop/menu.go、tray.go）

**Files:**
- Create: `scripts/gen-desktop-assets/main.go`
- Create: `internal/desktop/assets/tray.png`、`internal/desktop/assets/icon.png`（生成物，入库）
- Create: `internal/desktop/assets.go`（go:embed）
- Create: `internal/desktop/menu.go`（Controller 接口 + 菜单构建 + 动态更新）

**Interfaces:**
- Consumes: MyGo `mygo.NewMenu`、`mygo.MenuItem{Label, ID, Click, Submenu}`、`mygo.Separator`、`mygo.NewTray(TrayOptions{...})`、`mygo.NewMenuIcon(png, template)`、`menu.ItemByID(id).SetEnabled/SetLabel`
- Produces:
  - `type Controller interface { ToggleServe(); Rebuild(); OpenSiteFolder(); OpenSiteInBrowser(); SwitchSite(path string); ChooseSite(); OpenLogs(); ShowWindow(); Quit() }`
  - `func BuildTrayMenu(c Controller, recents []string) *mygo.Menu`
  - `func BuildAppMenu(c Controller) *mygo.Menu`
  - `type MenuSet struct`（持 `Tray *mygo.Tray`、`AppMenu *mygo.Menu`、`TrayMenu *mygo.Menu` 及需动态更新的 item 引用）；`func BuildTray(c Controller, recents []string) (*MenuSet, error)`；`func (m *MenuSet) SetServing(serving bool)`；`func (m *MenuSet) SetBuilding(building bool)`；`func (m *MenuSet) RefreshRecents(c Controller, recents []string) error`（重建托盘菜单并 `tray.SetMenu`）
  - `embed` 变量 `trayPNG []byte`、`iconPNG []byte`

- [ ] **Step 1: 生成占位图标资源**

`scripts/gen-desktop-assets/main.go`（一次性生成器，画纯图形、不引字体）：

```go
// Command gen-desktop-assets writes placeholder tray and app icons for the
// huan desktop shell. Run once: go run ./scripts/gen-desktop-assets
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func circle(m *image.NRGBA, cx, cy, r int, c color.Color) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				m.Set(x, y, c)
			}
		}
	}
}

func main() {
	outDir := filepath.Join("internal", "desktop", "assets")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	// 18x18 template tray icon: solid disc, black+alpha (macOS renders it adaptively).
	tray := image.NewNRGBA(image.Rect(0, 0, 18, 18))
	draw.Draw(tray, tray.Bounds(), image.Transparent, image.Point{}, draw.Src)
	circle(tray, 9, 9, 6, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	f, err := os.Create(filepath.Join(outDir, "tray.png"))
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, tray); err != nil {
		panic(err)
	}
	f.Close()

	// 512x512 app icon: dark rounded square with a light disc.
	const size = 512
	app := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(app, app.Bounds(), image.Transparent, image.Point{}, draw.Src)
	dark := color.NRGBA{R: 30, G: 41, B: 59, A: 255}
	for y := 64; y < size-64; y++ {
		for x := 64; x < size-64; x++ {
			app.Set(x, y, dark)
		}
	}
	circle(app, size/2, size/2, 110, color.NRGBA{R: 148, G: 163, B: 184, A: 255})
	f2, err := os.Create(filepath.Join(outDir, "icon.png"))
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f2, app); err != nil {
		panic(err)
	}
	f2.Close()
}
```

```bash
go run ./scripts/gen-desktop-assets
ls -la internal/desktop/assets/   # tray.png + icon.png 存在
```

- [ ] **Step 2: 核对 MyGo 菜单/托盘 API 并写 assets.go**

```bash
go doc github.com/egoist/mygo NewTray
go doc github.com/egoist/mygo TrayOptions
go doc github.com/egoist/mygo NewMenuIcon
go doc github.com/egoist/mygo MenuItem
go doc github.com/egoist/mygo Menu.ItemByID
```

`internal/desktop/assets.go`：

```go
package desktop

import _ "embed"

//go:embed assets/tray.png
var trayPNG []byte

//go:embed assets/icon.png
var iconPNG []byte
```

- [ ] **Step 3: 写 menu.go**

```go
package desktop

import (
	"github.com/egoist/mygo"
)

// Controller abstracts the app-level actions the menus trigger. It keeps
// menu wiring testable and decoupled from the MyGo app lifecycle (Task 8
// implements it on top of ServeRuntime).
type Controller interface {
	ToggleServe()        // start/stop the in-process server
	Rebuild()            // one-click production build
	OpenSiteFolder()     // reveal site root in Finder
	OpenSiteInBrowser()  // open siteURL in default browser
	SwitchSite(path string)
	ChooseSite()         // open directory picker and switch
	OpenLogs()           // reveal log directory in Finder
	ShowWindow()         // create/focus the admin window
	Quit()
}

// Menu item IDs used for dynamic updates.
const (
	itemToggleServe = "toggle-serve"
	itemRebuild     = "rebuild"
)

// BuildTrayMenu assembles the tray menu; recents are the recent site paths.
func BuildTrayMenu(c Controller, recents []string) *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "打开预览窗口", ID: "show-window", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
		{Label: "启动/停止预览服务", ID: itemToggleServe, Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
		{Label: "一键构建站点", ID: itemRebuild, Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
		mygo.Separator(),
		{Label: "打开站点目录", ID: "open-folder", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
		{Label: "在浏览器打开站点", ID: "open-browser", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
		{Label: "打开日志目录", ID: "open-logs", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenLogs() }},
		mygo.Separator(),
	}
	if len(recents) > 0 {
		subs := make([]*mygo.MenuItem, 0, len(recents)+1)
		for _, p := range recents {
			p := p
			subs = append(subs, &mygo.MenuItem{Label: p, Click: func(*mygo.MenuItem, *mygo.Window) { c.SwitchSite(p) }})
		}
		subs = append(subs, &mygo.MenuItem{Label: "选择其他目录…", ID: "choose-site", Click: func(*mygo.MenuItem, *mygo.Window) { c.ChooseSite() }})
		items = append(items, &mygo.MenuItem{Label: "切换站点", Type: mygo.MenuItemSubmenu, Submenu: subs})
	} else {
		items = append(items, &mygo.MenuItem{Label: "选择站点目录…", ID: "choose-site", Click: func(*mygo.MenuItem, *mygo.Window) { c.ChooseSite() }})
	}
	items = append(items, mygo.Separator(), &mygo.MenuItem{Label: "退出 huan", ID: "quit", Click: func(*mygo.MenuItem, *mygo.Window) { c.Quit() }})
	return mygo.NewMenu(items)
}

// BuildAppMenu assembles the macOS application menu.
func BuildAppMenu(c Controller) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "站点", Submenu: []*mygo.MenuItem{
			{Label: "打开预览窗口", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
			{Label: "启动/停止预览服务", Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
			{Label: "一键构建站点", Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
			mygo.Separator(),
			{Label: "打开站点目录", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
			{Label: "在浏览器打开站点", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
		}},
		{Role: mygo.RoleClose},
		{Role: mygo.RoleQuit},
	})
}

// MenuSet holds the live tray/app menus and the items the app updates
// dynamically as serve/build state changes.
type MenuSet struct {
	Tray    *mygo.Tray
	AppMenu *mygo.Menu
	TrayMenu *mygo.Menu
}

// BuildTray creates the tray icon with its menu.
func BuildTray(c Controller, recents []string) (*MenuSet, error) {
	icon, err := mygo.NewMenuIcon(trayPNG, true)
	if err != nil {
		return nil, err
	}
	trayMenu := BuildTrayMenu(c, recents)
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           icon,
		IconIsTemplate: true,
		ToolTip:        "huan",
		Menu:           trayMenu,
	})
	if err != nil {
		return nil, err
	}
	appMenu := BuildAppMenu(c)
	mygo.App.SetMenu(appMenu)
	tray.OnClick(func() { c.ShowWindow() })
	return &MenuSet{Tray: tray, AppMenu: appMenu, TrayMenu: trayMenu}, nil
}

// SetServing updates the serve toggle label and disables conflicting items
// while stopped/starting. Native menu items update in place.
func (m *MenuSet) SetServing(serving bool) {
	label := "启动预览服务"
	if serving {
		label = "停止预览服务"
	}
	if item := m.TrayMenu.ItemByID(itemToggleServe); item != nil {
		item.SetLabel(label)
	}
}

// SetBuilding disables the build item while a build is in flight.
func (m *MenuSet) SetBuilding(building bool) {
	if item := m.TrayMenu.ItemByID(itemRebuild); item != nil {
		item.SetEnabled(!building)
	}
}

// RefreshRecents rebuilds the tray menu (recent list changed).
func (m *MenuSet) RefreshRecents(c Controller, recents []string) error {
	menu := BuildTrayMenu(c, recents)
	m.TrayMenu = menu
	m.Tray.SetMenu(menu)
	return nil
}
```

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/desktop/ && go vet ./internal/desktop/
go test ./internal/desktop/   # 既有测试不回归
```

Expected: 编译通过（原生层行为在 Task 8 装配后手动验收）。`MenuItem.Type/Submenu` 字段名以 go doc 实际为准。

- [ ] **Step 5: 提交**

```bash
git add scripts/gen-desktop-assets internal/desktop/assets internal/desktop/assets.go internal/desktop/menu.go
git commit -m "feat(desktop): tray/app menus with placeholder icons"
```

---

### Task 8: 绑定 + 应用装配（internal/desktop/bindings.go、app.go）+ 入口更新

**Files:**
- Create: `internal/desktop/bindings.go`
- Create: `internal/desktop/app.go`（替换 spike 版）
- Modify: `cmd/huan-desktop/main.go`（不变，仍调 `desktop.Run()`；如 spike 版 Run 签名一致则无需改动）

**Interfaces:**
- Consumes: Task 3-7 全部 Produces；MyGo `mygo.Bind`、`Dialog.Open(OpenDialogOptions{Directory:true})`、`Dialog.Error`、`Dialog.Message`、`NewNotification(NotificationOptions{...}).Show()`、`mygo.ErrNotificationsDenied`、`mygo.Shell.ShowItemInFolder/OpenExternal`
- Produces:
  - `type DesktopBindings struct { app *App }`；`func (b DesktopBindings) AppVersion() string`；`func (b DesktopBindings) RevealSiteFolder() error`（TrustedOrigins 通路的验证绑定）
  - `func Run() error`（完整装配；覆盖 Task 1 spike 版）

- [ ] **Step 1: 核对对话框/通知/Shell/绑定 API**

```bash
go doc github.com/egoist/mygo Dialog.Open
go doc github.com/egoist/mygo OpenDialogOptions
go doc github.com/egoist/mygo Dialog.Error
go doc github.com/egoist/mygo NewNotification
go doc github.com/egoist/mygo Shell.ShowItemInFolder
go doc github.com/egoist/mygo Bind
go doc github.com/egoist/mygo PageOptions   # 定位 TrustedOrigins 字段位置
go doc github.com/egoist/mygo WindowOptions # 确认窗口标题/尺寸/关闭行为字段
go doc github.com/egoist/mygo Window        # 找关闭回调（OnClose/Closed 类），供 showWindow 清理引用
```

- [ ] **Step 2: 写 bindings.go**

```go
package desktop

import (
	"fmt"

	"github.com/egoist/mygo"

	"github.com/iannil/huan/internal/version"
)

// DesktopBindings is the minimal typed IPC surface exposed to the admin SPA.
// Phase 1 exists to prove the TrustedOrigins + bindings path; the generated
// client is kept for future use and the SPA is not modified.
type DesktopBindings struct {
	app *App
}

// AppVersion returns the huan version string.
func (b DesktopBindings) AppVersion() string { return version.String() }

// RevealSiteFolder reveals the current site root in Finder.
func (b DesktopBindings) RevealSiteFolder() error {
	dir := b.app.currentSourceDir()
	if dir == "" {
		return fmt.Errorf("no site selected")
	}
	mygo.Shell.ShowItemInFolder(dir)
	return nil
}
```

- [ ] **Step 3: 写完整 app.go（替换 spike）**

```go
package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/observability"
)

// App wires the MyGo lifecycle, the serve runtime, menus, and persistence
// into the desktop shell.
type App struct {
	mu     sync.Mutex
	cfg    *DesktopConfig
	cfgDir string
	rt     *ServeRuntime
	win    *mygo.Window
	menus  *MenuSet
	log    *observability.Logger
	logDir string
}

// Run starts the desktop app and blocks until it quits.
func Run() error {
	app := &App{}

	// Structured JSON log file (Full-Lifecycle Observability).
	dir, err := DefaultConfigDir()
	if err == nil {
		app.logDir = filepath.Join(dir, "logs")
		_ = os.MkdirAll(app.logDir, 0o755)
		f, ferr := os.OpenFile(filepath.Join(app.logDir, "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr == nil {
			app.log = observability.NewLoggerWithWriter("", io.MultiWriter(f, os.Stderr))
		}
	}
	if app.log == nil {
		app.log = observability.NewLogger("")
	}

	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	cfg, err := LoadOrDefault(cfgPath)
	if err != nil {
		return err
	}
	app.cfg = cfg

	mygo.Bind(DesktopBindings{app: app})
	mygo.App.WhenReady(app.onReady)
	return mygo.App.Run()
}

// currentSourceDir returns the active site directory ("" if none).
func (a *App) currentSourceDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rt == nil || !a.rt.Running() {
		if len(a.cfg.RecentSites) > 0 {
			return a.cfg.RecentSites[0]
		}
		return ""
	}
	return a.rt.SourceDir()
}

// notify shows a best-effort system notification; denial is not fatal.
func (a *App) notify(title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body})
	if err := n.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
		a.log.LogError("notify", err, nil)
	}
}

// onReady runs once the MyGo app is ready: tray, site, server, window.
func (a *App) onReady() {
	menus, err := BuildTray(a, a.cfg.RecentSites)
	if err != nil {
		a.log.LogError("tray", err, nil)
	}
	a.mu.Lock()
	a.menus = menus
	a.mu.Unlock()

	// Ensure a site: use most-recent, else ask. Invalid entries fall through
	// to the picker with an explanatory error dialog.
	site := ""
	if len(a.cfg.RecentSites) > 0 {
		if err := ValidateSite(a.cfg.RecentSites[0]); err == nil {
			site = a.cfg.RecentSites[0]
		} else {
			mygo.Dialog.Error("站点不可用", err.Error())
		}
	}
	if site == "" {
		s, ok := a.pickSiteDialog()
		if !ok {
			a.log.Log("onReady", observability.EventBranch, map[string]any{"branch": "no-site-selected"})
			return // stay in tray; user can pick via menu
		}
		site = s
	}
	a.startAndServe(site)
	a.showWindow()
}

// pickSiteDialog loops the directory picker until a valid site or cancel.
func (a *App) pickSiteDialog() (string, bool) {
	for {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:     "选择 huan 站点目录",
			Directory: true,
		})
		if err != nil {
			a.log.LogError("pick-site", err, nil)
			return "", false
		}
		if len(paths) == 0 || paths[0] == "" {
			return "", false // user canceled
		}
		if err := ValidateSite(paths[0]); err != nil {
			mygo.Dialog.Error("不是有效的 huan 站点", err.Error())
			continue
		}
		return paths[0], true
	}
}

// startAndServe starts (or restarts) the server for the given site and
// reloads the window. Called from the ready flow and site switches.
func (a *App) startAndServe(site string) {
	startLog := a.log.LogFunctionStart("start-serve", map[string]any{"site": site})
	if a.rt != nil {
		_ = a.rt.Stop(5 * time.Second)
	}
	rt := NewServeRuntime(ServeOptions{
		SourceDir: site,
		Logf:      func(format string, args ...any) { a.log.Log("serve", observability.EventPoint, map[string]any{"msg": fmt.Sprintf(format, args...)}) },
	})
	rt.OnServeExited = func(err error) {
		// Intentional stop (nil / context.Canceled): update UI quietly.
		// Unexpected exit: also notify the user (spec §6).
		if err == nil || errors.Is(err, context.Canceled) {
			if a.menus != nil {
				a.menus.SetServing(false)
			}
			a.showOffline()
			return
		}
		a.log.LogError("serve-exited", err, nil)
		a.notify("huan 预览服务已停止", "可通过托盘菜单重新启动")
		if a.menus != nil {
			a.menus.SetServing(false)
		}
		a.showOffline()
	}
	rt.OnRebuildFailed = func(err error) {
		a.notify("重建失败", err.Error())
	}
	adminURL, err := rt.Start()
	if err != nil {
		a.log.LogError(startLog, err, nil)
		mygo.Dialog.Error("预览服务启动失败", err.Error())
		if a.menus != nil {
			a.menus.SetServing(false)
		}
		return
	}
	a.mu.Lock()
	a.rt = rt
	a.cfg.AddRecentSite(site)
	a.mu.Unlock()
	if err := Save(filepath.Join(a.cfgDir, "config.json"), a.cfg); err != nil {
		a.log.LogError("save-config", err, nil)
	}
	if a.menus != nil {
		a.menus.SetServing(true)
		a.menus.RefreshRecents(a, a.cfg.RecentSites)
	}
	a.log.LogFunctionEnd(startLog, map[string]any{"admin_url": adminURL})
	a.loadAdmin(adminURL)
}

// showWindow creates or focuses the admin window.
func (a *App) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		return // Phase 1: single window; created once in onReady flow
	}
	newWin, err := mygo.NewWindow(mygo.WindowOptions{
		Title:  "huan",
		Width:  1280,
		Height: 800,
		// TrustedOrigins lets the admin SPA origin call Go bindings. Field
		// location per go doc (PageOptions) — adjust if the type differs.
		Page: mygo.PageOptions{TrustedOrigins: []string{"http://127.0.0.1"}},
	})
	if err != nil {
		mygo.Dialog.Error("huan", fmt.Sprintf("create window: %v", err))
		return
	}
	a.mu.Lock()
	a.win = newWin
	a.mu.Unlock()
	// Window close (Cmd+W) must clear a.win so ShowWindow can recreate it.
	// Wire the close callback found via `go doc mygo Window`; shape:
	// newWin.OnClose(func() { a.mu.Lock(); a.win = nil; a.mu.Unlock() })
	// If the framework exits on last-window-close instead, keep the default
	// (tray quits with app) and drop this wiring — record which in the
	// progress doc.
	url := ""
	if a.rt != nil {
		url = a.rt.AdminURL()
	}
	if url != "" {
		if err := newWin.Page().LoadURL(url); err != nil {
			a.log.LogError("load-admin", err, nil)
		}
	}
}

// loadAdmin points the window at the given admin URL (or an offline page).
func (a *App) loadAdmin(adminURL string) {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	if adminURL == "" {
		a.showOffline()
		return
	}
	if err := win.Page().LoadURL(adminURL); err != nil {
		a.log.LogError("load-admin", err, nil)
	}
}

// showOffline renders a minimal local page when the server is stopped.
func (a *App) showOffline() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	_ = win.Page().LoadHTML(
		`<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8"><title>huan</title></head>
<body style="font-family:-apple-system,sans-serif;display:grid;place-items:center;height:100vh;margin:0">
<div style="text-align:center"><h1>预览服务已停止</h1><p>通过菜单栏托盘图标可重新启动预览服务。</p></div>
</body></html>`)
}

// ---- Controller implementation (menu callbacks run on the main thread;
// blocking work goes to goroutines, per MyGo dialog docs) ----

func (a *App) ToggleServe() {
	go func() {
		if a.rt != nil && a.rt.Running() {
			if err := a.rt.Stop(5 * time.Second); err != nil {
				mygo.Dialog.Error("停止失败", err.Error())
			}
			if a.menus != nil {
				a.menus.SetServing(false)
			}
			a.showOffline()
			return
		}
		site := a.currentSourceDir()
		if site == "" {
			a.ChooseSite()
			return
		}
		a.startAndServe(site)
	}()
}

func (a *App) Rebuild() {
	go func() {
		rt := a.rt
		if rt == nil || !rt.Running() {
			mygo.Dialog.Error("无法构建", "预览服务未运行，请先选择站点")
			return
		}
		if a.menus != nil {
			a.menus.SetBuilding(true)
		}
		cfg := rt.Config()
		outDir := filepath.Join(rt.SourceDir(), cfg.PublishDir)
		err := BuildSiteTo(rt.SourceDir(), outDir, func(format string, args ...any) {
			a.log.Log("build", observability.EventPoint, map[string]any{"msg": fmt.Sprintf(format, args...)})
		})
		if a.menus != nil {
			a.menus.SetBuilding(false)
		}
		if err != nil {
			a.log.LogError("build", err, map[string]any{"out": outDir})
			a.notify("构建失败", err.Error())
			return
		}
		a.notify("构建完成", "输出目录："+outDir)
	}()
}

func (a *App) OpenSiteFolder() {
	if dir := a.currentSourceDir(); dir != "" {
		mygo.Shell.ShowItemInFolder(dir)
	}
}

func (a *App) OpenSiteInBrowser() {
	if a.rt != nil && a.rt.SiteURL() != "" {
		mygo.Shell.OpenExternal(a.rt.SiteURL())
	}
}

func (a *App) SwitchSite(path string) {
	go func() {
		if err := ValidateSite(path); err != nil {
			mygo.Dialog.Error("站点不可用", err.Error())
			return
		}
		a.startAndServe(path)
	}()
}

func (a *App) ChooseSite() {
	go func() {
		site, ok := a.pickSiteDialog()
		if !ok {
			return
		}
		a.startAndServe(site)
	}()
}

func (a *App) OpenLogs() {
	if a.logDir != "" {
		mygo.Shell.ShowItemInFolder(a.logDir)
	}
}

func (a *App) Quit() {
	go func() {
		if a.rt != nil {
			_ = a.rt.Stop(5 * time.Second)
		}
		// App.Quit name per go doc; fall back to os.Exit(0) if absent.
		mygo.App.Quit()
		os.Exit(0)
	}()
}
```

- [ ] **Step 4: 编译 + 全量测试**

```bash
go build ./... && go vet ./internal/desktop/ ./cmd/huan-desktop/
go test ./internal/desktop/ -v
```

Expected: 编译通过、测试 PASS。若 `PageOptions.TrustedOrigins`/`App.Quit`/`Page.LoadHTML` 字段或方法名与实际不符，按 go doc 修正（语义不变）。

- [ ] **Step 5: 手动冒烟验收（对照 spec §7 人工清单）**

```bash
go run ./cmd/huan-desktop
```

逐项核对并记录到进度文档：
1. 启动后弹目录选择框 → 选一个 huan 站点（如 `../zhurongshuo`）→ 窗口加载 admin SPA
2. 托盘图标出现；「一键构建站点」→ 构建期间禁用 → 完成通知
3. 「停止预览服务」→ 窗口显示离线页；「启动预览服务」→ 恢复 admin
4. 「打开站点目录」→ Finder reveal；「在浏览器打开站点」→ 浏览器打开
5. 「切换站点」→ 选另一目录 → 窗口 reload 到新 admin
6. 「退出 huan」→ 进程退出、端口释放（`lsof -i :<port>` 确认）

- [ ] **Step 6: 提交**

```bash
git add internal/desktop/bindings.go internal/desktop/app.go cmd/huan-desktop docs/progress/2026-11-17-desktop-phase1.md
git commit -m "feat(desktop): full app assembly — startup flow, site switching, tray wiring"
```

---

### Task 9: 打包 .app + 发布归档 + 验收文档

**Files:**
- Create: `release/desktop/v0.13.0/`（.app zip、.dmg、SHA256SUMS、README.md）
- Create: `docs/reports/2026-11-17-desktop-phase1-acceptance.md`
- Modify: `docs/progress/2026-11-17-desktop-phase1.md` → 完成后移至 `docs/reports/completed/2026-11-17-desktop-phase1.md`
- Modify: `memory/daily/2026-11-17.md`（追加完成记录）

**Interfaces:**
- Consumes: Task 8 的完整应用；`mygo build`
- Produces: 可分发的 macOS .app 归档 + 验收文档

- [ ] **Step 1: 确认 admin dist 与构建产物**

```bash
ls internal/admin/dist/index.html || (cd web/admin && npm ci && npm run build)
mygo build -platform darwin/arm64
find build/desktop -maxdepth 3 \( -name "*.app" -o -name "*.dmg" \) 2>/dev/null
```

Expected: 产出 `huan.app`（及可选 DMG）。产物实际路径以 mygo 输出为准。

- [ ] **Step 2: .app 冒烟验收**

```bash
open build/desktop/darwin-arm64/huan.app   # 路径按 Step 1 实际输出
```

未签名应用首启需右键 → 打开（或 `xattr -cr build/desktop/darwin-arm64/huan.app`）。重复 Task 8 Step 5 的六项清单，结果记入验收文档。

- [ ] **Step 3: 归档到 release/**

```bash
mkdir -p release/desktop/v0.13.0
ditto -c -k --sequesterRsrc --keepParent build/desktop/darwin-arm64/huan.app release/desktop/v0.13.0/huan-v0.13.0-darwin-arm64.app.zip
# 若产出 DMG 一并拷入；生成校验和
cd release/desktop/v0.13.0 && shasum -a 256 *.zip *.dmg > SHA256SUMS 2>/dev/null || shasum -a 256 *.zip > SHA256SUMS
```

`release/desktop/v0.13.0/README.md`（中文）：系统要求（macOS arm64）、首启需右键打开的原因（Phase 1 无签名公证）、功能清单（预览服务/一键构建/切换站点）、日志位置（`~/Library/Application Support/huan-desktop/logs/`）、已知限制（引用 spec §9 风险）。

- [ ] **Step 4: 写验收文档并归档进度**

`docs/reports/2026-11-17-desktop-phase1-acceptance.md`：验收标准（spec §1）、逐项人工验收结果（Task 8 Step 5 + Step 2）、`go test ./...` 与 `go vet ./...` 输出摘要、产物清单与 SHA256、遗留事项（签名公证、自动更新、Win/Linux、图标美化）。进度文档追加完成条目后 `git mv` 至 `docs/reports/completed/2026-11-17-desktop-phase1.md`。

- [ ] **Step 5: 更新记忆并提交**

`memory/daily/2026-11-17.md` 追加完成记录；若 MEMORY.md 需新增「桌面端 Phase 1 已发布」条目则一并更新。

```bash
git add release/desktop docs/reports memory/daily
git commit -m "chore(release): huan desktop v0.13.0 phase 1 (.app) with acceptance report"
```

---

## Self-Review 记录

- **Spec 覆盖**：§3 架构（Task 5/8）✓；§4 目录（各任务 Files）✓；§5 启动流/菜单/状态机/绑定（Task 4/5/7/8）✓；§6 错误处理表逐行（Task 3 配置损坏、Task 5 启动失败/panic recover、Task 8 Dialog/通知/日志目录/退出超时）✓；§7 测试分层（各任务 + Task 8 Step 5 人工清单）✓；§8 发布（Task 9）✓；§9 风险（spike 前置消解 pre-1.0 API 风险、`TrustedOrigins` go doc 核对）✓。spec §4「前置小重构」落实为 Task 2（含 dev.MarkdownOnlyChanges）。
- **占位符扫描**：无 TBD/TODO；两处「以 go doc 实际签名为准」是对 pre-1.0 依赖的显式核对程序，均给出了语义不变的具体代码。
- **类型一致性**：`Controller` 接口（Task 7 定义、Task 8 实现）九个方法一一对应；`ServeRuntime` 的 `Config()/AdminURL()/SiteURL()/SourceDir()/Running()` 在 Task 8 中全部按 Task 5 签名使用；`BuildSiteTo`、`ValidateSite`、`DesktopConfig.AddRecentSite` 跨任务签名一致。
- **复审修正**（已改入正文）：① Task 5 `Start()` 增加 `starting` 进行中守卫，防菜单双击起两个 server；② Task 8 `OnServeExited` 按 `errors.Is(err, context.Canceled)` 区分主动停止与异常退出，仅异常退出弹通知（对齐 spec §6）；③ Task 8 `showWindow` 增加窗口关闭回调接线说明，修复窗口关闭后无法重建的问题；④ `notify` 改用 `errors.Is` 判断 `ErrNotificationsDenied`。
