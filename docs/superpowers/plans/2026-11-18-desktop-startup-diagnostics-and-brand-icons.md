# 桌面端启动诊断 + 品牌图标接入 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 插件加载警告等启动期问题在桌面端可见（收集器 + 菜单详情 + 系统通知），huan 品牌图标接入 bundle/托盘/运行时 Dock 三处，插件 .so 以当前工具链重编归零警告，版本 bump 0.13.1 并重出发布物。

**Architecture:** `pkg/plugin.Registry.Warnings` 显式承载加载警告（打印职责上移：CLI stderr 格式逐字节不变，桌面端经 `ServeOptions.OnPluginWarnings` 回调进 `internal/desktop.Diagnostics` 收集器）；UI 走托盘/应用菜单「启动诊断…」项 + `mygo.Dialog.Message` 详情 + `mygo` 系统通知（ID 固定防堆积，点击打开详情）；图标由品牌资产目录经 `gen-brand-icons.sh`（iconutil）与 `gen-desktop-assets`（Go 绘制 template 字形）生成。

**Tech Stack:** Go 1.27（html/template 仓库主语言）、mygo v0.9.2（Dialog/Notification/Dock/Tray API）、iconutil/sips（macOS 系统工具）、bash。

**Spec:** `docs/superpowers/specs/2026-11-18-desktop-startup-diagnostics-and-brand-icons-design.md`（已批准，commit b611d36）

## Global Constraints

- 交流与文档中文，代码与注释英文（CLAUDE.md 语言约定）。
- CLI stderr 警告格式逐字节保留：`huan: plugin load warning: <name>: <err>\n`（经 `plugin.PrintWarnings` 单点输出）。
- admin SPA（`web/admin/`）一行不改。
- `*.so`、`release/`、`build/` 不入库；`web/admin/public/brand/` 设计资产入库（未被 .gitignore 覆盖，当前 untracked，随 Task 7 提交）。
- 托盘图标：黑+alpha template PNG，18×18 画布，`IconIsTemplate: true` 不变。
- 品牌几何：`huan-primary.svg` viewBox 1000，三平行四边形 `(240,295)(570,205)(760,315)(430,405)` / `(240,470)(570,380)(760,490)(430,580)` / `(240,645)(570,555)(760,665)(430,755)`。
- 通知触发条件：`len(Snapshot()) > 0`（警告级也算 —— 插件版本不匹配是警告级，是需求源头）；通知 ID 固定 `startup-diag`。
- 版本：`mygo.json` version `0.13.0 → 0.13.1`；发布物落 `release/desktop/v0.13.1/`。
- 全链路可观测：诊断 Add/报告走 `observability.Logger` 结构化事件。
- 每任务一次提交，conventional commits（`feat(desktop):` / `feat(plugin):` / `chore(release):` / `docs:`）。
- 前例：desktop 工作曾走特性分支（daily note 记录 iannil/fde-next）；执行前与用户确认分支策略，默认当前 master 上开新特性分支。

---

### Task 1: 警告显式化 — Registry.Warnings + Loader 收集 + CLI 静态路径打印

**Files:**
- Modify: `pkg/plugin/plugin.go:30-40`（Registry struct + LoadWarning 类型）
- Modify: `internal/plugin/plugin.go`（类型别名）
- Modify: `internal/plugin/loader.go`（Loader struct + ScanAndLoad:195 + ScanAndLoadByCategory:250）
- Modify: `internal/plugin/registry_config.go`（扫描后转存 loader.Warnings → r.Warnings）
- Create: `internal/plugin/warnings.go`（PrintWarnings 单点输出）
- Modify: `cmd/huan/plugins.go`（newPluginRegistry 打印上移）
- Test: `internal/plugin/loader_test.go`、`internal/plugin/registry_config_test.go`、`cmd/huan/plugins_test.go`

**Interfaces:**
- Consumes: 现有 `Loader.ScanAndLoad() ([]ScanAndLoadResult, error)`、`NewRegistryFromConfig(cfg *config.Config, sourceDir, pluginDirOverride string) (*Registry, error)`。
- Produces:
  - `pkg/plugin`: `type LoadWarning struct { Name string; Err error }`；`Registry.Warnings []LoadWarning` 字段。
  - `internal/plugin`: `type LoadWarning = pkgplugin.LoadWarning` 别名；`Loader.Warnings []LoadWarning` 字段（每次扫描入口重置）；`func PrintWarnings(w io.Writer, ws []LoadWarning)`。
  - 不变量：CLI `newPluginRegistry` 后 stderr 出现与旧版逐字节相同的警告行。

- [ ] **Step 1: 写失败测试（Loader 收集）**

追加到 `internal/plugin/loader_test.go`：

```go
func TestScanAndLoadCollectsWarningsInsteadOfPrinting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.so"), []byte("not a plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	l := NewLoader(dir)
	results, err := l.ScanAndLoad()
	if err != nil {
		t.Fatalf("ScanAndLoad: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("want 0 results, got %d", len(results))
	}
	if len(l.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d: %+v", len(l.Warnings), l.Warnings)
	}
	if l.Warnings[0].Name != "broken.so" {
		t.Errorf("warning name = %q, want broken.so", l.Warnings[0].Name)
	}
	if l.Warnings[0].Err == nil {
		t.Error("warning err is nil")
	}
}

func TestScanAndLoadResetsWarningsPerCall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.so"), []byte("not a plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	l := NewLoader(dir)
	for i := 0; i < 2; i++ {
		if _, err := l.ScanAndLoad(); err != nil {
			t.Fatalf("scan %d: %v", i, err)
		}
		if len(l.Warnings) != 1 {
			t.Fatalf("scan %d: want 1 warning (reset per call), got %d", i, len(l.Warnings))
		}
	}
}
```

- [ ] **Step 2: 写失败测试（Registry 转存）**

追加到 `internal/plugin/registry_config_test.go`：

```go
func TestNewRegistryFromConfigCollectsWarnings(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "broken.so"), []byte("not a plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Plugins: map[string]config.PluginConfig{
		"broken": {Category: CategoryStatic},
	}}
	reg, err := NewRegistryFromConfig(cfg, dir, pluginsDir)
	if err != nil {
		t.Fatalf("NewRegistryFromConfig: %v", err)
	}
	if len(reg.Warnings) != 1 {
		t.Fatalf("want 1 registry warning, got %d: %+v", len(reg.Warnings), reg.Warnings)
	}
	if reg.Warnings[0].Name != "broken.so" {
		t.Errorf("warning name = %q, want broken.so", reg.Warnings[0].Name)
	}
}
```

- [ ] **Step 3: 写失败测试（CLI stderr 逐字节格式）**

追加到 `cmd/huan/plugins_test.go`（该文件 package main，已 import config/os/filepath/testing；按需补 `io`/`strings`）：

```go
func TestNewPluginRegistryPrintsWarnings(t *testing.T) {
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "broken.so"), []byte("not a plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Plugins: map[string]config.PluginConfig{
		"broken": {Category: "static"},
	}}
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	reg, err := newPluginRegistry(cfg, dir, pluginsDir)
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("newPluginRegistry: %v", err)
	}
	if reg == nil {
		t.Fatal("registry is nil")
	}
	if !strings.Contains(string(out), "huan: plugin load warning: broken.so: ") {
		t.Errorf("stderr missing canonical warning line, got: %q", string(out))
	}
}
```

- [ ] **Step 4: 运行三个测试确认失败**

Run: `go test ./internal/plugin/ -run 'TestScanAndLoadCollectsWarnings|TestScanAndLoadResetsWarnings|TestNewRegistryFromConfigCollectsWarnings' -v && go test ./cmd/huan/ -run TestNewPluginRegistryPrintsWarnings -v`
Expected: FAIL（`l.Warnings`/`reg.Warnings` 字段不存在 → 编译错误，即失败）

