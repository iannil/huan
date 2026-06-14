package qwen3

import (
	"strings"
	"testing"
)

func newTestChecker() *qualityChecker {
	return newQualityChecker(QualityConfig{
		LengthRatioMin:             0.5,
		LengthRatioMax:             2.5,
		TargetLanguageThreshold:    0.8,
		MarkdownStructureTolerance: 2,
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
	// Pure English: 0 CJK
	if got := detectLanguageFraction("the quick brown fox"); got > 0.01 {
		t.Errorf("pure English fraction = %f, want ~0", got)
	}
	// Pure Chinese: 1.0 CJK
	if got := detectLanguageFraction("法不净空觉无性也"); got < 0.99 {
		t.Errorf("pure CJK fraction = %f, want ~1", got)
	}
	// Mixed: 2 CJK / 4 letters = 0.5
	got := detectLanguageFraction("ab 世界")
	if got < 0.4 || got > 0.6 {
		t.Errorf("mixed fraction = %f, want ~0.5", got)
	}
}

func TestCheckLanguageDetection(t *testing.T) {
	q := newTestChecker()
	// Pure English passes (threshold 0.8 = max 20% CJK)
	if !q.CheckLanguageDetection("The quick brown fox jumps over the lazy dog") {
		t.Error("pure English should pass language detection")
	}
	// Pure Chinese fails
	if q.CheckLanguageDetection("法不净空觉无性也") {
		t.Error("pure Chinese should fail language detection")
	}
	// Borderline: 20% CJK should pass
	if !q.CheckLanguageDetection("hello world 你好") {
		t.Error("borderline 20% CJK should pass")
	}
}

func TestCountMarkdownStructure_Headings(t *testing.T) {
	src := `# H1
## H2
### H3
body`
	c := countMarkdownStructure(src)
	if c.Headings != 3 {
		t.Errorf("headings = %d, want 3", c.Headings)
	}
}

func TestCountMarkdownStructure_Lists(t *testing.T) {
	src := `- item 1
- item 2
* star item
+ plus item
body`
	c := countMarkdownStructure(src)
	if c.ListItems != 4 {
		t.Errorf("list items = %d, want 4", c.ListItems)
	}
}

func TestCountMarkdownStructure_Links(t *testing.T) {
	src := `[text1](/url1/) and [text2](/url2/) and ![image](/img.png)`
	c := countMarkdownStructure(src)
	if c.Links != 2 {
		t.Errorf("links = %d, want 2", c.Links)
	}
	if c.Images != 1 {
		t.Errorf("images = %d, want 1", c.Images)
	}
}

func TestCountMarkdownStructure_CodeFences(t *testing.T) {
	src := "```go\ncode here\n```\n\nmore\n\n```python\nx = 1\n```"
	c := countMarkdownStructure(src)
	if c.CodeFences != 2 {
		t.Errorf("code fences = %d, want 2", c.CodeFences)
	}
}

func TestCheckMarkdownStructure_ExactMatch(t *testing.T) {
	q := newTestChecker()
	src := `# H1

paragraph

- item 1
- item 2

[text](/url/)
`
	out := src // identical
	if !q.CheckMarkdownStructure(src, out) {
		t.Error("identical markdown should pass structure check")
	}
}

func TestCheckMarkdownStructure_HeadingCountMismatch(t *testing.T) {
	q := newTestChecker()
	src := `# H1
# H2
# H3
# H4`
	out := `# Only One`
	if q.CheckMarkdownStructure(src, out) {
		t.Error("heading count diff 3 > tolerance 2 should fail")
	}
}

func TestCheckMarkdownStructure_ImageMismatch(t *testing.T) {
	q := newTestChecker()
	src := `![img1](/a.png) ![img2](/b.png)`
	out := `![only one](/a.png)`
	if q.CheckMarkdownStructure(src, out) {
		t.Error("image count mismatch should fail (exact match required)")
	}
}

func TestCheckLengthRatio_NormalRange(t *testing.T) {
	q := newTestChecker()
	// 10 Chinese chars source → ~10 tokens
	src := strings.Repeat("法", 10)
	// 10 English words output → ratio ~1.0
	out := "one two three four five six seven eight nine ten"
	ratio, ok := q.CheckLengthRatio(src, out)
	if !ok {
		t.Errorf("ratio %f should be in range, got fail", ratio)
	}
	if ratio < 0.5 || ratio > 2.5 {
		t.Errorf("ratio %f out of expected range", ratio)
	}
}

func TestCheckLengthRatio_TooShort(t *testing.T) {
	q := newTestChecker()
	src := strings.Repeat("法", 100) // 100 CJK tokens
	out := "tiny"                    // 1 word
	_, ok := q.CheckLengthRatio(src, out)
	if ok {
		t.Error("ratio 0.01 should fail (too short)")
	}
}

func TestCheckLengthRatio_TooLong(t *testing.T) {
	q := newTestChecker()
	src := "法" // 1 CJK token
	out := strings.Repeat("word ", 100)
	_, ok := q.CheckLengthRatio(src, out)
	if ok {
		t.Error("ratio 100 should fail (too long)")
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
	// Output has neither Chinese term — passes
	if !q.CheckGlossaryCompliance("The focus and awareness are key.", glossary) {
		t.Error("output without source terms should pass")
	}
}

func TestCheckGlossaryCompliance_SourceTermPresent(t *testing.T) {
	q := newTestChecker()
	glossary := map[string]string{
		"专注": "focus",
	}
	// Output still has 专注 — LLM failed to translate it
	if q.CheckGlossaryCompliance("The 专注 is key.", glossary) {
		t.Error("output with untranslated source term should fail")
	}
}

func TestAbs(t *testing.T) {
	if abs(0) != 0 || abs(-5) != 5 || abs(5) != 5 {
		t.Error("abs function incorrect")
	}
}
