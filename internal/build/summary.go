package build

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
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
// algorithm (resources/page/page_markup.go:ExtractSummaryFromHTML).
//
// Hugo's algorithm walks paragraph-by-paragraph (using `</p>` as the
// paragraph-tag terminator for Markdown content), counting words in each
// segment between consecutive `</p>` boundaries. The segment between two
// `</p>`s may contain other block elements (e.g. `<h2>`), all of which
// contribute to that segment's word count.
//
// Hugo's exact source loop (paraphrased):
//
//	for j := 0; j < len(input); {
//	    closingIndex := strings.Index(input[j:], "</p>")
//	    if closingIndex == -1 { break }
//	    s := input[j : j+closingIndex]
//	    var wi int
//	    for i, r := range s {
//	        if unicode.IsSpace(r) || (i+utf8.RuneLen(r) == len(s)) {
//	            word := s[wi:i]   // NOTE: i is the byte offset of the current rune,
//	                              // so this EXCLUDES the current (last) rune when
//	                              // the end-of-segment trigger fires.
//	            count += countWord(word, isCJK)
//	            wi = i
//	            if count >= numWords { break }
//	        }
//	    }
//	    if count >= numWords {
//	        return input[:j+closingIndex+len("</p>")]
//	    }
//	    j += closingIndex + len("p") + 2  // advance past </p> (skips "</p>" minus 1 char)
//	}
//
// Two non-obvious quirks this preserves:
//
//  1. End-of-segment word is s[wi:i] where i is byte offset of the LAST rune,
//     so the last rune of each segment is excluded from its own word. This
//     means each CJK-only paragraph contributes runeCount-1 to the cumulative
//     count (not runeCount).
//  2. Between `</p>`s, intermediate block elements (`<h2>`, etc.) contribute
//     their text to the next segment's word count, since the segment spans
//     from the previous `</p>` to the next `</p>`.
//
// Hugo's countWord for CJK strips HTML tags and counts every rune (multi-
// byte) per whitespace-separated field. HTML-tag-like tokens (matched by
// `^</?[A-Za-z]+>?$` or `^[A-Za-z]+=...`) count as 0 words.
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
		// Segment includes everything from j to the next </p>.
		// This may contain other block tags (<h2>, <h3>, etc.) from
		// the previous paragraph's close up to the next paragraph's close.
		segment := htmlStr[j : j+closingIndex]
		// Iterate runes Hugo-style. On each whitespace or end-of-segment,
		// take s[wi:i] as a word (where i is byte offset of current rune).
		var wi int
		for i, r := range segment {
			isEnd := i+utf8.RuneLen(r) == len(segment)
			if unicode.IsSpace(r) || isEnd {
				word := segment[wi:i]
				count += hugoCountWord(word)
				wi = i
				if count >= n {
					break
				}
			}
		}
		if count >= n {
			return htmlStr[:j+closingIndex+len(paragraphClose)]
		}
		// Advance past </p>: Hugo's source uses `j += closingIndex + len(ptag.tagName) + 2`
		// which is `closingIndex + 1 + 2 = closingIndex + 3`. This skips "</p" but
		// leaves the trailing ">" as the start of the next segment (which the
		// regex-based isProbablyHTMLToken then ignores as an HTML token).
		j += closingIndex + len("p") + 2
	}

	// No paragraph reached numWords — return entire input. This matches
	// Hugo's behavior of setting SummaryHigh = high when the loop falls
	// through without an early return.
	return htmlStr
}

// hugoCountWord counts a single whitespace-separated token the way Hugo's
// ExtractSummaryFromHTML does. For CJK content (zhurongshuo isCJKLanguage=true),
// HTML tags are stripped and each multi-byte rune counts as 1 word (with
// all-ASCII tokens counting as 1 word). HTML-tag-shaped tokens (e.g. "<p>",
// "<h2>", "href=") count as 0 words.
func hugoCountWord(word string) int {
	word = strings.TrimSpace(word)
	if len(word) == 0 {
		return 0
	}
	if hugoIsProbablyHTMLToken(word) {
		return 0
	}
	// Strip HTML tags inside the word (Hugo calls tpl.StripHTML).
	stripped := stripHTMLTagsInWord(word)
	runeCount := utf8.RuneCountInString(stripped)
	if len(stripped) == runeCount {
		// All-ASCII token: counts as 1 word.
		return 1
	}
	// Contains multi-byte (CJK etc.) runes: count every rune.
	return runeCount
}

var (
	hugoHTMLTagRe       = regexp.MustCompile(`^</?[A-Za-z]+>?$`)
	hugoHTMLAttrRe      = regexp.MustCompile(`^[A-Za-z]+=["']`)
)

// hugoIsProbablyHTMLToken matches Hugo's regexps for tokens that should be
// ignored during word counting (HTML tags and attribute-shaped fragments).
func hugoIsProbablyHTMLToken(s string) bool {
	return s == ">" || hugoHTMLTagRe.MatchString(s) || hugoHTMLAttrRe.MatchString(s)
}

// stripHTMLTagsInWord strips inline HTML tags within a single whitespace-
// separated token (Hugo's tpl.StripHTML). Uses stateful char-by-char stripping
// (not a regex) so that an unclosed `<tag` fragment at the end of a token
// (which arises in Hugo's summary algorithm because segments are sliced at
// the byte offset of `<` from `</p>`) is also stripped. A regex-based
// stripper requiring a closing `>` would leave the trailing `<em` behind,
// inflating the rune count and breaking summary truncation byte-equivalence.
func stripHTMLTagsInWord(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			sb.WriteRune(r)
		}
	}
	return sb.String()
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