- [ ] **Step 5: 最小实现**

`pkg/plugin/plugin.go` — 在 `type Registry struct` 定义之前新增类型，Registry 增加字段：

```go
// LoadWarning records one .so that failed to load during a directory scan.
// Name is the .so file name; Err is the loader error. Load failures are
// non-fatal — scanning continues with the remaining files. Printing is the
// caller's job (the CLI prints to stderr; the desktop app surfaces warnings
// in its startup diagnostics), so a Finder-launched .app never loses them.
type LoadWarning struct {
	Name string
	Err  error
}
```

```go
type Registry struct {
	plugins map[string]Plugin
	order   []string
	// Warnings lists plugins that failed to load while building this
	// registry. Empty after a clean scan. Callers decide how to surface
	// them (see LoadWarning).
	Warnings []LoadWarning
}
```

`internal/plugin/plugin.go` — 别名区（`type Registry = pkgplugin.Registry` 附近）加：

```go
type LoadWarning = pkgplugin.LoadWarning
```

`internal/plugin/loader.go` — Loader struct 加字段：

```go
// Loader discovers and loads .so plugin files from a directory.
type Loader struct {
	pluginDir string
	// Warnings accumulates load failures of the most recent scan. Reset at
	// the entry of each ScanAndLoad/ScanAndLoadByCategory call.
	Warnings []LoadWarning
}
```

`ScanAndLoad`（172 行起）：循环开头 `results` 声明后加 `l.Warnings = nil`；195 行：

```go
			p, err := l.LoadPlugin(fullPath, nil)
			if err != nil {
				l.Warnings = append(l.Warnings, LoadWarning{Name: entry.Name(), Err: err})
				continue
			}
```

`ScanAndLoadByCategory`（208 行起）：同样在函数体开头 `results` 声明后加 `l.Warnings = nil`；250 行同样替换为 append。

`internal/plugin/warnings.go`（新文件）：

```go
package plugin

import (
	"fmt"
	"io"
)

// PrintWarnings writes load warnings in the canonical CLI format. The exact
// wording is load-bearing: operators grep for "huan: plugin load warning".
func PrintWarnings(w io.Writer, ws []LoadWarning) {
	for _, warning := range ws {
		fmt.Fprintf(w, "huan: plugin load warning: %s: %v\n", warning.Name, warning.Err)
	}
}
```

`internal/plugin/registry_config.go` — `ScanAndLoadByCategory` 成功返回后、`for _, result := range results` 之前加：

```go
	// Transfer scan warnings onto the registry; printing is the caller's job.
	r.Warnings = append(r.Warnings, loader.Warnings...)
```

`cmd/huan/plugins.go` — `newPluginRegistry` 改为：

```go
func newPluginRegistry(cfg *config.Config, sourceDir string, pluginDirOverride string) (*plugin.Registry, error) {
	reg, err := plugin.NewRegistryFromConfig(cfg, sourceDir, pluginDirOverride)
	if err != nil {
		return nil, err
	}
	plugin.PrintWarnings(os.Stderr, reg.Warnings)
	return reg, nil
}
```

（`cmd/huan/plugins.go` 按需补 import `"os"`。）

- [ ] **Step 6: 运行三个测试确认通过**

Run: `go test ./internal/plugin/ -run 'TestScanAndLoadCollectsWarnings|TestScanAndLoadResetsWarnings|TestNewRegistryFromConfigCollectsWarnings' -v && go test ./cmd/huan/ -run TestNewPluginRegistryPrintsWarnings -v`
Expected: PASS（全部）

- [ ] **Step 7: 全包回归**

Run: `go test ./internal/plugin/... ./cmd/huan/... && go vet ./internal/plugin/... ./cmd/huan/...`
Expected: PASS（loader.go 移除打印后，任何直接依赖旧行为的测试若失败则修其断言——预期没有）

- [ ] **Step 8: Commit**

```bash
git add pkg/plugin/plugin.go internal/plugin/plugin.go internal/plugin/loader.go internal/plugin/registry_config.go internal/plugin/warnings.go internal/plugin/loader_test.go internal/plugin/registry_config_test.go cmd/huan/plugins.go cmd/huan/plugins_test.go
git commit -m "feat(plugin): collect load warnings on Registry instead of stderr printing"
```

---

### Task 2: 生命周期路径警告合并 + CLI dev/daemon 回调（动态插件路径）

**Files:**
- Modify: `internal/plugin/lifecycle.go`（LifecycleManager 加 OnWarnings 字段 + Start:124 后合并）
- Modify: `cmd/huan/dev.go:257`、`cmd/huan/daemon.go:158`（OnWarnings 接 stderr）
- Test: `internal/plugin/lifecycle_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Loader.Warnings`、`LoadWarning`、`PrintWarnings`。
- Produces: `LifecycleManager.OnWarnings func([]LoadWarning)` 公开字段（构造后、Start 前设置；serve goroutine 语境调用）；`LifecycleManager.Start` 后 `registry.Warnings` 已合并动态扫描警告。

- [ ] **Step 1: 写失败测试**

追加到 `internal/plugin/lifecycle_test.go`（package plugin；该文件已有 eventbus/context 构造先例，按需补 import `os`/`filepath`）：

```go
func TestLifecycleManagerOnWarningsFiresOnBrokenSo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.so"), []byte("garbage"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	mgr := NewLifecycleManager(reg, NewLoader(dir), eventbus.NewChannelBus())
	var got []LoadWarning
	mgr.OnWarnings = func(ws []LoadWarning) { got = ws }
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	if len(got) != 1 {
		t.Fatalf("want 1 warning from OnWarnings, got %d", len(got))
	}
	if got[0].Name != "broken.so" {
		t.Errorf("warning name = %q, want broken.so", got[0].Name)
	}
	if len(reg.Warnings) != 1 {
		t.Errorf("registry warnings = %d, want 1 (merged)", len(reg.Warnings))
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/plugin/ -run TestLifecycleManagerOnWarningsFiresOnBrokenSo -v`
Expected: FAIL（`mgr.OnWarnings` 字段不存在 → 编译错误）

- [ ] **Step 3: 实现**

`internal/plugin/lifecycle.go` — LifecycleManager struct（registry 字段附近）加：

```go
	// OnWarnings fires with .so files that failed to load during Start's
	// directory scan. Nil drops them — warnings still merge onto the
	// registry. Printing is the caller's job; invoked from the Start
	// goroutine.
	OnWarnings func([]LoadWarning)
```

`Start` 内 `results, err := m.loader.ScanAndLoad()` 错误检查之后、`for _, result := range results` 之前加：

```go
	// Merge scan warnings onto the registry and hand them to the caller.
	if len(m.loader.Warnings) > 0 {
		m.registry.Warnings = append(m.registry.Warnings, m.loader.Warnings...)
		if m.OnWarnings != nil {
			m.OnWarnings(m.loader.Warnings)
		}
	}
```

`cmd/huan/dev.go:257` 之后（`adminPluginMgr := plugin.NewLifecycleManager(...)` 语句后）加：

```go
	adminPluginMgr.OnWarnings = func(ws []plugin.LoadWarning) {
		plugin.PrintWarnings(os.Stderr, ws)
	}
```

`cmd/huan/daemon.go:158` 的 `plugin.NewLifecycleManager(...)` 赋值语句后同样加（变量名以该处实际为准，`d.pluginManager`）。两个文件按需补 import（`os`、已用的 `plugin` 包）。

- [ ] **Step 4: 运行确认通过 + 回归**

