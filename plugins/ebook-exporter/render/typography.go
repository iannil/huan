package render

import "strings"

// TypographCJK normalizes halfwidth punctuation to proper CJK typography in
// export pipelines (memory only; source files are never written back):
//
//   - straight double quotes " become curly “ ” via an open/close state
//     machine (single-quote ' uses the same pairing)
//   - halfwidth parens adjacent to CJK become fullwidth （）
//   - a lone em dash — between CJK becomes a double ——
//   - a colon after CJK becomes fullwidth ： (URL colons protected)
//
// Inline markdown syntax is respected: code-span content (between backtick
// pairs) and link URLs (inside ]( … )) are copied verbatim. Strings without
// any CJK character are returned untouched, so English content naturally
// skips normalization.
func TypographCJK(s string) string {
	if !strings.ContainsFunc(s, isCJK) {
		return typographLatinQuotes(s)
	}
	rs := []rune(s)
	out := make([]rune, 0, len(rs)+len(rs)/8)
	inDQ, inSQ := false, false // double / single quote pairing state
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch r {
		case '`':
			// Code span: only a PAIR of backticks starts a span; a lone
			// backtick is literal. Content copied verbatim.
			if j := nextRune(rs, i+1, '`'); j >= 0 {
				out = append(out, rs[i:j+1]...)
				i = j
			} else {
				out = append(out, r)
			}
		case ']':
			// Link URL: ]( … ) is copied verbatim (URL colons/parens safe).
			if i+1 < len(rs) && rs[i+1] == '(' {
				end := nextRune(rs, i+2, ')')
				if end < 0 {
					end = len(rs) - 1
				}
				out = append(out, rs[i:end+1]...)
				i = end
			} else {
				out = append(out, r)
			}
		case '"':
			if !quoteContext(rs, i) {
				out = append(out, r)
				continue
			}
			if inDQ {
				out = append(out, '”')
			} else {
				out = append(out, '“')
			}
			inDQ = !inDQ
		case '\'':
			if !quoteContext(rs, i) {
				out = append(out, r)
				continue
			}
			if inSQ {
				out = append(out, '’')
			} else {
				out = append(out, '‘')
			}
			inSQ = !inSQ
		case '(', ')':
			if parenCJKContext(rs, i) {
				if r == '(' {
					out = append(out, '（')
				} else {
					out = append(out, '）')
				}
			} else {
				out = append(out, r)
			}
		case ':':
			// URL protection: a colon inside http(s)://… never follows CJK,
			// but keep the explicit next=='/' guard for scheme-relative cases.
			p := prevContextRune(rs, i)
			if p >= 0 && isCJK(rs[p]) && (i+1 >= len(rs) || rs[i+1] != '/') {
				out = append(out, '：')
			} else {
				out = append(out, r)
			}
		case '—':
			switch {
			case i+1 < len(rs) && rs[i+1] == '—':
				// Already a double dash: copy the pair (idempotent).
				out = append(out, '—', '—')
				i++
			case i > 0 && rs[i-1] == '—':
				out = append(out, r) // tail of a pair handled above
			case i > 0 && isCJK(rs[i-1]) && i+1 < len(rs) && isCJK(rs[i+1]):
				out = append(out, '—', '—') // lone dash between CJK → double
			default:
				out = append(out, r)
			}
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// typographLatinQuotes handles strings with no CJK: paired straight double
// quotes become curly (correct English book typography; the audit flags a
// straight quote that directly follows a CJK heading in EN chapters).
// Unpaired quotes and apostrophes are left untouched.
func typographLatinQuotes(s string) string {
	if n := strings.Count(s, `"`); n == 0 || n%2 != 0 {
		return s
	}
	var sb strings.Builder
	open := true
	for _, r := range s {
		if r == '"' {
			if open {
				sb.WriteRune('“')
			} else {
				sb.WriteRune('”')
			}
			open = !open
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// nextRune returns the index of the next occurrence of want at or after
// from, or -1.
func nextRune(rs []rune, from int, want rune) int {
	for j := from; j < len(rs); j++ {
		if rs[j] == want {
			return j
		}
	}
	return -1
}

// quoteContext extends cjkContext for quotes: a quote at the very START or
// END of a (CJK-containing) string also counts — table cells like
// `"CRM 的 VIP 用户列表"` start with a Latin letter but are Chinese-context
// quotations. (TypographCJK only reaches here when the string has CJK.)
func quoteContext(rs []rune, i int) bool {
	if i == 0 || i == len(rs)-1 {
		return true
	}
	return cjkContext(rs, i)
}

// cjkContext reports whether the quote at index i sits in a CJK context:
// the nearest preceding non-space rune (skipping at most one space) is CJK,
// or the immediately following rune is CJK. Matches the audit definition
// ("CJK, optional single space, quote" / "quote, CJK").
func cjkContext(rs []rune, i int) bool {
	if p := prevContextRune(rs, i); p >= 0 && isCJK(rs[p]) {
		return true
	}
	return i+1 < len(rs) && isCJK(rs[i+1])
}

// parenCJKContext reports whether the paren at index i is adjacent to CJK
// (nearest non-space neighbor on either side).
func parenCJKContext(rs []rune, i int) bool {
	if p := prevContextRune(rs, i); p >= 0 && isCJK(rs[p]) {
		return true
	}
	if n := nextContextRune(rs, i); n >= 0 && isCJK(rs[n]) {
		return true
	}
	return false
}

// prevContextRune returns the index of the nearest preceding non-space rune,
// skipping at most ONE space, or -1.
func prevContextRune(rs []rune, i int) int {
	p := i - 1
	if p >= 0 && rs[p] == ' ' {
		p--
	}
	if p < 0 {
		return -1
	}
	return p
}

// nextContextRune mirrors prevContextRune looking forward.
func nextContextRune(rs []rune, i int) int {
	n := i + 1
	if n < len(rs) && rs[n] == ' ' {
		n++
	}
	if n >= len(rs) {
		return -1
	}
	return n
}

// isCJK reports whether r is a Han character, CJK punctuation, or a
// fullwidth form — the contexts where Chinese typography applies. Em
// dashes and curly quotes are deliberately NOT CJK here: a Latin sentence
// containing them must stay on the Latin path (paired-quote conversion),
// not the CJK context rules.
func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3400 && r <= 0x4DBF: // Extension A
		return true
	case r >= 0xF900 && r <= 0xFAFF: // Compatibility Ideographs
		return true
	case r >= 0x3001 && r <= 0x303F: // CJK punctuation （、。〈〉《》…）
		return true
	case r >= 0xFF01 && r <= 0xFF60: // fullwidth forms ！＂…｠
		return true
	}
	return false
}
