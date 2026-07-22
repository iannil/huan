# JIT Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 daemon 的 JIT 渲染——把 `Builder.RenderPageJIT` 的 stub 替换为真实实现，让 daemon 能按需渲染未预构建的页面（draft、构建时跳过的、增量未触及的），复用 PipelineCache 保证性能。

**Architecture:** URL→源文件推导（DAG 反向查找 + URL fallback）+ `build.RenderPageWithCache`（复用 PipelineCache 重建内容/context，渲染单页返回 HTML）+ JITCache 在构建后自动失效。

**Tech Stack:** Go 1.26.2, DAG (internal/daemon/dag/), build pipeline (internal/build/), JITCache (internal/daemon/cache/)

## Global Constraints

- serving 层零改动（jitFallback 已搭好，只需 RenderPageJIT 实现）
- JIT 渲染输出必须与全量构建一致（含 LiveReload、列表页 context）
- JIT 强制 IncludeDrafts=true（主要服务 draft 场景）
- PipelineCache 不存在时优雅降级（fallback setupTemplatesAndWriter）
- 依赖方向：build 包不依赖 daemon/dag；resolveSourceFile（组合 DAG+URL）放在 daemon 包
- 所有现有测试必须通过

---

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

### Task 2: build/jit.go — resolveSourceFromURL URL 推导

**Files:**
- Create: `internal/build/jit.go` — resolveSourceFromURL 函数
- Create: `internal/build/jit_test.go` — URL 推导测试

**Interfaces:**
- Produces: `resolveSourceFromURL(pageURL string) string` — 纯函数，无 DAG 依赖

**说明：** 当 URL 不在 DAG 中时（新建 draft 等），按 huan URL 规则推导源文件路径。

- [ ] **Step 1: 编写测试**

`internal/build/jit_test.go`：

```go
package build

import "testing"

func TestResolveSourceFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"home", "/", "_index.md"},
		{"section", "/posts/", "posts/_index.md"},
		{"simple page", "/posts/hello/", "posts/hello.md"},
		{"nested page", "/posts/2026/new-year/", "posts/2026/new-year.md"},
		{"deep page", "/books/v1/ch1/", "books/v1/ch1.md"},
		{"explicit _index", "/posts/_index/", "posts/_index.md"},
		{"no trailing slash", "/posts/hello", "posts/hello.md"},
		{"leading+trailing slash stripped", "/posts/hello/", "posts/hello.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveSourceFromURL(tc.url)
			if got != tc.want {
				t.Errorf("resolveSourceFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/build/ -run "TestResolveSourceFromURL" -v
```
Expected: COMPILATION ERROR (no resolveSourceFromURL)

- [ ] **Step 3: 创建 jit.go 并实现 resolveSourceFromURL**

`internal/build/jit.go`：

```go
package build

import "strings"

// resolveSourceFromURL derives the source file path (relative to content/)
// from a page URL. Used by JIT rendering when the URL is not in the DAG
// (e.g., a newly-created draft not yet captured by a full build).
//
// URL conventions (match Hugo/huan content layout):
//   /                          → _index.md            (home)
//   /posts/                    → posts/_index.md      (section)
//   /posts/hello/              → posts/hello.md       (regular page)
//   /posts/2026/new-year/      → posts/2026/new-year.md
//   /posts/_index/             → posts/_index.md      (explicit)
//
// Returns "" only if the input cannot be parsed (effectively never for a
// normalized URL); callers should still verify the file exists on disk.
func resolveSourceFromURL(pageURL string) string {
	u := strings.Trim(pageURL, "/")
	if u == "" {
		return "_index.md" // home
	}
	parts := strings.Split(u, "/")
	last := parts[len(parts)-1]
	if last == "_index" {
		// /posts/_index/ → posts/_index.md
		return strings.Join(parts, "/") + ".md"
	}
	if len(parts) == 1 {
		// /posts/ → posts/_index.md (single segment = section index)
		return parts[0] + "/_index.md"
	}
	// /posts/hello/ → posts/hello.md
	return strings.Join(parts, "/") + ".md"
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/build/ -run "TestResolveSourceFromURL" -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/build/jit.go internal/build/jit_test.go
git commit -m "feat(build): add resolveSourceFromURL for JIT URL→source derivation"
```

---

### Task 3: build/jit.go — RenderPageWithCache 单页渲染

