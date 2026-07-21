// Package langdetect provides language-mix detection helpers.
// This is a self-contained copy of internal/i18n/langdetect/langdetect.go for plugin isolation.
package langdetect

import (
	"strings"
	"unicode"
)

// CountLatinWords returns the number of whitespace-separated tokens in s.
func CountLatinWords(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// CountCJKRunes returns the number of CJK Unified Ideograph (Han) runes in s.
func CountCJKRunes(s string) int {
	count := 0
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			count++
		}
	}
	return count
}

// CJKFraction returns the fraction of CJK (Han) runes over total letter runes.
func CJKFraction(s string) float64 {
	total := 0
	cjk := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			total++
			if unicode.In(r, unicode.Han) {
				cjk++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(cjk) / float64(total)
}

// CJKRunesOutsideCode counts CJK Han runes in s, EXCLUDING any that appear
// inside fenced code blocks or inline code spans.
func CJKRunesOutsideCode(s string) int {
	count := 0
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		count += CountCJKRunes(StripInlineCode(line))
	}
	return count
}

// StripInlineCode removes inline code spans from a single line.
func StripInlineCode(line string) string {
	var b strings.Builder
	inCode := false
	for _, r := range line {
		if r == '`' {
			inCode = !inCode
			continue
		}
		if !inCode {
			b.WriteRune(r)
		}
	}
	return b.String()
}