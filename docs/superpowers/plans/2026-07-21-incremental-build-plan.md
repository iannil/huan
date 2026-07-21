# Incremental Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现增量构建：内容文件变更时，只重新渲染受影响的页面，跳过未受影响页面的模板渲染，从而将 daemon 模式下的重建时间降到全量构建的约 10%。

**Architecture:** PipelineCache 缓存渲染基础设施（模板/i18n/markdown/writer），增量构建时重建内容+context（保证正确性），但只渲染 DAG 计算出的受影响页面。模板/i18n/config 变更时自动回退到全量构建。

**Tech Stack:** Go 1.26.2, DAG (internal/daemon/dag/), build pipeline (internal/build/)

## Global Constraints

- 不改动已有 build pipeline 的 8 个 stage 结构
- 增量构建输出必须与全量构建完全一致（正确性）
- 模板/i18n/config/themes 变更时自动回退全量构建
- PipelineCache 只缓存渲染基础设施，不缓存 page/context（避免内容变更时的引用失效）
- 增量构建通过现有的 QueueBuild 序列化（不并发）
- 所有现有测试必须通过

---

### Task 1: PipelineCache 结构体 + hasTemplateChanges

**Files:**
- Create: `internal/build/cache.go` — PipelineCache 结构体 + NewPipelineCache + hasTemplateChanges
- Create: `internal/build/cache_test.go` — 测试

**Interfaces:**
- Produces: `build.PipelineCache` 结构体, `build.NewPipelineCache()`, `build.HasTemplateChanges(changedFiles []string, sourceDir string) bool`

- [ ] **Step 1: 编写测试**

`internal/build/cache_test.go`：

```go
package build

import (
	"testing"
)

func TestNewPipelineCache_Empty(t *testing.T) {
	c := NewPipelineCache()
	if c == nil {
		t.Fatal("NewPipelineCache returned nil")
	}
	if c.Templates != nil {
		t.Error("Templates should be nil for fresh cache")
	}
	if c.BuiltAt.IsZero() {
		t.Error("BuiltAt should be set")
	}
}

func TestHasTemplateChanges_LayoutsFile(t *testing.T) {
	changed := []string{"/site/layouts/_default/single.html"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("layouts/ change should trigger full build")
	}
}

func TestHasTemplateChanges_ContentFile(t *testing.T) {
	changed := []string{"/site/content/posts/hello.md"}
	if HasTemplateChanges(changed, "/site") {
		t.Error("content/ change should NOT trigger full build")
	}
}

func TestHasTemplateChanges_HuanYaml(t *testing.T) {
	changed := []string{"/site/huan.yaml"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("huan.yaml change should trigger full build")
	}
}

func TestHasTemplateChanges_I18nFile(t *testing.T) {
	changed := []string{"/site/i18n/en.yaml"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("i18n/ change should trigger full build")
	}
}

func TestHasTemplateChanges_ThemesFile(t *testing.T) {
	changed := []string{"/site/themes/zozo/layouts/baseof.html"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("themes/ change should trigger full build")
	}
}

func TestHasTemplateChanges_MixedFiles(t *testing.T) {
	// One content, one template → should trigger (template change wins)
	changed := []string{
		"/site/content/posts/hello.md",
		"/site/layouts/_default/list.html",
	}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("mixed files with one template change should trigger full build")
	}
}

func TestHasTemplateChanges_EmptyList(t *testing.T) {
	if HasTemplateChanges([]string{}, "/site") {
		t.Error("empty change list should not trigger full build")
	}
}

func TestHasTemplateChanges_OutsideSourceDir(t *testing.T) {
	// File outside sourceDir (Rel fails or gives long path) → not a template change
	changed := []string{"/other/path/file.html"}
	if HasTemplateChanges(changed, "/site") {
		t.Error("file outside sourceDir should not trigger full build")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/build/ -run "TestNewPipelineCache|TestHasTemplateChanges" -v
```
Expected: COMPILATION ERROR (no cache.go yet)

- [ ] **Step 3: 实现 cache.go**

`internal/build/cache.go`：

