package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Manifest tracks the content hash of each exported unit so repeated
// exports can skip units whose sources have not changed (incremental build).
// The key is "<kind>/<level>/<base>.<lang>" (e.g. "books/individual/rc.zh",
// "books/volumes/volume-1.zh", "practices/complete/complete.zh") — derived
// from the same components as the output path so cross-kind collisions are
// structurally impossible.
type Manifest struct {
	Entries map[string]string
}

// manifestFileName is the manifest file stored at the root of OutputDir.
const manifestFileName = ".ebook-manifest.json"

// LoadManifest reads the manifest from outDir. A missing or unreadable
// manifest yields an empty one — incremental builds simply regenerate.
func LoadManifest(outDir string) *Manifest {
	m := &Manifest{Entries: make(map[string]string)}
	raw, err := os.ReadFile(filepath.Join(outDir, manifestFileName))
	if err != nil {
		return m
	}
	var loaded map[string]string
	if json.Unmarshal(raw, &loaded) == nil && loaded != nil {
		m.Entries = loaded
	}
	return m
}

// SaveManifest writes the manifest as JSON into outDir, creating the
// directory if needed.
func SaveManifest(outDir string, m *Manifest) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.Entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, manifestFileName), raw, 0o644)
}

// ComputeHash aggregates the content of paths into one stable hash:
// per-file sha256 digests are concatenated and the concatenation is hashed
// once more. A missing file contributes the fixed marker "<missing>"
// instead of being skipped, so [missing, a] hashes differently from [a]
// and a disappeared file is always seen as a change. Path order is
// significant.
func ComputeHash(paths []string) string {
	h := sha256.New()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			h.Write([]byte("<missing>"))
			continue
		}
		d := sha256.New()
		if _, err := io.Copy(d, f); err != nil {
			f.Close()
			h.Write([]byte("<missing>"))
			continue
		}
		f.Close()
		h.Write(d.Sum(nil))
	}
	return hex.EncodeToString(h.Sum(nil))
}
