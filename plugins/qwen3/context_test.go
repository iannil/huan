package main

import (
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"short ASCII", "hello", 1},     // 5 chars → 5/3 = 1
		{"CJK string", "你好世界", 1},     // 4 chars → 4/3 = 1
		{"mixed CJK and ASCII", "你好 world", 2}, // 8 chars → 8/3 = 2
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.input)
			if got != tt.want {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlidingWindowContext(t *testing.T) {
	tests := []struct {
		name     string
		previous []string
		budget   int
		want     string // empty if no context expected
	}{
		{
			name:     "zero budget returns empty",
			previous: []string{"chunk1", "chunk2"},
			budget:   0,
			want:     "",
		},
		{
			name:     "empty previous returns empty",
			previous: []string{},
			budget:   100,
			want:     "",
		},
		{
			name:     "single chunk fits budget",
			previous: []string{"short"},
			budget:   10,
			want:     "short",
		},
		{
			name:     "multiple chunks within budget",
			previous: []string{"a", "b", "c"},
			budget:   100,
			want:     "a\n\n---\n\nb\n\n---\n\nc",
		},
		{
			name:     "last chunk only when budget tight",
			previous: []string{"a very long chunk that exceeds budget", "b"},
			budget:   3, // only "b" (1 char = 1 token) fits
			want:     "b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slidingWindowContext(tt.previous, tt.budget)
			if got != tt.want {
				t.Errorf("slidingWindowContext = %q, want %q", got, tt.want)
			}
		})
	}
}