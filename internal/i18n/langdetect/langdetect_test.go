package langdetect

import "testing"

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
		if got := CountLatinWords(tc.in); got != tc.want {
			t.Errorf("CountLatinWords(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCountCJKRunes(t *testing.T) {
	if got := CountCJKRunes("hello world"); got != 0 {
		t.Errorf("CountCJKRunes(english) = %d, want 0", got)
	}
	if got := CountCJKRunes("hello 世界"); got != 2 {
		t.Errorf("CountCJKRunes(mixed) = %d, want 2", got)
	}
	if got := CountCJKRunes("法不净空"); got != 4 {
		t.Errorf("CountCJKRunes(all CJK) = %d, want 4", got)
	}
}

func TestCJKFraction(t *testing.T) {
	if got := CJKFraction("the quick brown fox"); got > 0.01 {
		t.Errorf("pure English fraction = %f, want ~0", got)
	}
	if got := CJKFraction("法不净空觉无性也"); got < 0.99 {
		t.Errorf("pure CJK fraction = %f, want ~1", got)
	}
	got := CJKFraction("ab 世界")
	if got < 0.4 || got > 0.6 {
		t.Errorf("mixed fraction = %f, want ~0.5", got)
	}
	if got := CJKFraction(""); got != 0 {
		t.Errorf("empty fraction = %f, want 0", got)
	}
}

func TestCJKRunesOutsideCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"clean English", "The quick brown fox.", 0},
		{"inline prose drop", "geopolitics and state-level博弈.", 2},
		{"fenced code ignored", "intro\n```go\n// 设置回调\nfmt.Println(\"你好\")\n```\nend", 0},
		{"inline code ignored", "use the `配置` flag here", 0},
		{"prose counted but code not", "the 博弈 of `规则` engines\n```\n回调\n```", 2},
		{"indented fence", "  ```\n  中文\n  ```\nclean", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CJKRunesOutsideCode(tc.in); got != tc.want {
				t.Errorf("CJKRunesOutsideCode(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
