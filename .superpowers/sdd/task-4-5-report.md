# Task 4 & 5 Report: Plugin Decoupling — Loader Enhancement + CLI Fallback Loading

**Status:** DONE

**Date:** 2026-07-21

## Summary

Implemented two tightly coupled tasks from the plugin decoupling plan:

1. **Task 4:** Loader signature change — `LoadPlugin(path string)` → `LoadPlugin(path string, pluginCfg map[string]any)` to pass config to plugin `InitPlugin`.
2. **Task 5:** CLI command fallback — `deploy.cloudflare` and `translate.qwen3` now try to load from `.so` when the plugin is not compiled in.

## Changes Made

### Part A: Loader Enhancement

- **`internal/plugin/loader.go`**:
  - `LoadPlugin(path string)` → `LoadPlugin(path string, pluginCfg map[string]any)`
  - If `pluginCfg` is nil, passes an empty map to `InitPlugin` (backward-compatible)
  - `ScanAndLoad()` now calls `LoadPlugin(fullPath, nil)` (auto-discovery, no config)

- **`internal/plugin/lifecycle.go`**:
  - `Load(soPath string)` → `Load(soPath string, pluginCfg map[string]any)`
  - `Reload(name string, newSO string)` → `Reload(name string, newSO string, pluginCfg map[string]any)`
  - Both pass config through to `loader.LoadPlugin`

- **`internal/plugin/loader_test.go`**:
  - Updated existing tests to pass `nil` as config
  - Added `TestLoader_LoadPlugin_NilConfig` — verifies nil config doesn't panic

- **`internal/admin/api.go`**:
  - Updated `loadPlugin` handler: `h.pluginManager.Load(req.Path, nil)`
  - Updated `reloadPlugin` handler: `h.pluginManager.Reload(req.Name, req.Path, nil)`

- **`cmd/huan/plugin_load.go`**:
  - Updated `load` command: `loader.LoadPlugin(pluginPath, nil)`

### Part B: CLI Command Updates

- **`cmd/huan/deploy.go`**:
  - Added `--plugin-dir` flag to `deploy.cloudflare` command
  - When `registry.Get("cloudflare")` returns not found, probes `<sourceDir>/plugins/cloudflare.so` (or `--plugin-dir` override)
  - Passes `cfg.Plugins["cloudflare"]` config map to `loader.LoadPlugin`
  - Registers the loaded plugin on success

- **`cmd/huan/translate_cmd.go`**:
  - Added `--plugin-dir` flag to `translate.qwen3` command
  - Added `"github.com/iannil/huan/internal/plugin"` import
  - When `registry.Get("qwen3_translate")` returns not found, probes `<sourceDir>/plugins/qwen3.so`
  - Passes `cfg.Plugins["qwen3_translate"]` config with `_project_root` injected
  - On failure, prints a graceful skip message (same as before) and returns nil

## Commits

```
git add internal/plugin/loader.go internal/plugin/lifecycle.go internal/plugin/loader_test.go internal/admin/api.go cmd/huan/plugin_load.go cmd/huan/deploy.go cmd/huan/translate_cmd.go
git commit -m "feat(plugin): support passing config to .so plugin InitPlugin + CLI fallback loading"
```

## Test Results

- `go test ./internal/plugin/... -v` — **PASS** (30 tests, 0.484s)
- `go test ./cmd/huan/... -v` — **PASS** (48 tests, 0.645s)
- `go build ./...` — **PASS** (no errors)

## Concerns

- The `daemon.go` `Start()` flow uses `ScanAndLoad()` which passes nil config. If daemon-started plugins need config, they'll need a separate mechanism to associate config from `cfg.Plugins[name]` — this is noted as a future enhancement.
- The `plugin_cmd.go` `load` and `reload` commands (Admin API calls) pass nil config. The Admin API endpoint could be extended to accept config in the JSON body, but that's out of scope for this PR.