```go
package build

import (
	"html/template"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/i18n"
	"github.com/iannil/huan/internal/markdown"
	"github.com/iannil/huan/internal/output"
	"github.com/iannil/huan/internal/shortcode"
)

// PipelineCache holds reusable build state across incremental builds.
// Populated after a full build completes. Incremental builds reuse this
// state to avoid re-parsing templates, reloading i18n bundles, and
// re-initializing the writer.
//
// DESIGN NOTE: This cache intentionally does NOT hold per-page content or
// template contexts. Those reference page pointers that change on every
// content edit, so caching them would produce stale output for list pages.
// Instead, incremental builds reload content + rebuild contexts (cheap)
// and reuse only the rendering infrastructure here (expensive to rebuild).
type PipelineCache struct {
	// Rendering infrastructure (valid until template/i18n/config changes)
	Templates  *template.Template
	I18nBundle *i18n.Bundle
	SCRegistry *shortcode.Registry
	MDRenderer *markdown.Renderer

	// Site config (valid until huan.yaml changes)
	SiteCfg *config.Config

	// Output writer (valid across builds; writes to the same OutputDir)
	Writer *output.Writer

	// BuiltAt records when this cache was populated (the last full build time).
	BuiltAt time.Time
}

// NewPipelineCache returns an empty PipelineCache with BuiltAt set to now.
func NewPipelineCache() *PipelineCache {
	return &PipelineCache{
		BuiltAt: time.Now(),
	}
}

// HasTemplateChanges reports whether any changed file invalidates the
// pipeline cache. Files under layouts/, i18n/, themes/, or the huan.yaml
// config itself require a full rebuild because they affect the cached
// templates/i18n/config.
//
// Returns false for content/ and static/ changes (those are handled
// incrementally).
func HasTemplateChanges(changedFiles []string, sourceDir string) bool {
	for _, f := range changedFiles {
		rel, err := filepath.Rel(sourceDir, f)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		// If the file is outside sourceDir, Rel returns a "../" path.
		// Those are not template changes relative to this site.
		if strings.HasPrefix(rel, "../") {
			continue
		}
		switch {
		case strings.HasPrefix(rel, "layouts/"):
			return true
		case strings.HasPrefix(rel, "i18n/"):
			return true
		case rel == "huan.yaml":
			return true
		case strings.HasPrefix(rel, "themes/"):
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/build/ -run "TestNewPipelineCache|TestHasTemplateChanges" -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/build/cache.go internal/build/cache_test.go
git commit -m "feat(build): add PipelineCache and HasTemplateChanges"
```

---

### Task 2: build.go — 支持 PipelineCache 填充

**Files:**
- Modify: `internal/build/build.go` — Options 新增 PipelineCache 字段，BuildSite 末尾填充
- Modify: `internal/build/pipeline.go` — 新增 pipeline.populateCache 方法

**Interfaces:**
- Consumes: `build.PipelineCache` (Task 1)
- Produces: `Options.PipelineCache` 字段, `BuildSite` 自动填充 cache

- [ ] **Step 1: 在 Options 中添加 PipelineCache 字段**

修改 `internal/build/build.go`，在 `Options` 结构体中（`AfterBuildSite` 字段之后）添加：

```go
	// PipelineCache, if non-nil, is populated with reusable build state
	// after BuildSite completes successfully. Used by daemon for
	// incremental builds. Pass a cache created by NewPipelineCache().
	// Experimental: API may change in future versions.
	PipelineCache *PipelineCache
```

- [ ] **Step 2: 在 pipeline 中添加 populateCache 方法**

在 `internal/build/pipeline.go` 末尾添加：

```go
// populateCache fills the PipelineCache with reusable rendering state
// after a successful full build. Called by BuildSite when opts.PipelineCache
// is non-nil.
func (p *pipeline) populateCache(cache *PipelineCache) {
	cache.Templates = p.tmpls
	cache.I18nBundle = p.i18nBundle
	cache.SCRegistry = p.scRegistry
	cache.MDRenderer = p.md
	cache.SiteCfg = p.cfg
	cache.Writer = p.writer
	cache.BuiltAt = time.Now()
}
```

注意：`pipeline.go` 顶部已 import `time`，无需新增。

- [ ] **Step 3: 在 BuildSite 中调用 populateCache**

修改 `internal/build/build.go` 的 `BuildSite` 函数，在 `AfterBuildSite` 回调之后、`return` 之前添加：

