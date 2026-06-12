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

// TruncateHTMLToBlockBoundary truncates HTML to at least N words, then extends
// forward to the closing tag of the enclosing block-level element. This matches
// Hugo's actual summary behavior: summaryLength is a minimum, and Hugo scans
// forward to a block boundary (</p>, </h2>, </li>, etc.) to produce a
// well-formed summary that doesn't end mid-paragraph.
//
// See https://github.com/gohugoio/hugo/issues/11863 for Hugo's behavior.
//
// Algorithm:
//  1. Run TruncateHTMLByWords to get the word-boundary truncation (with open
//     tags closed).
//  2. If the input was shorter than N words, TruncateHTMLByWords returns the
//     input unchanged — short content needs no extension.
//  3. Otherwise, find the byte offset where the original input diverges from
//     the truncated result (common prefix length). The truncated result may
//     have appended synthetic close tags (e.g. `</strong></p>`) that are not
//     in the original at that position — the first divergence point is the
//     right boundary.
//  4. From that divergence point, scan forward in the original HTML for the
//     next block-level closing tag and truncate just after it. Hugo assumes
//     well-formed input; all tags up to that point are balanced.
//  5. If no block boundary is found after the cut point (rare edge case),
//     fall back to the word-boundary truncation result.
func TruncateHTMLToBlockBoundary(htmlStr string, n int) string {
	truncated := TruncateHTMLByWords(htmlStr, n)
	if truncated == htmlStr {
		// Content was shorter than N words; no truncation needed.
		return htmlStr
	}

	// Find the byte position where TruncateHTMLByWords cut. The truncated
	// string is a prefix of htmlStr up to some point, plus closing tags that
	// were added. The common prefix ends at the first differing byte, which
	// is the true cut point in the original HTML.
	cutLen := commonPrefixLen(htmlStr, truncated)

	// Scan forward from cutLen to find the next block-level closing tag.
	blockCloseTags := []string{
		"</p>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>",
		"</div>", "</li>", "</ul>", "</ol>", "</blockquote>", "</pre>",
		"</table>", "</tr>", "</td>", "</th>", "</section>", "</article>",
		"</header>", "</footer>", "</aside>", "</nav>", "</main>",
		"</figure>", "</figcaption>",
	}

	earliest := -1
	lower := strings.ToLower(htmlStr)
	for _, tag := range blockCloseTags {
		if idx := strings.Index(lower[cutLen:], strings.ToLower(tag)); idx >= 0 {
			absIdx := cutLen + idx + len(tag)
			if earliest == -1 || absIdx < earliest {
				earliest = absIdx
			}
		}
	}

	if earliest == -1 {
		// No block boundary found; fall back to word-boundary truncation.
		return truncated
	}

	// Truncate at the block boundary. All tags up to this point should be
	// properly balanced (Hugo assumes well-formed input HTML).
	return htmlStr[:earliest]
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
