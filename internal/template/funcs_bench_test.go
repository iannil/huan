package template

import (
	"regexp"
	"strings"
	"testing"
)

func BenchmarkReplaceREWhitespaceCollapse(b *testing.B) {
	// Mirrors a long search-index body after plainify: Chinese and ASCII text,
	// paragraph boundaries, and indentation, repeated to page-scale input.
	src := strings.Repeat("祝融说的长篇正文\n\tcontains searchable English text。  \r\n下一段内容\f", 1_000)
	oracle := regexp.MustCompile(`\s+`).ReplaceAllString(src, " ")
	if got := replaceREFunc(`\s+`, " ", src); got != oracle {
		b.Fatalf("fast path differs from regexp: got %q, want %q", got, oracle)
	}

	b.Run("fast_path", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(src)))
		for i := 0; i < b.N; i++ {
			_ = replaceREFunc(`\s+`, " ", src)
		}
	})

	b.Run("regexp_oracle", func(b *testing.B) {
		re := regexp.MustCompile(`\s+`)
		b.ReportAllocs()
		b.SetBytes(int64(len(src)))
		for i := 0; i < b.N; i++ {
			_ = re.ReplaceAllString(src, " ")
		}
	})
}
