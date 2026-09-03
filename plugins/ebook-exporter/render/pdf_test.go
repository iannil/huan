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