```go
	// Populate the pipeline cache if requested (for daemon incremental builds).
	if opts.PipelineCache != nil {
		p.populateCache(opts.PipelineCache)
	}

	return p.result, nil
```

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/build/...
```
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行现有测试确保无回归**

```bash
go test ./internal/build/... -v
```
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/build/build.go internal/build/pipeline.go
git commit -m "feat(build): populate PipelineCache after full build"
```

---

### Task 3: build.go — IncrementalRender 函数

**Files:**
- Modify: `internal/build/build.go` — 新增 IncrementalRender 函数

**Interfaces:**
- Consumes: `build.Options`, `build.PipelineCache`, `output.URLToFilePath`, `tmpl.Renderer`, pipeline stages
- Produces: `build.IncrementalRender(opts Options, cache *PipelineCache, affectedURLs []string) error`

**说明：** 这是增量构建的核心。它复用 cache 中的模板/i18n/writer，重新加载内容+重建 context（保证正确性），但只渲染 `affectedURLs` 中的页面。

- [ ] **Step 1: 在 build.go 末尾添加 IncrementalRender 函数**

```go
// IncrementalRender re-renders only the pages whose URLs are in affectedURLs,
// after a content change. It reuses cached templates/i18n/writer from a prior
// full build (via cache), but reloads all content + rebuilds contexts to
// ensure correctness (list pages must see updated page content).
//
// Only pages in affectedURLs are re-rendered and written to OutputDir;
// other pages are left untouched on disk. This skips the expensive
// per-page template rendering for the ~95% of unaffected pages.
//
// Use HasTemplateChanges() first: if templates changed, call BuildSite
// (full build) instead. Pass cache=nil to rebuild everything from scratch
// (slower, but always correct).
func IncrementalRender(opts Options, cache *PipelineCache, affectedURLs []string) error {
	if len(affectedURLs) == 0 {
		return nil
	}

	logf := opts.logf()
	p := newPipeline(opts)

	// Stage 1: load config. Reuse cached cfg when available (faster).
	if cache != nil && cache.SiteCfg != nil {
		p.cfg = cache.SiteCfg
		// Apply serve-mode overrides (they may differ from the cached build).
		if opts.BaseURLOverride != "" {
			p.cfg.BaseURL = opts.BaseURLOverride
		}
		if opts.MinifyOverride != nil {
			p.cfg.Minify = *opts.MinifyOverride
		}
	} else {
		if err := p.loadConfig(); err != nil {
			return fmt.Errorf("IncrementalRender loadConfig: %w", err)
		}
	}

	// Stage 2: reload ALL content. This is required for correctness:
	// list/tag/section pages reference child pages, so their contexts must
	// see the updated page content. Content loading is cheap relative to
	// template rendering.
	if err := p.loadContent(); err != nil {
		return fmt.Errorf("IncrementalRender loadContent: %w", err)
	}

	// Stage 3: render markdown + build content tree + taxonomies.
	if err := p.renderMarkdownAndTree(); err != nil {
		return fmt.Errorf("IncrementalRender renderMarkdownAndTree: %w", err)
	}

	// Stage 4: reuse cached rendering infrastructure OR rebuild.
	if cache != nil && cache.Templates != nil && cache.Writer != nil {
		p.tmpls = cache.Templates
		p.i18nBundle = cache.I18nBundle
		p.scRegistry = cache.SCRegistry
		p.md = cache.MDRenderer
		// Renderer wraps the cached templates with the current BaseURL FuncMap.
		p.renderer = tmpl.NewRenderer(cache.Templates, tmpl.FuncMap(p.cfg.BaseURL))
		p.writer = cache.Writer
		// Ensure the i18n bundle is wired into the template package (global state).
		tmpl.SetI18nBundle(p.i18nBundle)
	} else {
		if err := p.setupTemplatesAndWriter(); err != nil {
			return fmt.Errorf("IncrementalRender setupTemplatesAndWriter: %w", err)
		}
	}

	// Stage 5: build contexts (site-wide + per-page). Always rebuilt —
	// contexts reference page pointers that changed during content reload.
	p.buildContexts()

	// Stage 6: render ONLY affected pages. This is the performance win:
	// ~95% of pages are skipped.
	affectedSet := make(map[string]bool, len(affectedURLs))
	for _, u := range affectedURLs {
		affectedSet[u] = true
	}

	renderedCount := 0
	errors := 0
	for _, pg := range p.site.Pages {
		if !affectedSet[pg.URL] {
			continue
		}
		tmplName := ResolveTemplateName(p.tmpls, pg)
		if tmplName == "" {
			continue
		}
		ctx := p.lookup[pg]
		if ctx == nil {
			continue
		}
		// Section/home pages expose their pages via .Data.Pages.
		if pg.Kind == "section" || pg.Kind == "home" {
			ctx.Data = &tmpl.DataAccessor{Pages: ctx.RegularPages}
		}

		html, err := p.renderer.Render(tmplName, ctx)
		if err != nil {
			logf("  WARN: incremental render %s: %v\n", pg.URL, err)
			errors++
			continue
		}

		outPath := output.URLToFilePath(pg.URL, "")
		if err := p.writer.Write(outPath, html); err != nil {
			logf("  WARN: incremental write %s: %v\n", pg.URL, err)
			errors++
			continue
		}
		renderedCount++
	}

	logf("  Incremental render: %d pages re-rendered (%d affected, %d errors)\n",
		renderedCount, len(affectedURLs), errors)
	return nil
}
```

