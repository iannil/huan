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
		{"fullwidth digit", "１２３", 1},            // 0xFF10-0xFF19 NOT Han; grouped as 1 word
		{"ideographic space mid-text", "你　好", 2}, // U+3000 space between CJK
		{"mixed ascii+cjk", "hello 你好 world", 4},  // 1 + 2 + 1 = 4
		{"empty string", "", 0},
		{"only whitespace", " \t\n", 0},
		{"only ideographic space", "　", 0},
	}
	for _, c := range cases {
		got := CountWordsInPlain(c.in)
		if got != c.want {
			t.Errorf("%s: CountWordsInPlain(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}
