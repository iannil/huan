package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// AdminTokenEnvVar is the environment variable name for the admin auth token.
// When set, all admin API requests must include it as a Bearer token.
// When unset and bind is loopback, a one-time token is auto-generated and
// printed to stderr. When unset and bind is non-loopback, CheckBindSafety
// returns an error and the server refuses to start.
const AdminTokenEnvVar = "HUAN_ADMIN_TOKEN"

// CheckBindSafety enforces ADR 0011 L1: a non-loopback bind requires
// HUAN_ADMIN_TOKEN to be set, otherwise the admin panel would expose
// unrestricted file write to the LAN. Loopback binds are safe by default
// (CSRF + local privilege already implied by local access).
func CheckBindSafety(bindAddr, token string) error {
	if isLoopback(bindAddr) {
		return nil
	}
	if token == "" {
		return fmt.Errorf(
			"huan: admin panel requires %s env when binding to non-loopback address (%s);\n"+
				"set %s to a random string, e.g. `export %s=$(openssl rand -hex 32)`,\n"+
				"or bind to 127.0.0.1 / localhost",
			AdminTokenEnvVar, bindAddr, AdminTokenEnvVar, AdminTokenEnvVar)
	}
	return nil
}

// isLoopback reports whether bindAddr is a loopback host (127.0.0.1, ::1,
// localhost, or any IP that net.ParseIP says is loopback). Empty defaults
// to loopback-safe because Go's net.Listen treats "" as all-interfaces ONLY
// when paired with port; here we use it from --bind which defaults to
// 127.0.0.1 in the CLI.
func isLoopback(bindAddr string) bool {
	switch strings.TrimSpace(bindAddr) {
	case "", "localhost":
		return true
	}
	if ip := net.ParseIP(bindAddr); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// GenerateToken returns a 32-char hex token from 16 bytes of crypto/rand.
// Suitable as a one-time admin token printed to stderr.
func GenerateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ResolveToken reads HUAN_ADMIN_TOKEN from the environment. Returns
// (token, fromEnv). When fromEnv=false, the env was unset; the caller is
// responsible for deciding what to do based on bind safety:
//
//   - Loopback bind: auto-generate via GenerateToken() and print once.
//   - Non-loopback bind: CheckBindSafety returns an error before this
//     point, so this branch is unreachable.
//
// Splitting env-read from generation lets CheckBindSafety enforce the
// "non-loopback requires explicit env token" rule — auto-generated tokens
// must NOT satisfy non-loopback binds (otherwise fail-fast is bypassed).
func ResolveToken() (token string, fromEnv bool) {
	if env := os.Getenv(AdminTokenEnvVar); env != "" {
		return env, true
	}
	return "", false
}

// TokenMiddleware enforces ADR 0011 L2: every admin API request must carry
// the configured token. The token may be in the Authorization header
// (Bearer scheme) or X-Huan-Admin-Token header. Comparison uses
// crypto/subtle.ConstantTimeCompare to prevent timing attacks.
//
// The static SPA under /admin/ (without /api/) is intentionally NOT gated —
// the SPA needs to load to prompt for the token. The SPA serves no
// sensitive data itself; all sensitive data flows through /admin/api/*.
func TokenMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := extractToken(r)
		if provided == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="huan-admin"`)
			writeJSON(w, http.StatusUnauthorized, APIError{Error: "missing admin token"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="huan-admin"`)
			writeJSON(w, http.StatusUnauthorized, APIError{Error: "invalid admin token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractToken pulls the admin token from either supported header.
// Returns "" if neither is present.
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		// Trim leading/trailing whitespace; accept "Bearer <token>"
		// case-insensitively per RFC 7235.
		s := strings.TrimSpace(auth)
		if len(s) >= 7 && strings.EqualFold(s[:7], "Bearer ") {
			return strings.TrimSpace(s[7:])
		}
		// Some clients send the bare token in Authorization.
		return s
	}
	if t := r.Header.Get("X-Huan-Admin-Token"); t != "" {
		return strings.TrimSpace(t)
	}
	return ""
}
