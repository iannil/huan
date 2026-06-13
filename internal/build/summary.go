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

// TruncateHTMLToBlockBoundary truncates HTML following Hugo's actual summary
// algorithm. For zhurongshuo (Markdown content, media.Type.SubType = markdown),
// Hugo uses `</p>` as the paragraph tag and walks paragraph-by-paragraph
// accumulating word count.
//
// Source: github.com/gohugoio/hugo resources/page/page_markup.go,
// ExtractSummaryFromHTML. The relevant loop:
//
//	for j := wrapperStart; j < high; {
//	    closingIndex := strings.Index(input[j:], "</p>")
//	    if closingIndex == -1 { break }
//	    s := input[j : j+closingIndex]
//	    // Count words in this paragraph (Hugo: strings.Fields semantics +
//	    // CJK per-rune counting via countWord/StripHTML).
//	    // Accumulate; if count >= numWords, set SummaryHigh = j+closingIndex+5
//	    // (length of "</p>" is 4, +3 for "p" + 2 = +5? Actually +len("p")+3).
//	    // Skip past </p>: j += closingIndex + len("p") + 2.
//	}
//	// If no paragraph reached numWords, summary = everything (high).
//
// Key behaviors:
//
//  1. Crosses paragraph boundaries when paragraph 1's word count < numWords
//     (it accumulates into paragraph 2, 3, ...).
//  2. When the count reaches numWords AT OR BEFORE the end of paragraph K,
//     returns input[:end_of_paragraph_K] — the entire paragraph K, never a
//     mid-paragraph truncation.
//  3. If no paragraph ever reaches numWords, returns the entire input.
//
// This differs from a naive "extend forward to next block close tag" approach
// because Hugo never extends beyond the paragraph where the count was reached.
func TruncateHTMLToBlockBoundary(htmlStr string, n int) string {
	if n <= 0 {
		return htmlStr
	}

	paragraphClose := "</p>"
	count := 0
	j := 0
	for j < len(htmlStr) {
		// Find next </p> from current position.
		closingIndex := strings.Index(htmlStr[j:], strings.ToLower(paragraphClose))
		if closingIndex == -1 {
			break
		}
		// Extract this paragraph's content (without the closing tag).
		paragraphContent := htmlStr[j : j+closingIndex]
		// Count words in this paragraph (HTML tags stripped, then CJK-aware
		// per-rune counting — same as CountWordsInPlain).
		plain := StripHTMLTagsForSummary(paragraphContent)
		count += CountWordsInPlain(plain)
		if count >= n {
			// Stop at the end of this paragraph. Summary = input up to and
			// including </p>.
			return htmlStr[:j+closingIndex+len(paragraphClose)]
		}
		// Advance past this </p>.
		j += closingIndex + len(paragraphClose)
	}

	// No paragraph reached numWords — return entire input. This matches
	// Hugo's behavior of setting SummaryHigh = high when the loop falls
	// through without an early return.
	return htmlStr
}

// commonPrefixLen returns the length of the longest common byte prefix of a and b.
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
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
