package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSiteTo_ProducesOutput(t *testing.T) {
	src := writeSite(t)
	out := filepath.Join(t.TempDir(), "public")
	if err := BuildSiteTo(src, out, t.Logf); err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	for _, rel := range []string{"index.html", filepath.Join("posts", "alpha", "index.html")} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("output missing %s: %v", rel, err)
		}
	}
}

func TestBuildSiteTo_InvalidSiteFails(t *testing.T) {
	if err := BuildSiteTo(t.TempDir(), filepath.Join(t.TempDir(), "out"), t.Logf); err == nil {
		t.Fatal("BuildSiteTo on non-site dir should fail")
	}
}
