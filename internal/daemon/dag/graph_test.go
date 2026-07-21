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

func TestOrderByDependency_LeafBeforeParent(t *testing.T) {
	dg := NewDependencyGraph()
	// Build a minimal graph: /posts/hello/ depends on /posts/ and /
	// (so /posts/ and / are "depended by" /posts/hello/ in reverse edges).
	dg.nodes["/posts/hello/"] = &Node{
		PagePath:   "/posts/hello/",
		DependsOn:  []string{"/posts/", "/"},
		DependedBy: []string{},
	}
	dg.nodes["/posts/"] = &Node{
		PagePath:   "/posts/",
		DependsOn:  []string{"/"},
		DependedBy: []string{"/posts/hello/"},
	}
	dg.nodes["/"] = &Node{
		PagePath:   "/",
		DependsOn:  []string{},
		DependedBy: []string{"/posts/hello/", "/posts/"},
	}

	affected := []string{"/posts/hello/", "/posts/", "/"}
	ordered := dg.OrderByDependency(affected)

	// The leaf (/posts/hello/) must come before pages that depend on it.
	helloIdx := indexOf(ordered, "/posts/hello/")
	postsIdx := indexOf(ordered, "/posts/")
	homeIdx := indexOf(ordered, "/")
	if helloIdx > postsIdx {
		t.Errorf("leaf /posts/hello/ (idx %d) must come before /posts/ (idx %d)", helloIdx, postsIdx)
	}
	if helloIdx > homeIdx {
		t.Errorf("leaf /posts/hello/ (idx %d) must come before / (idx %d)", helloIdx, homeIdx)
	}
}

func TestOrderByDependency_SinglePage(t *testing.T) {
	dg := NewDependencyGraph()
	dg.nodes["/only/"] = &Node{PagePath: "/only/"}
	ordered := dg.OrderByDependency([]string{"/only/"})
	if len(ordered) != 1 || ordered[0] != "/only/" {
		t.Errorf("OrderByDependency single = %v, want [/only/]", ordered)
	}
}

func TestOrderByDependency_Empty(t *testing.T) {
	dg := NewDependencyGraph()
	ordered := dg.OrderByDependency([]string{})
	if len(ordered) != 0 {
		t.Errorf("OrderByDependency empty = %v, want []", ordered)
	}
}

func TestOrderByDependency_UnknownPathsPreserved(t *testing.T) {
	dg := NewDependencyGraph()
	// Path not in graph should still appear in output.
	ordered := dg.OrderByDependency([]string{"/unknown/"})
	if len(ordered) != 1 || ordered[0] != "/unknown/" {
		t.Errorf("unknown path = %v, want [/unknown/]", ordered)
	}
}

func TestOrderByDependency_UnknownPathsAtEnd(t *testing.T) {
	dg := NewDependencyGraph()
	dg.nodes["/leaf/"] = &Node{PagePath: "/leaf/", DependsOn: []string{"/parent/"}}
	dg.nodes["/parent/"] = &Node{PagePath: "/parent/", DependsOn: []string{}, DependedBy: []string{"/leaf/"}}

	// Mixed input: known leaf, unknown, known parent, unknown
	ordered := dg.OrderByDependency([]string{"/leaf/", "/mystery1/", "/parent/", "/mystery2/"})

	// All known paths must come before all unknown paths.
	leafIdx := indexOf(ordered, "/leaf/")
	parentIdx := indexOf(ordered, "/parent/")
	mystery1Idx := indexOf(ordered, "/mystery1/")
	mystery2Idx := indexOf(ordered, "/mystery2/")
	maxKnown := leafIdx
	if parentIdx > maxKnown {
		maxKnown = parentIdx
	}
	minUnknown := mystery1Idx
	if mystery2Idx < minUnknown {
		minUnknown = mystery2Idx
	}
	if maxKnown > minUnknown {
		t.Errorf("unknown paths must come after all known paths; known max idx %d, unknown min idx %d, order: %v", maxKnown, minUnknown, ordered)
	}
}

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}
