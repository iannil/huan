package dag

import (
	"testing"

	"github.com/iannil/huan/internal/content"
)

func TestBuildFromSite(t *testing.T) {
	site := &content.Site{
		Pages: []*content.Page{
			{URL: "/", Kind: "home", RelPath: ""},
			{URL: "/posts/", Kind: "section", RelPath: "posts/_index.md", Section: "posts"},
			{URL: "/posts/hello/", Kind: "page", RelPath: "posts/hello.md", Section: "posts", Tags: []string{"go"}},
			{URL: "/tags/go/", Kind: "term", RelPath: "tags/go/_index.md"},
		},
	}

	dg := NewDependencyGraph()
	dg.BuildFromSite(site)

	if n := dg.NodeCount(); n != 4 {
		t.Errorf("expected 4 nodes, got %d", n)
	}

	// Check /posts/hello/ depends on section, home, and tag
	node, ok := dg.nodes["/posts/hello/"]
	if !ok {
		t.Fatal("expected node for /posts/hello/")
	}
	if node.Kind != "page" {
		t.Errorf("expected kind=page, got %s", node.Kind)
	}
	foundSection := false
	for _, dep := range node.DependsOn {
		if dep == "/posts/" {
			foundSection = true
		}
	}
	if !foundSection {
		t.Error("/posts/hello/ should depend on /posts/")
	}
}

func TestAffectedBy_PageChange(t *testing.T) {
	site := &content.Site{
		Pages: []*content.Page{
			{URL: "/", Kind: "home", RelPath: ""},
			{URL: "/posts/", Kind: "section", RelPath: "posts/_index.md", Section: "posts"},
			{URL: "/posts/hello/", Kind: "page", RelPath: "posts/hello.md", Section: "posts", Tags: []string{"go"}},
			{URL: "/tags/go/", Kind: "term", RelPath: "tags/go/_index.md"},
		},
	}

	dg := NewDependencyGraph()
	dg.BuildFromSite(site)

	// Changing posts/hello.md should affect: itself
	// Note: home page is NOT affected because it has no DependedBy edges from /posts/hello/
	// The dependency direction is: /posts/hello/ -> /, not / -> /posts/hello/
	affected := dg.AffectedBy([]string{"posts/hello.md"})
	if len(affected) != 1 {
		t.Fatalf("expected 1 affected page, got %d: %v", len(affected), affected)
	}

	// The affected page should be /posts/hello/ itself
	if affected[0] != "/posts/hello/" {
		t.Errorf("expected /posts/hello/ to be affected, got %s", affected[0])
	}
}

func TestSerializeDeserialize(t *testing.T) {
	site := &content.Site{
		Pages: []*content.Page{
			{URL: "/", Kind: "home", RelPath: ""},
			{URL: "/posts/", Kind: "section", RelPath: "posts/_index.md", Section: "posts"},
			{URL: "/posts/hello/", Kind: "page", RelPath: "posts/hello.md", Section: "posts"},
		},
	}

	dg := NewDependencyGraph()
	dg.BuildFromSite(site)

	data, err := dg.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	dg2 := NewDependencyGraph()
	if err := dg2.Deserialize(data); err != nil {
		t.Fatalf("Deserialize: %v", err)
	}

	if dg2.NodeCount() != 3 {
		t.Errorf("expected 3 nodes after deserialize, got %d", dg2.NodeCount())
	}
}

func TestEmptyGraph(t *testing.T) {
	dg := NewDependencyGraph()
	if n := dg.NodeCount(); n != 0 {
		t.Errorf("expected 0 nodes, got %d", n)
	}
	affected := dg.AffectedBy([]string{"nonexistent.md"})
	if len(affected) != 0 {
		t.Errorf("expected 0 affected, got %d", len(affected))
	}
}
