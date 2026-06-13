package release

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// SHA256File returns the hex-encoded sha256 of the file at path. Reads in
// 32KB chunks to handle large files without buffering entire content.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ChecksumsLine renders one shasum-compatible line: "<sha256>  <filename>\n".
// The two-space separator matches `shasum -a 256` output so users can verify
// with `shasum -a 256 -c file.txt`.
func ChecksumsLine(filename, sha256 string) string {
	return fmt.Sprintf("%s  %s\n", sha256, filename)
}

// WriteChecksumsFile writes a shasum-compatible file containing one line per
// artifact. Lines are sorted by filename for deterministic output. The file
// is written atomically (write to temp, then rename) to avoid leaving a
// partially-written checksums file if the process is interrupted.
//
// outDir must exist; the file is named via ChecksumsFilename(version).
func WriteChecksumsFile(outDir, version string, artifacts []Artifact) (string, error) {
	// Sort by name for deterministic output.
	sorted := make([]Artifact, len(artifacts))
	copy(sorted, artifacts)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var buf []byte
	for _, a := range sorted {
		buf = append(buf, []byte(ChecksumsLine(a.Name, a.SHA256))...)
	}

	finalPath := filepath.Join(outDir, ChecksumsFilename(version))
	if err := atomicWrite(finalPath, buf, 0o644); err != nil {
		return "", fmt.Errorf("write checksums: %w", err)
	}
	return finalPath, nil
}

// atomicWrite writes data to path via a sibling temp file + rename. This
// ensures readers either see the old file or the complete new file, never a
// partial write. The temp file is created in the same directory as path so
// the rename is guaranteed atomic on POSIX (same filesystem).
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if anything below fails.
	defer func() {
		if tmp != nil {
			_ = tmp.Close()
		}
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	tmp = nil // marker so the deferred cleanup skips Close
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
