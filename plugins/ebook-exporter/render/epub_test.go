package render

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func mkBook(t *testing.T, lang content.Lang) *content.BookEntry {
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("---\ntitle: "+name+"\n---\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "part-01"), 0o755)
	write("introduction.md", "导言内容。\n\n### 导言小节\n")
	write("part-01/chapter-01.md", "第一章正文。\n\n## 章内节\n\n段落二。")
	write("epilogue.md", "结语内容。")
	return &content.BookEntry{
		Slug: "demo", TitleZH: "示范书", TitleEN: "Demo Book", Version: "rc",
		LastUpdated: "2026-09-01", Dir: dir,
		Sections: []content.Section{
			{Type: "introduction", Title: "引言", Chapters: []content.Chapter{{SourcePath: filepath.Join(dir, "introduction.md"), Title: "引言"}}},
			{Type: "part", ID: "part-01", Title: "第一部", Chapters: []content.Chapter{{SourcePath: filepath.Join(dir, "part-01", "chapter-01.md"), Title: "第一章"}}},
			{Type: "epilogue", Title: "结语", Chapters: []content.Chapter{{SourcePath: filepath.Join(dir, "epilogue.md"), Title: "结语"}}},
		},
	}
}

func TestRenderEPUBStructure(t *testing.T) {
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "demo.epub")
	if err := RenderEPUB(book, content.LangZH, out, EPUBOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var sawMimetype, sawNav bool
	var firstEntry string
	for i, f := range r.File {
		if i == 0 {
			firstEntry = f.Name
		}
		if f.Name == "mimetype" {
			sawMimetype = true
		}
		if strings.HasPrefix(f.Name, "EPUB/nav.") || strings.Contains(f.Name, "nav.xhtml") {
			sawNav = true
		}
	}
	if firstEntry != "mimetype" || !sawMimetype || !sawNav {
		t.Fatalf("epub structure: first=%q mimetype=%v nav=%v", firstEntry, sawMimetype, sawNav)
	}
	// a chapter's CJK text must be inside some xhtml section
	found := false
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".xhtml") {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(string(data), "第一章正文") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("chapter body not found in epub xhtml")
	}
}

func TestInlineXHTMLOddDelimitersBalanced(t *testing.T) {
	out := inlineXHTML("**a** b **c")
	if strings.Count(out, "<strong>") != strings.Count(out, "</strong>") {
		t.Fatalf("unbalanced strong tags: %q", out)
	}
	if strings.Count(out, "<em>") != strings.Count(out, "</em>") {
		t.Fatalf("unbalanced em tags: %q", out)
	}
}

func TestInlineXHTMLLinkURLProtected(t *testing.T) {
	out := inlineXHTML("[docs](https://x.com/a_b*c?d=1)")
	if !strings.Contains(out, `href="https://x.com/a_b*c?d=1"`) {
		t.Fatalf("href corrupted: %q", out)
	}
	if strings.Contains(out, "<em>") {
		t.Fatalf("em leaked into URL: %q", out)
	}
}

func TestUniqueFname(t *testing.T) {
	used := map[string]bool{"x-2": true}
	cases := []struct{ base, want string }{
		{"introduction", "introduction"},
		{"introduction", "introduction-2"},
		{"introduction", "introduction-3"},
		{"x", "x"},
		{"x", "x-3"}, // x-2 pre-existing, must be skipped
	}
	for _, c := range cases {
		if got := uniqueFname(c.base, used); got != c.want {
			t.Errorf("uniqueFname(%q) = %q, want %q", c.base, got, c.want)
		}
	}
}

func TestRenderEPUBMergedIntroductions(t *testing.T) {
	// Regression: volume/complete units merge several books, each with its
	// own introduction — duplicate section filenames used to fail AddSection.
	book := mkBook(t, content.LangZH)
	intro := book.Sections[0]
	book.Sections = append([]content.Section{intro, intro}, book.Sections[1:]...) // two introductions
	out := filepath.Join(t.TempDir(), "merged.epub")
	if err := RenderEPUB(book, content.LangZH, out, EPUBOptions{}); err != nil {
		t.Fatalf("RenderEPUB with duplicate introductions: %v", err)
	}
}
