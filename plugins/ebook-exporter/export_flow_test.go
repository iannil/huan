package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestExportRenamesSlugArtifacts verifies the filename localization rollout:
// a stale slug-named artifact left by a pre-v4 export is removed once the
// unit re-exports under its localized title, and the localized file exists.
func TestExportRenamesSlugArtifacts(t *testing.T) {
	if _, ferr := style.FindCJKFont(""); ferr != nil {
		t.Skipf("no CJK font: %v", ferr)
	}
	root := writeBookProject(t)
	p, _ := InitPlugin(nil)
	ex := p.(plugin.Exporter)

	// Simulate a pre-v4 export: slug-named epub sitting in the output dir.
	legacy := filepath.Join(root, "developer/export/epub/books/individual/demo-book.epub")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "epub"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(res, res.Succeeded, "epub"); len(got) != 1 {
		t.Fatalf("epub succeeded = %+v failed=%+v", res.Succeeded, res.Failed)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy slug artifact still exists (err=%v)", err)
	}
	localized := filepath.Join(root, "developer/export/epub/books/individual/示范书.epub")
	if _, err := os.Stat(localized); err != nil {
		t.Fatalf("localized artifact missing: %v", err)
	}
}

// TestExportFallsBackWhenConfiguredFontsMissing is the regression test for
// the 2026-09-09 incident: huan.yaml font paths pointing at pre-generated
// files that no longer exist must fall back to system-derived fonts instead
// of failing every item.
func TestExportFallsBackWhenConfiguredFontsMissing(t *testing.T) {
	if _, ferr := style.FindCJKFont(""); ferr != nil {
		t.Skipf("no CJK font: %v", ferr)
	}
	root := writeBookProject(t)
	cfg, err := ParseConfig(map[string]any{
		"pdf_font":         "developer/audit-tools/publication-fonts/body-cover-cjk.ttf",
		"cover_font":       "developer/audit-tools/publication-fonts/body-cover-cjk.ttf",
		"cover_latin_font": "developer/audit-tools/publication-fonts/cover-latin.ttf",
	})
	if err != nil {
		t.Fatal(err)
	}
	ex := New(cfg)

	res, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(res, res.Succeeded, "pdf"); len(got) == 0 {
		t.Fatalf("pdf items all failed: failed=%+v", res.Failed)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("auto-derivation must be surfaced as a warning, got none")
	}
	foundFontWarning := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "fonts") {
			foundFontWarning = true
		}
	}
	if !foundFontWarning {
		t.Fatalf("want a font-related warning, got %q", res.Warnings)
	}
}
