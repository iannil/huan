# Task 3 Report — RenderPageWithCache (JIT single-page rendering)

**Status:** COMPLETE
**Date:** 2026-07-22
**Commit:** `bd960b6` — `feat(build): add RenderPageWithCache for JIT single-page rendering`

## What was done

Implemented `RenderPageWithCache` in `/Users/rong.zhu/Code/zhurong/huan/internal/build/jit.go` per the task-3 brief. The function:

1. Accepts `Options`, `*PipelineCache`, and a `pageURL` string.
2. Forces `IncludeDrafts=true` internally (JIT primarily serves drafts skipped at build time).
3. Stage 1 (config): reuses `cache.SiteCfg` when available (with serve-mode overrides), else falls back to `loadConfig`.
4. Stage 2-3 (content): always reloads ALL content + renders markdown + builds tree — required so list/tag/section contexts stay consistent.
5. Stage 4 (rendering infra): reuses `cache.Templates/I18nBundle/SCRegistry/MDRenderer/Writer` + rebuilds `Renderer` with current BaseURL FuncMap; else falls back to `setupTemplatesAndWriter`.
6. Stage 5: rebuilds contexts (`buildContexts`).
7. Stage 6: locates target page in `p.site.Pages` by URL equality; returns "page not found" error if absent.
8. Stage 7: calls `p.renderSinglePage(target)` and returns the HTML (does NOT write to disk).
9. Stage 8: optional LiveReload injection (parity with full build).

Signature (verbatim from brief):
```go
func RenderPageWithCache(opts Options, cache *PipelineCache, pageURL string) (string, error)
```

## Tests added

`/Users/rong.zhu/Code/zhurong/huan/internal/build/jit_test.go` now contains:

- `TestRenderPageWithCache_RendersSinglePage` — renders `/posts/hello/` via cached pipeline; asserts title + body present.
- `TestRenderPageWithCache_PageNotFound` — `/nonexistent/` returns error containing "not found".
- `TestRenderPageWithCache_NoCacheFallback` — `cache=nil` falls back to full pipeline setup and still renders.
- `TestRenderPageWithCache_DraftPage` — draft page excluded by full build (IncludeDrafts=false) renders successfully via JIT.

Helper functions added:
- `writeJITTestSite(t)` — minimal site (huan.yaml + hello.md + draft-page.md + single.html + list.html).
- `buildJITCacheViaFullBuild(t, tmpDir)` — runs `BuildSite` with `IncludeDrafts: false` and a `PipelineCache` to populate it.

## Deviations from brief

**Helper for file writes:** The brief's Note suggested adding `osWriteFileMkdir` using `os.MkdirAll + os.WriteFile`. An equivalent test helper `writeFile(t, dir, relPath, content)` already exists in `internal/build/multisite_test.go:355` and is used throughout the build package tests (e.g., `incremental_test.go`). I reused the existing helper to avoid duplication. Functionality is identical to the brief's prescribed `osWriteFileMkdir`.

## Test results

```
go test ./internal/build/ -run "TestRenderPageWithCache" -v
--- PASS: TestRenderPageWithCache_RendersSinglePage (0.01s)
--- PASS: TestRenderPageWithCache_PageNotFound (0.00s)
--- PASS: TestRenderPageWithCache_NoCacheFallback (0.00s)
--- PASS: TestRenderPageWithCache_DraftPage (0.00s)
PASS
```

Full build package: `ok  github.com/iannil/huan/internal/build  (cached)` — no regressions.
Full project (`go test ./...`): all packages PASS.

## Commits

- `bd960b6` — feat(build): add RenderPageWithCache for JIT single-page rendering
  - Files: `internal/build/jit.go`, `internal/build/jit_test.go`

## Concerns / notes

- The test fixture's `list.html` template produces warnings during the cache-populating full build (`.URL` field access on `interface{}` inside `range .Pages`). This is a fixture-quality issue only — it does not affect the JIT tests, which target regular/draft pages rendered via `single.html`. Cached templates still load successfully (`Templates: 11`), so the cache is populated correctly.
- `RenderPageWithCache` follows the same stage-reuse pattern as `IncrementalRender` in `build.go`. If that pattern changes (e.g., new pipeline fields added to the cache), this function will need parallel updates.
- The function intentionally shares global i18n state via `tmpl.SetI18nBundle(p.i18nBundle)` (same as `IncrementalRender`). Not safe for concurrent JIT renders from different sites — consistent with existing design.
