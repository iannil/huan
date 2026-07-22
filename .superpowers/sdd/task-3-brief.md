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