**Files:**
- Modify: `internal/build/jit.go` — 新增 RenderPageWithCache 函数
- Modify: `internal/build/jit_test.go` — 新增渲染测试

**Interfaces:**
- Consumes: `Options`, `PipelineCache`, pipeline stages (loadConfig/loadContent/renderMarkdownAndTree/setupTemplatesAndWriter/buildContexts/renderSinglePage), `tmpl.NewRenderer`/`tmpl.FuncMap`/`tmpl.SetI18nBundle`, `InjectLiveReload`
- Produces: `RenderPageWithCache(opts Options, cache *PipelineCache, pageURL string) (string, error)`

**说明：** 复用 PipelineCache 渲染单页返回 HTML（不写文件）。与 IncrementalRender 同策略（重建内容+context），但只渲染一个页面并返回 HTML，强制 IncludeDrafts=true。

- [ ] **Step 1: 在 jit.go 顶部添加 import**

修改 `internal/build/jit.go` 的 import 块：

```go
package build

import (
	"fmt"
	"strings"

	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)
```

- [ ] **Step 2: 实现 RenderPageWithCache**

在 `internal/build/jit.go` 末尾（resolveSourceFromURL 之后）添加：

```go
// RenderPageWithCache renders a single page on demand, reusing the cached
// rendering infrastructure (templates/i18n/markdown/writer). Unlike
// IncrementalRender, it returns the HTML without writing to disk, targets
// exactly one page identified by URL, and forces IncludeDrafts=true (JIT
// primarily serves draft pages that were skipped at build time).
//
// Used by daemon's JIT fallback when a requested URL is not in the
// pre-built static output. Content + contexts are always rebuilt (cheap)
// to keep list-page contexts consistent with the current on-disk content.
//
// When cache is nil or incomplete, falls back to a full pipeline setup
// (slower but always correct).
func RenderPageWithCache(opts Options, cache *PipelineCache, pageURL string) (string, error) {
	logf := opts.logf()
	p := newPipeline(opts)

	// JIT primarily serves drafts that were filtered out at build time.
	p.opts.IncludeDrafts = true

	// Stage 1: config. Reuse cached cfg when available.
	if cache != nil && cache.SiteCfg != nil {
		p.cfg = cache.SiteCfg
		if opts.BaseURLOverride != "" {
			p.cfg.BaseURL = opts.BaseURLOverride
		}
		if opts.MinifyOverride != nil {
			p.cfg.Minify = *opts.MinifyOverride
		}
	} else {
		if err := p.loadConfig(); err != nil {
			return "", fmt.Errorf("RenderPageWithCache loadConfig: %w", err)
		}
	}

	// Stage 2-3: reload ALL content + render markdown. Required for
	// correctness: list/tag/section page contexts reference child pages.
	if err := p.loadContent(); err != nil {
		return "", fmt.Errorf("RenderPageWithCache loadContent: %w", err)
	}
	if err := p.renderMarkdownAndTree(); err != nil {
		return "", fmt.Errorf("RenderPageWithCache renderMarkdownAndTree: %w", err)
	}

	// Stage 4: reuse cached rendering infrastructure OR rebuild.
	if cache != nil && cache.Templates != nil && cache.Writer != nil {
		p.tmpls = cache.Templates
		p.i18nBundle = cache.I18nBundle
		p.scRegistry = cache.SCRegistry
		p.md = cache.MDRenderer
		p.renderer = tmpl.NewRenderer(cache.Templates, tmpl.FuncMap(p.cfg.BaseURL))
		p.writer = cache.Writer
		tmpl.SetI18nBundle(p.i18nBundle)
	} else {
		if err := p.setupTemplatesAndWriter(); err != nil {
			return "", fmt.Errorf("RenderPageWithCache setupTemplatesAndWriter: %w", err)
		}
	}

	// Stage 5: rebuild contexts.
	p.buildContexts()

	// Stage 6: find the target page by URL.
	var target *content.Page
	for _, pg := range p.site.Pages {
		if pg.URL == pageURL {
			target = pg
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("RenderPageWithCache: page not found: %s", pageURL)
	}

	// Stage 7: render the single page (returns HTML, does NOT write to disk).
	html, err := p.renderSinglePage(target)
	if err != nil {
		return "", fmt.Errorf("RenderPageWithCache render %s: %w", pageURL, err)
	}

	// Stage 8: serve-mode LiveReload injection (parity with full build).
	if opts.InjectLiveReload && opts.LiveReloadURL != "" {
		html = InjectLiveReload(html, opts.LiveReloadURL)
	}

	logf("  JIT render: %s\n", pageURL)
	return html, nil
}
```

