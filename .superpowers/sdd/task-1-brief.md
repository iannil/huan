### Task 1: DAG — SourceFromPagePath 反向查找

**Files:**
- Modify: `internal/daemon/dag/graph.go` — 新增 SourceFromPagePath 方法
- Modify: `internal/daemon/dag/graph_test.go` — 测试

**Interfaces:**
- Produces: `DependencyGraph.SourceFromPagePath(pagePath string) (string, bool)` — 返回源文件路径（相对 content/）

- [ ] **Step 1: 编写测试**

在 `internal/daemon/dag/graph_test.go` 末尾添加：

```go
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
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/daemon/dag/ -run "TestSourceFromPagePath" -v
```
Expected: COMPILATION ERROR (no SourceFromPagePath method)

- [ ] **Step 3: 实现 SourceFromPagePath**

在 `internal/daemon/dag/graph.go` 的 `PagePathFromSource` 方法之后添加：

```go
// SourceFromPagePath returns the source file path (relative to content/)
// for the given page URL. This is the reverse lookup of PagePathFromSource.
// Returns "", false if the URL is not in the graph or has no source file.
//
// Used by daemon's JIT rendering to locate the .md file for a requested URL
// that was included in the last full build.
func (dg *DependencyGraph) SourceFromPagePath(pagePath string) (string, bool) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	node, ok := dg.nodes[pagePath]
	if !ok || node.SourceFile == "" {
		return "", false
	}
	return node.SourceFile, true
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/daemon/dag/ -run "TestSourceFromPagePath" -v
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
git commit -m "feat(dag): add SourceFromPagePath reverse lookup for JIT rendering"
```

---