Run: `go test ./internal/plugin/ -run TestLifecycleManagerOnWarningsFiresOnBrokenSo -v && go test ./internal/plugin/... ./cmd/huan/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plugin/lifecycle.go internal/plugin/lifecycle_test.go cmd/huan/dev.go cmd/huan/daemon.go
git commit -m "feat(plugin): merge dynamic-scan warnings onto registry with OnWarnings callback"
```

---

### Task 3: 桌面端 Diagnostics 收集器

**Files:**
- Create: `internal/desktop/diagnostics.go`
- Test: `internal/desktop/diagnostics_test.go`

**Interfaces:**
- Produces（Task 6 依赖，逐字签名）:
  - `type DiagLevel string`；`const LevelWarn DiagLevel = "warn"`、`LevelError DiagLevel = "error"`
  - `type DiagItem struct { Level DiagLevel; Component string; Message string }`（Component ∈ "plugin"|"serve"|"build"|"site"）
  - `type Diagnostics`；`func NewDiagnostics() *Diagnostics`；`func (d *Diagnostics) Add(level DiagLevel, component, msg string)`；`func (d *Diagnostics) Snapshot() []DiagItem`；`func (d *Diagnostics) HasErrors() bool`；`func (d *Diagnostics) Reset()`
  - `func diagSummary(items []DiagItem) (message, detail string)`；`func notifyTitle(errs, warns int) string`；`func truncMsg(s string, n int) string`；`const notifStartupDiag = "startup-diag"`

- [ ] **Step 1: 写失败测试**

Create `internal/desktop/diagnostics_test.go`:

```go
package desktop

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestDiagnosticsConcurrentAddSnapshot(t *testing.T) {
	d := NewDiagnostics()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				d.Add(LevelWarn, "plugin", "x")
				_ = d.Snapshot()
			}
		}()
	}
	wg.Wait()
	if got := len(d.Snapshot()); got != 400 {
		t.Fatalf("want 400 items, got %d", got)
	}
}

func TestDiagnosticsResetAndHasErrors(t *testing.T) {
	d := NewDiagnostics()
	if d.HasErrors() {
		t.Fatal("fresh diagnostics must have no errors")
	}
	d.Add(LevelWarn, "plugin", "w1")
	d.Add(LevelError, "serve", "e1")
	if !d.HasErrors() {
		t.Fatal("want HasErrors after error add")
	}
	d.Reset()
	if len(d.Snapshot()) != 0 || d.HasErrors() {
		t.Fatal("Reset must clear items and errors")
	}
}

func TestDiagSummary(t *testing.T) {
	msg, detail := diagSummary(nil)
	if msg != "本次启动未发现问题" || detail != "" {
		t.Fatalf("empty summary = %q / %q", msg, detail)
	}
	items := []DiagItem{
		{Level: LevelError, Component: "serve", Message: "listen: busy"},
		{Level: LevelWarn, Component: "plugin", Message: "broken.so: bad format"},
	}
	msg, detail = diagSummary(items)
	if msg != "本次启动发现 2 个问题" {
		t.Errorf("message = %q", msg)
	}
	for _, want := range []string{"[错误] serve: listen: busy", "[警告] plugin: broken.so: bad format"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q, got:\n%s", want, detail)
		}
	}
}

func TestNotifyTitle(t *testing.T) {
	if got := notifyTitle(0, 2); got != "huan 已启动，有 2 个警告" {
		t.Errorf("warn-only title = %q", got)
	}
	if got := notifyTitle(1, 2); got != "huan 启动存在问题（1 个错误，2 个警告）" {
		t.Errorf("error title = %q", got)
	}
}

func TestTruncMsg(t *testing.T) {
	if got := truncMsg("short", 10); got != "short" {
		t.Errorf("short passthrough = %q", got)
	}
	long := strings.Repeat("字", 120)
	got := truncMsg(long, 100)
	if !strings.HasPrefix(got, strings.Repeat("字", 100)) || !strings.HasSuffix(got, "…") {
		t.Errorf("truncMsg cut wrong: len=%d suffix=%q", len([]rune(got)), got[len(got)-3:])
	}
	if len([]rune(got)) != 101 {
		t.Errorf("want 101 runes (100 + ellipsis), got %d", len([]rune(got)))
	}
	// silence unused import in edge builds
	_ = fmt.Sprint
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/desktop/ -run 'TestDiagnostics|TestDiagSummary|TestNotifyTitle|TestTruncMsg' -v`
Expected: FAIL（`Diagnostics` 未定义 → 编译错误）

- [ ] **Step 3: 实现 diagnostics.go**

Create `internal/desktop/diagnostics.go`:

```go
package desktop

import (
	"fmt"
	"strings"
	"sync"
)

// DiagLevel classifies a startup diagnostic item.
type DiagLevel string

const (
	LevelWarn  DiagLevel = "warn"
	LevelError DiagLevel = "error"
)

// notifStartupDiag is the fixed notification ID: re-showing replaces the
// previous one (no pile-up), and the click handler routes on it.
const notifStartupDiag = "startup-diag"

// DiagItem is one startup-time problem surfaced in the desktop UI.
// Component is one of "plugin" | "serve" | "build" | "site".
type DiagItem struct {
	Level     DiagLevel
	Component string
	Message   string
}

// Diagnostics collects startup-time problems (plugin load warnings, serve
// start failures, ...). Safe for concurrent use: Add may be called from the
// serve goroutine while Snapshot is read on the main thread.
type Diagnostics struct {
	mu    sync.Mutex
	items []DiagItem
}

// NewDiagnostics returns an empty collector.
func NewDiagnostics() *Diagnostics { return &Diagnostics{} }

// Add records one diagnostic item. Structured logging is the caller's job.
func (d *Diagnostics) Add(level DiagLevel, component, msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items = append(d.items, DiagItem{Level: level, Component: component, Message: msg})
}

// Snapshot returns a copy of the collected items.
func (d *Diagnostics) Snapshot() []DiagItem {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DiagItem, len(d.items))
	copy(out, d.items)
	return out
}

// HasErrors reports whether any error-level item is present.
func (d *Diagnostics) HasErrors() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, it := range d.items {
		if it.Level == LevelError {
			return true
		}
	}
	return false
}

// Reset clears all items. startAndServe calls it when a new serve session
// begins, so diagnostics always describe the current startup.
func (d *Diagnostics) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items = nil
}

// diagSummary renders a snapshot as dialog message and detail block. An
// empty snapshot reads as "no issues found".
func diagSummary(items []DiagItem) (message, detail string) {
	if len(items) == 0 {
		return "本次启动未发现问题", ""
	}
	message = fmt.Sprintf("本次启动发现 %d 个问题", len(items))
	var b strings.Builder
	for _, it := range items {
		level := "警告"
		if it.Level == LevelError {
			level = "错误"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", level, it.Component, it.Message)
	}
	return message, strings.TrimRight(b.String(), "\n")
}

// notifyTitle renders the startup notification title by severity mix.
func notifyTitle(errs, warns int) string {
	if errs > 0 {
		return fmt.Sprintf("huan 启动存在问题（%d 个错误，%d 个警告）", errs, warns)
	}
	return fmt.Sprintf("huan 已启动，有 %d 个警告", warns)
}

// truncMsg caps s at n runes, appending an ellipsis when cut.
func truncMsg(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/desktop/ -run 'TestDiagnostics|TestDiagSummary|TestNotifyTitle|TestTruncMsg' -v`
Expected: PASS（全部）

- [ ] **Step 5: Commit**

```bash
git add internal/desktop/diagnostics.go internal/desktop/diagnostics_test.go
git commit -m "feat(desktop): add startup diagnostics collector with pure render helpers"
```

