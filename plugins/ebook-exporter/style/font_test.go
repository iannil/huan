package style

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindCJKFontCustomDir(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "NotoSansCJKsc-Regular.otf")
	os.WriteFile(fake, []byte("fake"), 0o644)
	got, err := FindCJKFont(dir)
	if err != nil || got != fake {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestFindCJKFontSourceHanSansFallback(t *testing.T) {
	dir := t.TempDir()
	// A non-CJK font that must be ignored.
	os.WriteFile(filepath.Join(dir, "TimesNewRoman.ttf"), []byte("fake"), 0o644)
	want := filepath.Join(dir, "SourceHanSansSC-Regular.otf")
	os.WriteFile(want, []byte("fake"), 0o644)
	got, err := FindCJKFont(dir)
	if err != nil || got != want {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestFindCJKFontMissing(t *testing.T) {
	if _, err := FindCJKFont(t.TempDir()); err == nil || !strings.Contains(err.Error(), "CJK") {
		t.Fatalf("want CJK-missing error, got %v", err)
	}
}
