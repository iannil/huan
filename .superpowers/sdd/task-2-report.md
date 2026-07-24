# Task 2 Report: SEO 注入器插件集成 — Plugin + 注册

**Status:** Completed

**Date:** 2026-07-24

**Commits:** (not yet committed)

## Summary

Integrated the SEO injector core (Task 1) into the huan plugin system as a compiled-in plugin implementing `build.Hook` and `plugin.Plugin`.

## Files Modified

1. **`internal/seo/injector/plugin.go`** — Added `SEOInjector` struct with:
   - `New(cfg *Config)` constructor
   - `SetLogf(fn)` logger setter
   - `Name()` returning `"seo_injector"`
   - `OnContentLoaded` (no-op)
   - `OnPageRendered` (no-op)
   - `OnOutputWritten` — globs `**/*.html`, runs `processFile` per file
   - `processFile` — reads HTML, computes relative URL, builds `InjectOptions`, calls `InjectHTML`, writes result
   - `guessKind` — infers `home`/`section`/`page` from relative path (with pagination exclusion for `/page/` paths)
   - `extractTitle` — extracts `<title>` from HTML via `golang.org/x/net/html`
   - Updated imports to include `context`, `os`, `path/filepath`, `strings`, `github.com/iannil/huan/internal/content`, `golang.org/x/net/html`

2. **`internal/seo/injector/plugin_test.go`** — Added 5 tests:
   - `TestNewSEOInjector` — verifies `Name()`
   - `TestSEOInjector_HooksReturnNil` — verifies `OnContentLoaded`/`OnPageRendered` return nil
   - `TestGuessKind` — 6 test cases covering home/section/page paths
   - `TestExtractTitle` — verifies `<title>` extraction
   - `TestProcessFile` — end-to-end test writing HTML, processing, and verifying injection marker + og:title

3. **`cmd/huan/plugins.go`** — Registered `seo_injector` case in `newPluginRegistry` switch, added `import "github.com/iannil/huan/internal/seo/injector"`

## Test Results

```
go test ./internal/seo/injector/ -v  => PASS (15 tests)
go test ./cmd/huan/ -v              => PASS (50 tests)
go build ./cmd/huan/                => success
```

## Key Decision

`guessKind` needed a pagination exclusion: paths like `posts/page/2/index.html` should be `"page"` not `"section"`. Added a check for `/page/` in the path to handle this.

## Concerns

- `OnOutputWritten` uses `filepath.Glob("**/*.html")` which works on most Go versions but `**` glob behavior varies by OS. On Windows the `filepath.Separator` replacement in `processFile` handles the path-to-URL conversion.
- `guessKind` is a heuristic — it may not match Hugo's exact kind classification for all edge cases (e.g., taxonomy pages, 404 pages). This is acceptable for v1.