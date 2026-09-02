package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/style"
	"github.com/iannil/huan/pkg/plugin"
)

// writeBookProject builds a minimal single-book books project in a temp dir
// (data/books.yaml + content tree) so Export() can discover it.
func writeBookProject(t *testing.T) string {
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
	must(os.MkdirAll(filepath.Join(bookDir, "part-01"), 0o755))
	write := func(path, title, body string) {
		must(os.WriteFile(path, []byte("---\ntitle: "+title+"\ndate: 2026-01-01T00:00:00+08:00\n---\n\n"+body), 0o644))
	}
	write(filepath.Join(bookDir, "introduction.md"), "引言", "引言正文")
	write(filepath.Join(bookDir, "part-01", "chapter-01.md"), "第一章", "正文一")
	return root
}

// findItems returns the items of result matching the given format.
func findItems(res plugin.ExportResult, list []plugin.ExportItem, format string) []plugin.ExportItem {
	var out []plugin.ExportItem
	for _, it := range list {
		if it.Format == format {
			out = append(out, it)
		}
	}
	return out
}

// TestExportFormatScopedIncremental is the regression test for the
// cross-format false-skip bug: an epub run records the manifest; a following
// pdf run must still generate pdfs.
func TestExportFormatScopedIncremental(t *testing.T) {
	if _, ferr := style.FindCJKFont(""); ferr != nil {
		t.Skipf("no CJK font: %v", ferr)
	}
	root := writeBookProject(t)
	p, _ := InitPlugin(nil)
	ex := p.(plugin.Exporter)

	first, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "epub"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(first, first.Succeeded, "epub"); len(got) == 0 {
		t.Fatalf("first epub run: no epub succeeded: %+v", first)
	}

	second, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(second, second.Succeeded, "pdf"); len(got) == 0 {
		t.Fatalf("second pdf run was falsely skipped: skipped=%+v failed=%+v", second.Skipped, second.Failed)
	}
	if len(second.Skipped) != 0 {
		t.Fatalf("second pdf run: unexpected skips: %+v", second.Skipped)
	}
	if len(second.Failed) != 0 {
		t.Fatalf("second pdf run: unexpected failures: %+v", second.Failed)
	}

	// Re-running pdf now skips (hash recorded).
	third, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(third, third.Skipped, "pdf"); len(got) == 0 {
		t.Fatalf("third pdf run: want skip, got skipped=%+v succeeded=%+v", third.Skipped, third.Succeeded)
	}
}

// TestExportMixedSkipAllFormats verifies that after a full "all" run, a
// single-format pdf run finds pdf items in Skipped.
func TestExportMixedSkipAllFormats(t *testing.T) {
	if _, ferr := style.FindCJKFont(""); ferr != nil {
		t.Skipf("no CJK font: %v", ferr)
	}
	root := writeBookProject(t)
	p, _ := InitPlugin(nil)
	ex := p.(plugin.Exporter)

	first, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(first, first.Succeeded, "pdf"); len(got) == 0 {
		t.Fatalf("first all run: no pdf succeeded (font): %+v failed=%+v", first.Succeeded, first.Failed)
	}

	second, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(second, second.Skipped, "pdf"); len(got) == 0 {
		t.Fatalf("second pdf run: want skip, got skipped=%+v succeeded=%+v", second.Skipped, second.Succeeded)
	}
	if len(second.Succeeded) != 0 {
		t.Fatalf("second pdf run: want no successes: %+v", second.Succeeded)
	}
}
