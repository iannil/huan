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
	SourceFile string   `json:"source_file"` // 源文件路径，如 content/posts/hello.md
	Kind       string   `json:"kind"`        // page / section / home / term
	DependsOn  []string `json:"depends_on"`  // 依赖的页面路径
	DependedBy []string `json:"depended_by"` // 被哪些页面依赖
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

// OrderByDependency returns the given page paths in rendering order for an
// incremental build: dependents first (e.g. a leaf article), then the pages
// they depend on (e.g. the section and home pages that list the article).
//
// Rationale: when an article changes, its own HTML must be re-rendered before
// the section/home list pages that reference it, so the lists reflect the
// updated article. In other words, for every dependency edge A -> B
// (A depends on B), A must be rendered before B.
//
// The sort is stable: among pages with no dependency relationship, the input
// order is preserved. Paths not present in the graph are appended at the end
// in input order.
func (dg *DependencyGraph) OrderByDependency(pagePaths []string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Partition input into known (present in dg.nodes) and unknown paths,
	// preserving input order within each. Per the spec, unknown paths are
	// appended at the end of the result in input order, regardless of where
	// they appear in the input slice.
	known := make([]string, 0, len(pagePaths))
	unknown := make([]string, 0)
	for _, p := range pagePaths {
		if _, ok := dg.nodes[p]; ok {
			known = append(known, p)
		} else {
			unknown = append(unknown, p)
		}
	}

	// Build the subgraph induced by known paths.
	inSet := make(map[string]bool, len(known))
	for _, p := range known {
		inSet[p] = true
	}

	// For each node, collect its forward dependencies that are also in the set.
	// A node is renderable once all nodes that depend on it (its dependers)
	// have already been rendered.
	dependers := make(map[string][]string, len(known))
	for _, p := range known {
		node := dg.nodes[p]
		for _, dep := range node.DependsOn {
			if inSet[dep] {
				// p depends on dep, so p must render before dep.
				dependers[dep] = append(dependers[dep], p)
			}
		}
	}

	// Kahn's algorithm with stable ordering (process in input order).
	rendered := make(map[string]bool)
	result := make([]string, 0, len(pagePaths))
	// Iterate multiple passes until no progress; this keeps input-order stability.
	for len(result) < len(known) {
		progress := false
		for _, p := range known {
			if rendered[p] {
				continue
			}
			ready := true
			for _, dep := range dependers[p] {
				if !rendered[dep] {
					ready = false
					break
				}
			}
			if ready {
				result = append(result, p)
				rendered[p] = true
				progress = true
			}
		}
		if !progress {
			// Cycle: append remaining known paths in input order.
			for _, p := range known {
				if !rendered[p] {
					result = append(result, p)
					rendered[p] = true
				}
			}
		}
	}

	// Append unknown paths at the end in input order.
	result = append(result, unknown...)
	return result
}