---

### Task 4: 菜单接入 — 启动诊断项 + 计数徽标

**Files:**
- Modify: `internal/desktop/menu.go`（Controller 接口 + 两个菜单项 + SetDiagCount + diagMenuLabel）
- Test: `internal/desktop/menu_test.go`（新文件，纯函数测试）

**Interfaces:**
- Consumes: 无（本任务独立于 Task 3 运行时）。
- Produces:
  - `Controller` 接口新增方法：`ShowDiagnostics()`
  - `func diagMenuLabel(n int) string`
  - `func (m *MenuSet) SetDiagCount(n int)`（托盘 + 应用菜单的 `ID: "startup-diag"` 项同时更新）

- [ ] **Step 1: 写失败测试**

Create `internal/desktop/menu_test.go`:

```go
package desktop

import "testing"

func TestDiagMenuLabel(t *testing.T) {
	if got := diagMenuLabel(0); got != "启动诊断…" {
		t.Errorf("n=0 label = %q", got)
	}
	if got := diagMenuLabel(-3); got != "启动诊断…" {
		t.Errorf("negative label = %q", got)
	}
	if got := diagMenuLabel(2); got != "启动诊断（2 个问题）…" {
		t.Errorf("n=2 label = %q", got)
	}
}

// compile-time check: a Controller implementation must gain ShowDiagnostics.
type diagControllerProbe struct{ controllerProbe }

func (diagControllerProbe) ShowDiagnostics() {}

var _ Controller = diagControllerProbe{}
```

注：若 `internal/desktop` 已有 Controller 的测试桩（grep `Controller =` 或 `controllerProbe`），复用其嵌入风格；没有则删掉 probe 两行，仅保留 `TestDiagMenuLabel`，Controller 接口新增方法的编译检查由 Task 6 的 App 实现天然覆盖（App 实现 Controller）。为让本任务独立可编译：若无现成桩，把 probe 换成下面最小实现并同时在 Task 4 内给 `App` 补空实现（见 Step 3 注）：

```go
type diagControllerProbe struct{}
func (diagControllerProbe) ToggleServe()       {}
func (diagControllerProbe) Rebuild()           {}
func (diagControllerProbe) CopyAdminToken()    {}
func (diagControllerProbe) OpenSiteFolder()    {}
func (diagControllerProbe) OpenSiteInBrowser() {}
func (diagControllerProbe) SwitchSite(string)  {}
func (diagControllerProbe) ChooseSite()        {}
func (diagControllerProbe) OpenLogs()          {}
func (diagControllerProbe) ShowWindow()        {}
func (diagControllerProbe) Quit()              {}
func (diagControllerProbe) ShowDiagnostics()   {}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/desktop/ -run TestDiagMenuLabel -v`
Expected: FAIL（`diagMenuLabel` 未定义）

- [ ] **Step 3: 实现 menu.go**

`Controller` 接口（`Quit()` 行后）加：

```go
	ShowDiagnostics() // open the startup-diagnostics dialog
```

菜单项 ID 常量区（`itemCopyToken` 后）加：

```go
	itemDiag = "startup-diag"
```

`BuildTrayMenu` 的 `itemCopyToken` 项之后加：

```go
		{Label: diagMenuLabel(0), ID: itemDiag, Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowDiagnostics() }},
```

`BuildAppMenu` 的「站点」子菜单 `复制管理 Token` 项之后加：

```go
			{Label: diagMenuLabel(0), ID: itemDiag, Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowDiagnostics() }},
```

`MenuSet` 方法区（`SetBuilding` 后）加：

```go
// diagMenuLabel renders the startup-diagnostics menu title for n items.
func diagMenuLabel(n int) string {
	if n <= 0 {
		return "启动诊断…"
	}
	return fmt.Sprintf("启动诊断（%d 个问题）…", n)
}

// SetDiagCount updates the startup-diagnostics item label in both menus.
func (m *MenuSet) SetDiagCount(n int) {
	label := diagMenuLabel(n)
	if item := m.TrayMenu.ItemByID(itemDiag); item != nil {
		item.SetLabel(label)
	}
	if item := m.AppMenu.ItemByID(itemDiag); item != nil {
		item.SetLabel(label)
	}
}
```

（`menu.go` 按需补 import `"fmt"`。）

若 Step 1 采用了独立 probe 且 App 尚未实现 `ShowDiagnostics`，本任务在 `app.go` 的 `App` 方法区加临时空实现保证编译（Task 6 替换为真实现）：

```go
// ShowDiagnostics opens the startup-diagnostics dialog (wired in full in the
// app-integration task).
func (a *App) ShowDiagnostics() { a.showDiagnostics() }
```

并同样加占位 `showDiagnostics`（Task 6 填充）：

```go
func (a *App) showDiagnostics() {}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/desktop/ -run TestDiagMenuLabel -v && go build ./...`
Expected: PASS + 构建成功

- [ ] **Step 5: Commit**

```bash
git add internal/desktop/menu.go internal/desktop/menu_test.go internal/desktop/app.go
git commit -m "feat(desktop): add startup-diagnostics menu items with count badge"
```

---

### Task 5: 警告流入桌面运行时 — ServeOptions.OnPluginWarnings + BuildSiteTo 参数

**Files:**
- Modify: `internal/desktop/server.go`（ServeOptions 加回调；Start 内注册表创建后与 LifecycleManager 创建后各接线一处）
- Modify: `internal/desktop/build.go`（BuildSiteTo 加回调参数）
- Modify: `internal/desktop/app.go:436`（Rebuild 调用点适配）
- Test: `internal/desktop/server_test.go`、`internal/desktop/build_test.go`

**Interfaces:**
- Consumes: Task 1 的 `plugin.LoadWarning`/`Registry.Warnings`、Task 2 的 `LifecycleManager.OnWarnings`；server_test.go 既有 `writeSite(t)` helper。
- Produces:
  - `ServeOptions.OnPluginWarnings func([]plugin.LoadWarning)`（可选；serve goroutine 语境，静态+动态扫描各触发一次）
  - `BuildSiteTo(sourceDir, outputDir string, logf func(format string, a ...any), onPluginWarnings func([]plugin.LoadWarning)) error`

- [ ] **Step 1: 写失败测试（serve 路径）**

追加到 `internal/desktop/server_test.go`（package desktop，已 import plugin? 若无按需补 `"github.com/iannil/huan/internal/plugin"`）：

```go
// writeSiteWithBrokenPlugin extends writeSite's site with a declared static
// plugin whose .so is garbage — the canonical startup-warning fixture.
func writeSiteWithBrokenPlugin(t *testing.T) string {
	t.Helper()
	src := writeSite(t)
	pluginsDir := filepath.Join(src, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "broken.so"), []byte("garbage"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(src, "huan.yaml")
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, append(body, []byte("plugins:\n  broken:\n    category: static\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestServeRuntimeReportsPluginWarnings(t *testing.T) {
	src := writeSiteWithBrokenPlugin(t)
	var mu sync.Mutex
	var seen []plugin.LoadWarning
	rt := NewServeRuntime(ServeOptions{
		SourceDir: src,
		OnPluginWarnings: func(ws []plugin.LoadWarning) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, ws...)
		},
	})
	url, err := rt.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rt.Stop(5 * time.Second)
	if url == "" {
		t.Fatal("empty admin url")
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, w := range seen {
		if w.Name == "broken.so" {
			found = true
		}
	}
	if !found {
		t.Fatalf("broken.so not among warnings: %+v", seen)
	}
}
```

（`sync` 已在 server_test import 列表——见文件头；`plugin`/`time` 按需补。`rt.Start()`/`rt.Stop(5 * time.Second)` 的签名以 server.go 现有 Start/Stop 为准——与 app.go startAndServe 的调用一致。）

