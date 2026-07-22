# Task 1 Report — ContentIndex: Load & Types

## Status

Complete. TDD cycle (red → green → commit) executed end to end.

## Commits

- `b22fa87` — `feat(contentindex): add ContentIndex with LoadFromDir`

## What Was Done

Created `internal/daemon/contentindex/` with the query API's in-memory index:

**`internal/daemon/contentindex/index.go`**
- `Item` struct — the query API return type. No `Plain` field (full-text body intentionally dropped; API returns metadata + summary, full content is served via the pre-built page or JIT rendering).
- `rawItem` struct — matches `output.ContentItem` JSON shape minus `Plain`. Used only for decoding; `URL` is absolute, `Section` is filled from the filename.
- `ContentIndex` struct — thread-safe (sync.RWMutex) in-memory slice of `Item`.
- `NewContentIndex(baseURL string) *ContentIndex` — empty index; `baseURL` used to convert absolute → relative URLs.
- `LoadFromDir(outputDir string) error` — loads `<outputDir>/api/*.json`. Missing `api/` directory is not an error (returns empty index). Malformed files (read error, JSON parse error) are logged to stderr and skipped, not fatal. Section name is derived from the filename (e.g. `posts.json` → `posts`).
- `Len() int` — count under RLock.
- `GetByURL(url string) (Item, bool)` — linear scan under RLock; matches on the relative URL.
- `toRelative(absURL string) string` — strips `baseURL` prefix; ensures the result starts with `/`.

**`internal/daemon/contentindex/index_test.go`**

Five tests following the brief verbatim:

| Test | Scenario | Outcome |
|------|----------|---------|
| `TestLoadFromDir_LoadsAllSections` | Two section files (posts + books) → `Len() == 2`. | PASS |
| `TestLoadFromDir_RelativeURL` | Absolute URL `https://example.com/posts/p1/` → relative `/posts/p1/`; `Section == "posts"`. | PASS |
| `TestLoadFromDir_MalformedJSON` | One good + one `broken.json` (`{not json`) → no error, `Len() == 1`. | PASS |
| `TestLoadFromDir_EmptyDir` | Tempdir with no `api/` → no error, `Len() == 0`. | PASS |
| `TestLoadFromDir_NoAPIDir` | Output dir missing `api/` subdir → no error. | PASS |

Helper `writeAPITestFile` writes a JSON array to `<dir>/api/<name>`; `mustJSON` wraps `encoding/json.Marshal`.

## TDD Verification

- **Red**: initial `go test -run TestLoadFromDir -v` failed to compile — `undefined: NewContentIndex` (no `index.go`), as expected.
- **Green**: after implementing `index.go`, all 5 new tests PASS in 0.41s.

## Regression Check

- `go build ./...` → clean (no output).
- `go vet ./internal/daemon/contentindex/` → clean (no output).

## Concerns

None.

- `rawItem` correctly omits `Plain`, matching the brief's instruction that the field is dropped. Verified against `internal/output/contentapi.go` (`output.ContentItem` does emit `plain`, which is simply ignored on decode here).
- Malformed-file handling matches the brief: skip + stderr log, non-fatal. Verified by `TestLoadFromDir_MalformedJSON` (the stderr warning line appears in test output, as expected for `fmt.Fprintf(os.Stderr, ...)`).
- Thread-safety follows the same RLock pattern used by sibling modules (`internal/daemon/dag`).
