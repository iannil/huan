package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCheckBindSafety_LoopbackVariants_Allow verifies that any loopback
// bind address is accepted without a token. Loopback is the default safe
// case — the OS network stack already prevents remote access.
func TestCheckBindSafety_LoopbackVariants_Allow(t *testing.T) {
	cases := []string{"127.0.0.1", "::1", "localhost", ""}
	for _, bind := range cases {
		t.Run(bind, func(t *testing.T) {
			if err := CheckBindSafety(bind, ""); err != nil {
				t.Errorf("CheckBindSafety(%q, %q) = %v, want nil", bind, "", err)
			}
		})
	}
}

// TestCheckBindSafety_NonLoopbackNoToken_FailFast verifies ADR 0011 L1:
// binding to a non-loopback address without HUAN_ADMIN_TOKEN set must
// return a descriptive error so the server refuses to start rather than
// silently exposing unrestricted file write to the LAN.
func TestCheckBindSafety_NonLoopbackNoToken_FailFast(t *testing.T) {
	cases := []string{"0.0.0.0", "::", "192.168.1.1", "10.0.0.1"}
	for _, bind := range cases {
		t.Run(bind, func(t *testing.T) {
			err := CheckBindSafety(bind, "")
			if err == nil {
				t.Fatalf("CheckBindSafety(%q, %q) = nil, want error", bind, "")
			}
			// Error message must mention the env var so users know how to fix.
			if !strings.Contains(err.Error(), AdminTokenEnvVar) {
				t.Errorf("error %q does not mention %s", err.Error(), AdminTokenEnvVar)
			}
			if !strings.Contains(err.Error(), bind) {
				t.Errorf("error %q does not mention bind addr %q", err.Error(), bind)
			}
		})
	}
}

// TestCheckBindSafety_NonLoopbackWithToken_Allow verifies the production
// escape hatch: operators who explicitly set HUAN_ADMIN_TOKEN can bind to
// any address (e.g., behind a reverse proxy on a private LAN).
func TestCheckBindSafety_NonLoopbackWithToken_Allow(t *testing.T) {
	if err := CheckBindSafety("0.0.0.0", "some-token"); err != nil {
		t.Errorf("CheckBindSafety(0.0.0.0, some-token) = %v, want nil", err)
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"":            true, // empty defaults to loopback-safe
		"localhost":   true,
		"127.0.0.1":   true,
		"::1":         true,
		"0.0.0.0":     false,
		"::":          false,
		"192.168.1.5": false,
		"example.com": false, // not an IP literal
	}
	for addr, want := range cases {
		t.Run(addr, func(t *testing.T) {
			if got := isLoopback(addr); got != want {
				t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
			}
		})
	}
}

// TestGenerateToken_LengthAndUniqueness verifies the auto-generated token
// is 32 hex chars (16 bytes) and statistically unique across many calls.
// 16 bytes of crypto/rand gives 128 bits of entropy — collision-resistant.
func TestGenerateToken_LengthAndUniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error: %v", err)
		}
		if len(tok) != 32 {
			t.Errorf("token length = %d, want 32 (16 bytes hex)", len(tok))
		}
		if seen[tok] {
			t.Errorf("token %q generated twice — entropy failure", tok)
		}
		seen[tok] = true
	}
}

// TestResolveToken_FromEnv verifies that when HUAN_ADMIN_TOKEN is set,
// ResolveToken returns that value with fromEnv=true (no auto-gen).
func TestResolveToken_FromEnv(t *testing.T) {
	t.Setenv(AdminTokenEnvVar, "env-value-123")
	tok, fromEnv := ResolveToken()
	if tok != "env-value-123" {
		t.Errorf("token = %q, want env-value-123", tok)
	}
	if !fromEnv {
		t.Errorf("fromEnv = false, want true")
	}
}

// TestResolveToken_NoEnv_ReturnsEmpty verifies the unset-env case: returns
// empty token + fromEnv=false. The caller is then responsible for either
// auto-generating (loopback) or failing fast (non-loopback). This split
// is what lets CheckBindSafety enforce the L1 rule without auto-gen
// bypassing it.
func TestResolveToken_NoEnv_ReturnsEmpty(t *testing.T) {
	t.Setenv(AdminTokenEnvVar, "")
	tok, fromEnv := ResolveToken()
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	if fromEnv {
		t.Errorf("fromEnv = true, want false")
	}
}

// TestTokenMiddleware_NoToken_Returns401 verifies the default-deny
// behavior: any API request without a token gets 401 + WWW-Authenticate
// header so curl/users know to provide credentials.
func TestTokenMiddleware_NoToken_Returns401(t *testing.T) {
	h := TokenMiddleware(okHandler(), "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/api/status", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Errorf("WWW-Authenticate header missing")
	}
}

// TestTokenMiddleware_WrongToken_Returns401 verifies that a present but
// incorrect token is also rejected. Same error path as missing — no oracle
// leak about whether the token exists.
func TestTokenMiddleware_WrongToken_Returns401(t *testing.T) {
	h := TokenMiddleware(okHandler(), "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// TestTokenMiddleware_BearerToken_Accepts verifies the standard
// Authorization: Bearer <token> path.
func TestTokenMiddleware_BearerToken_Accepts(t *testing.T) {
	h := TokenMiddleware(okHandler(), "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestTokenMiddleware_BearerToken_CaseInsensitiveScheme verifies RFC 7235
// compliance: the "Bearer" scheme is case-insensitive.
func TestTokenMiddleware_BearerToken_CaseInsensitiveScheme(t *testing.T) {
	h := TokenMiddleware(okHandler(), "secret")
	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "BeArEr"} {
		t.Run(scheme, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/admin/api/status", nil)
			req.Header.Set("Authorization", scheme+" secret")
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", scheme, rec.Code)
			}
		})
	}
}

// TestTokenMiddleware_XHuanAdminTokenHeader verifies the custom header
// fallback — useful for clients that can't easily set Authorization
// (e.g., HTML <img> or <iframe> with query-limited control).
func TestTokenMiddleware_XHuanAdminTokenHeader(t *testing.T) {
	h := TokenMiddleware(okHandler(), "secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/status", nil)
	req.Header.Set("X-Huan-Admin-Token", "secret")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestExtractToken covers all supported input shapes.
func TestExtractToken(t *testing.T) {
	cases := []struct {
		name   string
		header string // Authorization
		xToken string // X-Huan-Admin-Token
		want   string
	}{
		{"bearer", "Bearer abc", "", "abc"},
		{"bearer with extra space", "Bearer   abc", "", "abc"},
		{"bearer lowercase scheme", "bearer abc", "", "abc"},
		{"bare authorization", "abc", "", "abc"},
		{"x-huan header", "", "abc", "abc"},
		{"authorization preferred over x-huan", "Bearer from-auth", "from-x", "from-auth"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.xToken != "" {
				req.Header.Set("X-Huan-Admin-Token", tc.xToken)
			}
			if got := extractToken(req); got != tc.want {
				t.Errorf("extractToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

// okHandler returns 200 OK — used as the "next" handler in middleware tests
// to verify the wrapper passes through on success.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