- [ ] **Step 2: 确认 import 完整**

`build.go` 顶部已有这些 import（确认存在，不需要新增）：
- `fmt`, `strings`, `time`
- `github.com/iannil/huan/internal/config`
- `github.com/iannil/huan/internal/content`

需要确认 `tmpl`（template 包别名）和 `output` 是否在 build.go 中已 import。检查：如果没有，它们已在 `pipeline.go` / `pipeline_setup.go` 中 import。由于 `IncrementalRender` 在 build.go 中，且用到 `tmpl.NewRenderer` / `tmpl.FuncMap` / `tmpl.SetI18nBundle` / `tmpl.DataAccessor` 和 `output.URLToFilePath`，需要在 build.go 添加 import：

```go
import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/output"
	tmpl "github.com/iannil/huan/internal/template"
)
```

- [ ] **Step 3: 编译验证**

```bash
go build ./internal/build/...
```
Expected: BUILD SUCCESS

- [ ] **Step 4: 运行测试确保无回归**

```bash
go test ./internal/build/... -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/build/build.go
git commit -m "feat(build): add IncrementalRender for DAG-driven partial re-render"
```

---

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

### Task 5: Builder — IncrementalBuild 完整实现

**Files:**
- Modify: `internal/daemon/builder.go` — BuilderOptions 新增 PipelineCache，IncrementalBuild 完整实现

**Interfaces:**
- Consumes: `build.PipelineCache`, `build.HasTemplateChanges`, `build.IncrementalRender`, `dag.OrderByDependency`
- Produces: `BuilderOptions.PipelineCache` 字段, `Builder.IncrementalBuild` 真正实现（不再回退到 FullBuild）

- [ ] **Step 1: 在 BuilderOptions 中添加 PipelineCache 字段**

修改 `internal/daemon/builder.go` 的 `BuilderOptions` 结构体，在 `OnAfterBuild` 之后添加：

```go
	// PipelineCache holds reusable build state for incremental builds.
	// Populated after the first full build (via build.Options.PipelineCache).
	// When nil, IncrementalBuild falls back to a full build.
	PipelineCache *build.PipelineCache
```

- [ ] **Step 2: 重写 IncrementalBuild 方法**

替换 `internal/daemon/builder.go` 中的 `IncrementalBuild` 方法（当前是回退到 `executeFullBuild` 的版本）：

