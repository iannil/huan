// Package style provides styling assets and font resolution for PDF/EPUB export.
package style

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultFontDirs lists the candidate font directories scanned (recursively)
// when no custom fonts_dir is configured.
func defaultFontDirs() []string {
	home, err := os.UserHomeDir()
	var userFonts string
	if err == nil {
		userFonts = filepath.Join(home, "Library", "Fonts")
	}
	dirs := []string{userFonts, "/System/Library/Fonts", "/Library/Fonts", "/usr/share/fonts"}
	var out []string
	for _, d := range dirs {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// PreferTTF sorts candidates so that .ttf files come before .otf files
// (gpdf works best with TTF); within the same extension the original
// order is preserved.
func PreferTTF(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		ti := strings.EqualFold(filepath.Ext(paths[i]), ".ttf")
		tj := strings.EqualFold(filepath.Ext(paths[j]), ".ttf")
		if ti != tj {
			return ti
		}
		// Keep stable otherwise; also prefer any non-.otf (e.g. .ttc) over .otf.
		ci := strings.EqualFold(filepath.Ext(paths[i]), ".otf")
		cj := strings.EqualFold(filepath.Ext(paths[j]), ".otf")
		return !ci && cj
	})
}

// fontCandidates collects font files under dirs whose filename (lowercased)
// contains ALL of the given substrings, keeping extension .ttf/.ttc/.otf.
func fontCandidates(dirs []string, substrings ...string) []string {
	var out []string
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".ttc" && ext != ".otf" {
				return nil
			}
			lower := strings.ToLower(filepath.Base(path))
			for _, s := range substrings {
				if !strings.Contains(lower, s) {
					return nil
				}
			}
			out = append(out, path)
			return nil
		})
	}
	return out
}

// FindCJKFont locates a suitable CJK font for PDF rendering and EPUB embedding.
// If fontsDir is non-empty, only that directory is searched; otherwise the
// standard macOS/Linux font directories are scanned recursively.
//
// Match priority: Noto Sans CJK SC (preferring -Regular), then any CJK font,
// then PingFang / Source Han Sans. Among equally ranked candidates, .ttf is
// preferred over .otf (see PreferTTF).
func FindCJKFont(fontsDir string) (string, error) {
	dirs := defaultFontDirs()
	if fontsDir != "" {
		dirs = []string{fontsDir}
	}

	try := func(substrings ...string) string {
		cands := fontCandidates(dirs, substrings...)
		if len(cands) == 0 {
			return ""
		}
		// Prefer "-Regular" weights.
		sort.SliceStable(cands, func(i, j int) bool {
			ri := strings.Contains(strings.ToLower(filepath.Base(cands[i])), "-regular")
			rj := strings.Contains(strings.ToLower(filepath.Base(cands[j])), "-regular")
			if ri != rj {
				return ri
			}
			return false
		})
		PreferTTF(cands)
		return cands[0]
	}

	// 1. Noto Sans CJK SC.
	if p := try("notosanscjk", "sc"); p != "" {
		return p, nil
	}
	// 2. Any CJK font.
	if p := try("cjk"); p != "" {
		return p, nil
	}
	// 3. PingFang / Source Han Sans fallbacks.
	if p := try("pingfang"); p != "" {
		return p, nil
	}
	if p := try("sourcehansans"); p != "" {
		return p, nil
	}
	if p := try("sourcehansc"); p != "" {
		return p, nil
	}

	return "", fmt.Errorf("no CJK font found (looked in %v); set plugins.ebook_exporter.fonts_dir or install Noto Sans CJK SC", dirs)
}

// ReadFontData reads the raw bytes of a font file.
func ReadFontData(path string) ([]byte, error) {
	return os.ReadFile(path)
}
