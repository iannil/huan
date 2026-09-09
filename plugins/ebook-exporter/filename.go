package main

import (
	"strings"
	"unicode"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

// sanitizeFilename makes s safe as a filename stem on all platforms:
// half-width colons become full-width (Chinese-style), Windows-illegal
// characters and control runes are dropped, whitespace runs collapse to a
// single space, and leading/trailing spaces and dots are trimmed. Whitespace
// immediately following a full-width colon is swallowed, since CJK typography
// never spaces after it. An empty return means the caller must fall back
// (see fileStem).
func sanitizeFilename(s string) string {
	var b strings.Builder
	suppressSpace := false
	for _, r := range s {
		switch {
		case r == ':' || r == '：':
			b.WriteRune('：')
			suppressSpace = true
		case suppressSpace && unicode.IsSpace(r):
			// dropped: whitespace right after a full-width colon
		case r < 0x20 || strings.ContainsRune(`\/*?"<>|`, r):
			// dropped
		default:
			suppressSpace = false
			b.WriteRune(r)
		}
	}
	return strings.Trim(strings.Join(strings.Fields(b.String()), " "), " .")
}

// fileStem returns the output filename stem for one unit in one language:
// the ZH title for zh; for en, the EN title, falling back to the ZH title
// when the EN one is empty (the plugin-wide missing-EN policy; aggregates
// carry synthesized titles from expandUnits). A title that sanitizes to
// empty falls back to the slug baseName so export never fails on naming.
// The "-en" suffix is appended whenever the final en stem equals the zh
// stem, so the two languages' files stay distinct.
func fileStem(u *unit, lang content.Lang) string {
	title := u.agg.TitleZH
	if lang == content.LangEN {
		title = u.agg.TitleEN
		if title == "" {
			title = u.agg.TitleZH
		}
	}
	stem := sanitizeFilename(title)
	if stem == "" {
		stem = u.baseName
	}
	if lang == content.LangEN && stem == fileStem(u, content.LangZH) {
		stem += "-en"
	}
	return stem
}
