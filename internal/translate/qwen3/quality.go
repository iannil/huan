package qwen3

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/iannil/huan/internal/i18n/langdetect"
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

// CheckLanguageDetection passes when the output's CJK fraction is below
// (1 - TargetLanguageThreshold). For threshold=0.8 (80% English), CJK
// fraction must be ≤ 0.2.
func (q *qualityChecker) CheckLanguageDetection(body string) bool {
	cjkFrac := langdetect.CJKFraction(body)
	maxCJKFrac := 1.0 - q.cfg.TargetLanguageThreshold
	return cjkFrac <= maxCJKFrac
}

// CheckResidualCJK passes when the number of CJK runes in prose (outside code
// blocks/spans) is at or below cfg.MaxResidualCJK. Default threshold 0 means
// the English sidecar must contain NO Chinese prose ("no Chinese in .en.md"
// policy).
//
// This complements CheckLanguageDetection: the fraction-based check catches
// "LLM returned mostly Chinese", but a handful of dropped terms in a long
// English document score a negligible fraction and slip through. This check
// names that failure mode with an absolute count. Soft check — triggers a
// retry, then surfaces in the report if still failing.
func (q *qualityChecker) CheckResidualCJK(body string) bool {
	return langdetect.CJKRunesOutsideCode(body) <= q.cfg.MaxResidualCJK
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
// This check names the failure mode explicitly.
func (q *qualityChecker) CheckFormatPurity(body string) bool {
	return !htmlBlockTagRe.MatchString(body)
}

// chunkStructure holds counts of structural elements within a single
// chunk (a section-level slice of the source). Used by CheckChunkStructure
// to verify the model preserved source structure 1:1 within the chunk.
type chunkStructure struct {
	Headings   int // ^#{1,6}\s lines (any level), NOT inside code fence
	Paragraphs int // prose paragraphs (blank-line-separated text blocks)
	ListItems  int // ^\s*([-*+])\s lines (bullet items)
}

// countChunkStructure extracts structural counts from a chunk's markdown.
//
// Paragraph counting treats any maximal run of non-blank lines (excluding
// headings and bullet items) as one paragraph. Code fence bodies are
// skipped to avoid false positives from arbitrary text inside code.
func countChunkStructure(s string) chunkStructure {
	lines := strings.Split(s, "\n")
	var c chunkStructure
	inCodeFence := false
	inParagraph := false

	headingRe := regexp.MustCompile(`^#{1,6}\s`)
	// orderedListItemRe matches `1.`, `12.`, `1)` style ordered list markers
	// at the start of a line. Recognizes them as list items so numbered
	// source lists aren't miscounted as paragraphs.
	orderedListItemRe := regexp.MustCompile(`^(\d+[.)]|[a-zA-Z][.)])\s`)

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")

		// Toggle fenced code block state on ``` lines.
		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = !inCodeFence
			inParagraph = false
			continue
		}
		// Skip content inside code fences entirely.
		if inCodeFence {
			continue
		}

		isHeading := headingRe.MatchString(trimmed)
		isListItem := strings.HasPrefix(trimmed, "- ") ||
			strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "+ ") ||
			orderedListItemRe.MatchString(trimmed)

		switch {
		case isHeading:
			c.Headings++
			inParagraph = false
		case isListItem:
			c.ListItems++
			inParagraph = false
		case strings.TrimSpace(line) == "":
			// Blank line ends current paragraph.
			inParagraph = false
		default:
			// Non-blank text line — start of new paragraph if not already in one.
			if !inParagraph {
				c.Paragraphs++
				inParagraph = true
			}
		}
	}

	return c
}

// CheckChunkStructureResult holds the per-element diff for diagnostic
// logging when the overall check fails.
type CheckChunkStructureResult struct {
	Pass                               bool
	SrcHeadings, OutHeadings           int
	SrcParagraphs, OutParagraphs       int
	SrcListItems, OutListItems         int
	SrcContentBlocks, OutContentBlocks int // Headings + Paragraphs + ListItems
	FailedReason                       string
}

// CheckChunkStructure verifies the model preserved chunk content 1:1.
//
// The invariant is **total content blocks** (headings + paragraphs + list
// items combined) within ±1 tolerance. This tolerates legitimate
// cross-format reformatting that English-writing conventions prefer:
//
//   - Chinese plain-text section dividers ("第一部分：...") → proper markdown
//     `### Part One: ...` headings (zhurongshuo appendix.md case).
//   - Parallel prose paragraphs → bullet list (zhurongshuo chapter-02.md
//     "概率性/瞬时性/不可逆性" case).
//
// In both cases, content is preserved — only the formatting category shifts.
// Counting all three categories together as "content blocks" recognizes
// this. The check still fails on real content loss (dropped paragraphs)
// or hallucination (added bullets/paragraphs).
//
// Removed from earlier design: heading exact-match (src==out). That was
// too strict — it rejected legitimate Chinese-plain-text→English-heading
// conversion. See docs/adr/0008-translator-capability-qwen3-plugin.md §10.
func (q *qualityChecker) CheckChunkStructure(srcChunk, outChunk string) bool {
	return q.checkChunkStructureDetailed(srcChunk, outChunk).Pass
}

func (q *qualityChecker) checkChunkStructureDetailed(srcChunk, outChunk string) CheckChunkStructureResult {
	src := countChunkStructure(srcChunk)
	out := countChunkStructure(outChunk)

	srcBlocks := src.Headings + src.Paragraphs + src.ListItems
	outBlocks := out.Headings + out.Paragraphs + out.ListItems

	r := CheckChunkStructureResult{
		SrcHeadings:      src.Headings,
		OutHeadings:      out.Headings,
		SrcParagraphs:    src.Paragraphs,
		OutParagraphs:    out.Paragraphs,
		SrcListItems:     src.ListItems,
		OutListItems:     out.ListItems,
		SrcContentBlocks: srcBlocks,
		OutContentBlocks: outBlocks,
	}

	blockDiff := outBlocks - srcBlocks
	tol := q.cfg.MarkdownStructureTolerance
	if abs(blockDiff) > tol {
		r.FailedReason = fmt.Sprintf("content_blocks diff %d beyond tol ±%d (src: headings=%d paragraphs=%d list=%d total=%d, out: headings=%d paragraphs=%d list=%d total=%d)",
			blockDiff, tol,
			src.Headings, src.Paragraphs, src.ListItems, srcBlocks,
			out.Headings, out.Paragraphs, out.ListItems, outBlocks)
		return r
	}

	r.Pass = true
	return r
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
// the output (LLM should have translated them all).
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
