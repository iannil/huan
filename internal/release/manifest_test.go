package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteManifest_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	report := &Report{
		TraceID:   "trace-abc",
		Version:   "0.1.0",
		GoVersion: "go1.26.2",
		GitSHA:    "87b2836",
		OutDir:    "/release/0.1.0",
		Targets:   []string{"darwin/arm64", "linux/amd64"},
		Artifacts: []Artifact{
			{Name: "huan_0.1.0_darwin_arm64.tar.gz", SHA256: "abc", Size: 1234, Binary: "huan", OS: "darwin", Arch: "arm64"},
		},
		DurationMs: 5000,
	}
	path, err := WriteManifest(dir, "0.1.0", report)
	if err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	parsed, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if parsed.TraceID != report.TraceID {
		t.Errorf("TraceID = %q, want %q", parsed.TraceID, report.TraceID)
	}
	if parsed.Version != report.Version {
		t.Errorf("Version = %q, want %q", parsed.Version, report.Version)
	}
	if len(parsed.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(parsed.Artifacts))
	}
	if parsed.Artifacts[0].Name != report.Artifacts[0].Name {
		t.Errorf("Artifacts[0].Name = %q", parsed.Artifacts[0].Name)
	}
}

func TestWriteManifest_Deterministic(t *testing.T) {
	dir := t.TempDir()
	report := &Report{
		TraceID:   "trace-fixed",
		Version:   "0.1.0",
		GoVersion: "go1.26.2",
		OutDir:    "/release/0.1.0",
		Targets:   []string{"darwin/arm64"},
		Artifacts: []Artifact{
			{Name: "huan_0.1.0_darwin_arm64.tar.gz", SHA256: "fixed-hash", Size: 100, Binary: "huan", OS: "darwin", Arch: "arm64"},
		},
	}
	path1, _ := WriteManifest(dir, "0.1.0", report)
	data1, _ := os.ReadFile(path1)

	// Re-write to a different name to compare bytes.
	report2 := *report
	path2, _ := WriteManifest(dir, "0.2.0", &report2)
	data2, _ := os.ReadFile(path2)

	// Strip the version line so we can compare structure determinism. The
	// only difference should be the version field.
	var p1, p2 map[string]any
	_ = json.Unmarshal(data1, &p1)
	_ = json.Unmarshal(data2, &p2)
	p1["version"] = "X"
	p2["version"] = "X"
	p1["out_dir"] = "X"
	p2["out_dir"] = "X"
	if !mapEqual(p1, p2) {
		t.Errorf("manifest not deterministic across versions")
	}
}

func mapEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		// shallow comparison is enough for this test
		jsA, _ := json.Marshal(v)
		jsB, _ := json.Marshal(bv)
		if string(jsA) != string(jsB) {
			return false
		}
	}
	return true
}

func TestParseManifest_Missing(t *testing.T) {
	_, err := ParseManifest(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}
