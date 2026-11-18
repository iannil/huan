package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/plugin"
)

func TestBuildSiteTo_ProducesOutput(t *testing.T) {
	src := writeSite(t)
	out := filepath.Join(t.TempDir(), "public")
	if err := BuildSiteTo(src, out, t.Logf, nil); err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	for _, rel := range []string{"index.html", filepath.Join("posts", "alpha", "index.html")} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("output missing %s: %v", rel, err)
		}
	}
}

func TestBuildSiteTo_InvalidSiteFails(t *testing.T) {
	if err := BuildSiteTo(t.TempDir(), filepath.Join(t.TempDir(), "out"), t.Logf, nil); err == nil {
		t.Fatal("BuildSiteTo on non-site dir should fail")
	}
}

func TestBuildSiteToReportsPluginWarnings(t *testing.T) {
	src := writeSiteWithBrokenPlugin(t)
	var got []plugin.LoadWarning
	err := BuildSiteTo(src, t.TempDir(), func(string, ...any) {}, func(ws []plugin.LoadWarning) { got = ws })
	if err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	if len(got) != 1 || got[0].Name != "broken.so" {
		t.Fatalf("want exactly broken.so warning, got %+v", got)
	}
}
