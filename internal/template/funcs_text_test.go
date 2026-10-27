package template

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// Text geometry preserves mixed CJK and Latin wrapping behavior.

func TestTextTextWidth(t *testing.T) {
	cn := textWidth("实在建构", 16)
	if math.Abs(cn-64) > 0.01 { // 4 CJK chars × 16
		t.Errorf("CN width = %v, want 64", cn)
	}
	en := textWidth("abcd", 16)
	if en <= 0 || en >= 64 { // ASCII narrower than CJK
		t.Errorf("EN width = %v, want in (0,64)", en)
	}
	mixed := textWidth("实在ab", 16)
	want := 2*16.0 + textWidth("ab", 16)
	if math.Abs(mixed-want) > 0.01 {
		t.Errorf("mixed = %v, want %v", mixed, want)
	}
}

func TestTextTruncate(t *testing.T) {
	short := textTruncate("短", 100, 16)
	if short != "短" {
		t.Errorf("short = %q, want unchanged", short)
	}
	long := textTruncate("这是一个非常长的中文标签文本", 50, 16) // 13 chars × 16 = 208 > 50
	if !strings.HasSuffix(long, "…") {
		t.Errorf("long = %q, want ellipsis suffix", long)
	}
	if textWidth(long, 16) > 50+16 { // ellipsis may slightly exceed
		t.Errorf("truncated width exceeds budget: %v", textWidth(long, 16))
	}
}

func TestTextWrap(t *testing.T) {
	lines := textWrap("可能性基底与观察收敛", 64, 16) // 10 chars → 2+ 行
	if len(lines) < 2 {
		t.Fatalf("wrap lines = %d, want >= 2", len(lines))
	}
	for _, ln := range lines {
		if textWidth(ln, 16) > 64+16 {
			t.Errorf("line %q too wide: %v", ln, textWidth(ln, 16))
		}
	}
	// ASCII wraps on word boundaries
	enLines := textWrap("Convergent Rationality Science", 80, 16)
	for _, ln := range enLines {
		if textWidth(ln, 16) > 80+16 {
			t.Errorf("en line %q too wide", ln)
		}
	}
}

func TestTextWrapWordBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width float64
		want  []string
	}{
		{"Bound the good result", 120, []string{"Bound the", "good result"}},
		{"Return feedback locally", 120, []string{"Return", "feedback", "locally"}},
		{"Keep reopening conditions", 120, []string{"Keep", "reopening", "conditions"}},
		{"  One   two  ", 120, []string{"One two"}},
		{"中文 abc def", 64, []string{"中文", "abc def"}},
	} {
		got := textWrap(tc.text, tc.width, 16)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.text, got, tc.want)
		}
	}
	for _, text := range []string{"UnbreakableLongIdentifier", "中文连续字符测试"} {
		lines := textWrap(text, 64, 16)
		if strings.Join(lines, "") != text {
			t.Errorf("lost text: %q => %q", text, lines)
		}
		for _, line := range lines {
			if textWidth(line, 16) > 64 {
				t.Errorf("overflow: %q", line)
			}
		}
	}
}
