package dag

import (
	"fmt"
	"testing"

	"github.com/iannil/huan/internal/content"
)

// BenchmarkDAG_BuildFromSite measures DAG construction time at scale.
// Creates N pages and measures BuildFromSite + AffectedBy performance.
func BenchmarkDAG_BuildFromSite(b *testing.B) {
	site := buildLargeSite(1000)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dg := NewDependencyGraph()
		dg.BuildFromSite(site)
	}
}

// BenchmarkDAG_AffectedBy measures BFS traversal performance.
func BenchmarkDAG_AffectedBy(b *testing.B) {
	site := buildLargeSite(1000)
	dg := NewDependencyGraph()
	dg.BuildFromSite(site)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = dg.AffectedBy([]string{"posts/page-500.md"})
	}
}

// BenchmarkDAG_Serialize measures JSON serialization performance.
func BenchmarkDAG_Serialize(b *testing.B) {
	site := buildLargeSite(1000)
	dg := NewDependencyGraph()
	dg.BuildFromSite(site)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = dg.Serialize()
	}
}

// buildLargeSite creates a site with N pages for benchmarking.
// Home + 1 section + N pages under that section + 1 tag.
func buildLargeSite(n int) *content.Site {
	pages := make([]*content.Page, 0, n+3)
	pages = append(pages, &content.Page{URL: "/", Kind: "home", RelPath: ""})
	pages = append(pages, &content.Page{URL: "/posts/", Kind: "section", RelPath: "posts/_index.md", Section: "posts"})

	for i := 0; i < n; i++ {
		pages = append(pages, &content.Page{
			URL:      fmt.Sprintf("/posts/page-%d/", i),
			Kind:     "page",
			RelPath:  fmt.Sprintf("posts/page-%d.md", i),
			Section:  "posts",
			Tags:     []string{"go"},
		})
	}

	pages = append(pages, &content.Page{URL: "/tags/go/", Kind: "term", RelPath: "tags/go/_index.md"})
	return &content.Site{Pages: pages}
}