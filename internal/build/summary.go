package build

import (
	"strings"
	"unicode"
)

// TruncateHTMLByWords truncates HTML content to approximately N "words"
// (CJK chars count as 1 word each, ASCII words split by whitespace).
// When N words are reached, it immediately cuts and closes any open tags.
// This matches Hugo's summary behavior (truncates mid-paragraph at word boundary).
func TruncateHTMLByWords(htmlStr string, n int) string {
	if n <= 0 {
		return htmlStr
	}
	count := 0
	inTag := false
	inWord := false
	var openTags []string

	for i := 0; i < len(htmlStr); i++ {
		c := htmlStr[i]
		if inTag {
			if c == '>' {
				inTag = false
			}
			continue
		}
		if c == '<' {
			inTag = true
			inWord = false
			// Track open/close tags for proper closing
			tagEnd := strings.IndexByte(htmlStr[i:], '>')
			if tagEnd > 0 {
				tagContent := htmlStr[i+1 : i+tagEnd]
				if len(tagContent) > 0 && tagContent[0] == '/' {
					// Closing tag
					if len(openTags) > 0 {
						openTags = openTags[:len(openTags)-1]
					}
				} else if tagContent != "br" && tagContent != "hr" &&
					!strings.HasPrefix(tagContent, "br/") &&
					!strings.HasPrefix(tagContent, "img") &&
					!strings.HasPrefix(tagContent, "hr/") {
					// Opening tag - extract name
					name := tagContent
					if idx := strings.IndexAny(name, " /"); idx > 0 {
						name = name[:idx]
					}
					openTags = append(openTags, name)
				}
			}
			continue
		}
		if c >= 0x80 {
			if c&0xC0 != 0x80 {
				count++
				inWord = false
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
		if count >= n {
			// Cut here, close any open tags
			result := htmlStr[:i+1]
			for j := len(openTags) - 1; j >= 0; j-- {
				result += "</" + openTags[j] + ">"
			}
			return result
		}
	}
	return htmlStr
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

// CountWordsInPlain counts words in plain text using Hugo's algorithm.
//
// Hugo's algorithm:
//   - Each CJK ideograph (Han), Hiragana, Katakana, or Hangul character
//     counts as 1 word.
//   - Other characters are grouped by whitespace; each non-empty run
//     counts as 1 word.
//   - The ideographic space (U+3000) and Unicode White_Space are treated
//     as word separators.
//
// This matches Hugo 0.x behavior including all CJK extension blocks.
func CountWordsInPlain(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) {
			count++
			inWord = false
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '　' ||
			unicode.Is(unicode.White_Space, r) {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}
