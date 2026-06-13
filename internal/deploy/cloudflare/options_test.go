package cloudflare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	raw := map[string]any{
		"accountId": "acc-123",
		"apiToken":  "tok-456",
		"pages": map[string]any{
			"project": "myproj",
			"branch":  "main",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.AccountID != "acc-123" {
		t.Errorf("AccountID = %q", cfg.AccountID)
	}
	if cfg.APIToken != "tok-456" {
		t.Errorf("APIToken = %q", cfg.APIToken)
	}
	if cfg.Pages.Project != "myproj" {
		t.Errorf("Pages.Project = %q", cfg.Pages.Project)
	}
	if cfg.Pages.Branch != "main" {
		t.Errorf("Pages.Branch = %q", cfg.Pages.Branch)
	}
}

func TestParseConfig_NilMap(t *testing.T) {
	_, err := ParseConfig(nil)
	if err == nil {
		t.Fatal("want error for nil map")
	}
}

func TestParseConfig_MissingAccountID(t *testing.T) {
	raw := map[string]any{
		"apiToken": "tok",
		"pages":    map[string]any{"project": "p", "branch": "b"},
	}
	_, err := ParseConfig(raw)
	if err == nil {
		t.Fatal("want error for missing accountId")
	}
	if !strings.Contains(err.Error(), "accountId") {
		t.Errorf("err = %q, want contains 'accountId'", err.Error())
	}
}

func TestParseConfig_MissingAPIToken(t *testing.T) {
	raw := map[string]any{
		"accountId": "acc",
		"pages":      map[string]any{"project": "p", "branch": "b"},
	}
	_, err := ParseConfig(raw)
	if err == nil {
		t.Fatal("want error for missing apiToken")
	}
}

func TestParseConfig_MissingProject(t *testing.T) {
	raw := map[string]any{
		"accountId": "acc",
		"apiToken":  "tok",
		"pages":     map[string]any{"branch": "b"},
	}
	_, err := ParseConfig(raw)
	if err == nil {
		t.Fatal("want error for missing project")
	}
}

func TestParseConfig_MissingBranch(t *testing.T) {
	raw := map[string]any{
		"accountId": "acc",
		"apiToken":  "tok",
		"pages":     map[string]any{"project": "p"},
	}
	_, err := ParseConfig(raw)
	if err == nil {
		t.Fatal("want error for missing branch")
	}
}

func TestParseConfig_ErrorMessagesActionable(t *testing.T) {
	// Error messages should mention the yaml path (plugins.cloudflare.xxx)
	// and typical env var pattern so users can fix without reading code.
	_, err := ParseConfig(map[string]any{
		"apiToken": "tok",
		"pages":    map[string]any{"project": "p", "branch": "b"},
	})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "accountId") {
		t.Errorf("err = %q, want mention accountId", err.Error())
	}
	// Should hint at typical env var.
	if !strings.Contains(err.Error(), "CLOUDFLARE_ACCOUNT_ID") {
		t.Errorf("err = %q, want mention CLOUDFLARE_ACCOUNT_ID", err.Error())
	}
}

// TestParseConfig_PropagatesAccountIDToR2 verifies that plugins.cloudflare.accountId
// is propagated to r2.accountId when the latter is not set explicitly. R2 needs
// accountId to construct its S3 endpoint, but requiring users to duplicate
// ${CLOUDFLARE_ACCOUNT_ID} under r2: is redundant — it's the same CF account.
func TestParseConfig_PropagatesAccountIDToR2(t *testing.T) {
	raw := map[string]any{
		"accountId": "acc-123",
		"apiToken":  "tok",
		"pages":     map[string]any{"project": "p", "branch": "main"},
		"r2": map[string]any{
			"accessKeyId":     "k",
			"secretAccessKey": "s",
			"bucket":          "b",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.R2.AccountID != "acc-123" {
		t.Errorf("R2.AccountID = %q, want %q (propagated from top-level)", cfg.R2.AccountID, "acc-123")
	}
}

// TestParseConfig_R2AccountIDExplicitWins verifies that an explicit r2.accountId
// is not overwritten by propagation.
func TestParseConfig_R2AccountIDExplicitWins(t *testing.T) {
	raw := map[string]any{
		"accountId": "top-level",
		"apiToken":  "tok",
		"pages":     map[string]any{"project": "p", "branch": "main"},
		"r2": map[string]any{
			"accountId":       "explicit",
			"accessKeyId":     "k",
			"secretAccessKey": "s",
			"bucket":          "b",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.R2.AccountID != "explicit" {
		t.Errorf("R2.AccountID = %q, want %q (explicit override)", cfg.R2.AccountID, "explicit")
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := []byte(`accountId: acc-from-file
apiToken: tok-from-file
pages:
  project: proj-from-file
  branch: main
`)
	path := filepath.Join(dir, "cf.yaml")
	if err := os.WriteFile(path, yamlContent, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}
	if cfg.AccountID != "acc-from-file" {
		t.Errorf("AccountID = %q", cfg.AccountID)
	}
	if cfg.Pages.Project != "proj-from-file" {
		t.Errorf("Pages.Project = %q", cfg.Pages.Project)
	}
}

// TestWorkerConfig_Validate_CompatibilityDate exercises audit M2 fix:
// compatibility_date must be YYYY-MM-DD format when set; empty is allowed
// (deployer fills default "2024-01-01").
func TestWorkerConfig_Validate_CompatibilityDate(t *testing.T) {
	cases := []struct {
		name string
		date string
		ok   bool
	}{
		{"empty allowed (default applied at deploy)", "", true},
		{"valid ISO date", "2024-01-01", true},
		{"valid recent date", "2026-06-13", true},
		{"slashes rejected", "2024/01/01", false},
		{"time rejected", "2024-01-01T00:00:00Z", false},
		{"relative rejected", "yesterday", false},
		{"month out of range", "2024-13-01", false},
		{"day out of range", "2024-01-32", false},
		{"two-digit year", "24-01-01", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := WorkerConfig{Name: "w", Script: "x.js", CompatibilityDate: tc.date}
			err := cfg.validate()
			if tc.ok && err != nil {
				t.Errorf("date %q: want nil, got %v", tc.date, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("date %q: want error, got nil", tc.date)
			}
		})
	}
}
