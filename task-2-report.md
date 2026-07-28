# Task 2 Report: Update internal/plugin to alias pkg/plugin types

**Date:** 2026-07-27

## Summary

Rewrote `internal/plugin/plugin.go` to alias all base types (`Plugin`, `PluginMeta`, `MetadataProvider`, `SchemaProvider`, `Schema`, `FieldSchema`, `Registry`) from `pkg/plugin/`. Removed `internal/plugin/schema.go` (types now defined in `plugin.go` via aliases). Kept `EventSubscriber` interface in `internal/plugin` since it references `internal/daemon/eventbus` types.

Also added `Sensitive` and `EnvVarHint` fields to `pkg/plugin/plugin.go`'s `FieldSchema` struct to match the existing `internal/plugin/schema.go` definitions.

## Changes

### `pkg/plugin/plugin.go`
- Added `Sensitive bool` and `EnvVarHint string` fields to `FieldSchema`

### `internal/plugin/plugin.go`
- Type-aliased to `pkg/plugin`: `Plugin`, `PluginMeta`, `MetadataProvider`, `SchemaProvider`, `Schema`, `FieldSchema`, `Registry`
- Replaced `NewRegistry()` with a forwarder to `pkgplugin.NewRegistry()`
- Replaced `Find[T]` with a forwarder to `pkgplugin.Find[T]()`
- Kept `EventSubscriber` interface (references `eventbus.EventType` and `eventbus.Event`)

### `internal/plugin/schema.go`
- Deleted (types now aliased in `plugin.go`)

## Verification

- `go build ./internal/plugin/` -- passed
- `go build ./...` -- passed (full project)
- `go test -count=1 ./internal/plugin/...` -- passed (0.376s)
- `go test -count=1 ./cmd/huan/...` -- passed (0.544s)
- `go test ./...` -- all packages passed
- Testdata `.so` plugin rebuild: succeeded (imports `internal/plugin`, which now transitively provides `pkg/plugin` types)

## Compatibility

All existing `.so` plugins use self-contained module paths (`huan-plugin-*`), not `internal/plugin`. No recompilation needed.
