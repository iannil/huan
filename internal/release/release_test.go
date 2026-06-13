package release

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/iannil/huan/internal/observability"
)

// newSilentLogger returns a logger writing to a discard buffer so tests
// don't pollute stdout. Buffer is returned for assertions if needed.
func newSilentLogger() *observability.Logger {
	return observability.NewLoggerWithWriter("test", &bytes.Buffer{})
}

// makeSourceDir creates a minimal huan source tree with LICENSE and a fake
// binary at cmd/huan (MockBuilder doesn't actually need a buildable cmd/huan
// since it stubs the compile step).
func makeSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "huan"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRelease_HappyPath_SingleTarget(t *testing.T) {
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{{OS: "darwin", Arch: "arm64"}},
		SourceDir: sourceDir,
	}
	builder := &MockBuilder{}

	report, err := Release(context.Background(), opts, builder, newSilentLogger())
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("Failures = %+v, want empty", report.Failures)
	}
	if len(report.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(report.Artifacts))
	}
	art := report.Artifacts[0]
	if art.Name != "huan_0.1.0_darwin_arm64.tar.gz" {
		t.Errorf("Name = %q", art.Name)
	}
	if art.OS != "darwin" || art.Arch != "arm64" {
		t.Errorf("OS/Arch = %s/%s", art.OS, art.Arch)
	}
	if art.Binary != "huan" {
		t.Errorf("Binary = %q, want huan", art.Binary)
	}
	if art.SHA256 == "" {
		t.Error("SHA256 empty")
	}

	// File should exist in outDir.
	if _, err := os.Stat(filepath.Join(outDir, art.Name)); err != nil {
		t.Errorf("archive missing: %v", err)
	}
	// Checksums + manifest should exist.
	if _, err := os.Stat(filepath.Join(outDir, ChecksumsFilename("0.1.0"))); err != nil {
		t.Errorf("checksums missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ManifestFilename("0.1.0"))); err != nil {
		t.Errorf("manifest missing: %v", err)
	}
}

func TestRelease_PartialFailure_ContinuesAndAggregates(t *testing.T) {
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	// Two targets: first fails compile, second succeeds. Per ADR 0004 §15
	// A1, release must continue with the second and aggregate failures.
	failingTarget := Target{OS: "linux", Arch: "amd64"}
	goodTarget := Target{OS: "darwin", Arch: "arm64"}
	builder := &MockBuilder{
		FailTargets: map[Target]error{
			failingTarget: errFake("compile failed for linux/amd64"),
		},
	}
	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{failingTarget, goodTarget},
		SourceDir: sourceDir,
	}
	report, err := Release(context.Background(), opts, builder, newSilentLogger())
	if err != nil {
		t.Fatalf("Release returned error on partial failure: %v", err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("Failures len = %d, want 1", len(report.Failures))
	}
	if report.Failures[0].Target != failingTarget {
		t.Errorf("failure target = %s, want %s", report.Failures[0].Target, failingTarget)
	}
	if report.Failures[0].Phase != "compile" {
		t.Errorf("failure phase = %q, want compile", report.Failures[0].Phase)
	}
	if len(report.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1 (good target should still succeed)", len(report.Artifacts))
	}
	if report.Artifacts[0].OS != "darwin" {
		t.Errorf("artifact OS = %q, want darwin", report.Artifacts[0].OS)
	}
	// Builder.BuildLog should record both targets (attempted in order).
	if len(builder.BuildLog) != 2 {
		t.Errorf("BuildLog len = %d, want 2", len(builder.BuildLog))
	}
}

func TestRelease_DryRun_DoesNotTouchOutDir(t *testing.T) {
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{{OS: "darwin", Arch: "arm64"}},
		SourceDir: sourceDir,
		DryRun:    true,
	}
	report, err := Release(context.Background(), opts, &MockBuilder{}, newSilentLogger())
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if !report.DryRun {
		t.Error("report.DryRun = false, want true")
	}
	// OutDir should be empty (release wrote to temp staging, then temp got cleaned up).
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("outDir has %d entries after dry-run, want 0: %v", len(entries), entries)
	}
	// But artifacts should still be described in the report.
	if len(report.Artifacts) != 1 {
		t.Errorf("Artifacts len = %d, want 1 (dry-run still computes manifest)", len(report.Artifacts))
	}
}

