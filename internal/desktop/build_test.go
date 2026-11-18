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
	if err := BuildSiteTo(src, out, t.Logf, nil, nil); err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	for _, rel := range []string{"index.html", filepath.Join("posts", "alpha", "index.html")} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("output missing %s: %v", rel, err)
		}
	}
}

func TestBuildSiteTo_InvalidSiteFails(t *testing.T) {
	if err := BuildSiteTo(t.TempDir(), filepath.Join(t.TempDir(), "out"), t.Logf, nil, nil); err == nil {
		t.Fatal("BuildSiteTo on non-site dir should fail")
	}
}

func TestBuildSiteToReportsPluginWarnings(t *testing.T) {
	src := writeSiteWithBrokenPlugin(t)
	var got []plugin.LoadWarning
	err := BuildSiteTo(src, t.TempDir(), func(string, ...any) {}, func(ws []plugin.LoadWarning) { got = ws }, nil)
	if err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	if len(got) != 1 || got[0].Name != "broken.so" {
		t.Fatalf("want exactly broken.so warning, got %+v", got)
	}
}

func TestBuildSiteToReportsConfigWarnings(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan
	src := writeSiteWithMissingPlugin(t)
	var got []string
	// nil onPluginWarnings + live onConfigWarnings: each channel is independent.
	err := BuildSiteTo(src, t.TempDir(), func(string, ...any) {}, nil, func(ws []string) { got = ws })
	if err != nil {
		t.Fatalf("BuildSiteTo: %v", err)
	}
	want := `plugin "ghost": declared in yaml but no .so found in $HUAN_HOME or plugins/`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("config warnings = %#v, want [%q]", got, want)
	}
}
