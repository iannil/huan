package template

import "unicode"

// textWidth estimates rendered text width. CJK chars and full-width
// punctuation measure 1.0×fontSize; other (ASCII/Latin) chars ≈0.55×.
// Estimation, not measurement — budgets in callers leave slack.
func textWidth(s string, fontSize int) float64 {
	w := 0.0
	for _, r := range s {
		if isWideRune(r) {
			w += float64(fontSize)
		} else {
			w += float64(fontSize) * 0.55
		}
	}
	return w
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK punctuation
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // fullwidth forms
		return true
	case r == 0x2014 || r == 0x2026: // em dash, ellipsis
		return true
	}
	return false
}

// textTruncate returns s unchanged when it fits maxWidth; otherwise cuts
// it at the character boundary that fits and appends "…".
func textTruncate(s string, maxWidth float64, fontSize int) string {
	if textWidth(s, fontSize) <= maxWidth {
		return s
	}
	runes := []rune(s)
	ellipsisW := textWidth("…", fontSize)
	acc := 0.0
	for i, r := range runes {
		rw := textWidth(string(r), fontSize)
		if acc+rw+ellipsisW > maxWidth {
			return string(runes[:i]) + "…"
		}
		acc += rw
	}
	return s
}

// textWrap prefers word boundaries for Latin text and character boundaries
// for CJK. A word wider than the entire line is split as a last resort.
func textWrap(s string, maxWidth float64, fontSize int) []string {
	var lines []string
	cur := ""
	pendingSpace := false
	flush := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			pendingSpace = true
			i++
			continue
		}
		j := i + 1
		if !isWideRune(runes[i]) {
			for j < len(runes) && !unicode.IsSpace(runes[j]) && !isWideRune(runes[j]) {
				j++
			}
		}
		token := string(runes[i:j])
		sep := ""
		if pendingSpace && cur != "" {
			sep = " "
		}
		if cur != "" && textWidth(cur+sep+token, fontSize) > maxWidth {
			flush()
			sep = ""
		}
		cur += sep
		for _, r := range token {
			if cur != "" && textWidth(cur+string(r), fontSize) > maxWidth {
				flush()
			}
			cur += string(r)
		}
		pendingSpace = false
		i = j
	}
	flush()
	return lines
}
