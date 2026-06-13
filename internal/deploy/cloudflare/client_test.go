package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iannil/huan/internal/deploy"
)

func newTestLogger() *deploy.Logger {
	var buf bytes.Buffer
	return deploy.NewLoggerWithWriter("test-trace", &buf)
}

// fastBackoffMu serializes fastBackoff swaps. Without this, two tests
// running with t.Parallel() could race on the package-global
// backoffSchedule var, leading to flaky test timing.
var fastBackoffMu sync.Mutex

// fastBackoff swaps backoffSchedule to near-zero for the duration of a test.
// Real CF retries use 200ms/1s/5s which would make the test suite take ~30s.
//
// Per audit M5: callers of fastBackoff MUST NOT use t.Parallel() — the
// package-global backoffSchedule var cannot be safely swapped in parallel.
// The mutex here serializes critical section but tests still must avoid
// running retry-timing assertions in parallel.
func fastBackoff(t *testing.T) {
	t.Helper()
	fastBackoffMu.Lock()
	original := backoffSchedule
	backoffSchedule = []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
	}
	fastBackoffMu.Unlock()
	t.Cleanup(func() {
		fastBackoffMu.Lock()
		backoffSchedule = original
		fastBackoffMu.Unlock()
	})
}

func TestClient_UploadToken_Success(t *testing.T) {
	var fetchCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
		if r.URL.Path != "/accounts/acc-1/pages/projects/myproj/upload-token" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"jwt":"my-jwt-123","expires_on":"2099-01-01T00:00:00Z"},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc-1", "test-api-token", newTestLogger()).WithBaseURL(srv.URL)
	tok1, err := c.UploadToken(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("UploadToken: %v", err)
	}
	if tok1 != "my-jwt-123" {
		t.Errorf("tok1 = %q", tok1)
	}

	// Second call should hit cache (no new fetch).
	tok2, err := c.UploadToken(context.Background(), "myproj")
	if err != nil {
		t.Fatalf("UploadToken 2: %v", err)
	}
	if tok2 != "my-jwt-123" {
		t.Errorf("tok2 = %q", tok2)
	}
	if got := atomic.LoadInt32(&fetchCount); got != 1 {
		t.Errorf("fetchCount = %d, want 1 (cached)", got)
	}
}

func TestClient_UploadToken_InvalidateForcesRefetch(t *testing.T) {
	var fetchCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"jwt":"jwt-v1","expires_on":"2099-01-01T00:00:00Z"},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc-1", "tok", newTestLogger()).WithBaseURL(srv.URL)
	_, _ = c.UploadToken(context.Background(), "proj")
	if got := atomic.LoadInt32(&fetchCount); got != 1 {
		t.Fatalf("first call fetchCount = %d, want 1", got)
	}

	c.InvalidateJWT("proj")
	_, _ = c.UploadToken(context.Background(), "proj")
	if got := atomic.LoadInt32(&fetchCount); got != 2 {
		t.Errorf("after invalidate fetchCount = %d, want 2", got)
	}
}

