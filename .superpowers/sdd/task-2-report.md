# Task 2 Report — build/jit.go `resolveSourceFromURL`

**Status:** ✅ Complete
**Date:** 2026-07-22
**Commit:** `fb2f73e` — `feat(build): add resolveSourceFromURL for JIT URL→source derivation`

## What was done

Implemented the pure function `resolveSourceFromURL(pageURL string) string` in
`internal/build/jit.go`, which derives the content-relative source file path
from a page URL. This is the fallback path used by JIT rendering when a URL is
not present in the DAG (e.g., a newly-created draft).

Followed strict TDD (red → green → commit):

1. Wrote `internal/build/jit_test.go` (table-driven, 8 cases).
2. Ran `go test` → confirmed RED: `undefined: resolveSourceFromURL`.
3. Implemented `resolveSourceFromURL` in `internal/build/jit.go`.
4. Ran `go test` → confirmed GREEN: all 8 subtests pass.
5. Committed both files in a single commit.

## Files

- **Created:** `/Users/rong.zhu/Code/zhurong/huan/internal/build/jit.go` (35 lines)
- **Created:** `/Users/rong.zhu/Code/zhurong/huan/internal/build/jit_test.go` (28 lines)

## Behavior (URL → source path)

| URL                       | Source path                  |
|---------------------------|------------------------------|
| `/`                       | `_index.md`                  |
| `/posts/`                 | `posts/_index.md`            |
| `/posts/hello/`           | `posts/hello.md`             |
| `/posts/2026/new-year/`   | `posts/2026/new-year.md`     |
| `/books/v1/ch1/`          | `books/v1/ch1.md`            |
| `/posts/_index/`          | `posts/_index.md`            |
| `/posts/hello`            | `posts/hello.md`             |

Algorithm:
- Strip leading/trailing `/`.
- Empty → `_index.md` (home).
- If last segment is `_index` → join all parts + `.md`.
- If only one segment → `<seg>/_index.md` (section index).
- Otherwise → join all parts + `.md` (regular page).

## Test summary

```
=== RUN   TestResolveSourceFromURL
--- PASS: TestResolveSourceFromURL (0.00s)
    --- PASS: home
    --- PASS: section
    --- PASS: simple_page
    --- PASS: nested_page
    --- PASS: deep_page
    --- PASS: explicit__index
    --- PASS: no_trailing_slash
    --- PASS: leading+trailing_slash_stripped
PASS
ok  	github.com/iannil/huan/internal/build	0.597s
```

- **8/8 subtests pass.**
- Full package test suite: `ok` — no regressions.
- `go vet ./internal/build/`: clean.
- `go build ./...`: clean (whole project compiles).

## Implementation notes

- Pure function, no DAG dependency — safe to call from any rendering path.
- No file-system access: callers must still verify the derived file exists on
  disk (documented in the function godoc).
- Implements exactly the spec from the task brief verbatim; no scope creep.

## Concerns / follow-ups

None blocking. One observation for downstream tasks: the function does not
distinguish a "leaf page that happens to be a section index" from a real
section when the URL is the bare section path — it assumes single-segment
paths are sections (returning `<seg>/_index.md`). This matches the Hugo/huan
content layout per the brief, but if a future content layout allows
top-level regular pages at `/foo/`, the heuristic would need revisiting.
