package render

import (
	"bytes"
	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/gpdf-dev/gpdf/template"
	"testing"
)

func TestCompactTableMovesWithHeader(t *testing.T) {
	doc := template.New(template.WithPageSize(document.A4), template.WithMargins(document.UniformEdges(document.Pt(72))), template.WithDefaultFont("Helvetica", 12))
	page := doc.AddPage()
	page.Row(document.Pt(620), func(r *template.RowBuilder) { r.Col(12, func(c *template.ColBuilder) { c.Text("Filler") }) })
	emitTableGrid(page, [][]string{{"TableStart", "Value", "Effect"}, {"Row1", "First", "Cost"}, {"Row2", "Second", "Cost"}, {"Row3", "Third", "Cost"}, {"TableEnd", "Last", "Cost"}})
	data, err := doc.Generate()
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdf.NewReader(data)
	if err != nil {
		t.Fatal(err)
	}
	count, err := r.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]int{}
	for i := 0; i < count; i++ {
		p, _ := r.Page(i)
		dict, _ := r.ResolveDict(p.Ref)
		stream, err := pdfStreamBytes(r, dict[pdf.Name("Contents")])
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{"TableStart", "TableEnd"} {
			if bytes.Contains(stream, []byte(marker)) {
				if _, ok := found[marker]; ok {
					t.Fatalf("duplicated %s", marker)
				}
				found[marker] = i
			}
		}
	}
	if len(found) != 2 || found["TableStart"] != 1 || found["TableEnd"] != 1 {
		t.Fatalf("table should move intact to page 2: %v", found)
	}
}
