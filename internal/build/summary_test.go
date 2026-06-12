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
