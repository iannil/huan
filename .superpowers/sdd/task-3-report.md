# Task 3 Report — `IncrementalRender` 函数

**Date:** 2026-07-21
**Task:** Add `IncrementalRender(opts Options, cache *PipelineCache, affectedURLs []string) error` to `internal/build/build.go`.

## What was done

1. **Added imports** to `internal/build/build.go`:
   - `github.com/iannil/huan/internal/output`
   - `tmpl "github.com/iannil/huan/internal/template"`

2. **Implemented `IncrementalRender`** at the end of `build.go` per the task brief. Stages:
   - **Stage 1 — config:** reuse `cache.SiteCfg` when non-nil (applying `BaseURLOverride` / `MinifyOverride`), else fall back to `loadConfig()`.
   - **Stage 2 — content:** always `loadContent()` (correctness: list/section pages must see updated content).
   - **Stage 3 — markdown + tree:** always `renderMarkdownAndTree()` (computes Permalinks, taxonomies, summaries).
   - **Stage 4 — rendering infra:** when `cache.Templates` + `cache.Writer` are non-nil, reuse cached templates / i18nBundle / scRegistry / mdRenderer / writer, rebuild only the `*Renderer` wrapper (it depends on `cfg.BaseURL`), and re-wire the package-global i18n bundle via `tmpl.SetI18nBundle`. Otherwise fall back to `setupTemplatesAndWriter()`.
   - **Stage 5 — contexts:** always `buildContexts()` (contexts hold page pointers that changed during content reload).
   - **Stage 6 — render:** iterate `site.Pages`, skip any whose `URL` is not in `affectedURLs`, resolve template name, attach `tmpl.DataAccessor{Pages: ctx.RegularPages}` for section/home kinds, render, inject LiveReload in serve mode, and write to `OutputDir`.

3. **Deviation from brief (additive, justified):** also injected LiveReload for re-rendered pages in serve mode (`opts.InjectLiveReload && opts.LiveReloadURL != ""`). Without this, daemon serve mode would lose live-reload on incrementally rendered pages. Matches the full-build behavior in `pipeline_render.go:54-56`.

4. **Added unit tests** in `internal/build/incremental_test.go`:
   - `TestIncrementalRender_EmptyAffectedURLsIsNoop` — empty/nil `affectedURLs` short-circuits without creating the output dir.
   - `TestIncrementalRender_NilCacheBuildsInfrastructure` — `cache=nil` falls back to `setupTemplatesAndWriter`, renders only the affected page, and does not write non-affected pages.
   - `TestIncrementalRender_ReusesCacheFromFullBuild` — full `BuildSite` populates a `PipelineCache`, then `IncrementalRender` uses it to re-render only the mutated page; verifies the output reflects updated content and old content is gone.

## Verification

```
$ go build ./...                                  # BUILD SUCCESS (no output)
$ go test ./internal/build/... -run IncrementalRender -v
=== RUN  TestIncrementalRender_EmptyAffectedURLsIsNoop           --- PASS
=== RUN  TestIncrementalRender_NilCacheBuildsInfrastructure      --- PASS
=== RUN  TestIncrementalRender_ReusesCacheFromFullBuild          --- PASS
PASS

$ go test ./...                                   # ALL PASS (26 packages)
```

## Commits

- `feat(build): add IncrementalRender for DAG-driven partial re-render`

## Files changed

- `internal/build/build.go` — imports + `IncrementalRender` function (~130 lines added).
- `internal/build/incremental_test.go` — new file, 3 tests + shared `writeSite` helper.

## Concerns / Notes

- **`tmpl.SetI18nBundle` is package-global state.** The brief calls this out and the implementation wires it before rendering. If two sites build concurrently in the same process with different i18n bundles, they would clobber each other. This is pre-existing (full build has the same property via `setupTemplatesAndWriter`); flagged but not fixed in scope of this task.
- **`affectedURLs` must use canonical `pg.URL` form** (trailing slash, root-relative, e.g. `/posts/alpha/`). Callers (daemon DAG) must produce URLs in that form; otherwise the page won't match and will be silently skipped. Documented in the function godoc by example.
- **No RSS/markdown-mirror/section-list re-emission in this path.** IncrementalRender renders only the HTML body for affected URLs. It does not re-emit section RSS feeds, markdown mirrors, sitemap, or taxonomy pages — those are intentionally left for the next full build or a future task. The brief scope is HTML-only re-render; out-of-scope outputs will be stale until the next full build.
- **Minify reuse:** when reusing `cache.Writer`, minify/canonify settings are carried over from the prior full build. If `MinifyOverride` differs between builds, the cached writer's minifier is not rebuilt. The brief explicitly reuses `cache.Writer` as-is, so this matches spec, but note the limitation.
