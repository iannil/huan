# Performance Optimization — P1: ContextCache + 渲染并行化 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** 实现 ContextCache (L3 上下文缓存) 和渲染并行化，使全量构建从 ~5.4s 降至 < 3s，增量构建 < 500ms

**Architecture:**
- ContextCache 缓存 `*tmpl.Context` 对象，通过 page 版本号 + 元数据变更检测来失效
- 渲染并行化将 renderPages 从顺序改为 goroutine pool 并发，Writer 加锁保护
- PipelineCache 扩展增加 ContextCache 和 Renderer 字段

**Tech Stack:** Go 标准库（sync, container/list, runtime）

## Global Constraints

- 无需引入外部依赖
- 所有新增测试通过 `go test -race`
- ContextCache 默认 max=5000
- 并行渲染使用 `runtime.GOMAXPROCS(0)` 作为 worker 数
- Writer 并发安全通过 `sync.Mutex` 保护

---

### Task 4: Page 版本号字段

**Files:**
- Modify: `internal/content/page.go` — 增加 Version 字段
- Modify: `internal/content/load.go` — 加载时设置版本号

**Interfaces:**
- Consumes: 无
- Produces: `Page.Version uint64` 字段

**Task 4: 为 content.Page 增加版本号字段**

- [ ] **Step 1: 在 Page struct 中增加 Version 字段**

```go
// internal/content/page.go
type Page struct {
    // ... 现有字段 ...
    Version uint64  // 内容版本号，加载时递增
}
```

- [ ] **Step 2: 在 loadPageFromFrontmatter 中设置版本号**

```go
// internal/content/load.go 中 loadPageFromFrontmatter 末尾：
p.Version = versionCounter.Add(1)
```

使用全局 `atomic.Uint64` 计数器。

- [ ] **Step 3: 编译测试**

Run: `go build ./... && go test ./internal/content/... -count=1 -race`

- [ ] **Step 4: 提交**

---

### Task 5: ContextCache 数据结构

**Files:**
- Create: `internal/build/cache/context.go`
- Test: `internal/build/cache/context_test.go`

**Task 5: 实现 ContextCache**

ContextCache 缓存 `*tmpl.Context` 对象，按 page 指针 + 版本号管理。

- [ ] **Step 1: 实现 ContextCache**

```go
// internal/build/cache/context.go
package cache

import (
	"sync"

	"github.com/iannil/huan/internal/content"
	tmpl "github.com/iannil/huan/internal/template"
)

// ContextCache caches template contexts per page.
// Cache entries are invalidated when the page's version changes.
type ContextCache struct {
	mu    sync.RWMutex
	items map[*content.Page]*cachedContext
	max   int
}

type cachedContext struct {
	ctx         *tmpl.Context
	pageVersion uint64
}

func NewContextCache(max int) *ContextCache { ... }
func (c *ContextCache) Get(pg *content.Page) *tmpl.Context { ... }
func (c *ContextCache) Set(pg *content.Page, ctx *tmpl.Context) { ... }
func (c *ContextCache) Invalidate(pg *content.Page) { ... }
func (c *ContextCache) Clear() { ... }
```

- [ ] **Step 2: 写单元测试**

测试：Get/Set 命中、版本号变更后失效、Clear、Invalidate、并发安全

- [ ] **Step 3: 编译测试**

Run: `go test ./internal/build/cache/... -v -count=1 -race`

- [ ] **Step 4: 提交**

---

### Task 6: ContextCache 集成到 buildContexts

**Files:**
- Modify: `internal/build/cache.go` — PipelineCache 增加 ContextCache 字段
- Modify: `internal/build/pipeline_setup.go` — buildContexts 使用 ContextCache

**Task 6: 集成 ContextCache 到构建管线**

- [ ] **Step 1: PipelineCache 增加 ContextCache**

```go
// internal/build/cache.go
type PipelineCache struct {
    // ... 现有字段 ...
    ContentCache  *cache.ContentCache
    ContextCache  *cache.ContextCache  // 新增
    Renderer      *tmpl.Renderer       // 新增
}
```

- [ ] **Step 2: buildContexts 使用缓存**

```go
// pipeline_setup.go buildContexts()
func (p *pipeline) buildContexts() {
    var ctxCache *cache.ContextCache
    if p.opts.PipelineCache != nil {
        ctxCache = p.opts.PipelineCache.ContextCache
    }

    for _, pg := range p.site.Pages {
        // Try cache
        if ctxCache != nil {
            if cached := ctxCache.Get(pg); cached != nil {
                lookup[pg] = cached
                continue
            }
        }
        // Build new context
        ctx := tmpl.NewContext(pg, siteCtx, p.cfg)
        lookup[pg] = ctx
        if ctxCache != nil {
            ctxCache.Set(pg, ctx)
        }
    }
    // ... LinkPageRelationships, PopulateSitePages ...
}
```

- [ ] **Step 3: 编译测试**

Run: `go build ./... && go test ./internal/build/... -count=1 -race`

- [ ] **Step 4: 提交**

---

### Task 7: 渲染并行化

**Files:**
- Modify: `internal/build/pipeline_render.go` — 并发渲染
- Modify: `internal/output/writer.go` — Writer 加锁

**Task 7: 渲染管线并行化**

- [ ] **Step 1: Writer 加锁保护**

```go
// output/writer.go
type Writer struct {
    mu sync.Mutex  // 新增
    // ... 现有字段 ...
}

func (w *Writer) Write(relPath, content string) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    // ... 现有逻辑 ...
}
```

- [ ] **Step 2: renderPages 改为并发**

```go
// pipeline_render.go
func (p *pipeline) renderPages() {
    maxWorkers := runtime.GOMAXPROCS(0)
    sem := make(chan struct{}, maxWorkers)
    var wg sync.WaitGroup
    var mu sync.Mutex

    for _, pg := range p.site.Pages {
        if !p.shouldRender(pg) { continue }
        wg.Add(1)
        sem <- struct{}{}
        go func(pg *content.Page) {
            defer wg.Done()
            defer func() { <-sem }()
            // ... render + write ...
        }(pg)
    }
    wg.Wait()
}
```

- [ ] **Step 3: 编译测试**

Run: `go build ./... && go test ./internal/build/... -count=1 -race`

- [ ] **Step 4: 构建验证**

```bash
go build -o /tmp/huan-p1 ./cmd/huan
time /tmp/huan-p1 build --source /Users/rong.zhu/Code/zhurong/zhurongshuo --destination /tmp/huan-build-p1 2>&1
```

- [ ] **Step 5: 提交**

---

### Task 8: 增量构建验证

**Files:**
- Modify: `internal/build/build.go` — IncrementalRender 使用 ContextCache
- Test: 手动验证增量构建

**Task 8: 验证增量构建性能**

- [ ] **Step 1: 增量构建使用 ContextCache**

```go
// build.go IncrementalRender() 中，buildContexts 自动使用 PipelineCache.ContextCache
```

- [ ] **Step 2: 手动验证**

```bash
# 启动 daemon，修改一篇文章，验证增量构建时间
```

- [ ] **Step 3: 提交**