- [ ] **Step 3: 编写渲染测试**

在 `internal/build/jit_test.go` 末尾添加：

```go
func TestRenderPageWithCache_RendersSinglePage(t *testing.T) {
	tmpDir := writeJITTestSite(t)
	cache := buildJITCacheViaFullBuild(t, tmpDir)

	html, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: true,
		Logf:          t.Logf,
		PipelineCache: cache,
	}, cache, "/posts/hello/")

	if err != nil {
		t.Fatalf("RenderPageWithCache: %v", err)
	}
	if !strings.Contains(html, "Hello World") {
		t.Errorf("rendered HTML missing page title; got:\n%s", html)
	}
	if !strings.Contains(html, "Hello body") {
		t.Errorf("rendered HTML missing page content; got:\n%s", html)
	}
}

func TestRenderPageWithCache_PageNotFound(t *testing.T) {
	tmpDir := writeJITTestSite(t)
	cache := buildJITCacheViaFullBuild(t, tmpDir)

	_, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: true,
		Logf:          t.Logf,
		PipelineCache: cache,
	}, cache, "/nonexistent/")

	if err == nil {
		t.Fatal("expected error for nonexistent page")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q, want contains 'not found'", err.Error())
	}
}

func TestRenderPageWithCache_NoCacheFallback(t *testing.T) {
	tmpDir := writeJITTestSite(t)

	// cache=nil should fall back to full pipeline setup and still render.
	html, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: true,
		Logf:          t.Logf,
	}, nil, "/posts/hello/")

	if err != nil {
		t.Fatalf("RenderPageWithCache with nil cache: %v", err)
	}
	if !strings.Contains(html, "Hello World") {
		t.Errorf("nil-cache render missing title; got:\n%s", html)
	}
}

func TestRenderPageWithCache_DraftPage(t *testing.T) {
	tmpDir := writeJITTestSite(t)
	cache := buildJITCacheViaFullBuild(t, tmpDir)

	// A draft page should render because JIT forces IncludeDrafts=true.
	html, err := RenderPageWithCache(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Logf:          t.Logf,
		PipelineCache: cache,
	}, cache, "/posts/draft-page/")

	if err != nil {
		t.Fatalf("RenderPageWithCache draft: %v", err)
	}
	if !strings.Contains(html, "Draft Content") {
		t.Errorf("draft render missing content; got:\n%s", html)
	}
}

// writeJITTestSite creates a minimal site with a regular page, a draft page,
// and templates, then returns the temp dir.
func writeJITTestSite(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	mustWrite := func(rel, data string) {
		full := tmpDir + "/" + rel
		if err := osWriteFileMkdir(full, []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("huan.yaml", "baseURL: \"https://example.com/\"\ntitle: \"JIT Test\"\npublishDir: \"docs\"\n")
	mustWrite("content/posts/hello.md", "---\ntitle: \"Hello World\"\ndate: \"2026-01-01\"\n---\nHello body.\n")
	mustWrite("content/posts/draft-page.md", "---\ntitle: \"Draft\"\ndate: \"2026-01-02\"\ndraft: true\n---\nDraft Content.\n")
	mustWrite("layouts/_default/single.html", "<html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>")
	mustWrite("layouts/_default/list.html", "<html><body>{{ range .Pages }}<a href=\"{{ .URL }}\">{{ .Title }}</a>{{ end }}</body></html>")
	return tmpDir
}

// buildJITCacheViaFullBuild runs a full build to populate PipelineCache.
func buildJITCacheViaFullBuild(t *testing.T, tmpDir string) *PipelineCache {
	t.Helper()
	cache := NewPipelineCache()
	_, err := BuildSite(Options{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		IncludeDrafts: false, // drafts excluded so they need JIT
		Logf:          t.Logf,
		PipelineCache: cache,
	})
	if err != nil {
		t.Fatalf("BuildSite for cache setup: %v", err)
	}
	if cache.Templates == nil {
		t.Fatal("PipelineCache not populated after full build")
	}
	return cache
}

// osWriteFileMkdir writes data to path, creating parent dirs.
func osWriteFileMkdir(path string, data []byte) error {
	dir := path[:strings.LastIndex(path, "/")]
	if err := mkdirAll(dir); err != nil {
		return err
	}
	return writeFile(path, data)
}
```

