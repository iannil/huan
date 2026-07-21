# Task 1 Report: Remove cloudflare and qwen3 from compiled-in plugin registry

## Status: COMPLETED

## Summary
Successfully removed cloudflare and qwen3_translate from the compiled-in plugin registry as part of the plugin decoupling plan.

## Changes Made

### 1. `cmd/huan/plugins.go`
- Removed imports for:
  - `github.com/iannil/huan/internal/deploy/cloudflare`
  - `github.com/iannil/huan/internal/translate`
  - `github.com/iannil/huan/internal/translate/qwen3`
- Removed the `cloudflare` case from `newPluginRegistry` switch statement
- Removed the `qwen3_translate` case from `newPluginRegistry` switch statement
- Removed the `translate.Translator` type assertion from `capabilityLabels` function
- The `default` case still returns an error for unknown plugins (fail-fast behavior from ADR 0003)
- The `deploy.Deployer` check remains in `capabilityLabels` for future compiled deploy plugins

### 2. `cmd/huan/plugins_test.go`
- Removed `TestNewPluginRegistry_ValidCloudflare` test (no longer applicable)
- Removed `TestNewPluginRegistry_CloudflareMissingFieldsReturnsError` test (no longer applicable)
- Retained tests for:
  - `TestNewPluginRegistry_UnknownPluginReturnsError`
  - `TestNewPluginRegistry_MultipleUnknownPlugins_FailsOnFirst`
  - `TestNewPluginRegistry_EmptyPluginsMap`
  - `TestCapabilityLabels_DeployerPluginReturnsDeployLabel`
  - `TestCapabilityLabels_NonDeployerReturnsEmpty`

### 3. `cmd/huan/plugin_cmd.go`
- No changes required (no translate import was present)

## Verification

### Build Status
```
go build ./...   # SUCCESS
go vet ./...     # SUCCESS
```

### Test Results
```
go test ./cmd/huan/... -v
```
All 34 tests passed, including:
- `TestNewPluginRegistry_UnknownPluginReturnsError`
- `TestNewPluginRegistry_MultipleUnknownPlugins_FailsOnFirst`
- `TestNewPluginRegistry_EmptyPluginsMap`
- `TestCapabilityLabels_DeployerPluginReturnsDeployLabel`
- `TestCapabilityLabels_NonDeployerReturnsEmpty`

## Commit
```
0b03d2a refactor(plugin): remove cloudflare and qwen3_translate from compiled-in plugins
```

## Notes
- The plugin code in `internal/deploy/cloudflare/` and `internal/translate/qwen3/` was intentionally left intact (Task 2 will migrate these to independent repositories)
- The fail-fast behavior for unknown plugins remains: any plugin specified in `huan.yaml` that is not compiled in will cause an error at startup
- This change prepares the codebase for the external .so plugin architecture