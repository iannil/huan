package template

import (
	"regexp"
	"strings"
	"testing"
)

func FuzzStripTagsParity(f *testing.F) {
	for _, s := range []string{"plain", "<p>中文</p>\n<br />x", "a<<b>c>", "a<unclosed", "<x\ny>z", "<>x", "a\xff<b>\xfe", `<a title=">">x</a>`} {
		f.Add(s)
	}
	re := regexp.MustCompile(`<[^>]*>`)
	f.Fuzz(func(t *testing.T, s string) {
		if got, want := stripTags(s), re.ReplaceAllString(s, ""); got != want {
			t.Fatalf("input %q: got %q, want %q", s, got, want)
		}
	})
}

func FuzzTruncateParity(f *testing.F) {
	for _, s := range []string{"", "中文 hello 🌍", "a\xff\xfez"} {
		f.Add(s, uint8(2))
	}
	f.Fuzz(func(t *testing.T, s string, n uint8) {
		want := s
		if runes := []rune(s); len(runes) > int(n) {
			want = string(runes[:n]) + "…"
		}
		if got := truncateFunc(int(n), s); got != want {
			t.Fatalf("input %q, n=%d: got %q, want %q", s, n, got, want)
		}
	})
}

func TestStripTagsAllocationBudget(t *testing.T) {
	src := strings.Repeat("<p>正文 and text</p>\n", 1000)
	allocs := testing.AllocsPerRun(20, func() { _ = stripTags(src) })
	if allocs > 1 {
		t.Fatalf("tag stripping allocates %.0f times; want at most one output buffer", allocs)
	}
}

func BenchmarkSearchText(b *testing.B) {
	src := strings.Repeat("<p>祝融说的长篇正文 contains searchable English text。</p>\n", 1000)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = truncateFunc(800, replaceREFunc(`\s+`, " ", plainify(src)))
	}
}
