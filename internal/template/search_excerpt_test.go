package template

import (
	"html/template"
	"strings"
	"testing"
)

func searchExcerptOracle(length int, input interface{}) string {
	return truncateFunc(length, replaceREFunc(`\s+`, " ", plainify(input)))
}

func TestSearchExcerptParity(t *testing.T) {
	inputs := []interface{}{
		nil, 42, []byte("<p>中文</p>"), template.HTML("<p>trusted &amp; text</p>"),
		"", "exact", "plain\u2003 \v\ttext", "plain\xff\xfeinvalid",
		"<p>中文</p>\n<br />new<br>line", "<p> \u2003\t\vtext</p>",
		"a<<b>c>", "a<unclosed", "<x\ny>z", `<a title=">">x</a>`,
		"<oops </p>trailing", "<oops <br>trailing", "__<i>_hugonl_\n",
		"___hugonl_<p>x</p>", "___hugonl_>", "<b>___hugonl_</b>",
		"a\xc3<b>\xa9z", "a\xff<b>\xfe", "a<b>\xffend", "a>\u2003 \tx",
		strings.Repeat("<p>正文 text &amp; <strong>bold</strong></p>\n", 100),
	}
	fn := getFunc(t, "searchExcerpt").(func(int, interface{}) string)
	for _, input := range inputs {
		for _, length := range []int{0, 1, 2, 5, 12, 800, 10000} {
			if got, want := fn(length, input), searchExcerptOracle(length, input); got != want {
				t.Fatalf("length=%d input=%q got=%q want=%q", length, input, got, want)
			}
		}
	}
}

func TestSearchExcerptNegativeLength(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("negative limit must panic")
		}
	}()
	searchExcerpt(-1, "text")
}

func FuzzSearchExcerptParity(f *testing.F) {
	for _, s := range []string{"", "plain\u2003 \v\ttext", "plain\xffinvalid", "<p>正文</p>\n<br />more", "<oops </p>tail", "__<i>_hugonl_\n", "\xc3<b>\xa9", `<a title=">">x</a>`} {
		f.Add(s, uint16(8))
	}
	f.Fuzz(func(t *testing.T, s string, n uint16) {
		if got, want := searchExcerpt(int(n), s), searchExcerptOracle(int(n), s); got != want {
			t.Fatalf("input=%q length=%d got=%q want=%q", s, n, got, want)
		}
	})
}

func BenchmarkSearchExcerpt(b *testing.B) {
	for _, count := range []int{10, 1000} {
		src := strings.Repeat("<p>祝融说的长篇正文 contains searchable <strong>English</strong> text。</p>\n", count)
		name := "short"
		if count == 1000 {
			name = "long"
		}
		for _, fused := range []bool{false, true} {
			method := "composition"
			if fused {
				method = "fused"
			}
			b.Run(name+"/"+method, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if fused {
						_ = searchExcerpt(800, src)
					} else {
						_ = searchExcerptOracle(800, src)
					}
				}
			})
		}
	}
}
