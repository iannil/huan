package render

import (
	"strings"
)

// inlinePlain strips inline markdown syntax from a source-text snippet,
// producing plain text. This is the shared plain-text strategy used by the
// PDF and DOCX backends: emphasis/bold/code markers are removed and links
// collapse to their visible text. CJK punctuation normalization is applied
// first so metadata titles (which skip ParseChapter) normalize too; it is
// idempotent and self-guards on CJK presence.
func inlinePlain(s string) string {
	s = TypographCJK(s)
	s = linkRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}
