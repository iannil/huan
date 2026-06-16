package audit

import (
	"strings"
	"unicode"

	"github.com/iannil/huan/internal/i18n/langdetect"
)

// Finding kinds.
const (
	KindEnglishHasChinese  = "EnglishHasChinese"  // /en page prose is substantially Chinese
	KindChineseLooksEnglish = "ChineseLooksEnglish" // zh page prose looks like English
	KindMissingEN          = "MissingEN"          // zh source has body but no .en.md
	KindOrphanEN           = "OrphanEN"           // .en.md with no source .md
)

// Finding is a single audit issue. Ref identifies the page (URL or source
// path); Evidence is a short human-readable snippet supporting the finding.
type Finding struct {
	Ref      string
	Kind     string
	Evidence string
}

// minProseRunes is the minimum prose length (in runes) below which language
// checks are skipped — short snippets (nav crumbs, empty bodies) produce
// unreliable fractions and noisy findings.
const minProseRunes = 24

// CheckEnglish flags an English page whose prose is substantially Chinese.
// cjkThreshold is the maximum tolerated CJK fraction (default caller: 0.2).
// Returns nil when the page is acceptably English or too short to judge.
func CheckEnglish(ref, prose string, cjkThreshold float64) *Finding {
	if len([]rune(prose)) < minProseRunes {
		return nil
	}
	if langdetect.CJKFraction(prose) <= cjkThreshold {
		return nil
	}
	return &Finding{
		Ref:      ref,
		Kind:     KindEnglishHasChinese,
		Evidence: firstCJKSnippet(prose),
	}
}

// CheckChinese flags a Chinese page whose prose looks like English — almost no
// CJK yet a meaningful amount of Latin words. Catches pages accidentally left
// in English. Returns nil otherwise.
func CheckChinese(ref, prose string) *Finding {
	if len([]rune(prose)) < minProseRunes {
		return nil
	}
	if langdetect.CJKFraction(prose) >= 0.05 {
		return nil // has real Chinese content
	}
	if langdetect.CountLatinWords(prose) < 20 {
		return nil // not enough English to be confident it's misplaced
	}
	return &Finding{
		Ref:      ref,
		Kind:     KindChineseLooksEnglish,
		Evidence: snippet(prose, 80),
	}
}

// firstCJKSnippet returns a short window of text centered on the first CJK
// run, to show which Chinese leaked into an English page.
func firstCJKSnippet(s string) string {
	runes := []rune(s)
	idx := -1
	for i, r := range runes {
		if unicode.In(r, unicode.Han) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return snippet(s, 80)
	}
	start := idx - 20
	if start < 0 {
		start = 0
	}
	end := idx + 40
	if end > len(runes) {
		end = len(runes)
	}
	return strings.TrimSpace(collapseSpace(string(runes[start:end])))
}

// snippet returns the first n runes of s with whitespace collapsed.
func snippet(s string, n int) string {
	s = collapseSpace(strings.TrimSpace(s))
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n]) + "…"
	}
	return string(runes)
}

// collapseSpace replaces runs of whitespace with a single space.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
