package equiv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareDirs_Normalized_EquivalentHTML(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	htmlA := "<html><body>\n  <p>hi</p>\n</body></html>"
	htmlB := "<html><body><p>hi</p></body></html>"
	os.WriteFile(filepath.Join(dirA, "index.html"), []byte(htmlA), 0o644)
	os.WriteFile(filepath.Join(dirB, "index.html"), []byte(htmlB), 0o644)

	rep, err := CompareDirs(ModeNormalized, dirA, dirB, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Pass() {
		t.Errorf("expected pass, got %v", rep)
	}
}

func TestCompareDirs_Byte_DetectsRawDiff(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "index.html"), []byte("<p>a</p>"), 0o644)
	os.WriteFile(filepath.Join(dirB, "index.html"), []byte("<p>b</p>"), 0o644)

	rep, _ := CompareDirs(ModeByte, dirA, dirB, nil)
	if len(rep.Differing) != 1 {
		t.Errorf("expected 1 differing, got %v", rep)
	}
	if !rep.Pass() {
		t.Errorf("byte mode must always pass (radar)")
	}
}

func TestCompareDirs_Normalized_FailsOnDifferentContent(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "index.html"), []byte("<p>alpha</p>"), 0o644)
	os.WriteFile(filepath.Join(dirB, "index.html"), []byte("<p>beta</p>"), 0o644)

	rep, _ := CompareDirs(ModeNormalized, dirA, dirB, nil)
	if rep.Pass() {
		t.Errorf("expected FAIL for different content, got %+v", rep)
	}
	if len(rep.Differing) != 1 {
		t.Errorf("expected 1 differing, got %v", rep)
	}
}

func TestCompareDirs_DetectsMissingAndExtra(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "a.html"), []byte("<p>a</p>"), 0o644)
	os.WriteFile(filepath.Join(dirA, "b.html"), []byte("<p>b</p>"), 0o644)
	os.WriteFile(filepath.Join(dirB, "a.html"), []byte("<p>a</p>"), 0o644)
	os.WriteFile(filepath.Join(dirB, "c.html"), []byte("<p>c</p>"), 0o644)

	rep, _ := CompareDirs(ModeNormalized, dirA, dirB, nil)
	// b.html is extra in A; c.html is missing from A.
	if len(rep.ExtraInA) != 1 || rep.ExtraInA[0] != "b.html" {
		t.Errorf("ExtraInA: got %v", rep.ExtraInA)
	}
	if len(rep.MissingInA) != 1 || rep.MissingInA[0] != "c.html" {
		t.Errorf("MissingInA: got %v", rep.MissingInA)
	}
	if rep.Pass() {
		t.Errorf("expected FAIL due to missing/extra")
	}
}

func TestCompareDirs_WhitelistedDiffsPass(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "page.html"), []byte("<p>alpha</p>"), 0o644)
	os.WriteFile(filepath.Join(dirB, "page.html"), []byte("<p>beta</p>"), 0o644)

	// Without allowlist: should fail
	rep, _ := CompareDirs(ModeNormalized, dirA, dirB, nil)
	if rep.Pass() {
		t.Error("expected FAIL without allowlist")
	}

	// With allowlist: should pass
	allowlist := map[string]bool{"page.html": true}
	rep, _ = CompareDirs(ModeNormalized, dirA, dirB, allowlist)
	if !rep.Pass() {
		t.Errorf("expected PASS with allowlist, got %+v", rep)
	}
	if len(rep.Whitelisted) != 1 || rep.Whitelisted[0] != "page.html" {
		t.Errorf("expected 1 whitelisted, got %v", rep.Whitelisted)
	}
	if len(rep.Differing) != 0 {
		t.Errorf("expected 0 differing, got %v", rep.Differing)
	}
}
