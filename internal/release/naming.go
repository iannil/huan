package release

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// BinaryName returns the binary filename for a target. Windows needs the
// .exe suffix (without it, Windows won't execute); other platforms use
// the bare name.
func BinaryName(t Target) string {
	if t.OS == "windows" {
		return "huan.exe"
	}
	return "huan"
}

// ArchiveName returns the release archive filename for a target.
// Unix targets use tar.gz; windows uses zip (Windows users expect zip and
// many older tar implementations on Windows mishandle permissions).
func ArchiveName(t Target, version string) string {
	if t.OS == "windows" {
		return fmt.Sprintf("huan_%s_%s_%s.zip", version, t.OS, t.Arch)
	}
	return fmt.Sprintf("huan_%s_%s_%s.tar.gz", version, t.OS, t.Arch)
}

// ChecksumsFilename returns the checksums file name (shasum-compatible).
func ChecksumsFilename(version string) string {
	return fmt.Sprintf("huan_%s_checksums.txt", version)
}

// ManifestFilename returns the JSON manifest file name.
func ManifestFilename(version string) string {
	return fmt.Sprintf("huan_%s_manifest.json", version)
}

// OutDir returns the absolute path to the per-version release directory.
// outRoot is typically <sourceDir>/release.
func OutDir(outRoot, version string) string {
	return filepath.Join(outRoot, version)
}

// HostTarget returns the current host's Target (useful for `--targets=current`
// and for the smoke-test path).
func HostTarget() Target {
	return Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
}
