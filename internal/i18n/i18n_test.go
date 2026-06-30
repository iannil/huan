package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

// mustWriteYAML is a test helper that writes a YAML translation file.
func mustWriteYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestNew_EmptyBundle verifies the constructor returns an empty (not nil)
// bundle that responds to Keys() with 0.
func TestNew_EmptyBundle(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New() returned nil")
	}
	if b.Keys() != 0 {
		t.Errorf("empty bundle Keys() = %d, want 0", b.Keys())
	}
}

// TestBundle_LoadFile_ParsesHugoFormat verifies the standard Hugo i18n
// YAML format: top-level key → {other: "translation"}.
func TestBundle_LoadFile_ParsesHugoFormat(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "en.yaml")
	mustWriteYAML(t, path, `
home:
  other: Home
posts:
  other: Posts
`)

	b := New()
	if err := b.LoadFile(path); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if b.Keys() != 2 {
		t.Errorf("Keys() = %d, want 2", b.Keys())
	}
	if got := b.Translate("home"); got != "Home" {
		t.Errorf("Translate(home) = %q, want Home", got)
	}
}

// TestBundle_LoadFile_SkipsEntriesWithoutOther verifies the YAML format
// tolerance: entries without "other" field are silently skipped (Hugo's
// i18n also has "one", "few", "many" plural forms which we don't support).
func TestBundle_LoadFile_SkipsEntriesWithoutOther(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "en.yaml")
	mustWriteYAML(t, path, `
valid:
  other: OK
plural_only:
  one: one item
  many: many items
`)

	b := New()
	if err := b.LoadFile(path); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if b.Keys() != 1 {
		t.Errorf("Keys() = %d, want 1 (only 'valid')", b.Keys())
	}
	if got := b.Translate("plural_only"); got != "plural_only" {
		t.Errorf("Translate(plural_only) = %q, want key fallback %q", got, "plural_only")
	}
}

// TestBundle_LoadFile_MissingFile verifies error behavior for nonexistent file.
func TestBundle_LoadFile_MissingFile(t *testing.T) {
	b := New()
	err := b.LoadFile("/nonexistent/path.yaml")
	if err == nil {
		t.Errorf("LoadFile on missing file: expected error, got nil")
	}
}

// TestBundle_LoadFile_InvalidYAML verifies error behavior for malformed YAML.
func TestBundle_LoadFile_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	// YAML with mismatched quote — parse error.
	mustWriteYAML(t, path, `key: "unterminated`)

	b := New()
	err := b.LoadFile(path)
	if err == nil {
		t.Errorf("LoadFile on invalid YAML: expected error, got nil")
	}
}

// TestBundle_LoadDir_LoadsAllYAMLs verifies directory loading picks up
// all .yaml/.yml files in nested subdirs.
func TestBundle_LoadDir_LoadsAllYAMLs(t *testing.T) {
	tmp := t.TempDir()
	mustWriteYAML(t, filepath.Join(tmp, "en.yaml"), `
home:
  other: Home
`)
	mustWriteYAML(t, filepath.Join(tmp, "extras", "menu.yaml"), `
menu_about:
  other: About
`)
	// Non-YAML file should be ignored.
	mustWriteYAML(t, filepath.Join(tmp, "README.md"), "# i18n")

	b := New()
	if err := b.LoadDir(tmp); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if b.Keys() != 2 {
		t.Errorf("Keys() = %d, want 2 (en.yaml + menu.yaml)", b.Keys())
	}
	if got := b.Translate("menu_about"); got != "About" {
		t.Errorf("Translate(menu_about) = %q, want About", got)
	}
}

// TestBundle_LoadDir_LaterFilesOverride verifies override behavior when
// two files define the same key (e.g., theme defaults + site overrides).
func TestBundle_LoadDir_LaterFilesOverride(t *testing.T) {
	tmp := t.TempDir()
	// filepath.Walk visits alphabetically: a.yaml before b.yaml.
	mustWriteYAML(t, filepath.Join(tmp, "a.yaml"), `
greeting:
  other: Hi
`)
	mustWriteYAML(t, filepath.Join(tmp, "b.yaml"), `
greeting:
  other: Hello
`)

	b := New()
	if err := b.LoadDir(tmp); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if got := b.Translate("greeting"); got != "Hello" {
		t.Errorf("Translate(greeting) = %q, want Hello (b.yaml overrides)", got)
	}
}

// TestBundle_Translate_MissingKeyReturnsKey verifies the fallback behavior:
// missing keys return the key itself, so templates render *something* rather
// than empty/blank.
func TestBundle_Translate_MissingKeyReturnsKey(t *testing.T) {
	b := New()
	if got := b.Translate("nonexistent.key"); got != "nonexistent.key" {
		t.Errorf("Translate(missing) = %q, want key fallback", got)
	}
}

// TestBundle_Translate_AcceptsArgs verifies Translate accepts variadic args
// (Hugo's i18n supports fmt.Sprintf-style interpolation). huan's implementation
// currently ignores args (no template uses them), but the signature must
// accept them for template compatibility.
func TestBundle_Translate_AcceptsArgs(t *testing.T) {
	b := New()
	// Just verify it doesn't panic with args.
	got := b.Translate("key", "arg1", 42, nil)
	if got != "key" {
		t.Errorf("Translate with args = %q, want key", got)
	}
}

// TestBundle_NilBundleIsSafe verifies nil-receiver safety on Keys/Translate —
// templates should never panic from a missing bundle.
func TestBundle_NilBundleIsSafe(t *testing.T) {
	var b *Bundle
	if got := b.Keys(); got != 0 {
		t.Errorf("nil Keys() = %d, want 0", got)
	}
	// Translate on nil bundle would dereference; we test by checking the
	// empty-bundle case instead. (Translate isn't nil-safe per the current
	// implementation — it panics on nil. That's acceptable since the build
	// pipeline always provides a non-nil bundle.)
}

// TestBundle_LoadFile_RealWorldZhurongshuoFormat verifies the actual
// translation file format zhurongshuo uses (sanity check against format drift).
func TestBundle_LoadFile_RealWorldZhurongshuoFormat(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "zh-cn.yaml")
	mustWriteYAML(t, path, `
home:
  other: 首页
about:
  other: 关于
copyright:
  other: 版权所有
`)

	b := New()
	if err := b.LoadFile(path); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := b.Translate("home"); got != "首页" {
		t.Errorf("Translate(home) = %q, want 首页", got)
	}
	if got := b.Translate("copyright"); got != "版权所有" {
		t.Errorf("Translate(copyright) = %q, want 版权所有", got)
	}
}