- [ ] **Step 2: 写失败测试（build 路径）**

追加到 `internal/desktop/build_test.go`（package desktop）：

```go
func TestBuildSiteToReportsPluginWarnings(t *testing.T) {
	src := writeSiteWithBrokenPlugin(t)
	var got []plugin.LoadWarning
	err := BuildSiteTo(src, t.TempDir(), func(string, ...any) {}, func(ws []plugin.LoadWarning) { got = ws })
	if err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	if len(got) != 1 || got[0].Name != "broken.so" {
		t.Fatalf("want exactly broken.so warning, got %+v", got)
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/desktop/ -run 'TestServeRuntimeReportsPluginWarnings|TestBuildSiteToReportsPluginWarnings' -v`
Expected: FAIL（`OnPluginWarnings` 字段/`BuildSiteTo` 参数不存在 → 编译错误）

- [ ] **Step 4: 实现**

`internal/desktop/server.go` — `ServeOptions` struct 的 `Logger` 字段注释块后加：

```go
	// OnPluginWarnings is invoked with plugins that failed to load while
	// starting this serve session — the static registry scan and the
	// dynamic lifecycle scan each fire it once (empty slices are never
	// delivered). Nil drops warnings. Invoked from the serve goroutine.
	OnPluginWarnings func([]plugin.LoadWarning)
```

`Start` 内 `reg, err := plugin.NewRegistryFromConfig(...)` 的错误检查后加：

```go
	if len(reg.Warnings) > 0 && r.opts.OnPluginWarnings != nil {
		r.opts.OnPluginWarnings(reg.Warnings)
	}
```

`adminPluginMgr := plugin.NewLifecycleManager(...)`（约 296 行）后加：

```go
	adminPluginMgr.OnWarnings = r.opts.OnPluginWarnings
```

（OnWarnings 为 nil 安全——nil 回调时赋 nil 即可，LifecycleManager 已判 nil。）

`internal/desktop/build.go` — `BuildSiteTo` 签名改为：

```go
func BuildSiteTo(sourceDir, outputDir string, logf func(format string, a ...any), onPluginWarnings func([]plugin.LoadWarning)) error {
```

`reg, err := plugin.NewRegistryFromConfig(cfg, sourceDir, "")` 错误检查后加：

```go
	if len(reg.Warnings) > 0 && onPluginWarnings != nil {
		onPluginWarnings(reg.Warnings)
	}
```

`internal/desktop/app.go` — `Rebuild` 内 `BuildSiteTo(rt.SourceDir(), outDir, func(...))` 调用改为四参（warnings 落诊断，LevelWarn、组件 "build"）：

```go
		err := BuildSiteTo(rt.SourceDir(), outDir, func(format string, args ...any) {
			a.log.Log("build", observability.EventPoint, map[string]any{"msg": fmt.Sprintf(format, args...)})
		}, func(ws []plugin.LoadWarning) {
			for _, w := range ws {
				msg := fmt.Sprintf("%s: %v", w.Name, w.Err)
				a.diag.Add(LevelWarn, "build", msg)
				a.log.Log("diag", observability.EventPoint, map[string]any{"component": "build", "msg": msg})
			}
			a.refreshDiagMenu()
		})
```

（`a.diag`/`refreshDiagMenu` 在 Task 6 定义；本任务先加 `App` 的最小占位保证编译：`internal/desktop/app.go` 的 App struct 加字段 `diag *Diagnostics`，Run() 中 `app.sm = NewMachine()` 后加 `app.diag = NewDiagnostics()`，并加占位方法：

```go
// refreshDiagMenu updates the menu badge; full wiring lands with the app
// integration task.
func (a *App) refreshDiagMenu() {}
```
Task 6 替换其实现。若 `plugin` 包未在 app.go import，按需补。）

- [ ] **Step 5: 运行确认通过 + 回归**

Run: `go test ./internal/desktop/ -run 'TestServeRuntimeReportsPluginWarnings|TestBuildSiteToReportsPluginWarnings' -v && go test ./internal/desktop/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/desktop/server.go internal/desktop/build.go internal/desktop/app.go internal/desktop/server_test.go internal/desktop/build_test.go
git commit -m "feat(desktop): route plugin load warnings into serve/build runtimes"
```

---

### Task 6: App 集成 — 诊断接线、通知、对话框、Dock 图标

**Files:**
- Modify: `internal/desktop/app.go`（diag 字段真实接线、startAndServe/onReady/OnServeExited/OnRebuildFailed/Rebuild 五处登记、reportStartupDiag、showDiagnostics、OnNotificationClick、Dock.SetIcon）
- Modify: `internal/desktop/diagnostics_test.go`（补 refreshDiagMenu 可测部分——无，纯接线靠人工验收；不新增）

**Interfaces:**
- Consumes: Task 3 全部符号、Task 4 `SetDiagCount`/`diagMenuLabel`、Task 5 `ServeOptions.OnPluginWarnings`、`assets.go` 的 `iconPNG []byte`、mygo API：`mygo.App.OnNotificationClick(func(id string)) (off func())`、`mygo.App.Dock.SetIcon(png []byte) error`、`mygo.Dialog.Message(mygo.MessageOptions{Type, Title, Message, Detail}) (mygo.MessageResult, error)`、`mygo.MessageNone`/`mygo.MessageError`、`mygo.NewNotification(mygo.NotificationOptions{ID, Title, Body})`（现有 `a.notify` 封装已用 `Title/Body`，本任务在其调用点扩展 ID —— `notify` helper 加带 ID 变体 `notifyID(id, title, body string)`，`notify` 转调 `notifyID("", ...)` 保持既有调用不变）。
- Produces: `App` 实现 `ShowDiagnostics()`（替换 Task 4 占位）；`reportStartupDiag()`/`refreshDiagMenu()`/`showDiagnostics()` 实装。

- [ ] **Step 1: 实现 App 集成**

`app.go` 各处（占位替换 + 新增；保持既注释不变）：

a. `Run()` 中 `app.sm = NewMachine()` 后（Task 5 已加 diag 初始化则跳过）确认有 `app.diag = NewDiagnostics()`。

b. `onReady` 顶部（`menus, err := BuildTray(a, ...)` 之前）加：

```go
	// Brand dock icon for bare-binary dev launches. A .app bundle carries
	// its own icns; setting again with identical art is harmless.
	if err := mygo.App.Dock.SetIcon(iconPNG); err != nil {
		a.log.LogError("dock-icon", err, nil)
	}
	// Notification click → startup diagnostics dialog (fixed ID routing).
	mygo.App.OnNotificationClick(func(id string) {
		if id == notifStartupDiag {
			a.showDiagnostics()
		}
	})
```

c. `onReady` 站点校验失败分支（`mygo.Dialog.Error("站点不可用", err.Error())` 处）加：

```go
			a.diag.Add(LevelError, "site", err.Error())
			a.log.Log("diag", observability.EventPoint, map[string]any{"component": "site", "msg": err.Error()})
```

d. `onReady` 的 no-site early return（`return // stay in tray; user can pick via menu`）前加 `a.reportStartupDiag()`。

e. `onReady` 末尾 `a.startAndServe(site)` 之后、`a.showWindow()` 之前加 `a.reportStartupDiag()`。

f. `startAndServe`：`a.startMu.Lock()` 之后加 `a.diag.Reset()`；`ServeOptions{...}` 字面量中 `Logf` 之后加：

