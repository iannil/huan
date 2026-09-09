// Package style internal: publication font resolution chains. Each chain:
// explicit config path → known macOS system sources (TTC extraction) →
// system font scan. A non-empty return note means auto-derivation happened
// and should be surfaced by the caller.
package style

import (
	"fmt"
	"os"
)

// FontRef names one candidate font file; Index selects a font inside a TTC
// and is ignored for standalone files.
type FontRef struct{ Path string; Index int }

// publicationCJKSources are the approved publication-design sources
// (STHeiti, macOS system). Package-level for test overrides.
var publicationCJKSources = []FontRef{
	{Path: "/System/Library/Fonts/STHeiti Light.ttc", Index: 1},
	{Path: "/System/Library/Fonts/STHeiti Medium.ttc", Index: 1},
}

// readableTrueType reports whether the font named by ref exists with
// TrueType outlines. TTCs are extracted into cacheDir first (the extracted
// file is a standalone font, so the outline check is always index 0).
func readableTrueType(ref FontRef, cacheDir string) (string, bool) {
	path, err := ExtractTTC(ref.Path, ref.Index, cacheDir)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	if !HasTrueTypeOutlines(data) {
		return "", false
	}
	return path, true
}

// FindPublicationCJKFont resolves the PDF/cover CJK slot:
// cfgPath → publicationCJKSources → system scan (TrueType only).
func FindPublicationCJKFont(cfgPath, fontsDir, cacheDir string) (string, string, error) {
	if path, ok := readableTrueType(FontRef{Path: cfgPath, Index: 0}, cacheDir); cfgPath != "" && ok {
		return path, "", nil
	}
	for _, src := range publicationCJKSources {
		if path, ok := readableTrueType(src, cacheDir); ok {
			return path, "derived from " + src.Path, nil
		}
	}
	// System scan: prefer .ttf/.ttc over .otf (CFF cannot serve PDF).
	dirs := defaultFontDirs()
	if fontsDir != "" {
		dirs = []string{fontsDir}
	}
	for _, pattern := range [][]string{
		{"pingfang"}, {"hiragino"}, {"notosanscjk", "sc"},
		{"sourcehansans"}, {"sourcehansc"}, {"cjk"},
	} {
		cands := fontCandidates(dirs, pattern...)
		PreferTTF(cands)
		for _, c := range cands {
			if path, ok := readableTrueType(FontRef{Path: c}, cacheDir); ok {
				return path, "system fallback: " + c, nil
			}
		}
	}
	return "", "", fmt.Errorf("no TrueType CJK font found for pdf/cover (tried configured %q, %v, scan of %v); install Noto Sans CJK SC or set pdf_font",
		cfgPath, publicationCJKSources, dirs)
}

// FindPublicationLatinFont resolves the cover Latin slot:
// cfgPath → Times New Roman → Georgia → graceful "" (CJK font renders Latin).
func FindPublicationLatinFont(cfgPath string) string {
	if cfgPath != "" {
		if _, err := os.Stat(cfgPath); err == nil {
			return cfgPath
		}
	}
	for _, pattern := range [][]string{
		{"timesnewroman"}, {"georgia"}, {"didot"}, {"charter"},
	} {
		if cands := fontCandidates(defaultFontDirs(), pattern...); len(cands) > 0 {
			return cands[0]
		}
	}
	return ""
}
