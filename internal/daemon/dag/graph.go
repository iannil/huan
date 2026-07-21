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
//
// Edge direction (corrected — see rules.go):
//
//	Aggregator pages DEPEND ON the articles they aggregate. So when an
//	article changes, the aggregators that list it are reached by walking
//	DependedBy (who lists me) from the article.
//
// Concretely, this builds edges:
//
//	  section /<sec>/    → DependsOn every article page with Section == <sec>
//	  home    /          → DependsOn every section page
//	  term    /tags/<t>/ → DependsOn every article page whose Tags contains <t>
//
// Article pages ("page" kind) are leaves with no outgoing DependsOn edges.
func (dg *DependencyGraph) BuildFromSite(site *content.Site) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	dg.nodes = make(map[string]*Node)
	dg.sources = make(map[string]string)

	// First pass: create nodes for all pages (no edges yet).
	for _, pg := range site.Pages {
		url := pg.URL
		node := &Node{
			PagePath:   url,
			SourceFile: pg.RelPath,
			Kind:       pg.Kind,
			DependsOn:  []string{},
			DependedBy: []string{},
		}
		dg.nodes[url] = node
		if pg.RelPath != "" {
			dg.sources[pg.RelPath] = url
		}
	}

	// Second pass: compute forward edges based on aggregator semantics.
	// Index article pages by section and by tag so we can resolve
	// aggregator → articles without an O(N²) scan per aggregator.
	articlesBySection := make(map[string][]string) // section name → article URLs
	articlesByTag := make(map[string][]string)     // tag → article URLs
	sectionURLs := make([]string, 0)               // every section URL, for home's edges
	for _, pg := range site.Pages {
		switch pg.Kind {
		case "page":
			if pg.Section != "" {
				articlesBySection[pg.Section] = append(articlesBySection[pg.Section], pg.URL)
			}
			for _, tag := range pg.Tags {
				articlesByTag[tag] = append(articlesByTag[tag], pg.URL)
			}
		case "section":
			sectionURLs = append(sectionURLs, pg.URL)
		}
	}

	for _, pg := range site.Pages {
		node := dg.nodes[pg.URL]
		if node == nil {
			continue
		}
		switch pg.Kind {
		case "page":
			// Leaf: no forward dependencies.
		case "section":
			// Section list page renders its child articles.
			if arts, ok := articlesBySection[pg.Section]; ok {
				node.DependsOn = append(node.DependsOn, arts...)
			}
		case "home":
			// Home iterates sections; depend on every section page so that
			// editing an article flows home → section → article via DependedBy.
			node.DependsOn = append(node.DependsOn, sectionURLs...)
		case "term":
			// Term page URL looks like /tags/<tag>/. Extract the tag by
			// trimming the leading "/tags/" and trailing "/". This is
			// robust to the canonical tag URL form produced by the
			// content loader; unknown shapes fall back to no edges.
			tag := extractTermFromURL(pg.URL)
			if tag != "" {
				if arts, ok := articlesByTag[tag]; ok {
					node.DependsOn = append(node.DependsOn, arts...)
				}
			}
		case "taxonomy":
			// Taxonomy listing (e.g. /tags/) depends on every term page
			// below it. Not load-bearing for incremental correctness —
			// included for completeness.
			for _, other := range site.Pages {
				if other.Kind == "term" {
					node.DependsOn = append(node.DependsOn, other.URL)
				}
			}
		}
	}

	// Third pass: populate DependedBy (reverse edges).
	for _, node := range dg.nodes {
		for _, dep := range node.DependsOn {
			if target, ok := dg.nodes[dep]; ok {
				target.DependedBy = append(target.DependedBy, node.PagePath)
			}
		}
	}
}

// extractTermFromURL extracts the term name from a tag URL like
// "/tags/go/" → "go". Returns "" for URLs that don't match the /tags/<x>/
// shape.
func extractTermFromURL(url string) string {
	const prefix = "/tags/"
	const suffix = "/"
	if len(url) < len(prefix)+len(suffix) {
		return ""
	}
	if url[:len(prefix)] != prefix || url[len(url)-len(suffix):] != suffix {
		return ""
	}
	return url[len(prefix) : len(url)-len(suffix)]
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
// incremental build: dependencies first (e.g. the leaf article), then the
// aggregators that depend on them (e.g. the section and home pages that list
// the article).
//
// Rationale: an aggregator renders content drawn from its source articles, so
// the articles must be re-rendered (or at least sequenced) before the
// aggregators that list them. Concretely, for every dependency edge
// A -> B (A depends on B — i.e. A aggregates B), B is rendered before A.
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
	// A node is renderable once all of its dependencies have already been
	// rendered (dependencies first, then dependents).
	depsInSet := make(map[string][]string, len(known))
	for _, p := range known {
		node := dg.nodes[p]
		for _, dep := range node.DependsOn {
			if inSet[dep] {
				depsInSet[p] = append(depsInSet[p], dep)
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
			for _, dep := range depsInSet[p] {
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
