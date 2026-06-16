package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSidecarLang(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"foo.md", ""},
		{"foo.en.md", "en"},
		{"foo.zh-cn.md", "zh-cn"},
		{"_index.en.md", "en"},
		{"index.md", ""},
		{"foo.bar.md", "bar"},
		{"foo.a.md", ""},
		{"foo.UPPER.md", ""},
		{"foo.en_US.md", ""},
	}
	for _, tc := range tests {
		got := detectSidecarLang(tc.name)
		if got != tc.want {
			t.Errorf("detectSidecarLang(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestStripSidecarLangSuffix(t *testing.T) {
	tests := []struct {
		path string
		lang string
		want string
	}{
		{"content/posts/foo.en.md", "en", "content/posts/foo.md"},
		{"content/posts/foo.zh-cn.md", "zh-cn", "content/posts/foo.md"},
		{"content/posts/foo.md", "en", "content/posts/foo.md"}, // no lang suffix
	}
	for _, tc := range tests {
		got := stripSidecarLangSuffix(tc.path, tc.lang)
		if got != tc.want {
			t.Errorf("stripSidecarLangSuffix(%q, %q) = %q, want %q",
				tc.path, tc.lang, got, tc.want)
		}
	}
}

func TestExtractSourceHash(t *testing.T) {
	// Has frontmatter with source_hash
	markdown := `---
translation_of: posts/foo.md
source_hash: abc123def456
model: qwen3
---

Body content.
`
	hash, ok := extractSourceHash(markdown)
	if !ok {
		t.Error("expected source_hash to be found")
	}
	if hash != "abc123def456" {
		t.Errorf("hash = %q, want %q", hash, "abc123def456")
	}

	// No frontmatter
	markdown = "no frontmatter here"
	_, ok = extractSourceHash(markdown)
	if ok {
		t.Error("expected no source_hash for missing frontmatter")
	}

	// Frontmatter but no source_hash
	markdown = `---
title: Foo
---
Body`
	_, ok = extractSourceHash(markdown)
	if ok {
		t.Error("expected no source_hash when field absent")
	}
}

func TestCheckStaleTranslations_AllFresh(t *testing.T) {
	dir := t.TempDir()
	// Source file
	srcPath := filepath.Join(dir, "foo.md")
	srcContent := []byte("# Foo\n\nHello world.\n")
	if err := os.WriteFile(srcPath, srcContent, 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	// Compute hash
	hash := sha256HexForTest(srcContent)

	// Sidecar with matching hash
	sidecar := `---
translation_of: foo.md
source_hash: ` + hash + `
---

Body translation.
`
	if err := os.WriteFile(filepath.Join(dir, "foo.en.md"), []byte(sidecar), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	report, err := checkStaleTranslations(dir)
	if err != nil {
		t.Fatalf("checkStaleTranslations: %v", err)
	}
	if report.Stale != 0 {
		t.Errorf("stale = %d, want 0; files=%v", report.Stale, report.StaleFiles)
	}
	if report.Checked != 1 {
		t.Errorf("checked = %d, want 1", report.Checked)
	}
}

func TestCheckStaleTranslations_EmptyBodySourceSkipped(t *testing.T) {
	dir := t.TempDir()
	// Frontmatter-only source (e.g. a section _index.md) — translate SKIPs it,
	// so its manually-authored sidecar has no source_hash and must NOT be flagged.
	if err := os.WriteFile(filepath.Join(dir, "_index.md"),
		[]byte("---\ntitle: Books\n---\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_index.en.md"),
		[]byte("---\ntitle: Books\n---\n"), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	// A normal-body source with a hashless sidecar SHOULD still be flagged.
	if err := os.WriteFile(filepath.Join(dir, "real.md"),
		[]byte("---\ntitle: R\n---\nBody.\n"), 0644); err != nil {
		t.Fatalf("write real src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.en.md"),
		[]byte("---\ntitle: R\n---\nTranslated.\n"), 0644); err != nil {
		t.Fatalf("write real sidecar: %v", err)
	}

	report, err := checkStaleTranslations(dir)
	if err != nil {
		t.Fatalf("checkStaleTranslations: %v", err)
	}
	if report.Missing != 1 {
		t.Errorf("missing = %d, want 1 (only real.en.md); files=%v", report.Missing, report.MissingHashFiles)
	}
	for _, f := range report.MissingHashFiles {
		if f == "_index.en.md" {
			t.Errorf("_index.en.md (empty-body source) must not be flagged missing")
		}
	}
}

func TestMarkdownBodyIsEmpty(t *testing.T) {
	cases := map[string]bool{
		"---\ntitle: X\n---\n":            true,
		"---\ntitle: X\n---\n\n  \n":      true,
		"---\ntitle: X\n---\nBody here\n": false,
		"":                               true,
		"   ":                            true,
		"No frontmatter body":            false,
	}
	for in, want := range cases {
		if got := markdownBodyIsEmpty(in); got != want {
			t.Errorf("markdownBodyIsEmpty(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCheckStaleTranslations_StaleDetected(t *testing.T) {
	dir := t.TempDir()
	// Source file (current)
	if err := os.WriteFile(filepath.Join(dir, "foo.md"),
		[]byte("# Foo\n\nCurrent source content.\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	// Sidecar with OLD hash (doesn't match current source)
	sidecar := `---
translation_of: foo.md
source_hash: deadbeef0000000000000000000000000000000000000000000000000000dead
---

Old translation.
`
	if err := os.WriteFile(filepath.Join(dir, "foo.en.md"), []byte(sidecar), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	report, err := checkStaleTranslations(dir)
	if err != nil {
		t.Fatalf("checkStaleTranslations: %v", err)
	}
	if report.Stale != 1 {
		t.Errorf("stale = %d, want 1", report.Stale)
	}
	if len(report.StaleFiles) != 1 {
		t.Errorf("StaleFiles len = %d, want 1", len(report.StaleFiles))
	}
}

func TestCheckStaleTranslations_MissingHash(t *testing.T) {
	dir := t.TempDir()
	// Source
	if err := os.WriteFile(filepath.Join(dir, "foo.md"),
		[]byte("# Foo\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	// Sidecar without source_hash field (hand-created)
	sidecar := `---
translation_of: foo.md
---

Hand-translated without source_hash.
`
	if err := os.WriteFile(filepath.Join(dir, "foo.en.md"), []byte(sidecar), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	report, err := checkStaleTranslations(dir)
	if err != nil {
		t.Fatalf("checkStaleTranslations: %v", err)
	}
	if report.Missing != 1 {
		t.Errorf("missing = %d, want 1", report.Missing)
	}
	if report.Checked != 0 {
		t.Errorf("checked = %d, want 0 (missing doesn't count as checked)", report.Checked)
	}
}

func TestStrictI18nEnabled(t *testing.T) {
	// Save and restore env
	original := os.Getenv("HUAN_STRICT_I18N")
	defer os.Setenv("HUAN_STRICT_I18N", original)

	tests := []struct {
		env  string
		want bool
	}{
		{"", false},
		{"false", false},
		{"no", false},
		{"0", false},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
	}
	for _, tc := range tests {
		os.Setenv("HUAN_STRICT_I18N", tc.env)
		got := strictI18nEnabled()
		if got != tc.want {
			t.Errorf("strictI18nEnabled() with env=%q = %v, want %v",
				tc.env, got, tc.want)
		}
	}
}

func TestI18nStaleReport_Error(t *testing.T) {
	// Empty report → empty error
	r := &I18nStaleReport{}
	if got := r.Error(); got != "" {
		t.Errorf("empty report Error() = %q, want empty", got)
	}

	// Report with stale → non-empty error
	r = &I18nStaleReport{
		Stale:      2,
		StaleFiles: []string{"posts/foo.en.md", "posts/bar.en.md"},
	}
	got := r.Error()
	if got == "" {
		t.Error("stale report Error() should be non-empty")
	}
	if !contains(got, "stale") {
		t.Errorf("error should mention 'stale': %q", got)
	}
	if !contains(got, "posts/foo.en.md") {
		t.Errorf("error should list stale file: %q", got)
	}
}

// sha256HexForTest computes hex sha256 of input (test helper, not exported).
func sha256HexForTest(data []byte) string {
	return sha256HexString(data)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
