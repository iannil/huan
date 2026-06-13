package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

// MaxRetries is the number of retry attempts per request. The total number of
// attempts is MaxRetries+1 (initial + retries). Per ADR 0002 §9.
const MaxRetries = 3

// backoffSchedule is the wait duration before each retry. attempt=0 → wait
// before 1st retry; attempt=2 → wait before 3rd retry.
// Per ADR 0002 §9: 200ms / 1s / 5s.
//
// This is a var (not const) so tests can shorten it to avoid slow test runs.
// Restore with t.Cleanup or defer when overriding.
var backoffSchedule = []time.Duration{
	200 * time.Millisecond,
	1 * time.Second,
	5 * time.Second,
}

// Backoff returns the wait duration before the (attempt+1)-th retry. attempt
// is 0-indexed: Backoff(0) is the wait before the 1st retry.
//
// Returns zero when attempt exceeds the schedule length, indicating the
// caller should not retry further. Adds up to 20% jitter to avoid
// thundering-herd on simultaneous retries against the same CF edge.
//
// Tests that need deterministic timing should call backoffSchedule[attempt]
// directly.
func Backoff(attempt int) time.Duration {
	if attempt < 0 || attempt >= len(backoffSchedule) {
		return 0
	}
	base := backoffSchedule[attempt]
	jitter := time.Duration(rand.Int63n(int64(base) / 5))
	return base + jitter
}

// RetryDecision classifies an HTTP response/error as retryable or fatal.
type RetryDecision struct {
	Retryable bool
	Reason    string
}

// ClassifyError inspects the HTTP response (or underlying error) and decides
// whether the caller should retry.
//
// Retryable conditions (per ADR 0002 §9):
//   - HTTP 5xx (server errors, includes 500 gateway errors)
//   - HTTP 429 (rate limit)
//   - Network errors (DNS, connection refused, timeout) — but NOT context
//     cancellation, which propagates immediately
//
// Fatal conditions:
//   - HTTP 4xx (other than 429) — auth failures, malformed requests, etc.
//   - HTTP 2xx — success, no retry needed
//   - nil response AND nil err — programmer error
//   - Context cancellation / deadline exceeded
func ClassifyError(resp *http.Response, err error) RetryDecision {
	// Context cancellation propagates immediately — no point retrying.
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return RetryDecision{Retryable: false, Reason: "context canceled"}
	}
	// Other network / connection error → retry.
	if err != nil {
		return RetryDecision{Retryable: true, Reason: fmt.Sprintf("network error: %v", err)}
	}
	if resp == nil {
		return RetryDecision{Retryable: false, Reason: "nil response with nil error (programmer error)"}
	}

	status := resp.StatusCode
	switch {
	case status >= 200 && status < 300:
		return RetryDecision{Retryable: false, Reason: "success"}
	case status == http.StatusTooManyRequests: // 429
		return RetryDecision{Retryable: true, Reason: "rate limit (429)"}
	case status >= 500:
		return RetryDecision{Retryable: true, Reason: fmt.Sprintf("server error (%d)", status)}
	case status == http.StatusUnauthorized: // 401
		// 401 from upload-token endpoint means bad apiToken (fatal).
		// 401 from assets/* endpoints means JWT expired (handled by
		// Client.UploadToken refresh logic, not generic retry).
		return RetryDecision{Retryable: false, Reason: "unauthorized (401)"}
	case status >= 400:
		return RetryDecision{Retryable: false, Reason: fmt.Sprintf("client error (%d)", status)}
	default:
		return RetryDecision{Retryable: false, Reason: fmt.Sprintf("unexpected status %d", status)}
	}
}