```go
		OnPluginWarnings: func(ws []plugin.LoadWarning) {
			for _, w := range ws {
				msg := fmt.Sprintf("%s: %v", w.Name, w.Err)
				a.diag.Add(LevelWarn, "plugin", msg)
				a.log.Log("diag", observability.EventPoint, map[string]any{"component": "plugin", "msg": msg})
			}
			a.refreshDiagMenu()
		},
```

g. `rt.Start()` 失败分支（`mygo.Dialog.Error("预览服务启动失败", err.Error())` 处）加：

```go
		a.diag.Add(LevelError, "serve", err.Error())
		a.log.Log("diag", observability.EventPoint, map[string]any{"component": "serve", "msg": err.Error()})
		a.refreshDiagMenu()
```

h. `rt.OnServeExited` 非预期退出分支（`a.notify("huan 预览服务已停止", ...)` 处）加：

```go
		a.diag.Add(LevelError, "serve", err.Error())
		a.refreshDiagMenu()
```

i. `rt.OnRebuildFailed` 回调改为：

```go
	rt.OnRebuildFailed = func(err error) {
		a.diag.Add(LevelError, "build", err.Error())
		a.refreshDiagMenu()
		a.notify("重建失败", err.Error())
	}
```

（原实现是 `a.notify("重建失败", err.Error())` 单行，扩为上式。）

j. `notify` helper 重构 + 新增 `notifyID`：

```go
// notify shows a best-effort system notification; denial is not fatal.
func (a *App) notify(title, body string) {
	a.notifyID("", title, body)
}

func (a *App) notifyID(id, title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: title, Body: body})
	if err := n.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
		a.log.LogError("notify", err, nil)
	}
}
```

k. 替换 Task 4/5 占位为实装（`app.go` 方法区）：

```go
// refreshDiagMenu syncs the startup-diagnostics menu badge with the
// collector's current size.
func (a *App) refreshDiagMenu() {
	if a.menus == nil {
		return
	}
	a.menus.SetDiagCount(len(a.diag.Snapshot()))
}

// reportStartupDiag refreshes the menu badge and notifies when the startup
// collected any items — warnings included (a plugin version mismatch is
// warn-level and must not be silent; spec decision 2026-11-18).
func (a *App) reportStartupDiag() {
	items := a.diag.Snapshot()
	a.refreshDiagMenu()
	if len(items) == 0 {
		return
	}
	errs, warns := 0, 0
	for _, it := range items {
		if it.Level == LevelError {
			errs++
		} else {
			warns++
		}
	}
	a.log.Log("startup-diag", observability.EventBranch, map[string]any{"errors": errs, "warnings": warns})
	first := items[0]
	a.notifyID(notifStartupDiag, notifyTitle(errs, warns),
		truncMsg(fmt.Sprintf("%s: %s", first.Component, first.Message), 100)+"（点击通知查看详情）")
}

// ShowDiagnostics implements Controller: opens the startup-diagnostics dialog.
func (a *App) ShowDiagnostics() { a.showDiagnostics() }

// showDiagnostics renders the collector snapshot as a native dialog;
// error-level items switch the dialog to the error style.
func (a *App) showDiagnostics() {
	items := a.diag.Snapshot()
	title, detail := diagSummary(items)
	dialogType := mygo.MessageNone
	if a.diag.HasErrors() {
		dialogType = mygo.MessageError
	}
	if _, err := mygo.Dialog.Message(mygo.MessageOptions{
		Type:    dialogType,
		Title:   "启动诊断",
		Message: title,
		Detail:  detail,
	}); err != nil {
		a.log.LogError("show-diagnostics", err, nil)
	}
}
```

（app.go 按需补 import `"github.com/iannil/huan/internal/plugin"`——Task 5 若已补则跳过。）

- [ ] **Step 2: 编译 + 全包测试**

Run: `go build ./... && go test ./internal/desktop/... && go vet ./internal/desktop/...`
Expected: PASS（无新失败；既有测试若因 Rebuild 签名适配需更新调用则修测试）

- [ ] **Step 3: 冒烟（源码启动一次）**

Run: `go build -o /tmp/huan-desktop-smoke ./cmd/huan-desktop && /tmp/huan-desktop-smoke`（后台），等待 ~10s 后：

```bash
grep -c '"component":"plugin"' ~/Library/Application\ Support/huan-desktop/logs/desktop.log || true
tail -5 ~/Library/Application\ Support/huan-desktop/logs/desktop.log
```

Expected: 应用启动、窗口出现；若本机 `~/.huan/plugins` 仍有旧工具链插件，desktop.log 出现 `component:plugin` 诊断点且系统通知弹出。确认后 `pkill -f huan-desktop-smoke`。

- [ ] **Step 4: Commit**

```bash
git add internal/desktop/app.go
git commit -m "feat(desktop): wire diagnostics into app lifecycle with notification and dialog"
```

---

### Task 7: 品牌托盘字形 — gen-desktop-assets 重写

**Files:**
- Modify: `scripts/gen-desktop-assets/main.go`（整体重写：仅生成 tray.png，品牌几何黑 template）
- Test: 视觉验证（Read 生成的 PNG）+ 现有构建不回归

**Interfaces:**
- Produces: `internal/desktop/assets/tray.png`（18×18 黑+alpha，品牌三平行四边形，~78% 画布覆盖，8x 超采样）；不再生成占位 icon.png（icon.png 自 Task 8 起来自品牌资产复制）。

- [ ] **Step 1: 重写生成器**

`scripts/gen-desktop-assets/main.go` 全文替换为：

```go
// Command gen-desktop-assets writes the huan desktop tray icon from the
// brand geometry (web/admin/public/brand/huan-primary.svg): three cream
// parallelograms on a red square, reduced to a black template glyph that
// macOS tints adaptively in the menu bar. Run once from the repo root:
//
//	go run ./scripts/gen-desktop-assets
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// glyph holds the three brand parallelograms in huan-primary.svg coordinates
// (viewBox 0 0 1000 1000).
var glyph = [3][4][2]float64{
	{{240, 295}, {570, 205}, {760, 315}, {430, 405}},
	{{240, 470}, {570, 380}, {760, 490}, {430, 580}},
	{{240, 645}, {570, 555}, {760, 665}, {430, 755}},
}

func main() {
	outDir := filepath.Join("internal", "desktop", "assets")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	if err := writeTray(filepath.Join(outDir, "tray.png")); err != nil {
		panic(err)
	}
}

// writeTray renders the glyph into an 18x18 black+alpha template PNG,
// supersampled 8x with box downsampling for smooth edges. The glyph fills
// ~78% of the canvas, the macOS menu-bar convention (Template images stay
// black+alpha; macOS recolors for light/dark menus).
func writeTray(path string) error {
	const (
		size      = 18.0
		ss        = 8
		fillRatio = 0.78
	)
	minX, minY := glyph[0][0][0], glyph[0][0][1]
	maxX, maxY := minX, minY
	for _, poly := range glyph {
		for _, p := range poly {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
	}
	gw, gh := maxX-minX, maxY-minY
	scale := size * fillRatio / gw
	if size*fillRatio/gh < scale {
		scale = size * fillRatio / gh
	}
	ox := (size - gw*scale) / 2
	oy := (size - gh*scale) / 2

	// contains reports whether the canvas point (x, y) maps into any brand
	// parallelogram (convex quads: consistent cross-product sign test).
	contains := func(x, y float64) bool {
		bx := (x-ox)/scale + minX
		by := (y-oy)/scale + minY
		for _, poly := range glyph {
			sign := 0.0
			inside := true
			for i := 0; i < 4; i++ {
				a := poly[i]
				b := poly[(i+1)%4]
				cross := (b[0]-a[0])*(by-a[1]) - (b[1]-a[1])*(bx-a[0])
				if sign == 0 {
					sign = cross
				} else if (cross > 0) != (sign > 0) {
					inside = false
					break
				}
			}
			if inside {
				return true
			}
		}
		return false
	}

	img := image.NewNRGBA(image.Rect(0, 0, int(size), int(size)))
	for y := 0; y < int(size); y++ {
		for x := 0; x < int(size); x++ {
			count := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(ss)
					py := float64(y) + (float64(sy)+0.5)/float64(ss)
					if contains(px, py) {
						count++
					}
				}
			}
			if count > 0 {
				a := uint8(count * 255 / (ss * ss))
				img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: a})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
```

