# Task 4 Report — daemon 集成（serving + builder + daemon.go）

**Date:** 2026-07-22
**Commit:** `556f6a5` — feat(daemon): wire ContentIndex and /api/v1/ query API into daemon
**Status:** COMPLETE — build green, all daemon tests pass

## Summary

Wired the ContentIndex (Task 1) and Handler (Task 2-3) into the daemon so
`/api/v1/*` queries are served by `huan serve` whenever `ai.contentAPI: true`
is set in `huan.yaml`, and the index stays fresh after every full and
incremental build.

## Changes

### `internal/daemon/serving.go`
- Added `ContentAPI http.Handler` field to `ServingOptions` (optional).
- In `Start()`, registered `mux.Handle("/api/v1/", s.opts.ContentAPI)` **before**
  the `/` catch-all so the exact-prefix match wins (Go 1.22 ServeMux semantics
  also ensure `/api/v1/` matches all sub-paths while `/` remains the fallback).

### `internal/daemon/builder.go`
- Added import `github.com/iannil/huan/internal/daemon/contentindex`.
- Added `ContentIndex *contentindex.ContentIndex` field to `BuilderOptions`.
- Added a `LoadFromDir(b.opts.OutputDir)` reload hook:
  - in `executeFullBuild` right after `JITCache.Clear()` (full rebuild path),
  - in `IncrementalBuild` right after the `JITCache.Remove` loop (incremental
    path).
- Errors from `LoadFromDir` are logged via `b.opts.Logf` and swallowed — a
  failed index reload must never fail the build. `LoadFromDir` already treats
  a missing `api/` dir as a no-op (empty index), so the hook is safe during
  the initial build when `tmpDir` is empty.

### `internal/daemon/daemon.go`
- Added import `github.com/iannil/huan/internal/daemon/contentindex`.
- In `Run()` step 6.5 (between DAG init and Builder init), when
  `cfg.AI.ContentAPI` is true:
  - create `contentIdx := contentindex.NewContentIndex(cfg.BaseURL)`,
  - best-effort `contentIdx.LoadFromDir(tmpDir)` (logs on error, empty index
    is fine pre-build),
  - wrap with `contentindex.NewHandler(contentIdx)` → `contentAPI`.
- Injected `ContentIndex: contentIdx` into `BuilderOptions`.
- Injected `ContentAPI: contentAPI` into `ServingOptions`.
- When `ai.contentAPI` is false, both stay `nil` and the relevant log line
  says `daemon: content query API disabled (ai.contentAPI not set)`.

## Tests added (`internal/daemon/daemon_test.go`)

TDD-driven tests covering the wiring contract:

1. **`TestServing_ContentAPI_RoutesV1`** — boots `Serving.Start()` on an
   OS-assigned port, populates a `ContentIndex` from a fixture, and asserts:
   - `GET /api/v1/pages` returns `200` JSON with `"total":1`,
   - `GET /api/v1/tags` returns `200` JSON with `"go":1`,
   - a non-`/api/v1/` path falls through to the static file / JIT fallback
     (not the content handler).

2. **`TestBuilder_ContentIndex_ReloadAfterFullBuild`** — full build with
   `ContentIndex` wired in. Asserts:
   - `api/posts.json` is written by the build,
   - `idx.Len()` transitions from 0 → 1 after the build,
   - `GetByURL("/posts/hello/")` returns the expected item.

3. **`TestBuilder_ContentIndex_ReloadAfterIncrementalBuild`** — full build
   then an incremental edit of a page title. Asserts the index reflects the
   new title after `IncrementalBuild` returns (i.e. the incremental reload
   hook fires).

## Verification

```
$ go build ./...
(no output — success)

$ go test ./internal/daemon/...
ok  github.com/iannil/huan/internal/daemon            0.761s
ok  github.com/iannil/huan/internal/daemon/cache      (cached)
ok  github.com/iannil/huan/internal/daemon/contentindex 0.787s
ok  github.com/iannil/huan/internal/daemon/dag        (cached)
ok  github.com/iannil/huan/internal/daemon/eventbus   (cached)

$ go test ./...
(all packages pass, no regressions)
```

## Commits

- `556f6a5` — feat(daemon): wire ContentIndex and /api/v1/ query API into daemon
  - Files: `internal/daemon/serving.go`, `internal/daemon/builder.go`,
    `internal/daemon/daemon.go`, `internal/daemon/daemon_test.go`
  - Stat: 4 files changed, 341 insertions(+), 12 deletions(-)

## Concerns / Notes

- The pre-existing unstaged changes to `.superpowers/sdd/task-{1,2,3}-{brief,report}.md`
  and `task-4-brief.md` were already present in the working tree before this
  task started (scaffolding from earlier task setup); they are not part of
  this commit.
- The initial `LoadFromDir` in `daemon.Run` runs against an empty `tmpDir`
  (initial build hasn't happened yet). This is intentional and safe —
  `LoadFromDir` returns `nil` for a missing `api/` directory. The first
  meaningful reload happens inside the post-build hook.
- The reload hook swallows errors (logs only). This matches the contract
  that a content-index problem must not fail a successful build; the
  previous (possibly stale) index simply remains until the next build.
- `cfg.AI.ContentAPI` is the gate; `cfg.BaseURL` is used to convert absolute
  URLs in the source JSON to relative paths inside the index. Both exist
  in `internal/config/config.go` (lines 122 and 10 respectively).
