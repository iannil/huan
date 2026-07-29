package main

import (
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/markdown"
)

func TestFindDiagramBlocksGolden(t *testing.T) {
	src := "```mermaid\n" +
		"graph TD\n" +
		"  A[\"Start & \\\"go\\\"\"] --> B\n" +
		"  B --> C{中文}\n" +
		"```\n"
	r := markdown.NewRenderer(&config.MarkupConfig{})
	htmlOut, err := r.Render(src)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	blocks := findDiagramBlocks(htmlOut, []string{"mermaid", "d2"})
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1; html=%s", len(blocks), htmlOut)
	}
	b := blocks[0]
	if b.Lang != "mermaid" {
		t.Errorf("Lang = %q, want mermaid", b.Lang)
	}
	want := "graph TD\n  A[\"Start & \\\"go\\\"\"] --> B\n  B --> C{中文}\n"
	if b.Source != want {
		t.Errorf("Source mismatch:\n got: %q\nwant: %q", b.Source, want)
	}
}

func TestFindDiagramBlocksIgnoresNonAllowlisted(t *testing.T) {
	src := "```go\nfmt.Println(\"hi\")\n```\n"
	r := markdown.NewRenderer(&config.MarkupConfig{})
	htmlOut, _ := r.Render(src)
	if blocks := findDiagramBlocks(htmlOut, []string{"mermaid"}); len(blocks) != 0 {
		t.Errorf("got %d blocks for non-allowlisted lang, want 0", len(blocks))
	}
}
