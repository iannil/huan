package render

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMD(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "chapter-01.md")
	if err := os.WriteFile(p, []byte("---\ntitle: 测试\ndate: 2026-01-01T00:00:00+08:00\n---\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseChapterStripsFrontmatter(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "正文第一段。\n\n## 小节标题\n\n- 甲\n- 乙\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(du.Blocks) != 3 {
		t.Fatalf("blocks: %+v", du.Blocks)
	}
	if du.Blocks[0].Kind != BlockParagraph || du.Blocks[0].Text != "正文第一段。" {
		t.Fatalf("p0: %+v", du.Blocks[0])
	}
	if du.Blocks[1].Kind != BlockHeading || du.Blocks[1].Level != 2 || du.Blocks[1].Text != "小节标题" {
		t.Fatalf("h: %+v", du.Blocks[1])
	}
	if du.Blocks[2].Kind != BlockList || len(du.Blocks[2].Items) != 2 {
		t.Fatalf("list: %+v", du.Blocks[2])
	}
}

func TestParseChapterGuideCodeDropped(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "前文\n\n```guide\nbook: x\n```\n\n```python\nprint(1)\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	sawGuide, sawPython := false, false
	for _, b := range du.Blocks {
		if b.Kind == BlockCode {
			if b.Lang == "guide" {
				sawGuide = true
			}
			if b.Lang == "python" {
				sawPython = true
			}
		}
	}
	if sawGuide || !sawPython {
		t.Fatalf("guide=%v python=%v blocks=%+v", sawGuide, sawPython, du.Blocks)
	}
}

func TestShiftHeadings(t *testing.T) {
	du := &DocUnit{Blocks: []Block{{Kind: BlockHeading, Level: 2, Text: "x"}}}
	ShiftHeadings(du, 1)
	if du.Blocks[0].Level != 3 {
		t.Fatalf("shift: %+v", du.Blocks[0])
	}
}

func TestParseChapterTable(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "| A | B |\n|---|---|\n| 1 | 2 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(du.Blocks) != 1 || du.Blocks[0].Kind != BlockTable || len(du.Blocks[0].Rows) != 2 {
		t.Fatalf("table: %+v", du.Blocks)
	}
}
