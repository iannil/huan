package build

import (
	"strings"
	"unicode/utf8"
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