```go
// IncrementalBuild rebuilds only the pages affected by the given file changes.
// It uses the DAG to determine affected pages and build.IncrementalRender
// to re-render only those pages, reusing the cached pipeline state.
//
// Falls back to a full build when:
//   - a template/i18n/config/theme file changed (cache invalid)
//   - no PipelineCache is available
//   - the DAG is empty (no prior full build)
func (b *Builder) IncrementalBuild(ctx context.Context, changedFiles []string) error {
	// 1. Template/config/i18n change → full rebuild.
	if build.HasTemplateChanges(changedFiles, b.opts.SourceDir) {
		b.opts.Logf("builder: template/config change detected, doing full build")
		return b.executeFullBuild(ctx)
	}

	// 2. No cache or empty DAG → full build.
	if b.opts.PipelineCache == nil || b.opts.DAG.NodeCount() == 0 {
		b.opts.Logf("builder: no pipeline cache or empty DAG, doing full build")
		return b.executeFullBuild(ctx)
	}

	// 3. Compute affected pages via DAG.
	affected := b.opts.DAG.AffectedBy(changedFiles)
	if len(affected) == 0 {
		b.opts.Logf("builder: no pages affected by %d file changes", len(changedFiles))
		return nil
	}

	// 4. Order by dependency (leaf first, parents last).
	ordered := b.opts.DAG.OrderByDependency(affected)
	b.opts.Logf("builder: incremental build: %d pages affected by %d file changes",
		len(ordered), len(changedFiles))

	// 5. Re-render only affected pages, reusing the cached pipeline state.
	start := time.Now()
	err := build.IncrementalRender(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: b.opts.BuildDrafts,
		Logf:          b.opts.Logf,
		PipelineCache: b.opts.PipelineCache,
	}, b.opts.PipelineCache, ordered)

	if err != nil {
		b.opts.Logf("builder: incremental render failed: %v", err)
		return fmt.Errorf("incremental build: %w", err)
	}

	if b.opts.Metrics != nil {
		b.opts.Metrics.RecordBuild(time.Since(start))
	}

	b.opts.Logf("builder: incremental build complete in %v", time.Since(start))

	// 6. Publish build-completed event with incremental marker.
	_ = b.opts.Bus.Publish(ctx, eventbus.Event{
		Type:      eventbus.EventBuildCompleted,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"incremental": true,
			"pages":       len(ordered),
		},
	})
	return nil
}
```

- [ ] **Step 3: 确认 import**

`internal/daemon/builder.go` 已 import `build`、`eventbus`、`time`、`fmt`。确认无需新增。

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/daemon/...
```
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行现有 builder 测试确保无回归**

```bash
go test ./internal/daemon/... -run "TestBuilder" -v
```
Expected: ALL PASS

注意：`TestBuilder_IncrementalBuild_EmptyDAG` 当前期望返回 nil（空 DAG 时 AffectedBy 返回空）。改动后空 DAG 会回退到 full build（因为 `NodeCount() == 0`）。如果该测试失败，更新它：空 DAG 时 IncrementalBuild 应回退全量构建。

- [ ] **Step 6: 更新失败的测试（如有）**

如果 `TestBuilder_IncrementalBuild_EmptyDAG` 失败，修改它以反映新行为（空 DAG → 回退全量构建）。检查测试文件中的断言，确保与新的回退逻辑一致。

- [ ] **Step 7: 提交**

```bash
git add internal/daemon/builder.go internal/daemon/daemon_test.go
git commit -m "feat(daemon): implement real incremental build in Builder"
```

---

### Task 6: Daemon — 集成 PipelineCache

**Files:**
- Modify: `internal/daemon/daemon.go` — 创建 PipelineCache 并传给 Builder 和 build.Options

**说明：** 在 daemon 启动时创建 PipelineCache，传给 Builder，并在全量构建时通过 build.Options.PipelineCache 让 BuildSite 填充它。

- [ ] **Step 1: 在 daemon.go 中创建 PipelineCache**

修改 `internal/daemon/daemon.go` 的 `Run` 函数。在创建 Builder（`d.builder = NewBuilder(...)`）处，添加 PipelineCache 创建并传递：

```go
	// 7. Initialize Builder
	pipelineCache := build.NewPipelineCache()
	d.builder = NewBuilder(BuilderOptions{
		SourceDir:   opts.SourceDir,
		OutputDir:   tmpDir,
		Bus:         d.bus,
		DAG:         d.dag,
		JITCache:    d.jitCache,
		Metrics:     d.metrics,
		BuildDrafts: opts.BuildDrafts,
		Logf:        log.Printf,
		PipelineCache: pipelineCache,
		OnAfterBuild: func(r *build.Result) error {
			// PipelineCache is populated by BuildSite via build.Options.PipelineCache.
			return nil
		},
	})
```

- [ ] **Step 2: 在 executeFullBuild 中传递 PipelineCache 给 build.Options**

修改 `internal/daemon/builder.go` 的 `executeFullBuild` 方法。在 `build.BuildSite(build.Options{...})` 调用中添加 `PipelineCache` 字段：

```go
	result, err := build.BuildSite(build.Options{
		SourceDir:     b.opts.SourceDir,
		OutputDir:     b.opts.OutputDir,
		IncludeDrafts: b.opts.BuildDrafts,
		Logf:          b.opts.Logf,
		PipelineCache: b.opts.PipelineCache,
		AfterBuild: func(r *build.Result, fn build.RenderPageFunc) error {
			renderPageFn = fn // capture for later JIT use
			if b.opts.OnAfterBuild != nil {
				return b.opts.OnAfterBuild(r)
			}
			return nil
		},
		AfterBuildSite: b.buildAndPersistDAG,
	})
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 4: 运行 daemon 测试**

