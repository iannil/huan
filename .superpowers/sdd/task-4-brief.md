### Task 4: DAG — OrderByDependency 方法

**Files:**
- Modify: `internal/daemon/dag/graph.go` — 新增 OrderByDependency
- Modify: `internal/daemon/dag/graph_test.go` — 测试

**Interfaces:**
- Produces: `DependencyGraph.OrderByDependency(pagePaths []string) []string`

**说明：** 返回拓扑排序后的页面路径（被依赖的页面在前，依赖它们的页面在后）。例如：先渲染 `/posts/hello/`，再渲染引用它的 `/posts/`（section）和 `/`（home）。

- [ ] **Step 1: 编写测试**

在 `internal/daemon/dag/graph_test.go` 末尾添加：

```go
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

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/dag/ -run "TestOrderByDependency" -v
```
Expected: COMPILATION ERROR (no OrderByDependency method)

- [ ] **Step 3: 实现 OrderByDependency**

在 `internal/daemon/dag/graph.go` 的 `PagePathFromSource` 方法之后添加：

```go
// OrderByDependency returns the given page paths in topological order:
// pages that are depended-upon come first, pages that depend on them come
// later. This is the correct rendering order for incremental builds — a
// leaf page is re-rendered before the section/home pages that list it.
//
// The sort is stable: among pages with no dependency relationship, the
// input order is preserved. Paths not present in the graph are appended
// at the end in input order.
func (dg *DependencyGraph) OrderByDependency(pagePaths []string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Build the subgraph induced by pagePaths.
	inSet := make(map[string]bool, len(pagePaths))
	for _, p := range pagePaths {
		inSet[p] = true
	}

	// For each node, collect its forward dependencies that are also in the set.
	// We render a node only after all its in-set dependencies are rendered.
	deps := make(map[string][]string, len(pagePaths))
	for _, p := range pagePaths {
		node, ok := dg.nodes[p]
		if !ok {
			deps[p] = nil
			continue
		}
		for _, dep := range node.DependsOn {
			if inSet[dep] {
				deps[p] = append(deps[p], dep)
			}
		}
	}

	// Kahn's algorithm with stable ordering (process in input order).
	rendered := make(map[string]bool)
	var result []string
	// Iterate multiple passes until no progress; this keeps input-order stability.
	for len(result) < len(pagePaths) {
		progress := false
		for _, p := range pagePaths {
			if rendered[p] {
				continue
			}
			ready := true
			for _, dep := range deps[p] {
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
			// Cycle or unknown deps: append remaining in input order.
			for _, p := range pagePaths {
				if !rendered[p] {
					result = append(result, p)
					rendered[p] = true
				}
			}
		}
	}
	return result
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/dag/ -run "TestOrderByDependency" -v
```
Expected: ALL PASS

- [ ] **Step 5: 运行 DAG 全部测试确保无回归**

```bash
go test ./internal/daemon/dag/... -v
```
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/daemon/dag/graph.go internal/daemon/dag/graph_test.go
git commit -m "feat(dag): add OrderByDependency for incremental build ordering"
```

---

