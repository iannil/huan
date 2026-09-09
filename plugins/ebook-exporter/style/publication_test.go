package style

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeCJKSource points the known-source chain at a temp TTC whose font 1
// has TrueType outlines (buildTestTTC fixture) for the duration of the test.
func withFakeCJKSource(t *testing.T, cacheDir string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "fake.ttc")
	if err := os.WriteFile(src, buildTestTTC(t), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := publicationCJKSources
	publicationCJKSources = []FontRef{{Path: src, Index: 1}}
	t.Cleanup(func() { publicationCJKSources = orig })
	return src
}

func TestFindPublicationCJKFontPrefersConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.ttf")
	tt := buildTestFont(t, map[string][]byte{"glyf": []byte("g"), "loca": {0, 0, 0, 1}})
	if err := os.WriteFile(cfg, tt, 0o644); err != nil {
		t.Fatal(err)
	}
	got, note, err := FindPublicationCJKFont(cfg, "", filepath.Join(dir, "cache"))
	if err != nil || got != cfg || note != "" {
		t.Fatalf("want (%s, \"\", nil), got (%s, %q, %v)", cfg, got, note, err)
	}
}

func TestFindPublicationCJKFontFallsBackToKnownSource(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	src := withFakeCJKSource(t, cache)

	got, note, err := FindPublicationCJKFont(filepath.Join(dir, "missing.ttf"), "", cache)
	if err != nil {
		t.Fatal(err)
	}
	if want := "derived from " + src; note != want {
		t.Fatalf("note = %q, want %q", note, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !HasTrueTypeOutlines(data) {
		t.Fatal("derived font: want TrueType outlines")
	}
}

func TestFindPublicationCJKFontMissingConfigEmpty(t *testing.T) {
	// 空 cfgPath：直接走已知源（不报错）。
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	withFakeCJKSource(t, cache)
	if _, _, err := FindPublicationCJKFont("", "", cache); err != nil {
		t.Fatalf("empty cfgPath should try known sources, got %v", err)
	}
}

func TestFindPublicationLatinFont(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "times.ttf")
	if err := os.WriteFile(cfg, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindPublicationLatinFont(cfg); got != cfg {
		t.Fatalf("configured latin: got %q want %q", got, cfg)
	}
	// 系统扫描取决于宿主机（macOS 自带 Georgia.ttf），缺失 cfgPath 时
	// 期望要么 graceful degrade 返回 ""，要么命中已知回退名。
	got := FindPublicationLatinFont(filepath.Join(dir, "nope.ttf"))
	if got == "" {
		return
	}
	lower := strings.ToLower(got)
	for _, s := range []string{"timesnewroman", "georgia", "didot", "charter"} {
		if strings.Contains(lower, s) {
			return
		}
	}
	t.Fatalf("missing latin: want \"\" or a known system fallback, got %q", got)
}
