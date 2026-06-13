package cloudflare

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBackoff_KnownSchedule(t *testing.T) {
	// backoffSchedule entries are the floor (without jitter).
	if got := backoffSchedule[0]; got != 200*time.Millisecond {
		t.Errorf("backoffSchedule[0] = %v, want 200ms", got)
	}
	if got := backoffSchedule[1]; got != 1*time.Second {
		t.Errorf("backoffSchedule[1] = %v, want 1s", got)
	}
	if got := backoffSchedule[2]; got != 5*time.Second {
		t.Errorf("backoffSchedule[2] = %v, want 5s", got)
	}
}

func TestBackoff_IncludesJitterWithin20Percent(t *testing.T) {
	for attempt := 0; attempt < len(backoffSchedule); attempt++ {
		base := backoffSchedule[attempt]
		for i := 0; i < 50; i++ {
			got := Backoff(attempt)
			// Backoff returns base + jitter (jitter up to base/5 = 20%).
			if got < base {
				t.Errorf("Backoff(%d) = %v < base %v", attempt, got, base)
			}
			if got > base+base/5+1 {
				t.Errorf("Backoff(%d) = %v > base+jitter %v", attempt, got, base+base/5)
			}
		}
	}
}

func TestBackoff_OutOfRange(t *testing.T) {
	if got := Backoff(-1); got != 0 {
		t.Errorf("Backoff(-1) = %v, want 0", got)
	}
	if got := Backoff(len(backoffSchedule)); got != 0 {
		t.Errorf("Backoff(out-of-range) = %v, want 0", got)
	}
}

func TestClassifyError_Success(t *testing.T) {
	d := ClassifyError(&http.Response{StatusCode: 200}, nil)
	if d.Retryable {
		t.Errorf("200 retryable=true, want false")
	}
}

func TestClassifyError_5xxRetryable(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504} {
		d := ClassifyError(&http.Response{StatusCode: code}, nil)
		if !d.Retryable {
			t.Errorf("%d retryable=false, want true", code)
		}
	}
}

func TestClassifyError_429Retryable(t *testing.T) {
	d := ClassifyError(&http.Response{StatusCode: 429}, nil)
	if !d.Retryable {
		t.Errorf("429 retryable=false, want true")
	}
}

func TestClassifyError_401Fatal(t *testing.T) {
	d := ClassifyError(&http.Response{StatusCode: 401}, nil)
	if d.Retryable {
		t.Errorf("401 retryable=true, want false")
	}
	if !strings.Contains(d.Reason, "401") {
		t.Errorf("Reason = %q, want contains 401", d.Reason)
	}
}

func TestClassifyError_4xxOtherFatal(t *testing.T) {
	for _, code := range []int{400, 403, 404, 422} {
		d := ClassifyError(&http.Response{StatusCode: code}, nil)
		if d.Retryable {
			t.Errorf("%d retryable=true, want false", code)
		}
	}
}

func TestClassifyError_NetworkErrorRetryable(t *testing.T) {
	d := ClassifyError(nil, errors.New("connection refused"))
	if !d.Retryable {
		t.Errorf("network error retryable=false, want true")
	}
	if !strings.Contains(d.Reason, "network error") {
		t.Errorf("Reason = %q, want contains 'network error'", d.Reason)
	}
}

func TestClassifyError_ContextCanceledNotRetryable(t *testing.T) {
	d := ClassifyError(nil, context.Canceled)
	if d.Retryable {
		t.Errorf("context.Canceled retryable=true, want false")
	}
}

func TestClassifyError_ContextDeadlineNotRetryable(t *testing.T) {
	d := ClassifyError(nil, context.DeadlineExceeded)
	if d.Retryable {
		t.Errorf("context.DeadlineExceeded retryable=true, want false")
	}
}

func TestClassifyError_WrappedContextCanceledNotRetryable(t *testing.T) {
	wrapped := errors.Join(errors.New("wrapped"), context.Canceled)
	d := ClassifyError(nil, wrapped)
	if d.Retryable {
		t.Errorf("wrapped context.Canceled retryable=true, want false")
	}
}

func TestClassifyError_NilResponseAndErr(t *testing.T) {
	d := ClassifyError(nil, nil)
	if d.Retryable {
		t.Errorf("nil/nil retryable=true, want false")
	}
}
