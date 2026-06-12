package build

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// TruncateHTMLByWords truncates HTML content to the first N words and closes
// any open tags. Word counting follows the same rules as CountWordsInPlain
// (strings.Fields + rune count for CJK fields).
//
// Truncation happens at word boundaries: the moment we finish counting the
// Nth word, we cut immediately (without consuming the trailing separator) and
// close open tags. This matches Hugo's summary behavior.
func TruncateHTMLByWords(htmlStr string, n int) string {
	if n <= 0 {
		return htmlStr
	}
	var buf bytes.Buffer
	var openTags []string
	count := 0
	inWord := false
	runes := []rune(htmlStr)
	i := 0
	for i < len(runes) {
		r := runes[i]

		// Handle HTML tags: a tag always ends the current word.
		if r == '<' {
			if count >= n && inWord {
				return finalizeTruncated(buf.String(), openTags)
			}
			inWord = false
			end := indexRuneFrom(runes, '>', i)
			if end < 0 {
				buf.WriteString(string(runes[i:]))
				return finalizeTruncated(buf.String(), openTags)
			}
			tagStr := string(runes[i+1 : end])
			buf.WriteRune('<')
			buf.WriteString(tagStr)
			buf.WriteRune('>')
			if len(tagStr) > 0 && tagStr[0] == '/' {
				if len(openTags) > 0 {
					openTags = openTags[:len(openTags)-1]
				}
			} else if !isVoidTagName(tagStr) {
				name := tagName(tagStr)
				if name != "" {
					openTags = append(openTags, name)
				}
			}
			i = end + 1
			continue
		}

		// Word boundary detection — must match CountWordsInPlain semantics
		// (Hugo's CJK branch: every rune in a CJK field counts as a word).
		// Any non-ASCII rune is treated as part of a CJK field; this aligns
		// with CountWordsInPlain's `len == RuneCount` branch (ASCII = 1 word,
		// otherwise each rune counts).
		isCJKFieldRune := r >= 0x4E00
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '　'

		if isCJKFieldRune {
			// Each rune in a CJK field is its own "word" per Hugo algorithm.
			if count >= n && inWord {
				return finalizeTruncated(buf.String(), openTags)
			}
			count++
			buf.WriteRune(r)
			inWord = false
			i++
			if count >= n {
				return finalizeTruncated(buf.String(), openTags)
			}
			continue
		}
		if isSpace {
			if count >= n && inWord {
				return finalizeTruncated(buf.String(), openTags)
			}
			inWord = false
			buf.WriteRune(r)
			i++
			continue
		}
		// ASCII non-space: part of an ASCII word
		if !inWord {
			if count >= n {
				return finalizeTruncated(buf.String(), openTags)
			}
			count++
			inWord = true
		}
		buf.WriteRune(r)
		i++
	}
	return htmlStr
}

// finalizeTruncated closes any open tags after a truncation point.
func finalizeTruncated(s string, openTags []string) string {
	var b strings.Builder
	b.WriteString(s)
	for j := len(openTags) - 1; j >= 0; j-- {
		b.WriteString("</")
		b.WriteString(openTags[j])
		b.WriteString(">")
	}
	return b.String()
}

func indexRuneFrom(runes []rune, target rune, from int) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

func isVoidTagName(tagStr string) bool {
	s := strings.TrimSpace(tagStr)
	s = strings.TrimPrefix(s, "/")
	if idx := strings.IndexAny(s, " /"); idx > 0 {
		s = s[:idx]
	}
	switch strings.ToLower(s) {
	case "br", "hr", "img", "input", "meta", "link", "area", "base",
		"col", "embed", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func tagName(tagStr string) string {
	s := strings.TrimSpace(tagStr)
	s = strings.TrimPrefix(s, "/")
	if idx := strings.IndexAny(s, " /"); idx > 0 {
		return strings.ToLower(s[:idx])
	}
	return strings.ToLower(s)
}

// StripHTMLTagsForSummary strips HTML tags for plain text summary.
func StripHTMLTagsForSummary(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// CountWordsInPlain counts words in plain text using Hugo's actual algorithm
// for sites with hasCJKLanguage=true (zhurongshuo sets this).
//
// Source: hugolib/page__content.go (Hugo master):
//
//	result.plain = StripHTML(rendered.content)
//	result.plainWords = strings.Fields(result.plain)
//	if isCJKLanguage {
//	    for _, word := range result.plainWords {
//	        runeCount := utf8.RuneCountInString(word)
//	        if len(word) == runeCount {  // all-ASCII
//	            result.wordCount++
//	        } else {                     // contains multi-byte (CJK etc.)
//	            result.wordCount += runeCount
//	        }
//	    }
//	}
//
// Effectively: split by whitespace (unicode.IsSpace, which includes the
// ideographic space U+3000); each whitespace-separated field is 1 word if
// pure ASCII, otherwise contributes its rune count. Punctuation inside a
// field is counted as a rune — this matches Hugo's behavior exactly.
//
// Stage 1 limitation: Hugo gates this CJK branch behind `isCJKLanguage` (from
// `hasCJKLanguage` config). huan unconditionally applies the CJK algorithm,
// which is correct for zhurongshuo (hasCJKLanguage=true) but would under-count
// non-CJK sites. Stage 2 should add a config gate when huan is used for other
// sites.
func CountWordsInPlain(s string) int {
	words := strings.Fields(s)
	count := 0
	for _, w := range words {
		runeCount := utf8.RuneCountInString(w)
		if len(w) == runeCount {
			// All-ASCII field: counts as 1 word.
			count++
		} else {
			// Contains multi-byte (CJK etc.): count every rune.
			count += runeCount
		}
	}
	return count
}
