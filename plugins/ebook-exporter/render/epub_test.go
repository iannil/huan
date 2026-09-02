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