Note: 上述 helper 用了 `mkdirAll` 和 `writeFile`，它们可能不存在。改为标准库 `os.MkdirAll` + `os.WriteFile`：

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func osWriteFileMkdir(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
```

并移除对 `mkdirAll`/`writeFile` 的引用。把 `tmpDir + "/" + rel` 改为 `filepath.Join(tmpDir, rel)`。最终 `mustWrite`：

```go
mustWrite := func(rel, data string) {
	full := filepath.Join(tmpDir, rel)
	if err := osWriteFileMkdir(full, []byte(data)); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/build/...
```
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/build/ -run "TestRenderPageWithCache" -v
```
Expected: ALL PASS

- [ ] **Step 6: 运行 build 包全部测试确保无回归**

```bash
go test ./internal/build/... -v
```
Expected: ALL PASS

- [ ] **Step 7: 提交**

```bash
git add internal/build/jit.go internal/build/jit_test.go
git commit -m "feat(build): add RenderPageWithCache for JIT single-page rendering"
```

---

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

### Task 5: JITCache 失效钩子

**Files:**
- Modify: `internal/daemon/builder.go` — executeFullBuild 后 Clear，IncrementalBuild 后 Remove

**说明：** JIT 缓存的 HTML 在内容/模板变更时必须失效。

- [ ] **Step 1: executeFullBuild 成功后清空 JITCache**

在 `internal/daemon/builder.go` 的 `executeFullBuild` 方法中，找到发布 `EventBuildCompleted` 事件的位置（`b.opts.Logf("builder: full build complete...")` 附近），在该日志之后添加：

```go
	// Invalidate JIT cache — full rebuild means all cached HTML is stale.
	if b.opts.JITCache != nil {
		b.opts.JITCache.Clear()
	}
```

- [ ] **Step 2: IncrementalBuild 渲染后移除受影响 URL 的 JIT 缓存**

在 `internal/daemon/builder.go` 的 `IncrementalBuild` 方法中，找到 `build.IncrementalRender(...)` 调用之后、发布 `EventBuildCompleted` 之前，添加：

```go
	// Invalidate JIT cache for affected pages — their content changed.
	if b.opts.JITCache != nil {
		for _, url := range ordered {
			b.opts.JITCache.Remove(url)
		}
	}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 4: 运行测试确保无回归**

```bash
go test ./internal/daemon/... -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/builder.go
git commit -m "feat(daemon): invalidate JITCache on full and incremental builds"
```

---

### Task 6: 集成测试 + 全量验证

**Files:**
- Modify: `internal/daemon/daemon_test.go` — JIT 集成测试
- Modify: `docs/superpowers/specs/2026-07-22-jit-rendering-design.md` — 标记实现状态

- [ ] **Step 1: 编写 JIT 集成测试**

在 `internal/daemon/daemon_test.go` 末尾添加：

```go
// TestDaemon_JIT_RenderUnbuiltPage verifies that RenderPageJIT renders a
// draft page that was excluded from the full build, populates JITCache, and
// serves the cached HTML on the second call.
func TestDaemon_JIT_RenderUnbuiltPage(t *testing.T) {
	tmpDir := setupJITDaemonSite(t) // builds site with a draft page excluded
	cache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	jitCache := cache_pkg.NewJITCache(100, 5*time.Minute)
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      jitCache,
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   false, // drafts excluded → need JIT
		Logf:          t.Logf,
		PipelineCache: cache,
	})

	// Full build (excludes the draft).
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// JIT-render the draft page (not in pre-built output).
	html, err := builder.RenderPageJIT(context.Background(), "/posts/draft-page/")
	if err != nil {
		t.Fatalf("RenderPageJIT: %v", err)
	}
	if !strings.Contains(html, "Draft Content") {
		t.Errorf("JIT render missing draft content; got:\n%s", html)
	}

	// JITCache should now hold the entry.
	if jitCache.Len() != 1 {
		t.Errorf("JITCache.Len = %d, want 1", jitCache.Len())
	}
	entry := jitCache.Get("/posts/draft-page/")
	if entry == nil {
		t.Fatal("JITCache.Get returned nil after render")
	}
	if !strings.Contains(string(entry.HTML), "Draft Content") {
		t.Errorf("cached HTML missing content; got:\n%s", string(entry.HTML))
	}
}

