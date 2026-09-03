package render

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestRenderDOCXPublicationParts(t *testing.T) {
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "p.docx")
	if err := RenderDOCX(book, content.LangZH, out, DOCXOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var docXML, stylesXML string
	for _, f := range r.File {
		switch f.Name {
		case "word/document.xml":
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			docXML = string(b)
		case "word/styles.xml":
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			stylesXML = string(b)
		}
	}
	if !strings.Contains(docXML, "TOC") {
		t.Fatal("no TOC field in document.xml")
	}
	// eastAsia: the renderer declares fonts at run level (docxgo does not
	// expose styles.xml), so document.xml rFonts is the accepted location.
	if !strings.Contains(stylesXML, "w:eastAsia=") && !strings.Contains(docXML, "w:eastAsia=") {
		t.Fatal("no eastAsia font in styles.xml or document.xml")
	}
	// Page number: a footer part must exist and carry a PAGE field.
	var footerXML string
	hasFooterPart := false
	for _, f := range r.File {
		if strings.Contains(f.Name, "footer") {
			hasFooterPart = true
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			footerXML += string(b)
		}
	}
	if !hasFooterPart && !strings.Contains(docXML, "PAGE") {
		t.Fatal("no footer/page-number part")
	}
	if hasFooterPart && !strings.Contains(footerXML, "PAGE") {
		t.Fatal("footer part has no PAGE field")
	}
}

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
