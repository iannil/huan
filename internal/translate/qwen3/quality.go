package qwen3

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// qualityChecker runs post-translation quality checks against the parsed
// output. Each method returns a bool pass/fail. The orchestrator (plugin.go)
// assembles these into a translate.QualityResult.
type qualityChecker struct {
	cfg QualityConfig
}

// newQualityChecker constructs a checker from the typed QualityConfig.
func newQualityChecker(cfg QualityConfig) *qualityChecker {
	return &qualityChecker{cfg: cfg}
}

// countLatinWords returns the number of whitespace-separated tokens in s
// where the first non-space rune is a Latin letter. This is a rough word
// count for English output (used for length-ratio check).
func countLatinWords(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// countCJKRunes returns the number of CJK Unified Ideograph runes in s.
// Used to detect "LLM was lazy and returned Chinese source unchanged".
func countCJKRunes(s string) int {
	count := 0
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			count++
		}
	}
	return count
}

// detectLanguageFraction returns the fraction of CJK runes / total runes
// (alphabetic + CJK). For English output, this should be very low (< 0.2).
// A high fraction means the output is still mostly Chinese.
func detectLanguageFraction(s string) float64 {
	total := 0
	cjk := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			total++
			if unicode.In(r, unicode.Han) {
				cjk++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(cjk) / float64(total)
}

// CheckLanguageDetection passes when the output's CJK fraction is below
// (1 - TargetLanguageThreshold). For threshold=0.8 (80% English), CJK
// fraction must be ≤ 0.2.
func (q *qualityChecker) CheckLanguageDetection(body string) bool {
	cjkFrac := detectLanguageFraction(body)
	maxCJKFrac := 1.0 - q.cfg.TargetLanguageThreshold
	return cjkFrac <= maxCJKFrac
}

// htmlBlockTagRe matches opening HTML tags whose existence in translator
// output is a tell-tale sign that the model converted markdown to HTML
// rather than preserving it. Closing tags (</h2>) are not matched because
// any opening tag implies its closer.
//
// Blacklist covers markdown-equivalent block-level tags: headings,
// paragraphs, lists, list items, code blocks, blockquotes, and tables.
// Inline tags like <span>, <em>, <strong>, <a>, <br> are intentionally
// excluded — goldmark's unsafe=true mode allows them in source markdown,
// and we don't want false positives when the model preserves legitimate
// inline HTML.
var htmlBlockTagRe = regexp.MustCompile(`(?i)<\s*(/?)(h[1-6]|p|ul|ol|li|pre|blockquote|table|thead|tbody|tfoot|tr|td|th|dl|dt|dd|section|article|header|footer|nav|aside|div)\b`)

// CheckFormatPurity passes when the output contains no markdown-equivalent
// HTML block tags. The translator contract is raw markdown output (the
// sidecar is `.en.md`), so HTML-converted output is a hard failure.
//
// Why this exists: Qwen3-Next-80B (q4_K_M) on long zh→en inputs has a
// strong prior to emit <h2>, <p>, <ol>, <li> etc. instead of #, ##, - .
// markdown_structure check catches this indirectly (heading count → 0),
// but the error attribution is misleading. This check names the failure
// mode explicitly.
func (q *qualityChecker) CheckFormatPurity(body string) bool {
	return !htmlBlockTagRe.MatchString(body)
}

// markdownCounts holds counts of structural markers in markdown text.
type markdownCounts struct {
	Headings int // # / ## / ### etc.
	ListItems int // lines starting with - / * / +
	Links     int // [text](url) patterns
	Images    int // ![alt](url) patterns
	CodeFences int // ``` occurrences / 2
}

// countMarkdownStructure extracts structural marker counts from markdown.
// Used to compare source vs output: large divergence suggests the LLM
// corrupted structure.
func countMarkdownStructure(s string) markdownCounts {
	lines := strings.Split(s, "\n")
	var c markdownCounts
	headingRe := regexp.MustCompile(`^#{1,6}\s`)
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	imageRe := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if headingRe.MatchString(trimmed) {
			c.Headings++
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			c.ListItems++
		}
	}
	// Image regex `![alt](url)` inner part `[alt](url)` also matches link
	// regex, so subtract image count from raw link count to get true link count.
	c.Images = len(imageRe.FindAllString(s, -1))
	c.Links = len(linkRe.FindAllString(s, -1)) - c.Images
	if c.Links < 0 {
		c.Links = 0
	}
	c.CodeFences = strings.Count(s, "```") / 2
	return c
}

// CheckMarkdownStructure passes when each count in output matches source
// within ±Tolerance. Headings/CodeFences are exact match (tolerance 0)
// since these are critical structural elements.
func (q *qualityChecker) CheckMarkdownStructure(source, output string) bool {
	src := countMarkdownStructure(source)
	out := countMarkdownStructure(output)
	tol := q.cfg.MarkdownStructureTolerance

	// Headings: tolerance applies (LLM may add/remove some by mistake)
	if abs(out.Headings-src.Headings) > tol {
		return false
	}
	// List items: tolerance applies
	if abs(out.ListItems-src.ListItems) > tol {
		return false
	}
	// Links: tolerance applies (LLM may merge/split)
	if abs(out.Links-src.Links) > tol {
		return false
	}
	// Images: exact match (images are rare and important)
	if out.Images != src.Images {
		return false
	}
	// Code fences: exact match (critical for code blocks)
	if out.CodeFences != src.CodeFences {
		return false
	}
	return true
}

// CheckLengthRatio returns out_chars / src_chars and a pass flag. This is
// a cross-language character expansion ratio — stable across language
// pairs (zh→en typically 1.5-2.5; en→zh typically 0.4-0.7; same-language
// ~1.0). The previous metric (en_words / cjk_chars) was a poor fit for
// zh→en because English whitespace-tokenized words are sparse relative to
// dense CJK — normal translations scored ~0.5 and tripped the lower bound
// (false truncation signal).
//
// Bounds are configurable via QualityConfig.LengthRatioMin/Max. Defaults
// [0.5, 3.5] accommodate zh→en expansion (observed up to 3.0 on long
// philosophical prose) without false-positiving on shorter documents.
func (q *qualityChecker) CheckLengthRatio(source, output string) (float64, bool) {
	srcChars := utf8.RuneCountInString(source)
	outChars := utf8.RuneCountInString(output)
	if srcChars == 0 {
		return 0, false
	}
	ratio := float64(outChars) / float64(srcChars)
	pass := ratio >= q.cfg.LengthRatioMin && ratio <= q.cfg.LengthRatioMax
	return ratio, pass
}

// CheckGlossaryCompliance passes when no glossary source term appears in
// the output (LLM should have translated them all) AND all expected target
// translations appear at least once when the source term was in the input.
//
// v1 implements only the first check (source term absence); the second
// check requires matching input/output token positions which is complex
// and prone to false positives.
func (q *qualityChecker) CheckGlossaryCompliance(output string, glossary map[string]string) bool {
	if len(glossary) == 0 {
		return true
	}
	for srcTerm := range glossary {
		if strings.Contains(output, srcTerm) {
			return false
		}
	}
	return true
}

// abs returns the absolute value of n.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