- [ ] **Step 2: 生成并视觉验证**

Run: `go run ./scripts/gen-desktop-assets && file internal/desktop/assets/tray.png`
Expected: `PNG image data, 18 x 18, 8-bit/color RGBA, non-interlaced`

用 Read 工具查看 `internal/desktop/assets/tray.png`：应见三道黑色斜条（米白平行四边形的黑色单色版），边缘平滑、无整片糊黑。若形状碎裂/过粗，调 `fillRatio`（0.70–0.85）重跑。

- [ ] **Step 3: 编译回归**

Run: `go build ./...`
Expected: 成功（assets.go 的 embed 名称未变）

- [ ] **Step 4: Commit**

```bash
git add scripts/gen-desktop-assets/main.go internal/desktop/assets/tray.png
git commit -m "feat(desktop): draw brand tray template glyph from huan-primary geometry"
```

---

### Task 8: 品牌图标生成脚本 + 资产入库

**Files:**
- Create: `scripts/gen-brand-icons.sh`
- Create: `internal/desktop/assets/app.icns`（生成物，入库）
- Modify: `internal/desktop/assets/icon.png`（品牌 512 替换占位盘）
- 入库: `web/admin/public/brand/`（设计资产，untracked → git add）

**Interfaces:**
- Consumes: `web/admin/public/brand/huan-icon-{16,32,64,128,256,512}.png`、`huan-primary-1024.png`；Task 7 的 gen-desktop-assets。
- Produces: `internal/desktop/assets/app.icns`（Task 9 的 mygo.json 引用）、`icon.png`（Task 6 的 `Dock.SetIcon(iconPNG)` 用，512×512）。

- [ ] **Step 1: 写脚本**

Create `scripts/gen-brand-icons.sh`:

```bash
#!/usr/bin/env bash
#
# gen-brand-icons.sh — derive the desktop brand icons from the confirmed
# brand assets in web/admin/public/brand (PRODUCT-LOGOS.md):
#
#   internal/desktop/assets/app.icns  macOS bundle icon (Apple iconset sizes)
#   internal/desktop/assets/icon.png  runtime Dock icon (huan-icon-512)
#   internal/desktop/assets/tray.png  regenerated by gen-desktop-assets
#
# All iconset sizes map from existing brand PNGs — no rasterization needed.
# Run from the repo root: scripts/gen-brand-icons.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BRAND="$ROOT/web/admin/public/brand"
OUT="$ROOT/internal/desktop/assets"

for f in huan-icon-16.png huan-icon-32.png huan-icon-64.png huan-icon-128.png \
         huan-icon-256.png huan-icon-512.png huan-primary-1024.png; do
  [ -f "$BRAND/$f" ] || { echo "missing brand asset: $BRAND/$f" >&2; exit 1; }
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
ICONSET="$TMP/huan.iconset"
mkdir -p "$ICONSET"

cp "$BRAND/huan-icon-16.png"     "$ICONSET/icon_16x16.png"
cp "$BRAND/huan-icon-32.png"     "$ICONSET/icon_16x16@2x.png"
cp "$BRAND/huan-icon-32.png"     "$ICONSET/icon_32x32.png"
cp "$BRAND/huan-icon-64.png"     "$ICONSET/icon_32x32@2x.png"
cp "$BRAND/huan-icon-128.png"    "$ICONSET/icon_128x128.png"
cp "$BRAND/huan-icon-256.png"    "$ICONSET/icon_128x128@2x.png"
cp "$BRAND/huan-icon-256.png"    "$ICONSET/icon_256x256.png"
cp "$BRAND/huan-icon-512.png"    "$ICONSET/icon_256x256@2x.png"
cp "$BRAND/huan-icon-512.png"    "$ICONSET/icon_512x512.png"
cp "$BRAND/huan-primary-1024.png" "$ICONSET/icon_512x512@2x.png"

iconutil -c icns "$ICONSET" -o "$OUT/app.icns"
cp "$BRAND/huan-icon-512.png" "$OUT/icon.png"
go run "$ROOT/scripts/gen-desktop-assets"

echo "brand icons written:"
ls -la "$OUT"
```

- [ ] **Step 2: 运行脚本**

```bash
chmod +x scripts/gen-brand-icons.sh && scripts/gen-brand-icons.sh
```

Expected: `app.icns` 生成（数百 KB），`icon.png` 512×512，`tray.png` 18×18；无报错。

- [ ] **Step 3: 验证产物**

Run: `file internal/desktop/assets/icon.png internal/desktop/assets/app.icns`
Expected: `icon.png: PNG image data, 512 x 512`；`app.icns: Mac OS X icon file`（或 `MacOS icon file`）。再用 Read 查看 icon.png 确认为品牌红底三斜条图。

- [ ] **Step 4: 品牌资产与产物入库**

```bash
git add scripts/gen-brand-icons.sh internal/desktop/assets/app.icns internal/desktop/assets/icon.png web/admin/public/brand
git status --short   # 确认无 *.so / release / build 混入
git commit -m "feat(desktop): generate brand app icons from confirmed brand assets"
```

（icon.png/tray.png 若 Task 7 已提交且本任务无变化则无需重复 add。）

---

### Task 9: mygo.json 接图标 + 版本 0.13.1 + 重出发布物

**Files:**
- Modify: `mygo.json`
- Create: `release/desktop/v0.13.1/`（不入库；README/SHA256SUMS 随产物落盘）

**Interfaces:**
- Consumes: Task 8 的 `internal/desktop/assets/app.icns`；v0.13.0 发布流程（`mygo build -platform darwin/arm64` + ditto 归档 + SHA256SUMS，见 `release/desktop/v0.13.0/README.md`）。
- Produces: v0.13.1 的 .app/.zip/.dmg/SHA256SUMS/README。

- [ ] **Step 1: 更新 mygo.json**

全文替换为：

```json
{
  "name": "huan",
  "slug": "huan",
  "identifier": "io.github.iannil.huan.desktop",
  "version": "0.13.1",
  "main": "github.com/iannil/huan/cmd/huan-desktop",
  "out": "build/desktop",
  "macos": { "icon": "internal/desktop/assets/app.icns" }
}
```

- [ ] **Step 2: 构建 .app / DMG**

Run: `command -v mygo || go run github.com/egoist/mygo/cmd/mygo@v0.9.2 -h`（确认 CLI 可用），然后仓库根目录：

```bash
mygo build -platform darwin/arm64
ls -la build/desktop/
ls build/desktop/huan.app/Contents/Resources/ | grep -i icns
```

Expected: `build/desktop/huan.app` 与 DMG 生成；Resources 内出现 `AppIcon.icns`（或同义命名）。若无 icns，检查 mygo 输出日志中 icon 路径解析错误并修正 `macos.icon` 相对路径后重跑。

- [ ] **Step 3: 启动验证 bundle 图标**

```bash
open build/desktop/huan.app
sleep 5 && pgrep -fl huan | head -3
```

Expected: 应用启动，Dock 中显示 huan 品牌图标（朱红方底三斜条）。人工目检后 `pkill -x huan`（或经托盘菜单退出）。

- [ ] **Step 4: 归档发布物**

