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

	// Article is a leaf: no outgoing DependsOn edges.
	article, ok := dg.nodes["/posts/hello/"]
	if !ok {
		t.Fatal("expected node for /posts/hello/")
	}
	if article.Kind != "page" {
		t.Errorf("expected kind=page, got %s", article.Kind)
	}
	if len(article.DependsOn) != 0 {
		t.Errorf("article should be a leaf (no DependsOn), got %v", article.DependsOn)
	}

	// Section DependsOn the article(s) in that section.
	section := dg.nodes["/posts/"]
	foundArticle := false
	for _, dep := range section.DependsOn {
		if dep == "/posts/hello/" {
			foundArticle = true
		}
	}
	if !foundArticle {
		t.Error("/posts/ section should depend on /posts/hello/")
	}

	// Tag/term DependsOn the article(s) tagged with it.
	term := dg.nodes["/tags/go/"]
	foundTaggedArticle := false
	for _, dep := range term.DependsOn {
		if dep == "/posts/hello/" {
			foundTaggedArticle = true
		}
	}
	if !foundTaggedArticle {
		t.Error("/tags/go/ term should depend on tagged article /posts/hello/")
	}

	// Home DependsOn every section.
	home := dg.nodes["/"]
	foundSection := false
	for _, dep := range home.DependsOn {
		if dep == "/posts/" {
			foundSection = true
		}
	}
	if !foundSection {
		t.Error("/ home should depend on /posts/ section")
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

	// Changing posts/hello.md must affect: the article itself PLUS every
	// aggregator that lists it (section, home via section, tag term).
	affected := dg.AffectedBy([]string{"posts/hello.md"})

	want := map[string]bool{
		"/posts/hello/": true,
		"/posts/":       true,
		"/tags/go/":     true,
		"/":             true,
	}
	if len(affected) != len(want) {
		t.Fatalf("expected %d affected pages, got %d: %v", len(want), len(affected), affected)
	}
	for _, a := range affected {
		if !want[a] {
			t.Errorf("unexpected affected page: %s", a)
		}
	}
	for w := range want {
		found := false
		for _, a := range affected {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be affected, got: %v", w, affected)
		}
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
	// Corrected semantics: section/home DEPEND ON the article.
	//   /posts/hello/ is a leaf (no DependsOn).
	//   /posts/ depends on /posts/hello/ (aggregates the article).
	//   / depends on /posts/ (home aggregates sections).
	// Render order: leaf (article) first, then section, then home.
	dg.nodes["/posts/hello/"] = &Node{
		PagePath:   "/posts/hello/",
		DependsOn:  []string{},
		DependedBy: []string{"/posts/"},
	}
	dg.nodes["/posts/"] = &Node{
		PagePath:   "/posts/",
		DependsOn:  []string{"/posts/hello/"},
		DependedBy: []string{"/"},
	}
	dg.nodes["/"] = &Node{
		PagePath:   "/",
		DependsOn:  []string{"/posts/"},
		DependedBy: []string{},
	}

	affected := []string{"/posts/hello/", "/posts/", "/"}
	ordered := dg.OrderByDependency(affected)

	// The leaf (/posts/hello/) must come before the aggregators that
	// depend on it.
	helloIdx := indexOf(ordered, "/posts/hello/")
	postsIdx := indexOf(ordered, "/posts/")
	homeIdx := indexOf(ordered, "/")
	if helloIdx > postsIdx {
		t.Errorf("leaf /posts/hello/ (idx %d) must come before /posts/ (idx %d)", helloIdx, postsIdx)
	}
	if postsIdx > homeIdx {
		t.Errorf("/posts/ (idx %d) must come before home / (idx %d)", postsIdx, homeIdx)
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

func TestSourceFromPagePath_Found(t *testing.T) {
	dg := NewDependencyGraph()
	dg.nodes["/posts/hello/"] = &Node{
		PagePath:   "/posts/hello/",
		SourceFile: "posts/hello.md",
	}

	src, ok := dg.SourceFromPagePath("/posts/hello/")
	if !ok {
		t.Fatal("SourceFromPagePath: expected ok=true for existing node")
	}
	if src != "posts/hello.md" {
		t.Errorf("src = %q, want posts/hello.md", src)
	}
}

func TestSourceFromPagePath_NotFound(t *testing.T) {
	dg := NewDependencyGraph()
	src, ok := dg.SourceFromPagePath("/nonexistent/")
	if ok {
		t.Error("expected ok=false for missing node")
	}
	if src != "" {
		t.Errorf("src = %q, want empty", src)
	}
}

func TestSourceFromPagePath_EmptySourceFile(t *testing.T) {
	dg := NewDependencyGraph()
	// Node exists but has no SourceFile (shouldn't normally happen)
	dg.nodes["/weird/"] = &Node{PagePath: "/weird/", SourceFile: ""}
	src, ok := dg.SourceFromPagePath("/weird/")
	if ok {
		t.Error("expected ok=false when SourceFile is empty")
	}
	if src != "" {
		t.Errorf("src = %q, want empty", src)
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
