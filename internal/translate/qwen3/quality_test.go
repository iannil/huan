package qwen3

import (
	"strings"
	"testing"
)

func newTestChecker() *qualityChecker {
	return newQualityChecker(QualityConfig{
		LengthRatioMin:             0.5,
		LengthRatioMax:             3.5,
		TargetLanguageThreshold:    0.8,
		MarkdownStructureTolerance: 1,
		ChunkContextTokenBudget:    8000,
		EnforceGlossary:            true,
		RetryOnViolation:           1,
	})
}

func TestCountLatinWords(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  multiple   spaces  ", 2},
		{"one\ntwo\nthree", 3},
		{"punctuation! counts? yes.", 3},
	}
	for _, tc := range tests {
		got := countLatinWords(tc.in)
		if got != tc.want {
			t.Errorf("countLatinWords(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCountCJKRunes(t *testing.T) {
	if got := countCJKRunes("hello world"); got != 0 {
		t.Errorf("countCJKRunes(english) = %d, want 0", got)
	}
	if got := countCJKRunes("hello 世界"); got != 2 {
		t.Errorf("countCJKRunes(mixed) = %d, want 2", got)
	}
	if got := countCJKRunes("法不净空"); got != 4 {
		t.Errorf("countCJKRunes(all CJK) = %d, want 4", got)
	}
}

func TestDetectLanguageFraction(t *testing.T) {
	if got := detectLanguageFraction("the quick brown fox"); got > 0.01 {
		t.Errorf("pure English fraction = %f, want ~0", got)
	}
	if got := detectLanguageFraction("法不净空觉无性也"); got < 0.99 {
		t.Errorf("pure CJK fraction = %f, want ~1", got)
	}
	got := detectLanguageFraction("ab 世界")
	if got < 0.4 || got > 0.6 {
		t.Errorf("mixed fraction = %f, want ~0.5", got)
	}
}

func TestCheckLanguageDetection(t *testing.T) {
	q := newTestChecker()
	if !q.CheckLanguageDetection("The quick brown fox jumps over the lazy dog") {
		t.Error("pure English should pass language detection")
	}
	if q.CheckLanguageDetection("法不净空觉无性也") {
		t.Error("pure Chinese should fail language detection")
	}
	if !q.CheckLanguageDetection("hello world 你好") {
		t.Error("borderline 20% CJK should pass")
	}
}

func TestCountChunkStructure_HeadingsAndParagraphs(t *testing.T) {
	src := "# H1\n\npara 1 line a\npara 1 line b\n\n## H2\n\npara 2\n\n### H3\n\npara 3"
	c := countChunkStructure(src)
	if c.Headings != 3 {
		t.Errorf("headings = %d, want 3", c.Headings)
	}
	if c.Paragraphs != 3 {
		t.Errorf("paragraphs = %d, want 3", c.Paragraphs)
	}
	if c.ListItems != 0 {
		t.Errorf("list items = %d, want 0", c.ListItems)
	}
}

func TestCountChunkStructure_Lists(t *testing.T) {
	src := "- item 1\n- item 2\n* star\n+ plus\n\npara after list"
	c := countChunkStructure(src)
	if c.ListItems != 4 {
		t.Errorf("list items = %d, want 4", c.ListItems)
	}
	if c.Paragraphs != 1 {
		t.Errorf("paragraphs = %d, want 1", c.Paragraphs)
	}
}

func TestCountChunkStructure_CodeFenceIgnored(t *testing.T) {
	// Code fence contents should not be parsed as markdown structure.
	src := "para\n\n```\n## not a heading\n- not a list\n```\n\nafter"
	c := countChunkStructure(src)
	if c.Headings != 0 {
		t.Errorf("headings inside code fence should not count, got %d", c.Headings)
	}
	if c.ListItems != 0 {
		t.Errorf("list items inside code fence should not count, got %d", c.ListItems)
	}
	if c.Paragraphs != 2 {
		t.Errorf("paragraphs = %d, want 2 (para + after)", c.Paragraphs)
	}
}

func TestCheckChunkStructure_IdenticalPasses(t *testing.T) {
	q := newTestChecker()
	src := "## Section\n\nparagraph one\n\nparagraph two\n\n- bullet 1\n- bullet 2"
	if !q.CheckChunkStructure(src, src) {
		t.Error("identical chunk should pass structure check")
	}
}

func TestCheckChunkStructure_ParagraphToHeadingPasses(t *testing.T) {
	// Empirically observed on zhurongshuo appendix.md: Chinese source uses
	// plain-text "第一部分：..." as informal section dividers; model converts
	// them to proper markdown headings (### Part One). Content blocks count
	// (heading + paragraph + list) is preserved — should PASS.
	q := newTestChecker()
	// src: 1 heading + 6 paragraphs (intro + 5 "Part X" plain text)
	src := "## Appendix B\n\nintro paragraph.\n\n" +
		"第一部分：A.\n\nref 1.\n\n第二部分：B.\n\nref 2."
	// out: 3 headings + 4 paragraphs (Part X became headings, 1 paragraph each removed)
	out := "## Appendix B\n\nintro paragraph.\n\n### Part One: A\n\nref 1.\n\n### Part Two: B\n\nref 2."
	diag := q.checkChunkStructureDetailed(src, out)
	if !q.CheckChunkStructure(src, out) {
		t.Errorf("paragraph→heading conversion should pass (content blocks preserved): %s", diag.FailedReason)
	}
}

func TestCheckChunkStructure_ProseToBulletPasses(t *testing.T) {
	// Empirically observed on chapter-02.md: model converts parallel
	// prose paragraphs to bullet list. Content blocks preserved.
	q := newTestChecker()
	src := "At this level, convergence exhibits the following characteristics:\n\n" +
		"Paragraph A: description.\n\nParagraph B: description.\n\nParagraph C: description."
	out := "At this level, convergence exhibits the following characteristics:\n\n" +
		"- A: description.\n- B: description.\n- C: description."
	if !q.CheckChunkStructure(src, out) {
		diag := q.checkChunkStructureDetailed(src, out)
		t.Errorf("prose→bullet reformatting should pass (content blocks preserved); failed: %s", diag.FailedReason)
	}
}

func TestCheckChunkStructure_ContentDropFails(t *testing.T) {
	// Model drops 2 paragraphs: src 3 → out 1, diff -2 > tol.
	q := newTestChecker()
	src := "para 1\n\npara 2\n\npara 3"
	out := "only para"
	if q.CheckChunkStructure(src, out) {
		t.Error("dropping 2 of 3 paragraphs should fail (diff > tolerance)")
	}
}

func TestCheckChunkStructure_BulletAdditionFails(t *testing.T) {
	// Source has 1 content block (heading); model adds 4 bullets → out 5.
	// diff = +4 > tol → FAIL.
	q := newTestChecker()
	src := "## Heading"
	out := "## Heading\n\n- bullet 1\n- bullet 2\n- bullet 3\n- bullet 4"
	if q.CheckChunkStructure(src, out) {
		t.Error("adding 4 bullets to empty chunk should fail (diff > tolerance)")
	}
}

func TestCheckChunkStructure_MinorReformattingPasses(t *testing.T) {
	// Merge 2 paragraphs into 1: diff 1, within tolerance.
	q := newTestChecker()
	src := "para 1\n\npara 2"
	out := "para 1 merged with para 2"
	if !q.CheckChunkStructure(src, out) {
		t.Error("merging 2 paragraphs (diff 1) should pass within tolerance")
	}
}

func TestCheckChunkStructure_AppendixBFullyPreserved(t *testing.T) {
	// Mirror the real appendix.md chunk 2 scenario:
	// src: 1 ## + 24 paragraphs (incl 5 "Part X" plain text dividers)
	// out: 1 ## + 5 ### (Part X became headings) + 19 paragraphs
	// Both = 25 content blocks → PASS.
	q := newTestChecker()
	src := "## Appendix B: References\n\nintro\n\n" +
		"第一部分：A\n\nr1a\n\nr1b\n\nr1c\n\nr1d\n\n" +
		"第二部分：B\n\nr2a\n\nr2b\n\nr2c\n\nr2d\n\n" +
		"第三部分：C\n\nr3a\n\nr3b\n\nr3c\n\nr3d\n\n" +
		"第四部分：D\n\nr4a\n\nr4b\n\n" +
		"第五部分：E\n\nr5a\n\nr5b\n\nr5c"
	out := "## Appendix B: References\n\nintro\n\n" +
		"### Part One: A\n\nr1a\n\nr1b\n\nr1c\n\nr1d\n\n" +
		"### Part Two: B\n\nr2a\n\nr2b\n\nr2c\n\nr2d\n\n" +
		"### Part Three: C\n\nr3a\n\nr3b\n\nr3c\n\nr3d\n\n" +
		"### Part Four: D\n\nr4a\n\nr4b\n\n" +
		"### Part Five: E\n\nr5a\n\nr5b\n\nr5c"
	if !q.CheckChunkStructure(src, out) {
		diag := q.checkChunkStructureDetailed(src, out)
		t.Errorf("appendix B scenario (25 blocks → 25 blocks) should pass: %s", diag.FailedReason)
	}
}

func TestCheckLengthRatio_NormalRange(t *testing.T) {
	q := newTestChecker()
	src := strings.Repeat("法", 100)
	out := strings.Repeat("a", 200)
	ratio, ok := q.CheckLengthRatio(src, out)
	if !ok {
		t.Errorf("ratio %f should be in range [0.5, 3.5], got fail", ratio)
	}
	if ratio < 0.5 || ratio > 3.5 {
		t.Errorf("ratio %f out of expected range", ratio)
	}
	if ratio != 2.0 {
		t.Errorf("ratio = %f, want 2.0", ratio)
	}
}

func TestCheckLengthRatio_TooShort(t *testing.T) {
	q := newTestChecker()
	src := strings.Repeat("法", 100)
	out := "tiny"
	ratio, ok := q.CheckLengthRatio(src, out)
	if ok {
		t.Error("ratio 0.04 should fail (too short)")
	}
	if ratio > 0.5 {
		t.Errorf("ratio %f should be < 0.5", ratio)
	}
}

func TestCheckLengthRatio_TooLong(t *testing.T) {
	q := newTestChecker()
	src := "法"
	out := strings.Repeat("a", 500)
	ratio, ok := q.CheckLengthRatio(src, out)
	if ok {
		t.Error("ratio 500 should fail (too long)")
	}
	if ratio < 3.5 {
		t.Errorf("ratio %f should be > 3.5", ratio)
	}
}

func TestCheckLengthRatio_ZhEnExpansionObserved(t *testing.T) {
	q := newQualityChecker(QualityConfig{
		LengthRatioMin: 0.5,
		LengthRatioMax: 3.5,
	})
	src := strings.Repeat("法", 12000)
	out := strings.Repeat("a", 36000)
	ratio, ok := q.CheckLengthRatio(src, out)
	if !ok {
		t.Errorf("observed zh→en ratio %f should pass under [0.5, 3.5]", ratio)
	}
}

func TestCheckFormatPurity_PureMarkdown(t *testing.T) {
	q := newTestChecker()
	out := "# Heading 1\n\nparagraph text\n\n## Heading 2\n\n- list item\n- another item\n\n[a link](/url/)\n"
	if !q.CheckFormatPurity(out) {
		t.Error("pure markdown should pass format_purity")
	}
}

func TestCheckFormatPurity_HtmlHeadingFails(t *testing.T) {
	q := newTestChecker()
	out := "<h1>Title</h1>\n<p>paragraph</p>\n<h2>4.1 Section</h2>"
	if q.CheckFormatPurity(out) {
		t.Error("HTML <h1>/<p>/<h2> should fail format_purity")
	}
}

func TestCheckFormatPurity_HtmlListFails(t *testing.T) {
	q := newTestChecker()
	out := "<ul>\n  <li>one</li>\n  <li>two</li>\n</ul>"
	if q.CheckFormatPurity(out) {
		t.Error("HTML <ul>/<li> should fail format_purity")
	}
}

func TestCheckFormatPurity_ClosingTagFails(t *testing.T) {
	q := newTestChecker()
	out := "paragraph</p>"
	if q.CheckFormatPurity(out) {
		t.Error("HTML closing tag should fail format_purity")
	}
}

func TestCheckFormatPurity_InlineSpanPasses(t *testing.T) {
	q := newTestChecker()
	out := `<span class="red">red text</span> and <em>emphasis</em> and <br/>`
	if !q.CheckFormatPurity(out) {
		t.Error("inline span/em/br should pass format_purity")
	}
}

func TestCheckFormatPurity_CaseInsensitive(t *testing.T) {
	q := newTestChecker()
	out := "<H2>Heading</H2>"
	if q.CheckFormatPurity(out) {
		t.Error("uppercase <H2> should fail format_purity")
	}
}

func TestCheckFormatPurity_TableFails(t *testing.T) {
	q := newTestChecker()
	out := `<table><tr><td>a</td></tr></table>`
	if q.CheckFormatPurity(out) {
		t.Error("HTML <table>/<tr>/<td> should fail format_purity")
	}
}

func TestCheckFormatPurity_EmptyPasses(t *testing.T) {
	q := newTestChecker()
	if !q.CheckFormatPurity("") {
		t.Error("empty body should pass format_purity")
	}
}

func TestCheckGlossaryCompliance_NoGlossary(t *testing.T) {
	q := newTestChecker()
	if !q.CheckGlossaryCompliance("anything", nil) {
		t.Error("nil glossary should always pass")
	}
	if !q.CheckGlossaryCompliance("anything", map[string]string{}) {
		t.Error("empty glossary should always pass")
	}
}

func TestCheckGlossaryCompliance_SourceTermAbsent(t *testing.T) {
	q := newTestChecker()
	glossary := map[string]string{
		"专注": "focus",
		"觉察": "awareness",
	}
	if !q.CheckGlossaryCompliance("The focus and awareness are key.", glossary) {
		t.Error("output without source terms should pass")
	}
}

func TestCheckGlossaryCompliance_SourceTermPresent(t *testing.T) {
	q := newTestChecker()
	glossary := map[string]string{
		"专注": "focus",
	}
	if q.CheckGlossaryCompliance("The 专注 is key.", glossary) {
		t.Error("output with untranslated source term should fail")
	}
}

func TestAbs(t *testing.T) {
	if abs(0) != 0 || abs(-5) != 5 || abs(5) != 5 {
		t.Error("abs function incorrect")
	}
}
