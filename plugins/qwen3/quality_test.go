package main

import (
	"testing"
)

func TestCheckLanguageDetection(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		threshold float64
		want      bool
	}{
		{"pure English passes", "Hello world, this is English text.", 0.8, true},
		{"pure Chinese fails", "你好世界这是一段中文", 0.8, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qc := newQualityChecker(QualityConfig{TargetLanguageThreshold: tt.threshold})
			got := qc.CheckLanguageDetection(tt.body)
			if got != tt.want {
				t.Errorf("CheckLanguageDetection = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckResidualCJK(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		max    int
		want   bool
	}{
		{"no CJK passes", "Hello world", 0, true},
		{"one CJK rune exceeds zero", "Hello 世 world", 0, false},
		{"one CJK rune within max 1", "Hello 世 world", 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qc := newQualityChecker(QualityConfig{MaxResidualCJK: tt.max})
			got := qc.CheckResidualCJK(tt.body)
			if got != tt.want {
				t.Errorf("CheckResidualCJK = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckFormatPurity(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"pure markdown passes", "## Heading\n\nParagraph with **bold**.", true},
		{"block HTML tag fails", "<h2>Heading</h2>", false},
		{"inline HTML tag passes", "This is a <span>test</span>.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qc := newQualityChecker(QualityConfig{})
			got := qc.CheckFormatPurity(tt.body)
			if got != tt.want {
				t.Errorf("CheckFormatPurity = %v, want %v", got, tt.want)
			}
		})
	}
}