func TestClient_UploadToken_ErrorPropagated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"result":null,"success":false,"errors":[{"code":10000,"message":"API token invalid"}]}`))
	}))
	defer srv.Close()

	c := NewClient("acc-1", "bad-token", newTestLogger()).WithBaseURL(srv.URL)
	_, err := c.UploadToken(context.Background(), "proj")
	if err == nil {
		t.Fatal("UploadToken: want error, got nil")
	}
	var cfe *cfError
	if !errors.As(err, &cfe) {
		t.Errorf("err = %T, want *cfError", err)
	}
}

func TestClient_GetJSON_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %q", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"foo":"bar","num":42},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	var out struct {
		Foo string `json:"foo"`
		Num int    `json:"num"`
	}
	if err := c.GetJSON(context.Background(), "/anything", "Bearer tok", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out.Foo != "bar" || out.Num != 42 {
		t.Errorf("got %+v", out)
	}
}

func TestClient_PostJSON_SendsBodyAndAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer mytoken" {
			t.Errorf("Authorization = %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		_ = json.Unmarshal(body, &got)
		if got["hello"] != "world" {
			t.Errorf("body hello = %v", got["hello"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"ok":true},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	var out struct {
		OK bool `json:"ok"`
	}
	err := c.PostJSON(context.Background(), "/x", "Bearer mytoken", map[string]any{"hello": "world"}, &out)
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if !out.OK {
		t.Errorf("OK = false, want true")
	}
}

func TestClient_GetJSON_5xxRetries(t *testing.T) {
	fastBackoff(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	err := c.GetJSON(context.Background(), "/x", "Bearer tok", nil)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("hits = %d, want 3 (initial + 2 retries)", got)
	}
}

func TestClient_GetJSON_4xxNoRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"result":null,"success":false,"errors":[{"code":10000,"message":"forbidden"}]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	err := c.GetJSON(context.Background(), "/x", "Bearer tok", nil)
	if err == nil {
		t.Fatal("want error for 403")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("hits = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestClient_GetJSON_429Retries(t *testing.T) {
	fastBackoff(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	err := c.GetJSON(context.Background(), "/x", "Bearer tok", nil)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("hits = %d, want 2", got)
	}
}

func TestClient_GetJSON_ExhaustsRetries(t *testing.T) {
	fastBackoff(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	err := c.GetJSON(context.Background(), "/x", "Bearer tok", nil)
	if err == nil {
		t.Fatal("want error after retries exhausted")
	}
	// MaxRetries=3 means 4 total attempts (initial + 3 retries).
	if got := atomic.LoadInt32(&hits); got != int32(MaxRetries+1) {
		t.Errorf("hits = %d, want %d", got, MaxRetries+1)
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("err = %q, want contains 'exhausted'", err.Error())
	}
}

func TestClient_PostForm_SendsMultipart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		// We don't bother re-parsing multipart in the mock; just verify it
		// arrives and reply with success.
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"id":"dep-1","url":"https://x.pages.dev"},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	manifestJSON := []byte(`{"/index.html":"abc123"}`)
	fields := map[string]formField{
		"manifest": {contentType: "application/json", value: manifestJSON},
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := c.PostForm(context.Background(), "/dep", "Bearer tok", fields, &out); err != nil {
		t.Fatalf("PostForm: %v", err)
	}
	if out.ID != "dep-1" {
		t.Errorf("ID = %q", out.ID)
	}
}

func TestDecodeEnvelope_SuccessFalse_ReturnsCFError(t *testing.T) {
	body := []byte(`{"result":null,"success":false,"errors":[{"code":10000,"message":"bad request"}]}`)
	err := decodeEnvelope(body, 400, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var cfe *cfError
	if !errors.As(err, &cfe) {
		t.Errorf("err = %T, want *cfError", err)
	}
	if cfe.Status != 400 {
		t.Errorf("Status = %d", cfe.Status)
	}
	if !strings.Contains(cfe.Error(), "bad request") {
		t.Errorf("err = %q", cfe.Error())
	}
}

func TestDecodeEnvelope_EmptyBodyOn4xx(t *testing.T) {
	err := decodeEnvelope(nil, 500, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "empty body") {
		t.Errorf("err = %q", err.Error())
	}
}

func TestDecodeEnvelope_InvalidJSON(t *testing.T) {
	err := decodeEnvelope([]byte(`not json`), 200, nil)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestIsJWTExpired(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"jwt expired", &cfError{Status: 401, Errors: []string{"JWT has expired"}}, true},
		{"jwt invalid", &cfError{Status: 401, Errors: []string{"jwt is invalid"}}, true},
		{"non-jwt 401", &cfError{Status: 401, Errors: []string{"API token invalid"}}, false},
		{"unrelated err", errors.New("network"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isJWTExpired(tc.err); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClient_NetworkErrorRetries(t *testing.T) {
	fastBackoff(t)
	// Server that immediately closes connections to simulate network errors.
	// We use a separate listener-style approach: returning malformed response.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		// Hijack the connection to abort mid-response.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("server does not support hijack")
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
		_ = n
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	err := c.GetJSON(context.Background(), "/x", "Bearer tok", nil)
	_ = err // expected
	if got := atomic.LoadInt32(&hits); got != int32(MaxRetries+1) {
		t.Errorf("hits = %d, want %d", got, MaxRetries+1)
	}
}

func TestClient_APITokenAuth(t *testing.T) {
	c := NewClient("acc", "secret-token", newTestLogger())
	if got := c.APITokenAuth(); got != "Bearer secret-token" {
		t.Errorf("APITokenAuth = %q", got)
	}
}

// Verify Client is safe for concurrent use (JWT mutex + http client).
func TestClient_ConcurrentAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"jwt":"shared-jwt","expires_on":"2099-01-01T00:00:00Z"},"success":true,"errors":[]}`))
	}))
	defer srv.Close()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	const N = 10
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := c.UploadToken(context.Background(), "proj")
			if err != nil {
				t.Errorf("UploadToken: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestClient_ContextCancellationPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := NewClient("acc", "tok", newTestLogger()).WithBaseURL(srv.URL)
	err := c.GetJSON(ctx, "/x", "Bearer tok", nil)
	if err == nil {
		t.Fatal("want error")
	}
	// Either context.DeadlineExceeded or "exhausted retries" — both are
	// acceptable; we just need the request to terminate.
	_ = fmt.Sprintf("got expected err: %v", err)
}
