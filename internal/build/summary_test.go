package build

import "testing"

func TestCountWordsInPlain_CoversAllCJKRanges(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"pure ascii word", "hello", 1},
		{"ascii sentence", "hello world foo", 3},
		{"chinese basic", "你好世界", 4},            // 0x4E00-0x9FFF
		{"chinese ext A", "㐀㐁㐂", 3},             // 0x3400-0x4DBF
		{"hiragana", "こんにちは", 5},                // 0x3040-0x309F
		{"katakana", "コンニチハ", 5},                // 0x30A0-0x30FF
		{"hangul syllable", "안녕하세요", 5},         // 0xAC00-0xD7AF
		{"fullwidth digit", "１２３", 3},            // 0xFF10-0xFF19: multi-byte chars counted per-rune
		{"ideographic space mid-text", "你　好", 2}, // U+3000 space between CJK
		{"mixed ascii+cjk", "hello 你好 world", 4},  // 1 + 2 + 1 = 4
		{"empty string", "", 0},
		{"only whitespace", " \t\n", 0},
		{"only ideographic space", "　", 0},
		// Hugo's actual algorithm: strings.Fields + per-rune count for multi-byte words.
		// Quoted CJK text without spaces is ONE field, every rune counts (incl. punctuation).
		{"quoted cjk same field", "\"巧合\"", 4},      // " 巧 合 " = 4 runes
		{"cjk with fullwidth punct", "你好，世界。", 6}, // 6 runes, no spaces, one field
		{"cjk with ascii punct between", "你好.世界", 5},
		// "hello，world" has no whitespace, so it's ONE field per Hugo's strings.Fields.
		// All 11 runes (incl. fullwidth comma) count as words.
		{"ascii+cjk+ascii one field", "hello，world", 11},
	}
	for _, c := range cases {
		got := CountWordsInPlain(c.in)
		if got != c.want {
			t.Errorf("%s: CountWordsInPlain(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

func TestTruncateHTMLByWords_WordBoundary(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{
			name: "truncate after complete ascii word",
			in:   "<p>alpha beta gamma delta</p>",
			n:    2,
			want: "<p>alpha beta</p>",
		},
		{
			name: "preserve open tags at cutoff",
			in:   "<p>alpha <strong>beta gamma</strong> delta</p>",
			n:    2,
			want: "<p>alpha <strong>beta</strong></p>",
		},
		{
			name: "CJK counts each rune as 1 word",
			in:   "<p>你好世界你好世界</p>",
			n:    4,
			want: "<p>你好世界</p>",
		},
		{
			name: "zero or negative returns input",
			in:   "<p>x</p>",
			n:    0,
			want: "<p>x</p>",
		},
		{
			name: "truncate before first word boundary if N=1",
			in:   "<p>alpha beta</p>",
			n:    1,
			want: "<p>alpha</p>",
		},
	}
	for _, c := range cases {
		got := TruncateHTMLByWords(c.in, c.n)
		if got != c.want {
			t.Errorf("%s: TruncateHTMLByWords(%q, %d) = %q, want %q", c.name, c.in, c.n, got, c.want)
		}
	}
}

func TestTruncateHTMLByWords_BlockBoundaryExtension(t *testing.T) {
	// 5 short words inside one <p>, then a second <p>. summaryLength=3.
	// Hugo semantics: find 3rd word ("gamma"), then extend to end of enclosing <p>.
	in := "<p>alpha beta gamma delta epsilon</p><p>second paragraph</p>"
	got := TruncateHTMLToBlockBoundary(in, 3)
	want := "<p>alpha beta gamma delta epsilon</p>"
	if got != want {
		t.Errorf("TruncateHTMLToBlockBoundary(<p>alpha beta gamma delta epsilon</p><p>...</p>, 3):\n  got:  %q\n  want: %q", got, want)
	}
}

func TestTruncateHTMLByWords_BlockBoundaryAcrossNestedTags(t *testing.T) {
	// Word 3 is inside <strong>; enclosing block is the outer <p>.
	// Extend should close <strong> AND <p> at the end of <p>'s content.
	in := "<p>alpha <strong>beta gamma</strong> delta</p><p>second</p>"
	got := TruncateHTMLToBlockBoundary(in, 3)
	want := "<p>alpha <strong>beta gamma</strong> delta</p>"
	if got != want {
		t.Errorf("nested case:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestTruncateHTMLByWords_BlockBoundaryShortContentNoExtend(t *testing.T) {
	// Short content (< 120 words) should NOT extend — TruncateHTMLByWords already returns full content.
	// TruncateHTMLToBlockBoundary should behave identically for short content.
	in := "<p>short content</p>"
	got := TruncateHTMLToBlockBoundary(in, 120)
	want := "<p>short content</p>"
	if got != want {
		t.Errorf("short content should not extend:\n  got:  %q\n  want: %q", got, want)
	}
}

// Hugo's actual summary algorithm (from resources/page/page_markup.go,
// ExtractSummaryFromHTML) iterates over paragraph-level blocks delimited by
// </p>. For each paragraph it counts words using strings.Fields semantics
// (CJK: each rune in a field counts as a word). It accumulates the count
// across paragraphs and stops AT THE END of the paragraph where the running
// count first reaches numWords. If no paragraph reaches numWords, the summary
// is the entire input.
//
// This is fundamentally different from "do not cross paragraph boundaries":
// Hugo DOES cross boundaries when the first paragraph is shorter than
// numWords. But when the first paragraph alone has enough words (e.g., CJK
// paragraph with rune count >= numWords), Hugo stops at the end of paragraph
// one and does NOT include paragraph two.

func TestTruncateHTMLToBlockBoundary_CrossesParagraphWhenFirstShort(t *testing.T) {
	// Paragraph 1: "first" (1 word). Paragraph 2: 5 words.
	// summaryLength: 3 (falls in paragraph 2).
	// Hugo: count=1 after para1, count=3 after "alpha beta gamma" in para2.
	// Stops at end of paragraph 2.
	in := "<p>first</p><p>alpha beta gamma delta epsilon</p>"
	got := TruncateHTMLToBlockBoundary(in, 3)
	want := "<p>first</p><p>alpha beta gamma delta epsilon</p>"
	if got != want {
		t.Errorf("cross-paragraph (first short) case:\n  in:   %q\n  got:  %q\n  want: %q", in, got, want)
	}
}

func TestTruncateHTMLToBlockBoundary_AllContentWhenNumWordsNeverReached(t *testing.T) {
	// Paragraph 1: 2 words. Paragraph 2: 1 word. summaryLength: 5.
	// Hugo: never reaches 5; summary = everything.
	in := "<p>alpha beta</p><p>second</p>"
	got := TruncateHTMLToBlockBoundary(in, 5)
	want := "<p>alpha beta</p><p>second</p>"
	if got != want {
		t.Errorf("never-reaches-N case:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestTruncateHTMLToBlockBoundary_FullParagraphWhenLongFirstPara(t *testing.T) {
	// Paragraph 1: 5 words. summaryLength: 3.
	// Hugo: count=3 at "gamma" (3rd word) in paragraph 1. Stops at end of
	// paragraph 1. Hugo does NOT truncate mid-paragraph; it always returns
	// the full enclosing paragraph where the count was reached.
	in := "<p>alpha beta gamma delta epsilon</p><p>second</p>"
	got := TruncateHTMLToBlockBoundary(in, 3)
	want := "<p>alpha beta gamma delta epsilon</p>"
	if got != want {
		t.Errorf("long first paragraph case:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestTruncateHTMLToBlockBoundary_RealWorldZhurongshuoChapter(t *testing.T) {
	// Mirrors chapter-07: paragraph 1 is ~122 CJK runes (one whitespace-free
	// field per Hugo's strings.Fields; each rune counts as a word).
	// summaryLength: 120.
	// Hugo: in paragraph 1, the trailing-rune branch fires once with
	// word = s[0:len-1rune] (rune count = 121). 121 >= 120, so Hugo stops at
	// the end of paragraph 1 and does NOT include paragraph 2.
	para1 := "<p>想象一片平静的湖面，湖中的生态系统——鱼、虾、水草——构成了一个相对稳定的内在世界。我们可以深入研究这个生态系统的食物链、物种间的竞争关系、以及各个物种的生长周期。这就是我们在第一部分所做的工作：分析一个行业的内在商业逻辑、竞争格局和生命周期。</p>"
	para2 := "<p>然而，湖泊并非孤立存在。一场突如其来的暴雨，可能会让湖水泛滥。</p>"
	in := para1 + para2
	got := TruncateHTMLToBlockBoundary(in, 120)
	if got != para1 {
		t.Errorf("zhurongshuo chapter-07:\n  got:  %q\n  want: %q", got, para1)
	}
}

func TestTruncateHTMLToBlockBoundary_NoParagraphCloseReturnsAll(t *testing.T) {
	// Input has no </p> at all (e.g., raw text wrapped in inline tags only).
	// Hugo's loop breaks out and falls through to SummaryHigh = high, so the
	// summary is the entire input regardless of numWords.
	in := "<strong>alpha beta gamma delta epsilon</strong>"
	got := TruncateHTMLToBlockBoundary(in, 3)
	want := "<strong>alpha beta gamma delta epsilon</strong>"
	if got != want {
		t.Errorf("no-block-close fallback:\n  got:  %q\n  want: %q", got, want)
	}
}
