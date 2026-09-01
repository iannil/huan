package main

import (
	"math"
	"testing"
)

func TestSvgTextWidth(t *testing.T) {
	cn := svgTextWidth("实在建构", 16)
	if math.Abs(cn-64) > 0.01 { // 4 CJK chars × 16
		t.Errorf("CN width = %v, want 64", cn)
	}
	en := svgTextWidth("abcd", 16)
	if en <= 0 || en >= 64 { // ASCII narrower than CJK
		t.Errorf("EN width = %v, want in (0,64)", en)
	}
	mixed := svgTextWidth("实在ab", 16)
	want := 2*16.0 + svgTextWidth("ab", 16)
	if math.Abs(mixed-want) > 0.01 {
		t.Errorf("mixed = %v, want %v", mixed, want)
	}
}

func TestSvgTruncate(t *testing.T) {
	short := svgTruncate("短", 100, 16)
	if short != "短" {
		t.Errorf("short = %q, want unchanged", short)
	}
	long := svgTruncate("这是一个非常长的中文标签文本", 50, 16) // 13 chars × 16 = 208 > 50
	if !endsWithEllipsis(long) {
		t.Errorf("long = %q, want ellipsis suffix", long)
	}
	if svgTextWidth(long, 16) > 50+16 { // ellipsis may slightly exceed
		t.Errorf("truncated width exceeds budget: %v", svgTextWidth(long, 16))
	}
}

func endsWithEllipsis(s string) bool { return len(s) > 0 && s[len(s)-3:] == "…" }

func TestSvgWrap(t *testing.T) {
	lines := svgWrap("可能性基底与观察收敛", 64, 16) // 10 chars → 2+ 行
	if len(lines) < 2 {
		t.Fatalf("wrap lines = %d, want >= 2", len(lines))
	}
	for _, ln := range lines {
		if svgTextWidth(ln, 16) > 64+16 {
			t.Errorf("line %q too wide: %v", ln, svgTextWidth(ln, 16))
		}
	}
	// ASCII wraps on word boundaries
	enLines := svgWrap("Convergent Rationality Science", 80, 16)
	for _, ln := range enLines {
		if svgTextWidth(ln, 16) > 80+16 {
			t.Errorf("en line %q too wide", ln)
		}
	}
}