// TestDaemon_JIT_PageNotFound verifies JIT returns an error for a URL with
// no resolvable source.
func TestDaemon_JIT_PageNotFound(t *testing.T) {
	tmpDir := setupJITDaemonSite(t)
	cache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dag.NewDependencyGraph(),
		JITCache:      cache_pkg.NewJITCache(100, 5*time.Minute),
		Logf:          t.Logf,
		PipelineCache: cache,
	})
	builder.FullBuild(context.Background())

	_, err := builder.RenderPageJIT(context.Background(), "/totally/fake/")
	if err == nil {
		t.Fatal("expected error for fake URL")
	}
}

// TestJITCache_InvalidatedOnFullBuild verifies a full build clears JITCache.
func TestJITCache_InvalidatedOnFullBuild(t *testing.T) {
	tmpDir := setupJITDaemonSite(t)
	cache := build.NewPipelineCache()
	bus := eventbus.NewChannelBus()
	defer bus.Close()
	jitCache := cache_pkg.NewJITCache(100, 5*time.Minute)
	builder := NewBuilder(BuilderOptions{
		SourceDir: tmpDir, OutputDir: tmpDir, Bus: bus,
		DAG: dag.NewDependencyGraph(), JITCache: jitCache,
		Logf: t.Logf, PipelineCache: cache,
	})
	builder.FullBuild(context.Background())

	// Populate JIT cache.
	builder.RenderPageJIT(context.Background(), "/posts/draft-page/")
	if jitCache.Len() != 1 {
		t.Fatalf("expected 1 JIT entry, got %d", jitCache.Len())
	}

	// Full build should clear it.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("second FullBuild: %v", err)
	}
	if jitCache.Len() != 0 {
		t.Errorf("JITCache.Len after full build = %d, want 0", jitCache.Len())
	}
}

// setupJITDaemonSite creates a site with a published page + a draft page.
func setupJITDaemonSite(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte("baseURL: \"https://example.com/\"\ntitle: \"JIT\"\npublishDir: \"docs\"\n"))
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte("---\ntitle: \"Hello\"\ndate: \"2026-01-01\"\n---\nHello body.\n"))
	mustWriteIncFile(t, tmpDir, "content/posts/draft-page.md", []byte("---\ntitle: \"Draft\"\ndate: \"2026-01-02\"\ndraft: true\n---\nDraft Content.\n"))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html", []byte("<html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>"))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html", []byte("<html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>"))
	return tmpDir
}
```

注意：`mustWriteIncFile` 已在增量构建 Task 7 中存在（daemon_test.go）。确认它存在；如果不存在，参考其实现（os.MkdirAll + os.WriteFile）。`cache_pkg` 别名不需要——daemon_test.go 已 import `cache`（github.com/iannil/huan/internal/daemon/cache），直接用 `cache.NewJITCache`。把测试中 `cache_pkg.NewJITCache` 改为 `cache.NewJITCache`，但注意与 `build.NewPipelineCache` 的 `cache` 变量名冲突——用不同变量名（如 `jitCache := cache.NewJITCache(...)` 已避免冲突，因为局部变量叫 jitCache 而非 cache）。

- [ ] **Step 2: 运行集成测试**

```bash
go test ./internal/daemon/ -run "TestDaemon_JIT|TestJITCache_Invalidated" -v
```
Expected: ALL PASS

- [ ] **Step 3: 全量编译 + vet**

```bash
go build ./... && go vet ./...
```
Expected: SUCCESS

- [ ] **Step 4: 全量测试**

```bash
go test ./... -count=1
```
Expected: ALL PASS

- [ ] **Step 5: 更新设计文档状态**

修改 `docs/superpowers/specs/2026-07-22-jit-rendering-design.md`：`- **状态**：Draft` → `- **状态**：Implemented`

- [ ] **Step 6: 最终提交**

```bash
git add -A
git commit -m "feat: implement JIT rendering with PipelineCache reuse

- RenderPageWithCache reuses cached templates/i18n/writer, rebuilds
  content+context, renders single page returning HTML
- DAG.SourceFromPagePath reverse lookup + ResolveSourceFromURL fallback
- Builder.RenderPageJIT stub replaced with real implementation
- JITCache invalidated on full build (Clear) and incremental build (Remove)
- JIT forces IncludeDrafts=true to serve draft pages on demand

Co-Authored-By: Claude <noreply@anthropic.com>"
```