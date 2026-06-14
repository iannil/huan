package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_NotPresent(t *testing.T) {
	dir := t.TempDir()
	// dir has no .env file. loadDotEnv should be no-op.
	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv on empty dir: %v", err)
	}
	if _, ok := os.LookupEnv("HUAN_TEST_NONEXISTENT_VAR"); ok {
		t.Error("env should not have test var when .env absent")
	}
}

func TestLoadDotEnv_SetsVars(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "FOO=bar\nBAZ=qux\n")
	// Clear to ensure clean state
	os.Unsetenv("FOO")
	os.Unsetenv("BAZ")
	defer os.Unsetenv("FOO")
	defer os.Unsetenv("BAZ")

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("FOO"); got != "bar" {
		t.Errorf("FOO = %q, want bar", got)
	}
	if got := os.Getenv("BAZ"); got != "qux" {
		t.Errorf("BAZ = %q, want qux", got)
	}
}

func TestLoadDotEnv_DoesNotOverride(t *testing.T) {
	// Existing env var takes precedence over .env value.
	dir := t.TempDir()
	writeEnvFile(t, dir, "HUAN_TEST_VAR=from_env_file\n")
	os.Setenv("HUAN_TEST_VAR", "from_environment")
	defer os.Unsetenv("HUAN_TEST_VAR")

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("HUAN_TEST_VAR"); got != "from_environment" {
		t.Errorf("HUAN_TEST_VAR = %q, want from_environment (should not override)", got)
	}
}

func TestLoadDotEnv_QuotesStripped(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, `DOUBLE="quoted"
SINGLE='quoted'
NOQUOTE=bare
`)
	os.Unsetenv("DOUBLE")
	os.Unsetenv("SINGLE")
	os.Unsetenv("NOQUOTE")
	defer os.Unsetenv("DOUBLE")
	defer os.Unsetenv("SINGLE")
	defer os.Unsetenv("NOQUOTE")

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("DOUBLE"); got != "quoted" {
		t.Errorf("DOUBLE = %q, want quoted (double-quoted)", got)
	}
	if got := os.Getenv("SINGLE"); got != "quoted" {
		t.Errorf("SINGLE = %q, want quoted (single-quoted)", got)
	}
	if got := os.Getenv("NOQUOTE"); got != "bare" {
		t.Errorf("NOQUOTE = %q, want bare", got)
	}
}

func TestLoadDotEnv_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, `# Comment line
FOO=bar

# Another comment
BAZ=qux
`)
	os.Unsetenv("FOO")
	os.Unsetenv("BAZ")
	defer os.Unsetenv("FOO")
	defer os.Unsetenv("BAZ")

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("FOO"); got != "bar" {
		t.Errorf("FOO = %q, want bar", got)
	}
	if got := os.Getenv("BAZ"); got != "qux" {
		t.Errorf("BAZ = %q, want qux", got)
	}
}

func TestLoadDotEnv_InvalidFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"missing equals", "FOOBAR_no_equals"},
		{"invalid key", "1INVALID=value"},
		{"lowercase key", "lowercase=value"},
		{"empty key", "=value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeEnvFile(t, dir, tc.content+"\n")
			err := loadDotEnv(dir)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.content)
			}
		})
	}
}

func TestLoadDotEnv_ValueWithEquals(t *testing.T) {
	// Value containing = should not be split at second =
	dir := t.TempDir()
	writeEnvFile(t, dir, "URL=https://example.com/?foo=bar&baz=qux\n")
	os.Unsetenv("URL")
	defer os.Unsetenv("URL")

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	want := "https://example.com/?foo=bar&baz=qux"
	if got := os.Getenv("URL"); got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestIsValidEnvKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"", false},
		{"A", true},
		{"_", true},
		{"ABC", true},
		{"ABC_DEF", true},
		{"ABC_123", true},
		{"_PRIVATE", true},
		{"a", false},                  // lowercase
		{"1ABC", false},               // starts with digit
		{"ABC-DEF", false},            // dash
		{"ABC.DEF", false},            // dot
		{"中文", false},                  // non-ASCII
	}
	for _, tc := range tests {
		got := isValidEnvKey(tc.key)
		if got != tc.want {
			t.Errorf("isValidEnvKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestStripEnvValueQuotes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`"value"`, "value"},
		{`'value'`, "value"},
		{`value`, "value"},
		{`"value`, `"value`},          // unmatched quote, unchanged
		{`value"`, `value"`},          // unmatched quote, unchanged
		{`""`, ""},                    // empty quoted
		{`v`, "v"},                    // too short to quote-strip
		{`"embedded "quotes" inside"`, `embedded "quotes" inside`}, // double-quote strip only outermost
	}
	for _, tc := range tests {
		got := stripEnvValueQuotes(tc.in)
		if got != tc.want {
			t.Errorf("stripEnvValueQuotes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// writeEnvFile creates a .env file in dir with the given content.
func writeEnvFile(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}
