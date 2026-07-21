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

