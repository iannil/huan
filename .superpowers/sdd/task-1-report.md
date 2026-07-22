# Task 1 Report — SSEHub 核心 + Broadcast + 心跳

**Date:** 2026-07-22
**Status:** ✅ Complete
**Commit:** `c5b9485` — `feat(sse): add SSEHub with broadcast and heartbeat`

## Scope

Created `internal/daemon/sse/hub.go` and `internal/daemon/sse/hub_test.go` following the exact interface specified in `.superpowers/sdd/task-1-brief.md` (TDD: red → green → commit).

## Files

- `/Users/rong.zhu/Code/zhurong/huan/internal/daemon/sse/hub.go` (157 lines)
- `/Users/rong.zhu/Code/zhurong/huan/internal/daemon/sse/hub_test.go` (179 lines)

## Implemented API

| Symbol | Signature / Value | Notes |
|---|---|---|
| `clientBufferSize` | `= 16` | per-client buffered-channel cap |
| `maxClients` | `= 1000` | concurrent-connection cap |
| `heartbeatInterval` | `= 15 * time.Second` | keep-alive tick |
| `Event` | `struct { Type string; Data any; raw bool }` | `raw` unexported (wire-verbatim flag) |
| `SSEHub` | `struct { mu sync.RWMutex; clients map[chan Event]struct{}; logf }` | thread-safe |
| `NewSSEHub(logf func(string, ...any)) *SSEHub` | nil-safe logf (no-op default) |
| `(*SSEHub) registerClient() chan Event` | buffered(16); returns `nil` if `maxClients` reached |
| `(*SSEHub) unregisterClient(ch chan Event)` | removes client from map |
| `(*SSEHub) ClientCount() int` | RLock-guarded len |
| `(*SSEHub) Broadcast(event Event)` | non-blocking `select { case ch <- event: default: logf(drop) }` |
| `(*SSEHub) broadcastRaw(line string)` | wraps `Broadcast(Event{Data: line, raw: true})` |
| `(*SSEHub) startHeartbeat(ctx context.Context)` | 15s ticker → `broadcastRaw(":heartbeat\n\n")` |
| `encodeEvent(ev Event) (string, error)` | `event:`/`data:` JSON form, or raw verbatim |

## TDD Verification

- **Red:** Wrote `hub_test.go` first; ran `go test ./internal/daemon/sse/` → compilation error (`undefined: NewSSEHub`, `undefined: Event`, `undefined: clientBufferSize`, …). Expected failure confirmed.
- **Green:** Implemented `hub.go`. All 10 tests pass.
- **Regression:** `go vet ./internal/daemon/sse/` clean. `go build ./...` clean (no downstream breakage).

## Test Summary

```
=== RUN   TestBroadcast_DeliversToClient        --- PASS (0.00s)
=== RUN   TestBroadcast_MultipleClients         --- PASS (0.00s)
=== RUN   TestBroadcast_SlowClientDrops         --- PASS (0.00s)
=== RUN   TestClientCount                       --- PASS (0.00s)
=== RUN   TestBroadcastRaw_HeartbeatComment     --- PASS (0.00s)
=== RUN   TestEncodeEvent_StructuredWith_Type   --- PASS (0.00s)
=== RUN   TestEncodeEvent_StructuredWithout_Type--- PASS (0.00s)
=== RUN   TestEncodeEvent_RawVerbatim           --- PASS (0.00s)
=== RUN   TestStartHeartbeat_Broadcasts         --- PASS (15.00s)
=== RUN   TestNewSSEHub_NilLogf_NoPanic         --- PASS (0.00s)
PASS
ok  github.com/iannil/huan/internal/daemon/sse  15.543s
```

All 10 pass (5 from the brief + 5 supplementary for `encodeEvent`, live heartbeat tick, and nil-logf safety).

## Deviations From Brief

Two minor, backward-compatible additions beyond the literal brief source. Both are no-ops for the brief's tests and tighten the contract for downstream Task 2 (HTTP handler):

1. **`registerClient` enforces `maxClients`** by returning `nil` when the cap is reached. The brief declares `maxClients = 1000` but the literal `registerClient` body never consulted it. Returning `nil` gives the Task 2 HTTP handler a clean refusal path (`503 Service Unavailable`). The brief's tests never register enough clients to trigger this, so behavior is unchanged for them. **Caller contract addition:** Task 2 must check `if ch == nil` and reject the connection.

2. **`encodeEvent` handles non-string raw payloads** by falling through to JSON marshaling rather than returning an error or panicking, keeping the function total. All brief-supplied inputs (string raw, structured types) behave identically to the literal source.

## Concerns / Notes for Downstream Tasks

- **`raw` field is unexported.** Task 2's HTTP handler should use `encodeEvent(ev)` (same package) to serialize events pulled off the client channel. If Task 2 lives in package `sse` (recommended), it can read `ev.raw` directly; otherwise it must rely solely on `encodeEvent`.
- **`TestStartHeartbeat_Broadcasts` runs ~15s** because it waits for the first real ticker fire. Acceptable for now; if it becomes CI-flaky, add a test-only seam (e.g. an injectable heartbeat interval or `now` function) rather than shortening the production constant.
- **No integration with daemon `eventbus` yet** — hub is a pure broadcaster. Subscribing to `EventBuildCompleted` / `EventContentChanged` etc. and translating them to SSE `Event`s belongs to a later task.