func TestRelease_MissingLICENSE_FailsFast(t *testing.T) {
	sourceDir := t.TempDir()
	// No LICENSE written.
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{{OS: "darwin", Arch: "arm64"}},
		SourceDir: sourceDir,
	}
	_, err := Release(context.Background(), opts, &MockBuilder{}, newSilentLogger())
	if err == nil {
		t.Fatal("expected error for missing LICENSE")
	}
}

func TestRelease_BadVersion_FailsFast(t *testing.T) {
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "v0.1.0", // leading v rejected
		OutDir:    outDir,
		Targets:   []Target{{OS: "darwin", Arch: "arm64"}},
		SourceDir: sourceDir,
	}
	_, err := Release(context.Background(), opts, &MockBuilder{}, newSilentLogger())
	if err == nil {
		t.Fatal("expected error for v-prefixed version")
	}
}

func TestRelease_EmptyTargets_FailsFast(t *testing.T) {
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   nil,
		SourceDir: sourceDir,
	}
	_, err := Release(context.Background(), opts, &MockBuilder{}, newSilentLogger())
	if err == nil {
		t.Fatal("expected error for empty targets")
	}
}

func TestRelease_OverwriteExpectedFiles_LeavesExtras(t *testing.T) {
	// Per ADR 0004 §15 B1: re-running release overwrites expected archive
	// files but leaves operator-added extras (e.g. RELEASE_NOTES.md) alone.
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	// Pre-populate outDir with an extra file (simulating operator's manual
	// RELEASE_NOTES.md from a prior release).
	extras := filepath.Join(outDir, "RELEASE_NOTES.md")
	if err := os.WriteFile(extras, []byte("notes from operator"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{{OS: "darwin", Arch: "arm64"}},
		SourceDir: sourceDir,
	}
	if _, err := Release(context.Background(), opts, &MockBuilder{}, newSilentLogger()); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Extra file must still exist.
	if _, err := os.Stat(extras); err != nil {
		t.Errorf("operator extra file removed by release: %v", err)
	}
}

func TestRelease_WindowsTarget_ProducesZip(t *testing.T) {
	sourceDir := makeSourceDir(t)
	outDir := t.TempDir()

	opts := Options{
		Version:   "0.1.0",
		OutDir:    outDir,
		Targets:   []Target{{OS: "windows", Arch: "amd64"}},
		SourceDir: sourceDir,
	}
	report, err := Release(context.Background(), opts, &MockBuilder{}, newSilentLogger())
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if len(report.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(report.Artifacts))
	}
	if report.Artifacts[0].Name != "huan_0.1.0_windows_amd64.zip" {
		t.Errorf("Name = %q, want zip", report.Artifacts[0].Name)
	}
	if report.Artifacts[0].Binary != "huan.exe" {
		t.Errorf("Binary = %q, want huan.exe", report.Artifacts[0].Binary)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

// TestGitInfo_RealRepo verifies gitInfo returns sensible SHA + dirty flag
// when sourceDir is a real git repo. This test assumes the test process
// runs inside the huan repo (which it always does — it's part of internal/
// release/).
func TestGitInfo_RealRepo(t *testing.T) {
	// Walk up from this test file to find the huan project root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	// Skip if not actually a git repo (e.g. testing a tarball extraction).
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		t.Skip("not a git repo")
	}

	sha, dirty, ok := gitInfo(abs)
	if !ok {
		t.Fatal("gitInfo returned ok=false inside huan git repo")
	}
	if len(sha) < 7 {
		t.Errorf("SHA = %q, want at least 7 chars", sha)
	}
	// dirty flag must be a valid bool; can be true if there are uncommitted
	// test fixtures, but the call itself must succeed.
	_ = dirty
}

func TestGitInfo_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	sha, dirty, ok := gitInfo(dir)
	if ok {
		t.Errorf("expected ok=false in non-git dir, got sha=%q dirty=%v", sha, dirty)
	}
}
