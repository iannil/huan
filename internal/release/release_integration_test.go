//go:build integration

package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/observability"
)

// projectRoot returns the huan repo root by walking up from this test file.
// Robust regardless of where `go test` was invoked from.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../internal/release/release_integration_test.go
	// project root is two dirs up.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	// Sanity check: project root must contain LICENSE + cmd/huan.
	for _, must := range []string{"LICENSE", "cmd/huan", "README.md"} {
		if _, err := os.Stat(filepath.Join(abs, must)); err != nil {
			t.Skipf("project root sanity check failed (%s missing): %v — skipping integration test", must, err)
		}
	}
	return abs
}

// TestRelease_Integration_HostPlatform runs the full release pipeline against
// the real huan source tree for one target (host platform) using the real
// GoBuildBuilder. Verifies:
//   - The archive file exists at the expected path.
//   - The archive contains binary + LICENSE + READMEs at the root (flat).
//   - The checksums.txt file is shasum-compatible and matches the archive.
//   - The manifest.json file is valid JSON and matches the report.
func TestRelease_Integration_HostPlatform(t *testing.T) {
	root := projectRoot(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{HostTarget()},
		SourceDir: root,
	}
	builder := &GoBuildBuilder{
		SourceDir: root,
		Logger:    observability.NewLoggerWithWriter("integration", io.Discard),
	}

	report, err := Release(context.Background(), opts, builder, newSilentLogger())
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %+v", report.Failures)
	}
	if len(report.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(report.Artifacts))
	}
	art := report.Artifacts[0]

	// 1. Archive file exists at expected path.
	archivePath := filepath.Join(outDir, art.Name)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}

	// 2. Archive contains binary + LICENSE + READMEs at the root.
	names := listArchiveNames(t, archivePath, HostTarget())
	for _, want := range []string{BinaryName(HostTarget()), "LICENSE", "README.md"} {
		if !sliceContains(names, want) {
			t.Errorf("archive missing %q; got %v", want, names)
		}
	}
	// zh-CN README is expected too (we maintain both).
	if !sliceContains(names, "README.zh-CN.md") {
		t.Errorf("archive missing README.zh-CN.md; got %v", names)
	}

	// 3. Checksums file exists and matches the archive (shasum -c compatible).
	checksumsPath := filepath.Join(outDir, ChecksumsFilename("0.1.0"))
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	wantLine := ChecksumsLine(art.Name, art.SHA256)
	if !strings.Contains(string(data), wantLine) {
		t.Errorf("checksums file missing line %q; got:\n%s", wantLine, string(data))
	}

	// 4. Manifest is valid JSON and matches the report.
	manifestPath := filepath.Join(outDir, ManifestFilename("0.1.0"))
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var parsed Report
	if err := json.Unmarshal(manifestBytes, &parsed); err != nil {
		t.Fatalf("manifest not valid JSON: %v", err)
	}
	if parsed.TraceID != report.TraceID {
		t.Errorf("manifest TraceID = %q, want %q", parsed.TraceID, report.TraceID)
	}
	if len(parsed.Artifacts) != 1 || parsed.Artifacts[0].SHA256 != art.SHA256 {
		t.Errorf("manifest artifacts mismatch")
	}
}

// TestRelease_Integration_AllFiveStandardTargets verifies the default
// platform matrix compiles end-to-end. This is slow (~30s) so it's gated
// behind the integration tag.
func TestRelease_Integration_AllFiveStandardTargets(t *testing.T) {
	root := projectRoot(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   StandardTargets,
		SourceDir: root,
	}
	builder := &GoBuildBuilder{
		SourceDir: root,
		Logger:    observability.NewLoggerWithWriter("integration5", io.Discard),
	}
	report, err := Release(context.Background(), opts, builder, newSilentLogger())
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("all 5 targets should succeed; failures: %+v", report.Failures)
	}
	if len(report.Artifacts) != 5 {
		t.Fatalf("got %d artifacts, want 5", len(report.Artifacts))
	}
	// Verify each expected archive name is present.
	got := make(map[string]bool)
	for _, a := range report.Artifacts {
		got[a.Name] = true
	}
	for _, tgt := range StandardTargets {
		want := ArchiveName(tgt, "0.1.0")
		if !got[want] {
			t.Errorf("missing archive %q", want)
		}
	}
}

// listArchiveNames opens the archive (tar.gz for unix, zip for windows) and
// returns the file names at the root.
func listArchiveNames(t *testing.T, path string, target Target) []string {
	t.Helper()
	if target.OS == "windows" {
		return listZipNames(t, path)
	}
	return listTarGZIntegration(t, path)
}

func listTarGZIntegration(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	var out []string
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		out = append(out, h.Name)
	}
	sort.Strings(out)
	return out
}

func listZipNames(t *testing.T, path string) []string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer zr.Close()
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func sliceContains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
