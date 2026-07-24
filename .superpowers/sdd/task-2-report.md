# Task 2 Report: HTML 注入器插件集成 — Plugin + 注册

**Status:** Completed

**Date:** 2026-07-24

## Summary

Integrated the HTML injector core (Task 1) into the huan plugin system as a compiled-in plugin implementing `build.Hook` and `plugin.SchemaProvider`.

## Files Modified

1. **`internal/seo/htmlinjector/plugin.go`** — Added `HTMLInjector` struct with:
   - `New(cfg *Config)` constructor
   - `Name()` returning `"html_injector"`
   - `ConfigSchema()` returning the schema from cfg
   - `OnContentLoaded` (no-op, returns nil)
   - `OnPageRendered` — calls `InjectHTML` on `page.Content`, updates via `template.HTML` if changed
   - `OnOutputWritten` (no-op, returns nil)
   - Compile-time interface assertions for `plugin.Plugin` and `plugin.SchemaProvider`

2. **`internal/seo/htmlinjector/plugin_test.go`** — Added 3 tests:
   - `TestNewHTMLInjector` — verifies `Name()` returns `"html_injector"`
   - `TestHTMLInjector_OnPageRendered` — verifies script injection into page content
   - `TestHTMLInjector_HooksReturnNil` — verifies no-op hooks return nil without error

3. **`cmd/huan/plugins.go`** — Registered `html_injector` case in `newPluginRegistry` switch, added `import "github.com/iannil/huan/internal/seo/htmlinjector"`

## Test Results

```
go test ./internal/seo/htmlinjector/ ./cmd/huan/ -v  => ALL PASS (14 htmlinjector tests + 45 cmd/huan tests)
go build ./cmd/huan/                                   => success
```

## Key Decision

`HTMLInjector.OnPageRendered` uses `template.HTML` (from `html/template` package) for the content assignment, matching the `content.Page.Content` field type. The existing `content` package does not define a `content.HTML` type alias.

## Concerns

None.
