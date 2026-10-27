package render

import (
	"archive/zip"
	"bytes"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestNeutralPublicationArtifacts(t *testing.T) {
	book := &content.BookEntry{TitleZH: "测试", TitleEN: "Test", Slug: "test"}
	for _, lang := range []content.Lang{content.LangZH, content.LangEN} {
		for _, format := range []string{"epub", "docx"} {
			path := filepath.Join(t.TempDir(), "test."+format)
			var err error
			if format == "epub" {
				err = RenderEPUB(book, lang, path, EPUBOptions{})
			} else {
				err = RenderDOCX(book, lang, path, DOCXOptions{})
			}
			if err != nil {
				t.Fatal(err)
			}
			zr, err := zip.OpenReader(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range zr.File {
				r, err := entry.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(r)
				r.Close()
				if err != nil {
					t.Fatal(err)
				}
				for _, brand := range []string{"zhurongshuo.com", "Rong Zhu", "祝融"} {
					if bytes.Contains(data, []byte(brand)) {
						t.Errorf("%s %s contains implicit brand %s", format, entry.Name, brand)
					}
				}
			}
			zr.Close()
		}
	}
	cover, err := publicationCover(book, content.LangEN, CoverFonts{})
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(cover))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := im.At(im.Bounds().Dx()-5, 5).RGBA()
	if r != 65535 || g != 65535 || b != 65535 {
		t.Fatal("neutral cover still contains branded red bar")
	}
}

func TestConfiguredPublicationArtifacts(t *testing.T) {
	pub := content.Publication{BaseURL: "https://press.example/", Languages: map[content.Lang]content.PublicationIdentity{content.LangEN: {Author: "Other Writer"}}}
	if got := epubResolveLinks(`<a href="/posts/test/?q=a&amp;b=c#part">link</a>`, nil, pub.BaseURL); !strings.Contains(got, `https://press.example/posts/test/?q=a&amp;b=c#part`) {
		t.Fatal(got)
	}
	book := &content.BookEntry{TitleEN: "Example", TitleZH: "Example"}
	for _, format := range []string{"epub", "docx"} {
		path := filepath.Join(t.TempDir(), "test."+format)
		var err error
		if format == "epub" {
			err = RenderEPUB(book, content.LangEN, path, EPUBOptions{Publication: pub})
		} else {
			err = RenderDOCX(book, content.LangEN, path, DOCXOptions{Publication: pub})
		}
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range zr.File {
			if strings.HasSuffix(entry.Name, ".opf") || entry.Name == "docProps/core.xml" {
				r, _ := entry.Open()
				data, _ := io.ReadAll(r)
				r.Close()
				found = bytes.Contains(data, []byte("Other Writer"))
			}
		}
		zr.Close()
		if !found {
			t.Fatalf("%s missing configured author", format)
		}
	}
}

func TestPDFPublicationMetadata(t *testing.T) {
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skip(err)
	}
	book := &content.BookEntry{TitleZH: "Example", TitleEN: "Example", Slug: "example"}
	for _, author := range []string{"", "Independent Writer"} {
		pub := content.Publication{Languages: map[content.Lang]content.PublicationIdentity{content.LangEN: {Author: author, TitleCredit: author, Publisher: "Example Press", Copyright: "Copyright Example Press"}}}
		path := filepath.Join(t.TempDir(), "sample.pdf")
		if err := RenderPDF(book, content.LangEN, path, PDFOptions{FontPath: fontPath, Publication: pub}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, brand := range []string{"Rong Zhu", "zhurongshuo.com", "祝融"} {
			if bytes.Contains(data, []byte(brand)) {
				t.Fatalf("implicit PDF brand %s", brand)
			}
		}
		if author != "" && !bytes.Contains(data, []byte(author)) {
			t.Fatal("missing configured PDF metadata author")
		}
	}
}
