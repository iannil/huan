package qwen3

import (
	"strings"
	"testing"
)

func TestSplitBySection_PreambleAndSections(t *testing.T) {
	body := `intro paragraph 1

intro paragraph 2

## Section One

content of section one

more content

## Section Two

content of section two
`
	chunks := splitBySection(body)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 (preamble + 2 sections)", len(chunks))
	}

	// Chunk 1: preamble
	if !chunks[0].IsPreamble {
		t.Error("chunk 0 should be preamble")
	}
	if chunks[0].Heading != "" {
		t.Errorf("preamble heading = %q, want empty", chunks[0].Heading)
	}
	if !strings.Contains(chunks[0].Body, "intro paragraph 1") {
		t.Errorf("preamble body missing intro 1: %q", chunks[0].Body)
	}

	// Chunk 2: section one
	if chunks[1].Heading != "## Section One" {
		t.Errorf("chunk 1 heading = %q", chunks[1].Heading)
	}
	if !strings.Contains(chunks[1].Body, "content of section one") {
		t.Errorf("chunk 1 body missing content: %q", chunks[1].Body)
	}
	if chunks[1].IsPreamble {
		t.Error("section chunk should not be preamble")
	}

	// Chunk 3: section two
	if chunks[2].Heading != "## Section Two" {
		t.Errorf("chunk 2 heading = %q", chunks[2].Heading)
	}
}

func TestSplitBySection_NoHeadings(t *testing.T) {
	body := `just paragraphs

no headings here

another paragraph
`
	chunks := splitBySection(body)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 for body without headings", len(chunks))
	}
	if !chunks[0].IsPreamble {
		t.Error("single chunk should be preamble")
	}
	if chunks[0].Body != strings.TrimSpace(body) {
		t.Errorf("preamble body should be the whole body, got %q", chunks[0].Body)
	}
}

func TestSplitBySection_OnlyHeadingsNoPreamble(t *testing.T) {
	body := `## First

content

## Second

more
`
	chunks := splitBySection(body)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (no preamble)", len(chunks))
	}
	if chunks[0].Heading != "## First" {
		t.Errorf("chunk 0 heading = %q", chunks[0].Heading)
	}
}

func TestSplitBySection_SubHeadingsStayInSection(t *testing.T) {
	// ### / #### should NOT split chunks — they stay in the parent section
	body := `## Top

paragraph

### Sub A

sub content

#### Sub-sub

more

### Sub B

final
`
	chunks := splitBySection(body)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (### / #### should not split)", len(chunks))
	}
	if !strings.Contains(chunks[0].Body, "### Sub A") {
		t.Errorf("sub heading should be in body: %q", chunks[0].Body)
	}
	if !strings.Contains(chunks[0].Body, "#### Sub-sub") {
		t.Errorf("sub-sub heading should be in body: %q", chunks[0].Body)
	}
}

func TestSplitBySection_CodeFenceContainingHeading(t *testing.T) {
	// A "## " inside a fenced code block should NOT split
	body := "para\n\n```\n## not a heading\n```\n\n## Real Heading\n\ncontent\n"
	chunks := splitBySection(body)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (preamble + 1 real heading)", len(chunks))
	}
	if !chunks[0].IsPreamble {
		t.Error("chunk 0 should be preamble")
	}
	if !strings.Contains(chunks[0].Body, "## not a heading") {
		t.Errorf("preamble should contain the code-fence fake heading: %q", chunks[0].Body)
	}
	if chunks[1].Heading != "## Real Heading" {
		t.Errorf("chunk 1 heading = %q", chunks[1].Heading)
	}
}

func TestSplitBySection_HeadingLevel3Only(t *testing.T) {
	// Only ### (level 3), no ## — should be 1 chunk
	body := `### Sub Only

content
`
	chunks := splitBySection(body)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 for ###-only body", len(chunks))
	}
	if !strings.Contains(chunks[0].Body, "### Sub Only") {
		t.Errorf("### should stay in body: %q", chunks[0].Body)
	}
}

func TestSplitBySection_EmptyBody(t *testing.T) {
	chunks := splitBySection("")
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks for empty body, want 1", len(chunks))
	}
	if chunks[0].Body != "" {
		t.Errorf("empty body chunk body = %q, want empty", chunks[0].Body)
	}
}

func TestSplitBySection_Indexes(t *testing.T) {
	body := `preamble

## A

a

## B

b

## C

c
`
	chunks := splitBySection(body)
	for i, c := range chunks {
		if c.Index != i+1 {
			t.Errorf("chunk %d has Index=%d, want %d", i, c.Index, i+1)
		}
	}
}

func TestChunkSource_Reassembles(t *testing.T) {
	c := Chunk{
		Index:   2,
		Heading: "## Hello",
		Body:    "world\n",
	}
	src := c.Source()
	if src != "## Hello\nworld\n" {
		t.Errorf("Source() = %q", src)
	}

	// Preamble chunk
	p := Chunk{IsPreamble: true, Body: "preamble text"}
	if p.Source() != "preamble text" {
		t.Errorf("preamble Source() = %q", p.Source())
	}
}

func TestSplitBySection_AppendixStyle(t *testing.T) {
	// Mirror zhurongshuo appendix.md shape: 3 ## sections, each with
	// many ### inside (model previously restructured to add "Part" groupings)
	body := `## Appendix A: Glossary

### Term 1

def 1

### Term 2

def 2

## Appendix B: References

### Ref 1

ref content

## Appendix C: Exercises

### Exercise 1

ex content
`
	chunks := splitBySection(body)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 (A/B/C)", len(chunks))
	}
	wantHeadings := []string{"## Appendix A: Glossary", "## Appendix B: References", "## Appendix C: Exercises"}
	for i, want := range wantHeadings {
		if chunks[i].Heading != want {
			t.Errorf("chunk %d heading = %q, want %q", i, chunks[i].Heading, want)
		}
	}
}
