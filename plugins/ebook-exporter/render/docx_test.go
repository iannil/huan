package render

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestRenderDOCXStructure(t *testing.T) {
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "demo.docx")
	if err := RenderDOCX(book, content.LangZH, out, DOCXOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var docXML string
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			docXML = string(b)
		}
	}
	if docXML == "" {
		t.Fatal("word/document.xml missing")
	}
	// Heading1 style must be applied to chapter titles.
	if !strings.Contains(docXML, "Heading1") {
		t.Fatal("no Heading1 style in document.xml")
	}
	if !strings.Contains(docXML, "第一章正文") {
		t.Fatal("chapter body missing")
	}
}
