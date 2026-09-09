package main

import (
	"strings"
	"unicode"
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
