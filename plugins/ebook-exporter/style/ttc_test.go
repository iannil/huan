package style

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTTCExtractsFontOne(t *testing.T) {
	ttc := buildTestTTC(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ttc")
	if err := os.WriteFile(src, ttc, 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")

	out, err := ExtractTTC(src, 1, cache)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !HasTrueTypeOutlines(data) {
		t.Fatal("extracted font: want TrueType outlines")
	}
	if filepath.Dir(out) != cache {
		t.Fatalf("extracted into %s, want under %s", out, cache)
	}

	// Second call hits the cache: same path, file unchanged.
	out2, err := ExtractTTC(src, 1, cache)
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Fatalf("cache miss on second call: %s vs %s", out2, out)
	}
}

func TestExtractTTCIndexOutOfRange(t *testing.T) {
	ttc := buildTestTTC(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ttc")
	if err := os.WriteFile(src, ttc, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractTTC(src, 5, filepath.Join(dir, "cache")); err == nil {
		t.Fatal("index 5 in 2-font collection: want error")
	}
}

func TestExtractTTCPassesThroughSingleFont(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.ttf")
	tt := buildTestFont(t, map[string][]byte{"glyf": []byte("g"), "loca": {0, 0, 0, 1}})
	if err := os.WriteFile(src, tt, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ExtractTTC(src, 0, filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if out != src {
		t.Fatalf("single font: want pass-through, got %s", out)
	}
}

func TestExtractTTCMissingSource(t *testing.T) {
	if _, err := ExtractTTC(filepath.Join(t.TempDir(), "nope.ttc"), 0, t.TempDir()); err == nil {
		t.Fatal("missing source: want error")
	}
}
