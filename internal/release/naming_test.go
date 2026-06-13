package release

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinaryName(t *testing.T) {
	cases := []struct {
		target Target
		want   string
	}{
		{Target{OS: "darwin", Arch: "arm64"}, "huan"},
		{Target{OS: "linux", Arch: "amd64"}, "huan"},
		{Target{OS: "windows", Arch: "amd64"}, "huan.exe"},
	}
	for _, c := range cases {
		if got := BinaryName(c.target); got != c.want {
			t.Errorf("BinaryName(%s) = %q, want %q", c.target, got, c.want)
		}
	}
}

func TestArchiveName(t *testing.T) {
	cases := []struct {
		target  Target
		want    string
		suffix  string
	}{
		{Target{OS: "darwin", Arch: "arm64"}, "huan_0.1.0_darwin_arm64.tar.gz", ".tar.gz"},
		{Target{OS: "linux", Arch: "amd64"}, "huan_0.1.0_linux_amd64.tar.gz", ".tar.gz"},
		{Target{OS: "windows", Arch: "amd64"}, "huan_0.1.0_windows_amd64.zip", ".zip"},
	}
	for _, c := range cases {
		got := ArchiveName(c.target, "0.1.0")
		if got != c.want {
			t.Errorf("ArchiveName(%s, 0.1.0) = %q, want %q", c.target, got, c.want)
		}
		if !strings.HasSuffix(got, c.suffix) {
			t.Errorf("ArchiveName(%s, 0.1.0) = %q, want suffix %q", c.target, got, c.suffix)
		}
	}
}

func TestChecksumsAndManifestFilenames(t *testing.T) {
	if got := ChecksumsFilename("0.1.0"); got != "huan_0.1.0_checksums.txt" {
		t.Errorf("ChecksumsFilename = %q", got)
	}
	if got := ManifestFilename("0.1.0"); got != "huan_0.1.0_manifest.json" {
		t.Errorf("ManifestFilename = %q", got)
	}
}

func TestOutDir(t *testing.T) {
	got := OutDir("/tmp/release", "0.1.0")
	want := filepath.Join("/tmp/release", "0.1.0")
	if got != want {
		t.Errorf("OutDir = %q, want %q", got, want)
	}
}

func TestHostTarget(t *testing.T) {
	got := HostTarget()
	if got.OS != runtime.GOOS || got.Arch != runtime.GOARCH {
		t.Errorf("HostTarget = %s, want %s/%s", got, runtime.GOOS, runtime.GOARCH)
	}
}

func TestTargetString(t *testing.T) {
	tgt := Target{OS: "darwin", Arch: "arm64"}
	if got := tgt.String(); got != "darwin/arm64" {
		t.Errorf("Target.String() = %q, want darwin/arm64", got)
	}
}

func TestStandardTargets_Count(t *testing.T) {
	if len(StandardTargets) != 5 {
		t.Errorf("StandardTargets has %d entries, want 5", len(StandardTargets))
	}
	// Sanity: every entry must have non-empty OS and Arch.
	for _, tgt := range StandardTargets {
		if tgt.OS == "" || tgt.Arch == "" {
			t.Errorf("StandardTargets contains empty target: %+v", tgt)
		}
	}
}
