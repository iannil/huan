package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/content"
)

func sha256HexForTest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func countOccurrences(s, sub string) int {
	return strings.Count(s, sub)
}

func TestBackfillSidecarContent_MissingHashAddsAllFields(t *testing.T) {
	src := []byte("---\ntitle: \"源\"\n---\n\n正文内容\n")
	sidecar := "---\n" +
		"title: \"Foo\"\n" +
		"date: 2025-01-01T00:00:00+08:00\n" +
		"hidden: true\n" +
		"draft: false\n" +
		"---\n\n" +
		"Body text here.\n"

	out, changed, err := backfillSidecarContent(src, sidecar, "general/paper/index.md", "zh-cn", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for sidecar missing source_hash")
	}

	wantHash := sha256HexForTest(src)
	for _, want := range []string{
		"source_hash: " + wantHash,
		"translation_of: general/paper/index.md",
		"source_lang: zh-cn",
		"target_lang: en",
		"title: \"Foo\"", // existing field preserved
		"Body text here.", // body preserved
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}

	// Re-parsing the result must yield the stamped hash (build-side parity).
	fm, body, perr := content.ParseFrontmatter([]byte(out))
	if perr != nil {
		t.Fatalf("result does not re-parse: %v", perr)
	}
	if got, _ := fm["source_hash"].(string); got != wantHash {
		t.Errorf("re-parsed source_hash = %q, want %q", got, wantHash)
	}
	if strings.TrimSpace(body) != "Body text here." {
		t.Errorf("body altered: %q", body)
	}
}

func TestBackfillSidecarContent_AlreadyHasHashIsNoop(t *testing.T) {
	src := []byte("---\ntitle: \"源\"\n---\n\n正文\n")
	sidecar := "---\n" +
		"title: \"Foo\"\n" +
		"source_hash: deadbeef\n" +
		"---\n\nBody.\n"

	out, changed, err := backfillSidecarContent(src, sidecar, "x.md", "zh-cn", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected changed=false when source_hash already present")
	}
	if out != sidecar {
		t.Errorf("content modified despite existing hash:\n%s", out)
	}
}

func TestBackfillSidecarContent_OnlyAddsMissingManagedFields(t *testing.T) {
	src := []byte("---\ntitle: \"源\"\n---\n\n正文\n")
	sidecar := "---\n" +
		"title: \"Foo\"\n" +
		"translation_of: general/x.md\n" +
		"source_lang: zh-cn\n" +
		"target_lang: en\n" +
		"---\n\nBody.\n"

	out, changed, err := backfillSidecarContent(src, sidecar, "general/x.md", "zh-cn", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true (source_hash still missing)")
	}
	if !strings.Contains(out, "source_hash: "+sha256HexForTest(src)) {
		t.Error("source_hash not added")
	}
	// Existing managed fields must not be duplicated.
	if n := countOccurrences(out, "translation_of:"); n != 1 {
		t.Errorf("translation_of appears %d times, want 1", n)
	}
	if n := countOccurrences(out, "source_lang:"); n != 1 {
		t.Errorf("source_lang appears %d times, want 1", n)
	}
	if n := countOccurrences(out, "target_lang:"); n != 1 {
		t.Errorf("target_lang appears %d times, want 1", n)
	}
}

func TestBackfillSidecarContent_NoFrontmatterWrapsBody(t *testing.T) {
	src := []byte("---\ntitle: \"源\"\n---\n\n正文\n")
	sidecar := "Just body, no frontmatter.\n"

	out, changed, err := backfillSidecarContent(src, sidecar, "x.md", "zh-cn", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("expected new frontmatter block, got:\n%s", out)
	}
	if !strings.Contains(out, "Just body, no frontmatter.") {
		t.Error("original body lost")
	}
	if _, _, perr := content.ParseFrontmatter([]byte(out)); perr != nil {
		t.Errorf("wrapped result does not parse: %v", perr)
	}
}

func TestSplitFrontmatter_ExactReconstruction(t *testing.T) {
	cases := []string{
		"---\ntitle: x\ndate: y\n---\n\nbody here\n",
		"---\na: 1\n---\nno blank line before body\n",
		"---\nonly: field\n---", // no trailing content
	}
	for _, in := range cases {
		fm, rest, ok := splitFrontmatter(in)
		if !ok {
			t.Errorf("splitFrontmatter(%q) ok=false", in)
			continue
		}
		got := "---\n" + fm + "\n---" + rest
		if got != in {
			t.Errorf("reconstruction mismatch:\n in:  %q\n got: %q", in, got)
		}
	}

	if _, _, ok := splitFrontmatter("no frontmatter at all\n"); ok {
		t.Error("expected ok=false for content without frontmatter")
	}
	if _, _, ok := splitFrontmatter("---\nunterminated frontmatter\n"); ok {
		t.Error("expected ok=false for unterminated frontmatter")
	}
}

func TestFmHasNonEmptyString(t *testing.T) {
	fm := map[string]interface{}{
		"a": "value",
		"b": "   ",
		"c": "",
		"d": 123,
	}
	if !fmHasNonEmptyString(fm, "a") {
		t.Error("a should be non-empty")
	}
	if fmHasNonEmptyString(fm, "b") {
		t.Error("b is blank, should be false")
	}
	if fmHasNonEmptyString(fm, "c") {
		t.Error("c is empty, should be false")
	}
	if fmHasNonEmptyString(fm, "d") {
		t.Error("d is non-string, should be false")
	}
	if fmHasNonEmptyString(fm, "missing") {
		t.Error("missing key should be false")
	}
}
