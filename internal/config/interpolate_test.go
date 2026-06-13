package config

import (
	"errors"
	"strings"
	"testing"
)

func setEnvs(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestInterpolate_NoVars(t *testing.T) {
	setEnvs(t, nil)
	in := map[string]any{
		"baseURL":   "https://example.com/",
		"paginate":  10,
		"minify":    true,
		"keywords":  []any{"a", "b"},
		"nested":    map[string]any{"deep": "value"},
	}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got := out["baseURL"]; got != "https://example.com/" {
		t.Errorf("baseURL = %v", got)
	}
	if got := out["paginate"]; got != 10 {
		t.Errorf("paginate = %v, want 10", got)
	}
}

func TestInterpolate_SimpleVar(t *testing.T) {
	t.Setenv("MY_TOKEN", "abc123")
	in := map[string]any{"token": "${MY_TOKEN}"}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got := out["token"]; got != "abc123" {
		t.Errorf("token = %v, want abc123", got)
	}
}

func TestInterpolate_PartialString(t *testing.T) {
	t.Setenv("HOST", "example.com")
	in := map[string]any{"url": "https://${HOST}/path"}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got := out["url"]; got != "https://example.com/path" {
		t.Errorf("url = %v", got)
	}
}

func TestInterpolate_NestedMap(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "abc123def")
	in := map[string]any{
		"plugins": map[string]any{
			"cloudflare": map[string]any{
				"accountId": "${CLOUDFLARE_ACCOUNT_ID}",
				"pages":      map[string]any{"project": "zhurongshuo"},
			},
		},
	}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	cf := out["plugins"].(map[string]any)["cloudflare"].(map[string]any)
	if got := cf["accountId"]; got != "abc123def" {
		t.Errorf("accountId = %v", got)
	}
}

func TestInterpolate_ArrayElements(t *testing.T) {
	t.Setenv("K1", "alpha")
	t.Setenv("K2", "beta")
	in := map[string]any{
		"keywords": []any{"${K1}", "${K2}", "literal"},
	}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	got := out["keywords"].([]any)
	if got[0] != "alpha" || got[1] != "beta" || got[2] != "literal" {
		t.Errorf("keywords = %v, want [alpha beta literal]", got)
	}
}

func TestInterpolate_UnsetVar_StrictError(t *testing.T) {
	in := map[string]any{"token": "${DEFINITELY_NOT_SET_VAR_XYZ}"}
	_, err := Interpolate(in)
	if err == nil {
		t.Fatal("Interpolate: want error for unset var, got nil")
	}
	var e *ErrEnvVarNotSet
	if !errors.As(err, &e) {
		t.Errorf("err = %T, want *ErrEnvVarNotSet", err)
	}
	if e.VarName != "DEFINITELY_NOT_SET_VAR_XYZ" {
		t.Errorf("VarName = %q", e.VarName)
	}
}

func TestInterpolate_UnsetVar_NestedError_ContainsKeyPath(t *testing.T) {
	in := map[string]any{
		"plugins": map[string]any{
			"cloudflare": map[string]any{
				"apiToken": "${ANOTHER_UNSET_VAR}",
			},
		},
	}
	_, err := Interpolate(in)
	if err == nil {
		t.Fatal("want error")
	}
	// Walk wraps errors with key path. Innermost should still be ErrEnvVarNotSet.
	var e *ErrEnvVarNotSet
	if !errors.As(err, &e) {
		t.Errorf("err = %T, want *ErrEnvVarNotSet (wrapped)", err)
	}
	if !strings.Contains(err.Error(), "apiToken") {
		t.Errorf("err = %q, want contains 'apiToken'", err.Error())
	}
}

func TestInterpolate_MultipleVarsInSameString(t *testing.T) {
	t.Setenv("A", "foo")
	t.Setenv("B", "bar")
	in := map[string]any{"x": "${A}-${B}-${A}"}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got := out["x"]; got != "foo-bar-foo" {
		t.Errorf("x = %v", got)
	}
}

func TestInterpolate_NilValues(t *testing.T) {
	in := map[string]any{"empty": nil}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got := out["empty"]; got != nil {
		t.Errorf("empty = %v, want nil", got)
	}
}

func TestInterpolate_LowercaseVarName_NotMatched(t *testing.T) {
	// envPattern restricts to [A-Z_][A-Z0-9_]*; lowercase should not match.
	t.Setenv("lowercase_var", "value")
	in := map[string]any{"x": "${lowercase_var}"}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	// Lowercase var should pass through unchanged (no interpolation).
	if got := out["x"]; got != "${lowercase_var}" {
		t.Errorf("x = %v, want literal ${lowercase_var}", got)
	}
}

func TestInterpolate_EmptyMap(t *testing.T) {
	in := map[string]any{}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("got %v, want empty", out)
	}
}

func TestInterpolate_EnvValueWithYamlSpecialChars(t *testing.T) {
	// Env value containing yaml-significant chars (": ", " #") should be
	// safely interpolated as a string. The walk returns the value as a string;
	// yaml.Marshal will quote it appropriately during re-marshal.
	t.Setenv("URL_WITH_COLON", "https://example.com:8080/path?x=1")
	in := map[string]any{"url": "${URL_WITH_COLON}"}
	out, err := Interpolate(in)
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got := out["url"]; got != "https://example.com:8080/path?x=1" {
		t.Errorf("url = %v", got)
	}
}
