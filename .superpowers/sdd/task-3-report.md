# Task 3 Report — HTTP Handler for /api/v1/* Query Endpoints

**Status:** COMPLETE
**Date:** 2026-07-22
**Commit:** `73ddfe2` — `feat(contentindex): add HTTP handler for /api/v1/* query endpoints`

## What was done

Implemented `internal/daemon/contentindex/handler.go` exposing the public, read-only
content query API per the task-3 brief. The handler consumes `ContentIndex` (Task 1-2)
and routes four endpoints via a single `ServeHTTP`:

- `GET /api/v1/pages` — list with filter (`section`, `tag`, `q`) + pagination (`page`, `limit`) + `sort`. Delegates to `ContentIndex.Query`.
- `GET /api/v1/pages/{rest}` — single page detail. Normalizes the path back to a leading `/` and delegates to `ContentIndex.GetByURL`. Returns 404 on miss.
- `GET /api/v1/tags` — tag aggregation via `ContentIndex.Tags`.
- `GET /api/v1/sections` — section aggregation via `ContentIndex.Sections`.

Public surface produced (verbatim from brief):
```go
type Handler struct{ index *ContentIndex }
func NewHandler(index *ContentIndex) *Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Design notes:
- `Method != GET` → 405 `{"error":"method not allowed"}`.
- Unknown path under `/api/v1/*` → 404 `{"error":"not found"}`.
- Empty index (Len 0) still serves 200 with empty data — no 503. This makes the API
  available immediately at daemon startup, before the first build populates the index.
- All responses are JSON via `json.NewEncoder`; `Content-Type: application/json`.

## Tests added

`internal/daemon/contentindex/handler_test.go` — 8 tests, reusing `loadTestIndex`
from `index_test.go` (same package) and `httptest.NewRequest` + `httptest.NewRecorder`:

| Test | Asserts |
|------|---------|
| `TestHandler_PagesList` | 200, non-empty `Total`, default `Limit == 10` |
| `TestHandler_PagesFilter` | `?section=books` returns only books |
| `TestHandler_PageDetail` | `/api/v1/pages/posts/go/` → 200 with `URL == "/posts/go/"` |
| `TestHandler_PageDetail404` | `/api/v1/pages/nope/` → 404 |
| `TestHandler_Tags` | 200, `m["go"] > 0` |
| `TestHandler_Sections` | `m["posts"] > 0` |
| `TestHandler_NoAuthRequired` | No Authorization header → still 200 (public) |
| `TestHandler_IndexNotReady` | Empty index (`NewContentIndex(baseURL)`) → 200, not 503 |

Helper: `serveHandler(t, h, method, path, body)` builds the request, runs it through
`h.ServeHTTP`, and returns the recorder.

## TDD flow

1. Wrote `handler_test.go` verbatim from the brief.
2. Ran `go test -run TestHandler -v` → **compilation failure** (`undefined: NewHandler`,
   `undefined: Handler`) — expected red.
3. Implemented `handler.go` verbatim from the brief.
4. Re-ran the suite → all green.

## Deviation from brief

**Removed unused `"net/http"` import from `handler_test.go`.** The brief's test file
imported `net/http` but never references any symbol from it directly (only `httptest.*`
and `encoding/json`). Go rejects unused imports, so the test file would not compile as
written. Removed the single import line; no test logic, signatures, or assertions
changed. The implementation file `handler.go` is byte-for-byte the brief's version.

## Test results

```
go test ./internal/daemon/contentindex/ -v
--- PASS: TestHandler_PagesList (0.00s)
--- PASS: TestHandler_PagesFilter (0.00s)
--- PASS: TestHandler_PageDetail (0.00s)
--- PASS: TestHandler_PageDetail404 (0.00s)
--- PASS: TestHandler_Tags (0.00s)
--- PASS: TestHandler_Sections (0.00s)
--- PASS: TestHandler_NoAuthRequired (0.00s)
--- PASS: TestHandler_IndexNotReady (0.00s)
... (16 pre-existing index/query tests also PASS)
PASS
ok  	github.com/iannil/huan/internal/daemon/contentindex  0.497s
```

- `go vet ./internal/daemon/contentindex/` — clean.
- `go build ./...` — entire project builds; no regressions.

## Commits

- `73ddfe2` — feat(contentindex): add HTTP handler for /api/v1/* query endpoints
  - Files: `internal/daemon/contentindex/handler.go`,
    `internal/daemon/contentindex/handler_test.go`

## Concerns / notes

- The handler is deliberately thin and stateless; all query semantics live in
  `ContentIndex`. Daemon-side wiring (registering `Handler` at `/api/v1/` on the
  serve mux and reloading the index after each build) is out of scope for Task 3
  and belongs to a later integration task.
- `handlePageDetail` rebuilds the URL by prepending `/`. This works for the relative
  URLs stored by `ContentIndex` (always leading-slash, e.g. `/posts/go/`). If the
  index ever stores rootless URLs, this normalization will need to revisit.
- `serveHandler` accepts a `body string` parameter (per brief) but does not use it —
  all current tests are GET with no body. Kept for parity with the brief and for
  future POST/PUT extension room.