```bash
go test ./internal/daemon/... -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/daemon.go internal/daemon/builder.go
git commit -m "feat(daemon): wire PipelineCache into Builder and full build"
```

---

### Task 7: 集成测试 — 增量构建正确性

**Files:**
- Modify: `internal/daemon/daemon_test.go` — 新增增量构建测试

**说明：** 验证：(1) 全量构建后 PipelineCache 被填充；(2) 内容变更只触发增量渲染；(3) 模板变更回退全量构建。

- [ ] **Step 1: 编写集成测试**

在 `internal/daemon/daemon_test.go` 末尾添加：

```go
// TestPipelineCache_PopulatedAfterFullBuild verifies that a successful full
// build populates the PipelineCache with reusable rendering state.
func TestPipelineCache_PopulatedAfterFullBuild(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Minimal site: config + 2 pages + templates.
	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Test"
publishDir: "docs"
`))
	mustWriteIncFile(t, tmpDir, "content/posts/a.md", []byte(`---
title: "Page A"
date: "2026-01-01"
---
Content A.
`))
	mustWriteIncFile(t, tmpDir, "content/posts/b.md", []byte(`---
title: "Page B"
date: "2026-01-02"
---
Content B.
`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Pages }}<a href="{{ .URL }}">{{ .Title }}</a>{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	cache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache_pkg.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: cache,
	})

	// Full build.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// PipelineCache should be populated.
	if cache.Templates == nil {
		t.Error("PipelineCache.Templates not populated after full build")
	}
	if cache.Writer == nil {
		t.Error("PipelineCache.Writer not populated after full build")
	}
	if cache.SiteCfg == nil {
		t.Error("PipelineCache.SiteCfg not populated after full build")
	}
	if dagGraph.NodeCount() == 0 {
		t.Error("DAG should have nodes after full build")
	}
}

// TestIncrementalBuild_ContentChange verifies that a content file change
// triggers incremental re-rendering and produces updated output.
func TestIncrementalBuild_ContentChange(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Test"
publishDir: "docs"
`))
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte(`---
title: "Hello"
date: "2026-01-01"
---
Original content.
`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body><h1>{{ .Title }}</h1>{{ .Content }}</body></html>`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	cache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache_pkg.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: cache,
	})

	// Full build first.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// Capture the page output.
	outPath := filepath.Join(tmpDir, "posts", "hello", "index.html")
	origBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read original output: %v", err)
	}
	origOutput := string(origBytes)
	if !strings.Contains(origOutput, "Original content.") {
		t.Fatalf("original output missing expected content:\n%s", origOutput)
	}

	// Modify the content file.
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte(`---
title: "Hello"
date: "2026-01-01"
---
Updated content.
`))

	// Trigger incremental build with the changed file.
	changedFile := filepath.Join(tmpDir, "content", "posts", "hello.md")
	if err := builder.IncrementalBuild(context.Background(), []string{changedFile}); err != nil {
		t.Fatalf("IncrementalBuild: %v", err)
	}

	// Verify the output was updated.
	newBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read updated output: %v", err)
	}
	newOutput := string(newBytes)
	if !strings.Contains(newOutput, "Updated content.") {
		t.Errorf("output not updated after incremental build:\n%s", newOutput)
	}
	if strings.Contains(newOutput, "Original content.") {
		t.Errorf("output still contains old content after incremental build:\n%s", newOutput)
	}
}

// TestIncrementalBuild_TemplateChangeFallback verifies that a template
// change falls back to a full build.
func TestIncrementalBuild_TemplateChangeFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "huan-incr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mustWriteIncFile(t, tmpDir, "huan.yaml", []byte(`baseURL: "https://example.com/"
title: "Test"
publishDir: "docs"
`))
	mustWriteIncFile(t, tmpDir, "content/posts/hello.md", []byte(`---
