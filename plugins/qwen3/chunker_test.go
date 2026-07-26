package main

import (
	"testing"
)

func TestSplitBySection(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		count int
	}{
		{"empty body", "", 1},
		{"no headings", "plain text\nmore text", 1},
		{"single section", "## Section 1\ncontent here", 1}, // preamble empty → dropped
		{"two sections", "preamble\n\n## Section 1\ncontent\n## Section 2\nmore", 3},
		{"code fence protects headings", "preamble\n\n## Section 1\n```\n## Not a heading\n```\n## Section 2\nreal", 3},
		{"nested headings stay inside chunk", "## Section 1\n### Sub heading\ncontent\n## Section 2\ndone", 2}, // ### is not a section boundary
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := splitBySection(tt.body)
			if len(chunks) != tt.count {
				t.Errorf("splitBySection returned %d chunks, want %d", len(chunks), tt.count)
			}
		})
	}
}

func TestSplitBySection_Preamble(t *testing.T) {
	body := "preamble text\n\n## Section One\ncontent"
	chunks := splitBySection(body)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if !chunks[0].IsPreamble {
		t.Error("first chunk should be preamble")
	}
	if chunks[0].Heading != "" {
		t.Errorf("preamble heading = %q, want empty", chunks[0].Heading)
	}
	if chunks[1].IsPreamble {
		t.Error("second chunk should NOT be preamble")
	}
	if chunks[1].Heading != "## Section One" {
		t.Errorf("second chunk heading = %q, want '## Section One'", chunks[1].Heading)
	}
}

func TestSplitBySection_CodeFenceProtection(t *testing.T) {
	body := "## Real Section\n```\n## Fake heading\n```\nstill real section\n## Another Real\nmore"
	chunks := splitBySection(body)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Heading != "## Real Section" {
		t.Errorf("chunk 0 heading = %q", chunks[0].Heading)
	}
	if chunks[1].Heading != "## Another Real" {
		t.Errorf("chunk 1 heading = %q", chunks[1].Heading)
	}
}