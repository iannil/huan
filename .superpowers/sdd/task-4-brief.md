### Task 4: Builder — RenderPageJIT 重写 + resolveSourceFile

**Files:**
- Modify: `internal/daemon/builder.go` — 重写 RenderPageJIT（去 stub），新增 resolveSourceFile

**Interfaces:**
- Consumes: `build.RenderPageWithCache`, `build.PipelineCache`, `dag.DependencyGraph.SourceFromPagePath`, `build.resolveSourceFromURL`（需导出或复制）
- Produces: 重写后的 `Builder.RenderPageJIT(ctx, pageURL) (string, error)`，`resolveSourceFile(dg, pageURL) (string, bool)`

**重要：** `resolveSourceFromURL` 在 build 包是小写（未导出）。daemon 包无法调用。两种方案：
1. 导出为 `ResolveSourceFromURL`（改 build/jit.go）
2. 在 daemon 包内复制一份

方案 1 更清晰（DRY）。先导出 resolveSourceFromURL → ResolveSourceFromURL，更新 jit_test.go 的调用。

- [ ] **Step 1: 导出 resolveSourceFromURL**

修改 `internal/build/jit.go`，把 `func resolveSourceFromURL` 改为 `func ResolveSourceFromURL`，更新 godoc。

修改 `internal/build/jit_test.go` 中所有 `resolveSourceFromURL(` → `ResolveSourceFromURL(`。

- [ ] **Step 2: 验证导出后测试仍通过**

```bash
go test ./internal/build/ -run "TestResolveSourceFromURL" -v
```
Expected: ALL PASS

- [ ] **Step 3: 新增 resolveSourceFile 并重写 RenderPageJIT**

替换 `internal/daemon/builder.go` 中的 `RenderPageJIT` stub（当前返回 "not yet implemented"）：

```go
// RenderPageJIT renders a single page on demand for JIT fallback.
// Reuses the cached pipeline state for speed. Returns the HTML (not written
// to disk); the caller (serving.jitFallback) caches it in JITCache.
//
// Returns an error (→ 404 in serving layer) when the source file cannot
// be resolved or does not exist on disk.
func (b *Builder) RenderPageJIT(ctx context.Context, pageURL string) (string, error) {
	cache := b.opts.PipelineCache

	// 1. pageURL → source file (relative to content/).
	sourceRel, ok := resolveSourceFile(b.opts.DAG, pageURL)
	if !ok {
		return "", fmt.Errorf("JIT: cannot resolve source for %s", pageURL)
	}

	// 2. Verify the source file exists on disk.
	sourcePath := filepath.Join(b.opts.SourceDir, "content", sourceRel)
	if _, err := os.Stat(sourcePath); err != nil {
		return "", fmt.Errorf("JIT: source not found: %s", sourcePath)
	}

	// 3. Render with cached pipeline (forces IncludeDrafts=true).
	html, err := build.RenderPageWithCache(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: true,
		Logf:          b.opts.Logf,
		PipelineCache: cache,
	}, cache, pageURL)
	if err != nil {
		return "", fmt.Errorf("JIT render %s: %w", pageURL, err)
	}

	return html, nil
}

// resolveSourceFile maps a page URL to its source file path (relative to
// content/). Tries the DAG first (pages present in the last full build),
// then falls back to URL-based derivation for pages not yet built (drafts,
// new pages).
func resolveSourceFile(dg *dag.DependencyGraph, pageURL string) (string, bool) {
	// 1. DAG reverse lookup (exact, for built pages).
	if dg != nil {
		if src, ok := dg.SourceFromPagePath(pageURL); ok {
			return src, true
		}
	}
	// 2. URL derivation (fallback, for unbuilt pages).
	if src := build.ResolveSourceFromURL(pageURL); src != "" {
		return src, true
	}
	return "", false
}
```

- [ ] **Step 4: 确认 builder.go import**

`internal/daemon/builder.go` 已 import：`fmt`、`os`、`path/filepath`、`build`、`dag`、`context`。确认无需新增。`build` 和 `dag` 已在 import 列表（builder.go 使用 build.BuildSite、dag.DependencyGraph）。

- [ ] **Step 5: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 6: 运行现有 daemon 测试确保无回归**

```bash
go test ./internal/daemon/... -run "TestBuilder" -v
```
Expected: ALL PASS（注意：如有测试断言 RenderPageJIT 返回 "not yet implemented"，需更新）

- [ ] **Step 7: 更新依赖旧 stub 错误信息的测试（如有）**

搜索 daemon_test.go 中是否有测试断言 RenderPageJIT 返回 "not yet implemented"。如果有，更新为断言成功渲染或新的错误格式。

```bash
grep -rn "not yet implemented\|JIT rendering" internal/daemon/*_test.go
```

- [ ] **Step 8: 提交**

```bash
git add internal/build/jit.go internal/build/jit_test.go internal/daemon/builder.go
git commit -m "feat(daemon): implement RenderPageJIT with DAG+URL source resolution"
```

---