title: "Hello"
date: "2026-01-01"
---
Content.
`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/single.html",
		[]byte(`<!doctype html><html><body>{{ .Content }}</body></html>`))
	mustWriteIncFile(t, tmpDir, "layouts/_default/list.html",
		[]byte(`<!doctype html><html><body>{{ range .Pages }}{{ .Title }}{{ end }}</body></html>`))

	bus := eventbus.NewChannelBus()
	defer bus.Close()
	dagGraph := dag.NewDependencyGraph()
	cache := build.NewPipelineCache()
	builder := NewBuilder(BuilderOptions{
		SourceDir:     tmpDir,
		OutputDir:     tmpDir,
		Bus:           bus,
		DAG:           dagGraph,
		JITCache:      cache_pkg.NewJITCache(100, 5*time.Minute),
		Metrics:       NewMetricsCollector(),
		BuildDrafts:   true,
		Logf:          t.Logf,
		PipelineCache: cache,
	})

	// Full build.
	if err := builder.FullBuild(context.Background()); err != nil {
		t.Fatalf("FullBuild: %v", err)
	}

	// A template file change should be detected as requiring full build.
	changedFile := filepath.Join(tmpDir, "layouts", "_default", "single.html")
	if !build.HasTemplateChanges([]string{changedFile}, tmpDir) {
		t.Error("template change should be detected by HasTemplateChanges")
	}

	// IncrementalBuild with a template change should not error (it falls back
	// to full build internally).
	if err := builder.IncrementalBuild(context.Background(), []string{changedFile}); err != nil {
		t.Errorf("IncrementalBuild with template change should fall back cleanly, got: %v", err)
	}
}

// mustWriteIncFile is a test helper that writes a file, creating parent dirs.
func mustWriteIncFile(t *testing.T, root, relPath string, data []byte) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 确认 import**

测试文件需要这些 import（确认存在或添加）：

```go
import (
	// ... existing
	"path/filepath"
	"strings"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/daemon/cache"
	cache_pkg "github.com/iannil/huan/internal/daemon/cache"
)
```

注意：daemon_test.go 已 import `cache`（`github.com/iannil/huan/internal/daemon/cache`）。直接用 `cache.NewJITCache`，不要用别名。修正测试中的 `cache_pkg.NewJITCache` → `cache.NewJITCache`，并移除 `cache_pkg` 别名。

- [ ] **Step 3: 运行集成测试**

```bash
go test ./internal/daemon/ -run "TestPipelineCache_Populated|TestIncrementalBuild_" -v
```
Expected: ALL PASS

- [ ] **Step 4: 提交**

```bash
git add internal/daemon/daemon_test.go
git commit -m "test(daemon): add incremental build integration tests"
```

---

### Task 8: 全量测试与文档更新

**Files:**
- Modify: `docs/superpowers/specs/2026-07-21-incremental-build-design.md` — 标记实现状态

- [ ] **Step 1: 全量编译**

```bash
go build ./... && go vet ./...
```
Expected: SUCCESS

- [ ] **Step 2: 全量测试**

```bash
go test ./... -count=1
```
Expected: ALL PASS

- [ ] **Step 3: 更新设计文档状态**

修改 `docs/superpowers/specs/2026-07-21-incremental-build-design.md` 的状态行：`- **状态**：Draft` → `- **状态**：Implemented`

并在 §4.1 的 PipelineCache 注释中补充实现决策说明（缓存策略调整）：

```markdown
> **实现说明**：PipelineCache 只缓存渲染基础设施（模板/i18n/markdown/writer），
> 不缓存 PageLookup/ContextLookup。因为内容变更时 context 引用的 page 指针
> 会失效，缓存它们会导致列表页输出过期。增量构建时重建内容+context（开销小），
> 但只渲染受 DAG 影响的页面（跳过 ~95% 的模板渲染）。
```

- [ ] **Step 4: 最终提交**

```bash
git add -A
git commit -m "feat: implement incremental build with DAG-driven partial re-render

- PipelineCache caches rendering infrastructure (templates/i18n/markdown/writer)
- IncrementalRender re-renders only DAG-affected pages, reusing cached state
- HasTemplateChanges detects template/i18n/config/theme changes → full build
- DAG.OrderByDependency ensures correct rendering order (leaf before parents)
- Builder.IncrementalBuild wires it all together with full-build fallback
- Daemon populates PipelineCache via build.Options after full build

Co-Authored-By: Claude <noreply@anthropic.com>"
```