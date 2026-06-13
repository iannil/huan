package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteManifest serializes report as JSON to <outDir>/<ManifestFilename>.
// Output is deterministic: field order is fixed (struct declaration order),
// and json.Marshal with sorted map keys (none here, but defensive) is stable.
//
// Atomic write (temp + rename) so partial writes never appear on disk.
func WriteManifest(outDir, version string, report *Report) (string, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	// Trailing newline for POSIX-friendliness and git diff cleanliness.
	data = append(data, '\n')

	finalPath := filepath.Join(outDir, ManifestFilename(version))
	if err := atomicWrite(finalPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}
	return finalPath, nil
}

// ParseManifest reads + unmarshals a manifest JSON file. Used by future
// tooling (e.g. `huan upgrade`) to inspect what a release contains.
// Currently exercised by golden tests; left exported for symmetry.
func ParseManifest(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return &r, nil
}