```bash
mkdir -p release/desktop/v0.13.1
cd build/desktop
ditto -c -k --keepParent huan.app ../../release/desktop/v0.13.1/huan-v0.13.1-darwin-arm64.app.zip
# DMG 若 mygo build 已产出则直接复制：
ls *.dmg 2>/dev/null && cp ./*.dmg ../../release/desktop/v0.13.1/huan-v0.13.1-darwin-arm64.dmg || echo "DMG 需 mygo 子命令生成（见 mygo --help）"
cd ../..
shasum -a 256 release/desktop/v0.13.1/huan-v0.13.1-darwin-arm64.* > release/desktop/v0.13.1/SHA256SUMS
shasum -a 256 -c release/desktop/v0.13.1/SHA256SUMS
```

Expected: 校验 OK。

- [ ] **Step 5: 发布 README**

复制 `release/desktop/v0.13.0/README.md` 到 `release/desktop/v0.13.1/README.md` 并做三处修改：
1. 标题与产物表：v0.13.0 → v0.13.1；
2. 功能清单新增：「**启动诊断**：插件加载警告、serve 启动失败等问题统一收集；错误/警告经系统通知提示（点击通知打开详情），托盘与应用菜单常驻『启动诊断…』项（有问题时标题带计数）」；功能清单图标行更新为「应用图标为 huan 品牌几何图标（朱红方底）」；
3. 已知限制：删除第 4 条「应用图标为 MyGo 默认图」；其余保留。

- [ ] **Step 6: Commit**

```bash
git add mygo.json
git commit -m "chore(release): huan desktop v0.13.1 — brand icon into bundle, startup diagnostics"
```

（`release/`、`build/` 均不入库，符合发布约定。）

---

### Task 10: 插件运营修复 — 当前工具链重编并部署

**Files:**
- 产物（不入库）: `release/plugins/*.so`、`~/.huan/plugins/*.so`、`~/.huan/plugins-stale-backup/`

**Interfaces:**
- Consumes: `scripts/build-plugins.sh`（Name() 探测 + segcount 修复 + adhoc 重签）。
- Produces: 7 个 .so 与 go1.27.3 宿主匹配；警告归零（Task 11 验证）。

- [ ] **Step 1: 备份现有（旧）插件供复现验收**

```bash
mkdir -p ~/.huan/plugins-stale-backup && cp ~/.huan/plugins/*.so ~/.huan/plugins-stale-backup/
ls ~/.huan/plugins-stale-backup/
```

Expected: 7 个 .so 备份（供 Task 11 人工验收第 2 项复现「版本不匹配」警告）。

- [ ] **Step 2: 重编 + 部署**

```bash
scripts/build-plugins.sh release/plugins
cp release/plugins/*.so ~/.huan/plugins/
go version   # 记录编译工具链
```

Expected: `built 7 plugin(s)`（或仓库实际插件数），全部走 segcount 修复 + codesign。

- [ ] **Step 3: 快速验证加载**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo && go run /Users/rong.zhu/Code/zhurong/huan/cmd/huan serve 2>&1 | head -20
```

（或直接用桌面端启动，Task 11 统一验证。）Expected: 无 `plugin load warning` 行。Ctrl+C 退出。

---

### Task 11: 全量验证 + 人工验收 + 文档收尾

**Files:**
- Modify: spec 文档状态行、`docs/progress/`（如有进行中文档）、`docs/reports/2026-11-18-desktop-startup-diagnostics-acceptance.md`（新验收报告）、`memory/daily/2026-11-18.md`

- [ ] **Step 1: 自动化全量**

```bash
gofmt -l internal/ pkg/ cmd/ scripts/ | grep -v '^$' || true
go vet ./... && go test ./...
```

Expected: gofmt 无输出（或仅既有漂移文件，与本次改动交集为空）；vet/test 全绿。

- [ ] **Step 2: 源码启动 + 人工验收清单（spec §验收）**

`go build -o /tmp/huan-desktop-final ./cmd/huan-desktop && /tmp/huan-desktop-final`（后台），逐项核验：

1. **缺失警告复现**：`mv ~/.huan/plugins/html-injector.so /tmp/`，托盘菜单退出应用重启 → 系统通知「huan 已启动，有 1 个警告」弹出；托盘菜单显示「启动诊断（1 个问题）…」；点击 → 对话框列 `[警告] plugin: html-injector.so: ...`。验后还原文件。
2. **版本不匹配复现**：`cp ~/.huan/plugins-stale-backup/diagram-renderer.so ~/.huan/plugins/`（覆盖新版），重启 → 同上，警告消息含 `built with a different version`。验后用 `release/plugins/` 新版覆盖还原。
3. **三处图标目检**：源码启动 Dock 图标（运行时 SetIcon）；`release/desktop/v0.13.1` 的 .app 在 Finder/Dock 的 icns；菜单栏托盘 template（浅色/深色菜单各看一次，macOS 深浅切换验证自动着色）。
4. **bundle 图标**：Task 9 Step 3 已验，此处复核记录。
5. **干净启动**：插件齐全时重启 → 无通知，菜单显示「启动诊断…」，点击详情「本次启动未发现问题」。

每项记录：通过/失败 + 证据（截图/日志行）。失败则回到对应 Task 修复后重验。

- [ ] **Step 3: 通知点击路由验证**

在验收 1/2 的任一场景中：点击系统通知 → 启动诊断对话框弹出（`OnNotificationClick` 路由生效）。

- [ ] **Step 4: 文档收尾**

1. spec 状态行改为「已实现（见验收报告）」；
2. 写 `docs/reports/2026-11-18-desktop-startup-diagnostics-acceptance.md`：六项验收结果 + 遗留（托盘 @2x、registry_config.go 三处打印未上移——记录为后续项）；
3. `memory/daily/2026-11-18.md` 追加实现条目（时间戳、关键决策、验证结果）；
4. 若 `docs/progress/` 存在本主题进行中文档，迁移至 `docs/reports/completed/`。

- [ ] **Step 5: 最终提交**

```bash
git add docs/ memory/daily/2026-11-18.md
git commit -m "docs: desktop startup diagnostics + brand icons acceptance report"
```

---

## Self-Review 记录

- **Spec 覆盖**：§1 警告显式化 → Task 1/2；§2 收集器 → Task 3；§3 UI（菜单/对话框/通知）→ Task 4/6；§4 图标三处 → Task 6（Dock.SetIcon）/7（托盘）/8+9（icns + bundle）；§5 插件运营 → Task 10；验收 → Task 11；版本 0.13.1 → Task 9。范围外项（SPA 不改、registry_config 三处打印、@2x）已在 Task 11 遗留记录。
- **占位扫描**：无 TBD/TODO；所有代码步骤含完整代码；Task 4 的 Controller probe 与 Task 5 的占位实现给了两种分支的完整代码（以仓库现状择一）。
- **类型一致性**：`LoadWarning{Name, Err}`、`Diagnostics.Add(level, component, msg)`、`SetDiagCount(n int)`、`diagMenuLabel(n int) string`、`OnPluginWarnings func([]plugin.LoadWarning)`、`BuildSiteTo(..., onPluginWarnings func([]plugin.LoadWarning))`、`OnWarnings func([]LoadWarning)` 各任务间一致；`notifStartupDiag = "startup-diag"` 与菜单 `itemDiag` 同值不同常量（通知 ID vs 菜单 ID，语义不同，命名已区分）。
- **风险标注**：`rt.Start()`/`Stop` 签名、`mygo` CLI 子命令名、`config.Plugins` yaml 键名——各任务步骤内已给出核对方式（与 app.go 既有调用对齐 / `mygo --help` / grep config.go）。
