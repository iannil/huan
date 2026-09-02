package content

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestProject builds a minimal books tree:
// data/books.yaml (1 volume, 1 book, 1 part title) + content tree with
// introduction, part-01 (2 chapters + en sidecars), guide/, epilogue.
func writeTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "data"), 0o755))
	must(os.WriteFile(filepath.Join(root, "data", "books.yaml"), []byte(`collection:
  - volume: "第1卷"
    books:
      - slug: "demo-book"
        title: "示范书"
        subtitle: "Demo Book"
        version: "rc"
        last_updated: "2026-09-01"
part_titles:
  demo-book:
    part-01: "第一部分：起点"
`), 0o644))
	bookDir := filepath.Join(root, "content", "books", "volume-1", "demo-book")
	for _, d := range []string{filepath.Join(bookDir, "part-01"), filepath.Join(bookDir, "guide")} {
		must(os.MkdirAll(d, 0o755))
	}
	write := func(path, fm, body string) {
		must(os.WriteFile(path, []byte("---\ntitle: "+fm+"\ndate: 2026-01-01T00:00:00+08:00\n---\n\n"+body), 0o644))
	}
	write(filepath.Join(bookDir, "introduction.md"), "引言标题", "引言正文")
	write(filepath.Join(bookDir, "introduction.en.md"), "Introduction", "intro body")
	write(filepath.Join(bookDir, "part-01", "chapter-02.md"), "第二章", "二")
	write(filepath.Join(bookDir, "part-01", "chapter-01.md"), "第一章", "一")
	write(filepath.Join(bookDir, "part-01", "chapter-01.en.md"), "Chapter One", "one")
	write(filepath.Join(bookDir, "part-01", "chapter-10.md"), "第十章", "十")
	write(filepath.Join(bookDir, "epilogue.md"), "结语标题", "结")
	write(filepath.Join(bookDir, "guide", "index.md"), "导读", "```guide\nbook: demo-book\n```")
	return root
}

func TestDiscoverBooks(t *testing.T) {
	root := writeTestProject(t)
	c, err := Discover(root, "books")
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "books" || len(c.Volumes) != 1 {
		t.Fatalf("collection: %+v", c)
	}
	v := c.Volumes[0]
	if v.Number != 1 || v.Name != "第1卷" || len(v.Books) != 1 {
		t.Fatalf("volume: %+v", v)
	}
	b := v.Books[0]
	if b.Slug != "demo-book" || b.TitleZH != "示范书" || b.Version != "rc" || !b.HasEN {
		t.Fatalf("book: %+v", b)
	}
	ordered := b.OrderedSections()
	if len(ordered) != 3 {
		t.Fatalf("want intro+part+epilogue, got %d sections", len(ordered))
	}
	if ordered[0].Type != "introduction" || ordered[1].ID != "part-01" || ordered[1].Title != "第一部分：起点" || ordered[2].Type != "epilogue" {
		t.Fatalf("order: %+v", ordered)
	}
	// chapter sort is numeric, not lexicographic: chapter-01, chapter-02, chapter-10
	chs := ordered[1].Chapters
	if len(chs) != 3 || chs[0].Title != "第一章" || chs[1].Title != "第二章" || chs[2].Title != "第十章" {
		t.Fatalf("chapters: %+v", chs)
	}
}

func TestDiscoverEnglishSidecarPaths(t *testing.T) {
	root := writeTestProject(t)
	c, err := Discover(root, "books")
	if err != nil {
		t.Fatal(err)
	}
	b := c.Volumes[0].Books[0]
	intro := b.OrderedSections()[0]
	if len(intro.Chapters) == 0 || intro.Chapters[0].SourcePath == "" {
		t.Fatal("special section must carry its file in Chapters[0]")
	}
	if intro.Chapters[0].ENPath == "" {
		t.Fatalf("introduction.en.md sidecar should set ENPath: %+v", intro.Chapters[0])
	}
	part := b.OrderedSections()[1]
	if part.Chapters[0].ENPath == "" {
		t.Fatalf("chapter-01.en.md sidecar should set ENPath: %+v", part.Chapters[0])
	}
	if part.Chapters[1].ENPath != "" {
		t.Fatalf("chapter-02 has no sidecar, ENPath must be empty: %+v", part.Chapters[1])
	}
}
