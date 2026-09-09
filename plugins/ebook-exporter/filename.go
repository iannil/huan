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
// the ZH title for zh, the EN title for en (aggregates carry synthesized
// titles from expandUnits). A missing EN title falls back to the ZH side —
// the plugin-wide missing-EN policy — and then a "-en" suffix is appended so
// the two languages' files stay distinct. An empty title falls back to the
// slug baseName so export never fails on naming.
func fileStem(u *unit, lang content.Lang) string {
	title := u.agg.TitleZH
	if lang == content.LangEN {
		title = u.agg.TitleEN
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
