package build

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestMarkdownContentKeyPreservesRawExpandedAndMarkup(t *testing.T) {
	const bodyHash = "230d8358dc8e8890b4c58deeb62912ee2f20357ae92a5cc861b98e68fe31acb5"
	for _, tc := range []struct{ name, raw, expanded, rawHash, expandedHash string }{
		{"identical", "body", "body", bodyHash, bodyHash},
		{"equal independent strings", strings.Clone("body"), strings.Clone("body"), bodyHash, bodyHash},
		{"changed expansion", "body", "expanded", bodyHash, "691661b386fd34d08d9e14f20e3a4dffa5d74f03492e3a7ce8a6e648d25a28a3"},
		{"empty expansion", "body", "", bodyHash, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"shortcode", "{{< dynamic >}}", "body", "4830d030c0445abb832a9f70cdc45f4d86138ddf876be8bae580873bf7a478c9", bodyHash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			markup := [sha256.Size]byte{17}
			got := markdownContentKey(tc.raw, tc.expanded, markup)
			if fmt.Sprintf("%x", got.Raw) != tc.rawHash || fmt.Sprintf("%x", got.Expanded) != tc.expandedHash || got.Markup != markup {
				t.Fatalf("wrong key: %+v", got)
			}
		})
	}
}

var markdownKeyBenchmarkResult markdownKey

func BenchmarkMarkdownContentKey(b *testing.B) {
	raw := strings.Repeat("中文 Markdown **body** and [link](https://example.com).\n", 1000)
	for _, expanded := range []struct{ name, value string }{{"unchanged", raw}, {"changed", raw + "expanded"}} {
		for _, optimized := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/reuse=%v", expanded.name, optimized), func(b *testing.B) {
				markup := [sha256.Size]byte{17}
				b.ReportAllocs()
				b.SetBytes(int64(len(raw) + len(expanded.value)))
				for i := 0; i < b.N; i++ {
					if optimized {
						markdownKeyBenchmarkResult = markdownContentKey(raw, expanded.value, markup)
					} else {
						markdownKeyBenchmarkResult = markdownKey{Raw: sha256.Sum256([]byte(raw)), Expanded: sha256.Sum256([]byte(expanded.value)), Markup: markup}
					}
				}
			})
		}
	}
}
