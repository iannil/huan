package render

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"github.com/iannil/huan-plugin-ebook-exporter/style"
)

// Verify rendered geometry, not just the presence of table text: the old
// pipe-separated implementation preserved words but put columns at varying X.
func TestPDFTableColumnGeometry(t *testing.T) {
	converter, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext required for geometry check")
	}
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	src := book.Sections[1].Chapters[0].SourcePath
	body := "---\ntitle: Table\n---\n\n| LeftHeader | MiddleHeader | RightHeader |\n|---|---|---|\n| Short | CenterOne | TailOne |\n| LongerFirstCell | CenterTwo | TailTwo |\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "table.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, err := exec.Command(converter, "-bbox", out, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	type word struct {
		X    float64 `xml:"xMin,attr"`
		Text string  `xml:",chardata"`
	}
	var tree struct {
		Words []word `xml:"body>doc>page>word"`
	}
	if err := xml.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	positions := map[string]float64{}
	for _, w := range tree.Words {
		positions[w.Text] = w.X
	}
	for _, group := range [][]string{{"LeftHeader", "Short", "LongerFirstCell"}, {"MiddleHeader", "CenterOne", "CenterTwo"}, {"RightHeader", "TailOne", "TailTwo"}} {
		x, ok := positions[group[0]]
		if !ok {
			t.Fatalf("missing header %s", group[0])
		}
		for _, cell := range group[1:] {
			got, found := positions[cell]
			if !found || math.Abs(got-x) > 1 {
				t.Fatalf("%s: x=%v (found=%v), header x=%v", cell, got, found, x)
			}
		}
	}
	if positions["MiddleHeader"]-positions["LeftHeader"] < 80 || positions["RightHeader"]-positions["MiddleHeader"] < 80 {
		t.Fatal("columns do not have distinct horizontal regions")
	}
}

func TestPDFLongTablePagination(t *testing.T) {
	converter, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext required for geometry check")
	}
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	var body strings.Builder
	body.WriteString("---\ntitle: Long table\n---\n\n")
	for i := 0; i < 14; i++ {
		body.WriteString("Paragraph before the table to exercise a partially filled page.\n\n")
	}
	body.WriteString("| Identifier | Explanation | Constraint |\n|---|---|---|\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&body, "| Row%03d | A longer explanation requiring wrapping in a narrow column. | A separate constraint with another wrapped line. |\n", i)
	}
	if err := os.WriteFile(book.Sections[1].Chapters[0].SourcePath, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "long-table.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, err := exec.Command(converter, "-bbox", out, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	type word struct {
		X      float64 `xml:"xMin,attr"`
		Right  float64 `xml:"xMax,attr"`
		Y      float64 `xml:"yMin,attr"`
		Bottom float64 `xml:"yMax,attr"`
		Text   string  `xml:",chardata"`
	}
	var tree struct {
		Pages []struct {
			Width  float64 `xml:"width,attr"`
			Height float64 `xml:"height,attr"`
			Words  []word  `xml:"word"`
		} `xml:"body>doc>page"`
	}
	if err := xml.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for pageIndex, page := range tree.Pages {
		previousBottom := 0.0
		for _, w := range page.Words {
			if w.X < 0 || w.Right > page.Width || w.Y < 0 || w.Bottom > page.Height {
				t.Fatalf("page %d: out of bounds: %+v", pageIndex+1, w)
			}
			if strings.HasPrefix(w.Text, "Row") {
				counts[w.Text]++
				if w.Y < previousBottom {
					t.Fatalf("page %d: overlapping rows at %s", pageIndex+1, w.Text)
				}
				previousBottom = w.Bottom
			}
		}
	}
	for i := 0; i < 60; i++ {
		key := fmt.Sprintf("Row%03d", i)
		if counts[key] != 1 {
			t.Fatalf("%s rendered %d times", key, counts[key])
		}
	}
}

func TestPDFHeadingStaysWithOpeningParagraph(t *testing.T) {
	converter, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext required for geometry check")
	}
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	body := "---\ntitle: Heading test\n---\n\n" + strings.Repeat("Filler paragraph before the section.\n\n", 27) + "## HeadingMarker\n\nOpeningMarker" + strings.Repeat(" supporting context", 20) + ".\n"
	if err := os.WriteFile(book.Sections[1].Chapters[0].SourcePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "heading.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, err := exec.Command(converter, out, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, page := range strings.Split(string(data), "\f") {
		if strings.Contains(page, "HeadingMarker") {
			found = true
			if !strings.Contains(page, "OpeningMarker") {
				t.Fatal("heading stranded at page end")
			}
		}
	}
	if !found {
		t.Fatal("heading lost")
	}
}

// styleFindCJKFontForTest locates a CJK font on this machine; PDF tests skip
// when no font is available (CI containers without fonts).
func styleFindCJKFontForTest() (string, error) {
	return style.FindCJKFont("")
}

func TestRenderPDFHeaderAndPages(t *testing.T) {
	// Requires a real CJK font on this machine; skip when absent.
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "demo.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatal("not a PDF")
	}
	if len(data) < 2000 {
		t.Fatalf("suspiciously small pdf: %d bytes", len(data))
	}
	// /Count N pages object must exist with N ≥ 3 (cover + intro + chapter + epilogue)
	if !strings.Contains(string(data), "/Count") {
		t.Fatal("no page count object")
	}
}

func TestPDFHeaderTitleFallback(t *testing.T) {
	book := mkBook(t, content.LangEN)
	// ZH edition: the ZH title is used as-is.
	if got := pdfHeaderTitle(book, content.LangZH); got != "示范书" {
		t.Fatalf("ZH header = %q, want 示范书", got)
	}
	// EN with an EN title: the EN title wins.
	if got := pdfHeaderTitle(book, content.LangEN); got != "Demo Book" {
		t.Fatalf("EN header = %q, want Demo Book", got)
	}
	// EN with no EN title: the header must fall back to ZH instead of
	// silently disappearing (README ZH-fallback policy).
	book.TitleEN = ""
	if got := pdfHeaderTitle(book, content.LangEN); got != "示范书" {
		t.Fatalf("EN-missing header = %q, want ZH fallback 示范书", got)
	}
}

func TestWrapLongLatin(t *testing.T) {
	long := ""
	for i := 0; i < 120; i++ {
		long += "a"
	}
	got := wrapLongLatin(long, 60)
	if len(got) <= 60 {
		t.Fatalf("expected wrapped, got len %d", len(got))
	}
	// chunks must all be within the limit and joined by spaces
	for _, chunk := range strings.Split(got, " ") {
		if len(chunk) > 60 {
			t.Fatalf("chunk exceeds limit: %d", len(chunk))
		}
	}
	if wrapLongLatin("正常中文段落", 60) != "正常中文段落" {
		t.Fatal("short text must pass through")
	}
	mixed := "前文" + strings.Repeat("x", 80) + "后文"
	got2 := wrapLongLatin(mixed, 60)
	if strings.Contains(got2, strings.Repeat("x", 80)) {
		t.Fatalf("long run not split: %q", got2)
	}
	if !strings.HasPrefix(got2, "前文") || !strings.HasSuffix(got2, "后文") {
		t.Fatalf("context corrupted: %q", got2)
	}
}

func TestInlinePlain(t *testing.T) {
	cases := [][2]string{
		{"**bold** and *em* and `code`", "bold and em and code"},
		{"see [link](https://example.com) here", "see link here"},
		{"plain text", "plain text"},
	}
	for _, c := range cases {
		if got := inlinePlain(c[0]); got != c[1] {
			t.Errorf("inlinePlain(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}
