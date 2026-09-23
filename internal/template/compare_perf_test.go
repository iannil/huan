package template

import (
	"fmt"
	"html/template"
	"io"
	"strings"
	"testing"
)

type comparisonStringer string

func (v comparisonStringer) String() string { return "formatted:" + string(v) }

type comparisonFormatter string

func (v comparisonFormatter) Format(w fmt.State, verb rune) {
	_, _ = io.WriteString(w, "custom-format")
}

func referenceCompare(a, b interface{}) int {
	as, bs := fmt.Sprintf("%v", a), fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

func TestCompareStringFastPathDoesNotAllocate(t *testing.T) {
	for _, a := range []interface{}{strings.Repeat("a", 2048), template.HTML(strings.Repeat("<p>content</p>", 200))} {
		allocs := testing.AllocsPerRun(20, func() {
			if compare(a, "") <= 0 {
				t.Fatal("nonempty text must compare above empty text")
			}
		})
		if allocs != 0 {
			t.Errorf("%T comparison allocated %g times, want 0", a, allocs)
		}
	}
}

func TestComparePreservesFormattingAndLexicalOrder(t *testing.T) {
	var nilStringer *comparisonStringer
	values := []interface{}{
		"", "2", "10", "<nil>", "custom-format", "formatted:a",
		template.HTML("2"), template.HTML("<b>a</b>"),
		nil, nilStringer, 2, 10, float64(1.5), true,
		comparisonStringer("a"), comparisonFormatter("z"),
		[]byte("abc"), []string{"a", "b"}, map[string]int{"x": 1},
	}
	for _, a := range values {
		for _, b := range values {
			if got, want := compare(a, b), referenceCompare(a, b); got != want {
				t.Fatalf("compare(%#v, %#v) = %d, want %d", a, b, got, want)
			}
		}
	}
	if compare(2, 10) != 1 {
		t.Fatal("numeric values must retain lexical comparison")
	}
}

func FuzzCompareMatchesFormattingReference(f *testing.F) {
	f.Add("<p>中文 &amp;</p>", "<p>x</p>")
	f.Add("\x00\xff\xfe", "\n")
	f.Add("", "")
	f.Fuzz(func(t *testing.T, a, b string) {
		valuesA := []interface{}{a, template.HTML(a), comparisonStringer(a), comparisonFormatter(a)}
		valuesB := []interface{}{b, template.HTML(b), comparisonStringer(b), comparisonFormatter(b)}
		for _, av := range valuesA {
			for _, bv := range valuesB {
				if got, want := compare(av, bv), referenceCompare(av, bv); got != want {
					t.Fatalf("compare(%#v,%#v)=%d,want%d", av, bv, got, want)
				}
			}
		}
	})
}

func BenchmarkCompareText(b *testing.B) {
	for name, value := range map[string]interface{}{"short": "weight", "content": template.HTML(strings.Repeat(`<p>Article body with 中文。</p>`, 1000))} {
		for impl, fn := range map[string]func(interface{}, interface{}) int{"reference": referenceCompare, "optimized": compare} {
			b.Run(name+"/"+impl, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if fn(value, "") != 1 {
						b.Fatal("incorrect comparison")
					}
				}
			})
		}
	}
}
