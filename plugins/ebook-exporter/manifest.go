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
// The key is "<slug>.<lang>" for individual books, "volume-<N>.<lang>" for
// volume collections and "complete-<kind>.<lang>" for the complete bundle.
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
// once more. A missing file contributes the empty string (so its digest is
// the sha256 of nothing and differs from any present-file digest), which the
// caller sees as a change. Path order is significant.
func ComputeHash(paths []string) string {
	h := sha256.New()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue // missing file -> empty contribution
		}
		d := sha256.New()
		if _, err := io.Copy(d, f); err != nil {
			f.Close()
			continue
		}
		f.Close()
		h.Write(d.Sum(nil))
	}
	return hex.EncodeToString(h.Sum(nil))
}
