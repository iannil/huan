package render

import (
	"strings"
)

// normalizeInline applies the shared inline-text normalization for export
// pipelines: CJK typography first (curly quotes / fullwidth parens), then
// script-neutral punctuation (ellipsis / spaced hyphen). Code spans, link
// URLs, and EPUB placeholder tokens are protected verbatim.
func normalizeInline(s string) string {
	s = TypographCJK(s)
	s = TypographPunct(s)
	return s
}

// inlinePlain strips inline markdown syntax from a source-text snippet,
// producing plain text. This is the shared plain-text strategy used by the
// PDF and DOCX backends: emphasis/bold/code markers are removed and links
// collapse to their visible text. CJK punctuation normalization is applied
// first so metadata titles (which skip ParseChapter) normalize too; it is
// idempotent and self-guards on CJK presence.
func inlinePlain(s string) string {
	s = normalizeInline(s)
	s = linkRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}
