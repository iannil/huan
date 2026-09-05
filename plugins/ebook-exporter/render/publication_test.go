package render

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func TestCoverWrapPreservesLongTitle(t *testing.T) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 36, DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	for _, title := range []string{"The Construction of Reality and the Boundaries of Human Observation", strings.Repeat("W", 100)} {
		lines := wrapCoverText(title, face, 320)
		if strings.ReplaceAll(strings.Join(lines, ""), " ", "") != strings.ReplaceAll(title, " ", "") {
			t.Fatal("title lost characters")
		}
		for _, line := range lines {
			if font.MeasureString(face, line).Ceil() > 320 {
				t.Fatalf("line too wide: %q", line)
			}
		}
	}
}

func TestDOCXCoverWithoutMetadataKeepsTOC(t *testing.T) {
	book := mkBook(t, content.LangZH)
	book.Version = ""
	book.LastUpdated = ""
	out := filepath.Join(t.TempDir(), "book.docx")
	if err := RenderDOCX(book, content.LangZH, out, DOCXOptions{}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			s := string(b)
			for _, want := range []string{"目录", "TOC", "第一章正文", "rIdPublicationCover", "w:sectPr"} {
				if !strings.Contains(s, want) {
					t.Errorf("cover replacement lost %q", want)
				}
			}
		}
	}
}

func TestCoverReservesRoomForLongSubtitle(t *testing.T) {
	book := mkBook(t, content.LangZH)
	book.TitleZH = "远程工作基础：构建高度自主、高度信任与高质量交付的文化"
	book.SubtitleZH = "Fundamentals of Remote Work: Building a Culture of High Autonomy, High Trust, and High-Quality Delivery"
	if _, err := publicationCover(book, content.LangZH, CoverFonts{}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationPDFPhysicalPageNumbers(t *testing.T) {
	path, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skip(err)
	}
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "book.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: path}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdf.NewReader(data)
	if err != nil {
		t.Fatal(err)
	}
	n, err := r.PageCount()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		p, _ := r.Page(i)
		d, _ := r.ResolveDict(p.Ref)
		b, err := pdfStreamBytes(r, d[pdf.Name("Contents")])
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if strings.Contains(string(b), "PublicationNumber") {
				t.Fatal("cover has page number")
			}
			continue
		}
		if strings.Count(string(b), "/PublicationNumber 9 Tf") != 1 || !strings.Contains(string(b), fmt.Sprintf("(%d) Tj", i+1)) {
			t.Fatalf("page %d has wrong physical page number", i+1)
		}
	}
}
