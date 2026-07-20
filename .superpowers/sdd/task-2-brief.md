### Task 2: DAG 依赖图（graph.go + rules.go）

**Files:**
- Create: `internal/daemon/dag/graph.go`
- Create: `internal/daemon/dag/rules.go`
- Create: `internal/daemon/dag/graph_test.go`

**Interfaces:**
- Consumes: `content.Page` 类型（来自 `internal/content`）
- Produces: `DependencyGraph` 类型, `Node` 类型, `BuildFromSite()`, `AffectedBy()`, `Serialize()`, `Deserialize()`

- [ ] **Step 1: Write rules.go — 依赖规则推导**

```go
package dag

import "github.com/iannil/huan/internal/content"

// PageDependencies returns the list of page paths that a given page depends on.
// The dependency graph is built by traversing these edges.
func PageDependencies(pg *content.Page) []string {
	deps := make([]string, 0, 8)

	switch pg.Kind {
	case "page":
		// Article page depends on its section list page, home page, and tag pages.
		if pg.Section != "" {
			deps = append(deps, "/"+pg.Section+"/")
		}
		deps = append(deps, "/")
		for _, tag := range pg.Tags {
			deps = append(deps, "/tags/"+tag+"/")
		}
	case "section":
		// Section list page contains its child pages. Dependencies are
		// reverse: the section page is depended on by its children.
		// Section itself depends on home page.
		deps = append(deps, "/")
	case "home":
		// Home page depends on all published pages (reverse edges).
		// No forward dependencies.
	case "term":
		// Term page depends on home page.
		deps = append(deps, "/")
	}

	return deps
}

// IsReverseDependency reports whether a dependency is inherently "reverse"
// (i.e., A depends on the existence of B, but changes to A don't affect B).
// Such edges are not traversed in the forward direction during incremental builds.
func IsReverseDependency(from, to string) bool {
	// Section pages are "reverse" dependencies of their children:
	// changing a section page doesn't affect article pages.
	return false
}
```

- [ ] **Step 2: Write graph.go — DependencyGraph 构建 + BFS + 序列化**

```go
package dag

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/iannil/huan/internal/content"
)

// Node represents a single page in the dependency graph.
type Node struct {
	PagePath   string   `json:"page_path"`   // 页面路径，如 /posts/hello/
	SourceFile string   `json:"source_file"`  // 源文件路径，如 content/posts/hello.md
	Kind       string   `json:"kind"`          // page / section / home / term
	DependsOn  []string `json:"depends_on"`   // 依赖的页面路径
	DependedBy []string `json:"depended_by"`  // 被哪些页面依赖
}

// DependencyGraph tracks page-page dependencies for incremental rebuilds.
type DependencyGraph struct {
	mu      sync.RWMutex
	nodes   map[string]*Node  // 页面路径 → 节点
	sources map[string]string // 源文件路径 → 页面路径
}

// NewDependencyGraph creates an empty graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes:   make(map[string]*Node),
		sources: make(map[string]string),
	}
}

// BuildFromSite constructs the dependency graph from a fully built site.
// Called after a full build completes.
func (dg *DependencyGraph) BuildFromSite(site *content.Site) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	dg.nodes = make(map[string]*Node)
	dg.sources = make(map[string]string)

	// First pass: create nodes for all pages.
	for _, pg := range site.Pages {
		url := pg.URL
		node := &Node{
			PagePath:   url,
			SourceFile: pg.RelPath,
			Kind:       pg.Kind,
			DependsOn:  PageDependencies(pg),
			DependedBy: []string{},
		}
		dg.nodes[url] = node
		if pg.RelPath != "" {
			dg.sources[pg.RelPath] = url
		}
	}

	// Second pass: populate DependedBy (reverse edges).
	for _, node := range dg.nodes {
		for _, dep := range node.DependsOn {
			if target, ok := dg.nodes[dep]; ok {
				target.DependedBy = append(target.DependedBy, node.PagePath)
			}
		}
	}
}

// AffectedBy returns the set of page paths that need to be rebuilt when
// the given source files change. Uses BFS on DependedBy edges.
func (dg *DependencyGraph) AffectedBy(changedSourceFiles []string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	visited := make(map[string]bool)
	queue := make([]string, 0)

	// Seed queue with page nodes corresponding to changed source files.
	for _, sf := range changedSourceFiles {
		if pagePath, ok := dg.sources[sf]; ok {
			if !visited[pagePath] {
				visited[pagePath] = true
				queue = append(queue, pagePath)
			}
		}
	}

	// BFS along DependedBy edges (reverse direction).
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		node, ok := dg.nodes[current]
		if !ok {
			continue
		}
		for _, depender := range node.DependedBy {
			if !visited[depender] {
				visited[depender] = true
				queue = append(queue, depender)
			}
		}
	}

	result := make([]string, 0, len(visited))
	for path := range visited {
		result = append(result, path)
	}
	return result
}

// Serialize encodes the graph to JSON bytes.
func (dg *DependencyGraph) Serialize() ([]byte, error) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	data := map[string]interface{}{
		"nodes":   dg.nodes,
		"sources": dg.sources,
	}
	return json.Marshal(data)
}

// Deserialize decodes JSON bytes into the graph.
func (dg *DependencyGraph) Deserialize(data []byte) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	var raw struct {
		Nodes   map[string]*Node  `json:"nodes"`
		Sources map[string]string `json:"sources"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("dag deserialize: %w", err)
	}
	dg.nodes = raw.Nodes
	dg.sources = raw.Sources
	if dg.nodes == nil {
		dg.nodes = make(map[string]*Node)
	}
	if dg.sources == nil {
		dg.sources = make(map[string]string)
	}
	return nil
}

// NodeCount returns the number of nodes in the graph.
func (dg *DependencyGraph) NodeCount() int {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	return len(dg.nodes)
}

// PagePathFromSource maps a source file relative path to its page URL.
func (dg *DependencyGraph) PagePathFromSource(sourceFile string) (string, bool) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	path, ok := dg.sources[sourceFile]
	return path, ok
}
```

- [ ] **Step 3: Run `go vet` to verify DAG compiles**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go vet ./internal/daemon/dag/...
```
Expected: no errors

- [ ] **Step 4: Write graph_test.go — DAG 构建/BFS/序列化**

```go
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

	// Changing posts/hello.md should affect: itself, /posts/, /, /tags/go/
	affected := dg.AffectedBy([]string{"posts/hello.md"})
	if len(affected) < 1 {
		t.Fatal("expected at least 1 affected page")
	}

	// Check that home page is affected
	homeFound := false
	for _, a := range affected {
		if a == "/" {
			homeFound = true
		}
	}
	if !homeFound {
		t.Error("changing a post should affect home page")
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
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && go test ./internal/daemon/dag/... -v -count=1
```
Expected: 4 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/dag/
git commit -m "feat(daemon): add DependencyGraph (DAG) for incremental rebuild

- Node 定义：PagePath/SourceFile/Kind/DependsOn/DependedBy
- BuildFromSite：从 site.Pages 构建完整依赖图
- AffectedBy：BFS 反向遍历，收集内容变更影响的所有页面
- 依赖推导规则：page→section/home/tags, section→home
- 序列化/反序列化：持久化到 JSON，支持 daemon 重启恢复
- 4 个测试覆盖：构建/BFS/序列化/空图

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

