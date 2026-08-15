# `--plugins` Flag Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--plugins` flag to `huan build` and `huan dev` commands to override the default plugin directory.

> **状态**：✅ 已完成（2026-07-30，checkbox 于 2026-08-15 回勾）。
> Task 4 执行偏差：两个旧测试的签名调用已随 Task 1（`cb12eaa`）一并更新，故原计划的"更新旧测试"步骤实际改为新增回归测试 `TestNewPluginRegistry_PluginDirOverride`（`841b12e`）。全部提交：`cb12eaa` → `5b7ea1c` → `ef4d227`/`ab1fd50` → `841b12e`。

**Architecture:** Add a `--plugins` string flag to both `buildCmd` and `devCmd`, pass its value through `runBuild`/`runDev` to `newPluginRegistry`, which uses it to replace the default `sourceDir/plugins/` path when non-empty.

**Tech Stack:** Go, Cobra CLI

## Global Constraints

- `$HUAN_HOME/plugins/` global search path always takes precedence over the `--plugins` dir (existing behavior, unchanged)
- Empty/unset `--plugins` means default behavior (`sourceDir/plugins/`)
- `huan plugin list/info` subcommands are NOT affected

---

### Task 1: Update `newPluginRegistry` in `plugins.go`

**Files:**
- Modify: `cmd/huan/plugins.go:27-67`

**Interfaces:**
- Consumes: existing `pluginDirFromSource(sourceDir string) string` function
- Produces: updated `newPluginRegistry(cfg *config.Config, sourceDir string, pluginDirOverride string) (*plugin.Registry, error)` — when `pluginDirOverride` is non-empty, it replaces the default plugin dir

- [x] **Step 1: Modify `newPluginRegistry` signature**

Change from:
```go
func newPluginRegistry(cfg *config.Config, sourceDir string) (*plugin.Registry, error) {
```

Change to:
```go
func newPluginRegistry(cfg *config.Config, sourceDir string, pluginDirOverride string) (*plugin.Registry, error) {
```

- [x] **Step 2: Update the `pluginDirFromSource` call**

Inside the function body, change:
```go
pluginDir := pluginDirFromSource(sourceDir)
```

To:
```go
pluginDir := pluginDirFromSource(sourceDir)
if pluginDirOverride != "" {
    pluginDir = pluginDirOverride
}
```

- [x] **Step 3: Run existing tests to verify backward compatibility**

Run: `go test ./cmd/huan/ -run TestNewPluginRegistry -v`
Expected: PASS (existing tests pass `""` for the new param, so no behavior change)

- [x] **Step 4: Commit**

```bash
git add cmd/huan/plugins.go
git commit -m "feat: add pluginDirOverride param to newPluginRegistry"
```

---

### Task 2: Add `--plugins` flag to `huan build` in `main.go`

**Files:**
- Modify: `cmd/huan/main.go:28-38`

- [x] **Step 1: Add `--plugins` flag to build command**

After `buildCmd.Flags().Bool("minify", false, "minify output (overrides config)")` (line 38), add:

```go
buildCmd.Flags().String("plugins", "", "path to plugins directory (overrides sourceDir/plugins/)")
```

- [x] **Step 2: Pass the flag value to `newPluginRegistry` in `runBuild`**

Find the call `newPluginRegistry(cfg, sourceDir)` on line 86, change to:

```go
pluginsDir, _ := cmd.Flags().GetString("plugins")
reg, _ := newPluginRegistry(cfg, sourceDir, pluginsDir)
```

- [x] **Step 3: Build and verify**

Run: `go build -o huan ./cmd/huan && ./huan build --help`
Expected: output shows `--plugins` flag in the build command help

- [x] **Step 4: Commit**

```bash
git add cmd/huan/main.go
git commit -m "feat: add --plugins flag to huan build command"
```

---

### Task 3: Add `--plugins` flag to `huan dev` in `dev.go`

**Files:**
- Modify: `cmd/huan/dev.go:26-35`

- [x] **Step 1: Add `--plugins` flag to dev command**

After `devCmd.Flags().String("adminDev", "", "admin UI Vite dev server URL (e.g. http://localhost:5173) for hot reload")` (line 34), add:

```go
devCmd.Flags().String("plugins", "", "path to plugins directory (overrides sourceDir/plugins/)")
```

- [x] **Step 2: Pass the flag value to `newPluginRegistry` in `runDev`**

There are two calls to `newPluginRegistry` in `dev.go`:

**Line 98** (initial build):
```go
pluginsDir, _ := cmd.Flags().GetString("plugins")
reg, _ := newPluginRegistry(cfg, sourceDir, pluginsDir)
```

**Line 122** (inside `runBuild` closure, rebuild):
```go
reg, _ := newPluginRegistry(cfg, sourceDir, pluginsDir)
```
Note: `pluginsDir` is captured from the outer scope, so it's accessible inside the closure.

- [x] **Step 3: Build and verify**

Run: `go build -o huan ./cmd/huan && ./huan dev --help`
Expected: output shows `--plugins` flag in the dev command help

- [x] **Step 4: Commit**

```bash
git add cmd/huan/dev.go
git commit -m "feat: add --plugins flag to huan dev command"
```

---

### Task 4: Update existing tests for new signature

**Files:**
- Modify: `cmd/huan/plugins_test.go:48-61`

- [x] **Step 1: Update `TestNewPluginRegistry_UnknownPluginSilentlySkipped`**

Change the call on line 54:
```go
r, err := newPluginRegistry(cfg, "")
```
To:
```go
r, err := newPluginRegistry(cfg, "", "")
```

- [x] **Step 2: Update `TestNewPluginRegistry_EmptyPluginsMap`**

Change the call on line 65:
```go
r, err := newPluginRegistry(cfg, "")
```
To:
```go
r, err := newPluginRegistry(cfg, "", "")
```

- [x] **Step 3: Run all tests**

Run: `go test ./cmd/huan/ -v`
Expected: ALL PASS

- [x] **Step 4: Commit**

```bash
git add cmd/huan/plugins_test.go
git commit -m "test: update newPluginRegistry calls for new signature"
```