package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const completeSEOHead = `<head><title>Page title</title><meta name=description content=existing><meta property=og:description content=existing><meta name=twitter:description content=existing><meta property=og:title content=existing><meta property=og:url content=existing><meta property=og:type content=article><meta name=twitter:card content=summary><meta name=twitter:title content=existing></head>`

func TestCompleteSEOAvoidsBodyTextAllocation(t *testing.T) {
	src := `<html>` + completeSEOHead + `<body>` + strings.Repeat(`<p>A paragraph that should never need description extraction.</p>`, 200) + `</body></html>`
	measure := func(fn func(string, *InjectOptions) (string, error)) float64 {
		return testing.AllocsPerRun(10, func() {
			opts := &InjectOptions{PageTitle: "Title", PageURL: "/post/"}
			got, err := fn(src, opts)
			if err != nil || got != src {
				t.Fatalf("complete SEO output changed: %v", err)
			}
		})
	}
	baseline := measure(referenceInjectHTML)
	optimized := measure(InjectHTML)
	if optimized >= baseline-3 {
		t.Fatalf("body extraction still allocated: optimized %g, eager %g", optimized, baseline)
	}
}

func FuzzSEOInjectionMatchesReference(f *testing.F) {
	for _, src := range []string{
		`<html><head><title>Title &amp; more</title></head><body><nav>skip</nav><p>Hello <b>world</b> &amp; 中文.</p><script>ignored</script></body></html>`,
		`<html>` + completeSEOHead + `<body><p>Unneeded text</p></body></html>`,
		`<head></head><body><table>foster parent <tr><td>cell</table><footer>skip</footer> tail`,
		`<title>orphan</title><p>No literal head closing tag`,
		`<head><meta property="og:description" content="keep"></head><body><header>skip</header><div>Text</div>tail`,
		`<head></head><body>First<body>Second<meta name=description content=late>`,
		``,
	} {
		f.Add(src, "Page title", "/entry/", 160, true)
	}
	f.Fuzz(func(t *testing.T, src, title, url string, limit int, og bool) {
		if len(src) > 8192 || len(title) > 1024 || len(url) > 1024 {
			t.Skip()
		}
		// Keep the existing truncation semantics while bounding pathological sizes.
		limit = limit % 512
		opts := InjectOptions{PageTitle: title, PageURL: url, DescriptionMaxLength: limit, InjectOG: og, InjectTwitter: !og, PageKind: "page"}
		oldOpts := opts
		want, oldErr := referenceInjectHTML(src, &oldOpts)
		got, err := InjectHTML(src, &opts)
		if (err != nil) != (oldErr != nil) || got != want {
			t.Fatalf("src %q\ngot %q (%v)\nwant %q (%v)", src, got, err, want, oldErr)
		}
		if got := ExtractPlainText(src); got != referenceAnalyzeHTML(src).bodyText {
			t.Fatalf("plain text changed: %q", got)
		}
	})
}

func TestSEOProcessFileMatchesEagerOutput(t *testing.T) {
	root := t.TempDir()
	src := `<html><head><title>Page &amp; Title</title></head><body><p>Content with spaces and 中文</p></body></html>`
	path := filepath.Join(root, "index.html")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := &InjectOptions{PageTitle: "Page & Title", PageURL: "/index.html", PageKind: "home", DescriptionMaxLength: 160, InjectOG: true, InjectTwitter: true}
	want, err := referenceInjectHTML(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil)
	if err := p.processFile(path, root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("output mismatch: %v\ngot %q\nwant %q", err, got, want)
	}
	if err := p.processFile(path, root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || string(second) != want {
		t.Fatal("second run changed output")
	}
}

func BenchmarkSEOInjectionCompared(b *testing.B) {
	for _, complete := range []bool{false, true} {
		head := `<head><title>Page title</title></head>`
		name := "missing-tags"
		if complete {
			head = completeSEOHead
			name = "complete-tags"
		}
		src := `<html>` + head + `<body>` + strings.Repeat(`<p>Paragraph with <strong>formatting</strong> and text 中文。</p>`, 200) + `</body></html>`
		for impl, fn := range map[string]func(string, *InjectOptions) (string, error){"reference": referenceInjectHTML, "optimized": InjectHTML} {
			b.Run(name+"/"+impl, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(src)))
				for i := 0; i < b.N; i++ {
					opts := &InjectOptions{PageTitle: "Page title", PageURL: "/entry/"}
					if _, err := fn(src, opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkSEOProcessFileCompared(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "index.html")
	src := `<html>` + completeSEOHead + `<body>` + strings.Repeat(`<p>Paragraph with <strong>formatting</strong> and text 中文。</p>`, 200) + `</body></html>`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		b.Fatal(err)
	}
	p := New(nil)
	for impl, fn := range map[string]func(string, string) error{"reference": p.referenceProcessFile, "optimized": p.processFile} {
		b.Run(impl, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			for i := 0; i < b.N; i++ {
				if err := fn(path, root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
