# Task 3 Report — HandleSubscribe SSE HTTP handler

**Status:** Completed
**Date:** 2026-07-22
**Commit:** `79092fd feat(sse): add HandleSubscribe handler with stream + heartbeat start`

## Files

- `internal/daemon/sse/handler.go` (new, 100 lines) — `Start`, `ServeHTTP`, `HandleSubscribe`
- `internal/daemon/sse/handler_test.go` (new, 151 lines) — 4 tests + `waitForClientCount` helper

## What was built

### `Start(ctx context.Context)`
- Launches `startHeartbeat(ctx)` in a goroutine. Safe to call more than once
  (each call spawns an additional heartbeat loop); intended to be called once
  at daemon startup. Cancelling `ctx` stops the goroutine.

### `ServeHTTP(w, r)`
- Thin adapter so `*SSEHub` satisfies `http.Handler`. Lets callers pass the
  hub directly to `http.NewServeMux` or `httptest.NewServer` without an
  adapter function. Required for the `TestHandleSubscribe_ReadsStream`
  end-to-end test which uses `httptest.NewServer(h)`.

### `HandleSubscribe(w http.ResponseWriter, r *http.Request)`
Order of operations:
1. Method check — `GET` only, else `405 Method Not Allowed`.
2. `http.Flusher` check — else `500` (ResponseRecorder and real servers satisfy it).
3. `registerClient()` — returns `nil` when `maxClients` is reached → `503 Service Unavailable`.
   This relies on the Task 1 contract (registerClient enforces the cap internally).
4. SSE headers set **before** `WriteHeader`: `Content-Type: text/event-stream`,
   `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`
   (disables nginx buffering).
5. `WriteHeader(200)` + initial `flush`.
6. Loop `select` on `r.Context().Done()` and the client channel:
   - `ctx.Done()` → client disconnected → return.
   - channel closed → return (shutdown).
   - event received → `encodeEvent` → `fmt.Fprint(w, line)` → `flusher.Flush()`.
     Encode error is logged and skipped; write error is treated as client gone → return.
7. `defer h.unregisterClient(ch)` cleans up the map entry on every exit path.

### Deviation from the brief
The brief's step-3 code block shows a `ClientCount() >= maxClients` pre-check at the top of `HandleSubscribe`. The brief's own prose (and the Task 1 review note) says `registerClient` already enforces this and returns `nil` when full → 503. I followed the prose, not the code block, to avoid a TOCTOU race between the pre-check and the actual registration (two concurrent handlers could both pass the pre-check, then both register, overshooting the cap). The `nil`-return path is the single source of truth and is atomic under `h.mu`.

## Tests

| Test | What it asserts |
|---|---|
| `TestHandleSubscribe_SSEHeaders` | `Content-Type: text/event-stream`, `Cache-Control: no-cache` after handler exit. |
| `TestHandleSubscribe_EventFormat` | Body contains `event: build_completed` and `"pages":3` after broadcast. |
| `TestHandleSubscribe_MaxClientsRejected` | When `maxClients` already filled, returns `503`. |
| `TestHandleSubscribe_ReadsStream` | End-to-end via `httptest.NewServer` + `http.Get` + `bufio.Scanner`: client sees `content_changed` event streamed through a real flusher. |

### Test summary
```
go test ./internal/daemon/sse/ -v
--- PASS: TestHandleSubscribe_SSEHeaders (0.00s)
--- PASS: TestHandleSubscribe_EventFormat (0.00s)
--- PASS: TestHandleSubscribe_MaxClientsRejected (0.00s)
--- PASS: TestHandleSubscribe_ReadsStream (0.05s)
PASS  ok  github.com/iannil/huan/internal/daemon/sse  15.712s
```

All 15 tests in the package pass (4 new handler tests + 11 existing hub tests).

## Verification done beyond the brief

1. **Race detector.** First run found a data race: `ResponseRecorder.Body.String()` in the test reading while `fmt.Fprint` writes from the handler goroutine. Fixed by (a) replacing `time.Sleep` with a polling `waitForClientCount` helper to deterministically wait for registration, and (b) reading `rec.Header()`/`rec.Body` only **after** the handler returns (closing all client channels drives the loop to exit, then `<-done`). All tests now pass with `-race`.

2. **Flake check.** `-count=3 -race` runs cleanly, no flakiness.

3. **Whole-repo regression.** `go test ./...` — all packages pass.

4. **Static checks.** `go vet ./internal/daemon/sse/` clean, `gofmt -l` clean.

## Concerns / follow-ups

- **`ServeHTTP` not in the brief.** I added a `ServeHTTP` adapter so `*SSEHub` is an `http.Handler`. This is needed by the brief's own `TestHandleSubscribe_ReadsStream` test (`httptest.NewServer(h)`), and is also the natural shape for wiring the daemon's mux in Task 4. Flagging it because it's not listed in the brief's "Produces" line, but it's a strict superset of the listed API and doesn't change any signature.
- **No `Last-Event-ID` / resume support.** Out of scope per the design doc; clients just reconnect and lose in-flight events during the disconnect window. The 16-event per-client buffer softens this for brief blips.
- **Heartbeat lifetime.** `Start(ctx)` is intended to be called once. If a future caller calls it per-request by mistake, every request would spawn an extra heartbeat loop. Worth a comment or guard at the daemon wiring site (Task 4).
