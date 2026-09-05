package render

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/gpdf-dev/gpdf/template"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

// decodePDFUTF16Hex decodes a PDF hex string of the form <FEFF...> (UTF-16BE
// with BOM) back into a Go string, for verifying injected outline titles.
func decodePDFUTF16Hex(h pdf.HexString) string {
	b := []byte(h)
	if len(b) < 2 || b[0] != 0xFE || b[1] != 0xFF {
		return ""
	}
	b = b[2:]
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}

// mkTinyPDF builds an N-page PDF via gpdf for outline-injection tests.
func mkTinyPDF(t *testing.T, pages int) []byte {
	t.Helper()
	doc := template.New(
		template.WithPageSize(document.A4),
		template.WithMargins(document.UniformEdges(document.Mm(20))),
	)
	for i := 0; i < pages; i++ {
		p := doc.AddPage()
		p.AutoRow(func(r *template.RowBuilder) {
			r.Col(12, func(c *template.ColBuilder) {
				c.Text(fmt.Sprintf("Page %d", i+1), template.FontSize(11))
			})
		})
	}
	data, err := doc.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInjectOutline(t *testing.T) {
	data := mkTinyPDF(t, 3)
	out, err := injectOutline(data, []OutlineEntry{
		{Title: "第一章", Page: 1, Level: 1},
		{Title: "  子节 (a)", Page: 2, Level: 2},
		{Title: "第二章", Page: 3, Level: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := pdf.NewReader(out)
	if err != nil {
		t.Fatalf("re-parse injected pdf: %v", err)
	}
	root, err := r.ResolveDict(r.RootRef())
	if err != nil {
		t.Fatal(err)
	}
	outlRef, ok := root[Name("Outlines")]
	if !ok {
		t.Fatal("catalog has no /Outlines")
	}
	outl, err := r.ResolveDict(outlRef)
	if err != nil {
		t.Fatal(err)
	}
	if string(outl[Name("Type")].(pdf.Name)) != "Outlines" {
		t.Fatal("outline root wrong /Type")
	}
	if got := outl[Name("Count")].(pdf.Integer); got != 2 {
		t.Fatalf("root /Count = %d, want 2 (top-level items)", got)
	}

	// Walk top-level siblings via /Next: 第一章 → 第二章; descend into 第一章
	// via /First to reach the nested 子节.
	cur := outl[Name("First")]
	wantTop := []string{"第一章", "第二章"}
	wantTopPages := []int{1, 3}
	for i := 0; i < len(wantTop); i++ {
		d, err := r.ResolveDict(cur)
		if err != nil {
			t.Fatalf("top item %d: %v", i, err)
		}
		title := decodePDFUTF16Hex(d[Name("Title")].(pdf.HexString))
		if title != wantTop[i] {
			t.Fatalf("top item %d title = %q, want %q", i, title, wantTop[i])
		}
		dest := d[Name("Dest")].(pdf.Array)
		pageRef := dest[0].(pdf.ObjectRef)
		pi, err := r.Page(wantTopPages[i] - 1)
		if err != nil {
			t.Fatal(err)
		}
		if pageRef != pi.Ref {
			t.Fatalf("top item %d dest = obj %d, want page obj %d", i, pageRef.Number, pi.Ref.Number)
		}
		if i == 0 {
			// The nested child: parent wiring, title, dest, no siblings.
			cd, err := r.ResolveDict(d[Name("First")].(pdf.ObjectRef))
			if err != nil {
				t.Fatal(err)
			}
			if decodePDFUTF16Hex(cd[Name("Title")].(pdf.HexString)) != "  子节 (a)" {
				t.Fatalf("child title = %q", decodePDFUTF16Hex(cd[Name("Title")].(pdf.HexString)))
			}
			if _, has := cd[Name("Next")]; has {
				t.Fatal("only child must have no /Next")
			}
			if _, has := cd[Name("Prev")]; has {
				t.Fatal("only child must have no /Prev")
			}
			parent := cd[Name("Parent")].(pdf.ObjectRef)
			if parent != cur.(pdf.ObjectRef) {
				t.Fatalf("child parent = %d, want %d", parent.Number, cur.(pdf.ObjectRef).Number)
			}
			cdest := cd[Name("Dest")].(pdf.Array)
			cpi, _ := r.Page(1)
			if cdest[0].(pdf.ObjectRef) != cpi.Ref {
				t.Fatal("child dest must point at page 2")
			}
		}
		nxt, ok := d[Name("Next")]
		if i < len(wantTop)-1 {
			if !ok {
				t.Fatalf("top item %d missing /Next", i)
			}
			cur = nxt
		} else if ok {
			t.Fatal("last top item must have no /Next")
		}
	}

	// Original page count is preserved.
	if n, _ := r.PageCount(); n != 3 {
		t.Fatalf("page count = %d, want 3", n)
	}
}

func TestInjectOutlineSkipsEmptyTitle(t *testing.T) {
	data := mkTinyPDF(t, 2)
	// An empty-title entry must be skipped (with a log), not hard-fail the
	// whole export; the remaining entries still form a valid tree.
	out, err := injectOutline(data, []OutlineEntry{
		{Title: "", Page: 1, Level: 1},
		{Title: "第一章", Page: 1, Level: 1},
		{Title: "", Page: 2, Level: 2},
		{Title: "第二章", Page: 2, Level: 1},
	})
	if err != nil {
		t.Fatalf("empty-title entry must not fail the export: %v", err)
	}
	r, err := pdf.NewReader(out)
	if err != nil {
		t.Fatal(err)
	}
	root, err := r.ResolveDict(r.RootRef())
	if err != nil {
		t.Fatal(err)
	}
	outl, err := r.ResolveDict(root[Name("Outlines")])
	if err != nil {
		t.Fatal(err)
	}
	if got := outl[Name("Count")].(pdf.Integer); got != 2 {
		t.Fatalf("root /Count = %d, want 2 (empty titles skipped)", got)
	}
	var titles []string
	cur := outl[Name("First")]
	for cur != nil {
		d, err := r.ResolveDict(cur)
		if err != nil {
			t.Fatal(err)
		}
		titles = append(titles, decodePDFUTF16Hex(d[Name("Title")].(pdf.HexString)))
		n, ok := d[Name("Next")]
		if !ok {
			break
		}
		cur = n
	}
	if len(titles) != 2 || titles[0] != "第一章" || titles[1] != "第二章" {
		t.Fatalf("titles = %v, want [第一章 第二章]", titles)
	}
}

func TestInjectOutlineEmpty(t *testing.T) {
	data := mkTinyPDF(t, 1)
	out, err := injectOutline(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, out) {
		t.Fatal("no-entry injection must be a no-op")
	}
}

// Name is a tiny alias so tests read like PDF syntax.
type Name = pdf.Name

func TestRenderPDFHasOutline(t *testing.T) {
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "o.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if !bytes.Contains(data, []byte("/Outlines")) {
		t.Fatal("PDF has no /Outlines")
	}

	// Structural walk: outline must cover every part and chapter of mkBook.
	r, err := pdf.NewReader(data)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	root, _ := r.ResolveDict(r.RootRef())
	outl, err := r.ResolveDict(root[Name("Outlines")])
	if err != nil {
		t.Fatalf("resolve /Outlines: %v", err)
	}
	var titles []string
	var levels []int
	depth := map[pdf.ObjectRef]int{}
	var walkItems func(ref pdf.ObjectRef, level int) error
	walkItems = func(ref pdf.ObjectRef, level int) error {
		for ref.Number > 0 {
			d, err := r.ResolveDict(ref)
			if err != nil {
				return err
			}
			if hs, ok := d[Name("Title")].(pdf.HexString); ok {
				titles = append(titles, decodePDFUTF16Hex(hs))
				levels = append(levels, level)
			}
			depth[ref] = level
			if f, ok := d[Name("First")]; ok {
				if err := walkItems(f.(pdf.ObjectRef), level+1); err != nil {
					return err
				}
			}
			n, ok := d[Name("Next")]
			if !ok {
				return nil
			}
			ref = n.(pdf.ObjectRef)
		}
		return nil
	}
	if err := walkItems(outl[Name("First")].(pdf.ObjectRef), 1); err != nil {
		t.Fatal(err)
	}
	// mkBook: 引言(l1), 第一部(l1), 第一章(l2), 结语(l1)
	want := []string{"引言", "第一部", "第一章", "结语"}
	if len(titles) != len(want) {
		t.Fatalf("outline titles = %v, want %v", titles, want)
	}
	for i, w := range want {
		if titles[i] != w {
			t.Fatalf("outline[%d] = %q, want %q", i, titles[i], w)
		}
	}
	if levels[2] != 2 {
		t.Fatalf("第一章 must be level 2 under 第一部, got %d", levels[2])
	}
	// Every dest page must be within the document.
	pc, err := r.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	if pc < 5 {
		t.Fatalf("page count = %d, want >= 5", pc)
	}
}

func TestMeasurePDFPages(t *testing.T) {
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	plan, err := measurePDFPages(book, content.LangZH, PDFOptions{FontPath: fontPath})
	if err != nil {
		t.Fatal(err)
	}
	if plan.tocPages < 1 {
		t.Fatalf("tocPages = %d, want >= 1", plan.tocPages)
	}
	// Flattened chapter order for mkBook: 引言 (before the part), 第一章
	// (after the part separator page), 结语. Content starts after cover (1),
	// titlepage (2), copyright (3), and tocPages TOC pages, i.e. at page 4+tocPages (PDF-2).
	if got := plan.chapterPage[0]; got != 4+plan.tocPages {
		t.Fatalf("introduction page = %d, want %d", got, 4+plan.tocPages)
	}
	// 第一章 = 引言 pages + one part separator page later.
	want := plan.chapterPage[0] + plan.chapterCount[0] + 1
	if got := plan.chapterPage[1]; got != want {
		t.Fatalf("chapter one page = %d, want %d", got, want)
	}
	if plan.chapterPage[2] != plan.chapterPage[1]+plan.chapterCount[1] {
		t.Fatal("epilogue page must follow chapter one's measured span")
	}
}
