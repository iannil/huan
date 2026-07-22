# Task 4 Report — Builder: RenderPageJIT 重写 + resolveSourceFile

**Date:** 2026-07-22
**Status:** ✅ Complete
**Commit:** `a122c9e` on `master`

## Summary

Replaced the `RenderPageJIT` stub in `internal/daemon/builder.go` with a real
implementation that calls `build.RenderPageWithCache`. Added a new
`resolveSourceFile` helper that combines DAG reverse-lookup with URL-based
derivation. Exported `resolveSourceFromURL` → `ResolveSourceFromURL` so the
daemon package can compose it.

## Steps Completed

### Step 1 — Export `resolveSourceFromURL`

`internal/build/jit.go`:
- Renamed `func resolveSourceFromURL` → `func ResolveSourceFromURL`.
- Updated godoc to explain why it is exported (composed by daemon).

`internal/build/jit_test.go`:
- Updated both call sites (`resolveSourceFromURL(` → `ResolveSourceFromURL(`).

### Step 2 — Verify export test

```
go test ./internal/build/ -run TestResolveSourceFromURL -v
```
Result: PASS (all 8 sub-tests).

### Step 3 — `resolveSourceFile` + `RenderPageJIT` rewrite

`internal/daemon/builder.go`:
- Replaced the stub (which returned "not yet implemented") with the brief's
  exact implementation.
- `RenderPageJIT(ctx, pageURL)` flow:
  1. `resolveSourceFile(b.opts.DAG, pageURL)` → source rel path
  2. `os.Stat(SourceDir/content/<rel>)` verifies the file exists on disk
  3. `build.RenderPageWithCache(opts, cache, pageURL)` with `IncludeDrafts:true`
- New `resolveSourceFile(dg, pageURL)`:
  1. If `dg != nil`, try `dg.SourceFromPagePath(pageURL)` (built pages)
  2. Fall back to `build.ResolveSourceFromURL(pageURL)` (unbuilt drafts)

### Step 4 — Imports

No new imports needed; `fmt`, `os`, `path/filepath`, `build`, `dag` already
present in `builder.go`.

### Step 5 — Build

```
go build ./...
```
Result: SUCCESS (no output).

### Step 6 — Daemon regression tests

```
go test ./internal/daemon/... -run "TestBuilder|TestServing_jit" -v
```
Result: ALL PASS.

`TestServing_jitFallback` still returns 404 for `/nonexistent` because the
URL-derivation resolves to `nonexistent/_index.md` which does not exist on
disk → the new "source not found" error → 404 in `serving.jitFallback`.

### Step 7 — Stale stub assertion

Searched daemon tests for `"not yet implemented"` / `"JIT rendering"`:
- No test asserted the literal stub error string.
- One stale **comment** in `TestServing_jitFallback` (line 717) said
  "Should return 404 since JIT render is not implemented" — updated to
  "Should return 404: source file for /nonexistent cannot be resolved or
  found on disk".

### Step 8 — Commit

```
feat(daemon): implement RenderPageJIT with DAG+URL source resolution
```
Files in commit:
- `internal/build/jit.go` (export)
- `internal/build/jit_test.go` (caller updates)
- `internal/daemon/builder.go` (rewrite + new helper)
- `internal/daemon/daemon_test.go` (stale comment fix)
- `internal/daemon/builder_jit_test.go` (**new** — TDD coverage)

## New Tests (`internal/daemon/builder_jit_test.go`)

- `TestResolveSourceFile_URLFallback` — empty DAG → URL derivation for
  home / section / page / nested URLs.
- `TestResolveSourceFile_NilDAG` — nil DAG does not panic, falls back.
- `TestResolveSourceFile_DAGHit` — DAG miss falls back to URL derivation.
- `TestRenderPageJIT_RendersBuiltPage` — end-to-end: full build populates
  cache, then `RenderPageJIT` reuses it for an existing page.
- `TestRenderPageJIT_RendersDraftPage` — draft excluded from full build
  (`BuildDrafts:false`) still renders via JIT (forces `IncludeDrafts:true`).
- `TestRenderPageJIT_SourceNotFound` — unresolved/missing source returns
  an error containing "source not found".

## Full Test Results

```
go build ./...                         → SUCCESS
go test ./...                          → ALL PASS (all packages)
go test ./internal/daemon/...          → PASS
go test ./internal/build/...           → PASS
```

## Concerns / Notes

1. **`renderPageFn` field is now dead in the JIT path.** The Builder still
   captures `renderPageFn` from `AfterBuild` into `b.renderPageFn`
   (`builder.go:45,71,80,101`), but the new `RenderPageJIT` uses
   `build.RenderPageWithCache` instead. The field/assignment are kept
   because (a) the brief did not ask to remove them, (b) removing them is
   out of scope and may affect future tasks, and (c) the compiler accepts
   them as "used" because they are assigned to a struct field. A future
   cleanup task could remove this dead capture.

2. **`TestServing_jitFallback`** in `daemon_test.go` constructs a Builder
   with no `SourceDir`. Its `/nonexistent` request still returns 404
   (because `os.Stat` on a path under an empty `SourceDir` fails), so the
   test continues to pass with the new implementation. The stale comment
   was the only update needed there.

3. **Unrelated dirty files in working tree** (brief/report `.md` rewrites,
   `docs/superpowers/plans/2026-07-22-jit-rendering-plan.md`) were left
   unstaged — they are not part of this task and were not touched.
