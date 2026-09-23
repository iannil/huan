package template

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// searchExcerpt fuses plainify | replaceRE `\s+` " " | truncate. Ordinary
// HTML needs only enough visible text to decide whether an ellipsis is needed.
// Ambiguous markup uses the original composition to preserve its exact bytes.
func searchExcerpt(length int, input interface{}) string {
	if length < 0 {
		panic("truncate: negative length")
	}
	s := toString(input)
	hasMarkup := strings.ContainsAny(s, "<>")
	fallback := func() string {
		return truncateFunc(length, collapseASCIIWhitespace(string(plainify(s))))
	}
	var out strings.Builder
	out.Grow(min(len(s), 4096))
	count := 0
	wasSpace := false
	for i := 0; i < len(s); {
		start := i
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if hasMarkup && r == '<' {
			end := strings.IndexByte(s[start:], '>')
			if end < 0 {
				return fallback()
			}
			end += start
			if strings.ContainsRune(s[start+1:end], '<') {
				// Pre-replacing a nested </p> or <br> can remove the only
				// closing > of an earlier tag, changing what stripTags retains.
				return fallback()
			}
			i = end + 1
			switch s[start:i] {
			case "</p>", "<br>", "<br />":
				r = '\n'
			default:
				continue
			}
		} else if hasMarkup && (r == '_' || r == utf8.RuneError && size == 1) {
			// Placeholders can be assembled across removed tags, and their
			// interpretation depends on pre-replacements anywhere in the input.
			// Invalid UTF-8 can likewise become valid after tags are removed.
			return fallback()
		}
		isSpace := r < utf8.RuneSelf && isASCIIRegexSpace(byte(r))
		if hasMarkup {
			isSpace = unicode.IsSpace(r)
		}
		if isSpace && wasSpace {
			continue
		}
		wasSpace = isSpace
		if count == length {
			prefix := out.String()
			if !utf8.ValidString(prefix) {
				prefix = string([]rune(prefix))
			}
			return prefix + "…"
		}
		count++
		if r < utf8.RuneSelf && isASCIIRegexSpace(byte(r)) {
			out.WriteByte(' ')
		} else {
			out.WriteString(s[start:i])
		}
	}
	return out.String()
}
