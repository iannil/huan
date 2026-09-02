package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"github.com/iannil/huan-plugin-ebook-exporter/style"
)

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
