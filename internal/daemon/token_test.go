package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path (and parent dirs) with the given content. Test helper.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolveAdminToken verifies the daemon's admin token resolution flow:
// HUAN_ADMIN_TOKEN wins; loopback binds without env auto-generate a 32-hex
// token; non-loopback binds without env fail fast (ADR 0011 L1). Regression
// for the bug where daemon.go hardcoded Token: "" with a comment claiming
// env fallback that never existed — making every admin API request 401 even
// with the correct env token.
func TestResolveAdminToken(t *testing.T) {
	t.Run("env set wins", func(t *testing.T) {
		t.Setenv("HUAN_ADMIN_TOKEN", "from-env")
		tok, fromEnv, err := resolveAdminToken("127.0.0.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !fromEnv || tok != "from-env" {
			t.Fatalf("token=%q fromEnv=%v, want from-env", tok, fromEnv)
		}
	})
	t.Run("loopback no env generates", func(t *testing.T) {
		t.Setenv("HUAN_ADMIN_TOKEN", "")
		tok, fromEnv, err := resolveAdminToken("127.0.0.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fromEnv || len(tok) != 32 {
			t.Fatalf("token=%q fromEnv=%v, want generated 32-hex", tok, fromEnv)
		}
	})
	t.Run("non-loopback no env errors", func(t *testing.T) {
		t.Setenv("HUAN_ADMIN_TOKEN", "")
		if _, _, err := resolveAdminToken("0.0.0.0"); err == nil {
			t.Fatal("want error for non-loopback without env token")
		}
	})
}
