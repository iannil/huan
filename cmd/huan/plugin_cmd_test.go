package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/deploy"
)

func TestMaskSensitive_SimpleTokenField(t *testing.T) {
	in := map[string]any{
		"accountId": "acc-123",
		"apiToken":  "secret-value",
	}
	out := maskSensitive(in)
	if out["accountId"] != "acc-123" {
		t.Errorf("accountId = %v, want unchanged", out["accountId"])
	}
	if out["apiToken"] != "***" {
		t.Errorf("apiToken = %v, want ***", out["apiToken"])
	}
}

func TestMaskSensitive_NestedMapRecursive(t *testing.T) {
	in := map[string]any{
		"r2": map[string]any{
			"accessKeyId":     "AKIA-XYZ",
			"secretAccessKey": "shhh",
			"bucket":          "public-bucket",
		},
	}
	out := maskSensitive(in)
	r2 := out["r2"].(map[string]any)
	if r2["accessKeyId"] != "***" {
		t.Errorf("accessKeyId = %v, want ***", r2["accessKeyId"])
	}
	if r2["secretAccessKey"] != "***" {
		t.Errorf("secretAccessKey = %v, want ***", r2["secretAccessKey"])
	}
	if r2["bucket"] != "public-bucket" {
		t.Errorf("bucket = %v, want unchanged", r2["bucket"])
	}
}

func TestMaskSensitive_ArrayValues(t *testing.T) {
	in := map[string]any{
		"sync": []any{
			map[string]any{"from": "src", "to": "dst"},
		},
	}
	out := maskSensitive(in)
	sync, ok := out["sync"].([]any)
	if !ok || len(sync) != 1 {
		t.Fatalf("sync lost structure: %v", out["sync"])
	}
	entry := sync[0].(map[string]any)
	if entry["from"] != "src" {
		t.Errorf("from = %v", entry["from"])
	}
}

func TestMaskSensitive_NonSensitiveFieldsUntouched(t *testing.T) {
	in := map[string]any{
		"project": "zhurongshuo",
		"branch":  "main",
		"path":    "/some/path",
		"count":   42,
	}
	out := maskSensitive(in)
	for k, v := range in {
		if out[k] != v {
			t.Errorf("field %q changed: %v -> %v", k, v, out[k])
		}
	}
}

func TestMaskSensitive_CaseInsensitive(t *testing.T) {
	cases := map[string]bool{
		"token":         true,
		"Token":         true,
		"TOKEN":         true,
		"apiToken":      true,
		"API_TOKEN":     true,
		"accessKeyId":   true,
		"secretAccessKey": true,
		"password":      true,
		"PASSWORD":      true,
		"mySecretVar":   true,
		"apiKeyValue":   true,
		// Non-sensitive
		"project":     false,
		"branch":      false,
		"bucket":      false,
		"publicToken": true, // contains "token"
		"counter":     false,
	}
	for key, wantSensitive := range cases {
		t.Run(key, func(t *testing.T) {
			in := map[string]any{key: "some-value"}
			out := maskSensitive(in)
			got := out[key]
			if wantSensitive {
				if got != "***" {
					t.Errorf("key %q: got %v, want ***", key, got)
				}
			} else {
				if got != "some-value" {
					t.Errorf("key %q: got %v, want unchanged", key, got)
				}
			}
		})
	}
}

func TestMaskSensitive_NilInput(t *testing.T) {
	out := maskSensitive(nil)
	if out != nil {
		t.Errorf("maskSensitive(nil) = %v, want nil", out)
	}
}

func TestMaskSensitive_EmptyStringValueNotMasked(t *testing.T) {
	// Empty string for a sensitive field should not be masked (avoid
	// turning missing values into "***" which is misleading).
	in := map[string]any{"apiToken": ""}
	out := maskSensitive(in)
	if out["apiToken"] != "" {
		t.Errorf("empty apiToken = %v, want empty (not ***)", out["apiToken"])
	}
}

// TestPrintPluginInfo_MasksByDefault verifies the CLI output path: calling
// printPluginInfo without --show-secrets should produce output where
// sensitive fields show as ***.
func TestPrintPluginInfo_MasksByDefault(t *testing.T) {
	p := &stubDeployer{name: "test-plugin"}
	rawCfg := map[string]any{
		"accountId": "acc-1",
		"apiToken":  "real-secret-tok",
		"pages":     map[string]any{"project": "p", "branch": "main"},
	}
	out := captureStdout(t, func() {
		printPluginInfo(p, rawCfg, false)
	})
	if !strings.Contains(out, "***") {
		t.Errorf("output missing *** mask:\n%s", out)
	}
	if strings.Contains(out, "real-secret-tok") {
		t.Errorf("output leaked secret value:\n%s", out)
	}
	if !strings.Contains(out, "acc-1") {
		t.Errorf("output missing non-sensitive accountId:\n%s", out)
	}
}

func TestPrintPluginInfo_ShowSecretsReveals(t *testing.T) {
	p := &stubDeployer{name: "test-plugin"}
	rawCfg := map[string]any{
		"accountId": "acc-1",
		"apiToken":  "real-secret-tok",
	}
	out := captureStdout(t, func() {
		printPluginInfo(p, rawCfg, true)
	})
	if !strings.Contains(out, "real-secret-tok") {
		t.Errorf("output missing secret value with --show-secrets:\n%s", out)
	}
	if strings.Contains(out, "***") {
		t.Errorf("output masked despite --show-secrets:\n%s", out)
	}
}

func TestPrintPluginInfo_DeployCapabilityLabel(t *testing.T) {
	p := &stubDeployer{name: "test-plugin"}
	out := captureStdout(t, func() {
		printPluginInfo(p, map[string]any{}, false)
	})
	if !strings.Contains(out, "deploy") {
		t.Errorf("output missing 'deploy' capability label:\n%s", out)
	}
}

// captureStdout temporarily redirects os.Stdout to capture fmt.Println output,
// restoring it after the function returns. The captured string is returned.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		buf := new(bytes.Buffer)
		_, _ = io.Copy(buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

// Compile-time assertion that stubDeployer (defined in plugins_test.go)
// satisfies the deploy.Deployer contract used by capabilityLabels.
var _ deploy.Deployer = (*stubDeployer)(nil)

// _ context to keep the import in case tests need it later.
var _ = context.Background
