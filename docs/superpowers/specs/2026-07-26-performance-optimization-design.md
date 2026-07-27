# 设计文档：huan 构建管线极致性能优化

> **日期**：2026-07-26
> **状态**：草稿
> **目标**：全量构建 < 5s（~2000 页）、增量构建 < 500ms、JIT 渲染 < 5ms
> **背景**：[插件化架构](2026-07-21-image-pipeline-design.md)已就绪，下一步聚焦平台核心性能

---

## 一、当前性能瓶颈分析

通过代码走读，当前构建管线 7 阶段的性能热点：

| 阶段 | 文件 | 行数 | 当前行为 | 瓶颈程度 |
|------|------|------|---------|---------|
| loadContent | `pipeline.go` | 329 | 每次全量 `content.LoadDir`，N 个文件顺序读取+解析 frontmatter | ★★★ |
| renderMarkdownAndTree | `pipeline.go` | 329 | N 页顺序 shortcode 展开 + goldmark 渲染 | ★★★ |
| setupTemplatesAndWriter | `pipeline_setup.go` | 107 | 增量构建已缓存，但 Context 全量重建 | ★★ |
| buildContexts | `context.go` | 505 | N 页 `tmpl.NewContext` + `LinkPageRelationships`，O(N) 内存 | ★★ |
| renderPages | `pipeline_render.go` | 182 | N 页顺序 `renderer.Render` + `writer.Write` | ★★★ |
| renderFeedsAndSpecials | `pipeline_feeds.go` | 194 | 顺序渲染 taxonomy/sitemap/search 等 | ★ |
| copyStaticAndFinalize | `pipeline_write.go` | 46 | 顺序复制 static 目录 | ★ |

### 关键问题

1. **无内容缓存**：每次构建（含 JIT）都全量 `content.LoadDir` + `renderMarkdownAndTree`，即使文件未变
2. **无 Context 缓存**：`buildContexts` 每次都全量重建 O(N) Context 对象
3. **无函数缓存**：模板中纯函数（`i18n`、`relref`、`urlize`）每次调用重新计算
4. **顺序渲染**：渲染 + 写入全串行，CPU 利用率低
5. **JIT 全量管线**：JIT 渲染仍然走完整 7 阶段，O(N) 开销

---

## 二、总体架构

### 2.1 四层缓存架构

```
┌──────────────────────────────────────────────────────────────┐
│  L1: JITCache (已有, 优化)                                    │
│  类型: LRU + TTL        key: pageURL       value: HTML       │
│  失效: 全量Clear / 增量Remove                                  │
├──────────────────────────────────────────────────────────────┤
│  L2: ContentCache (新增)                                      │
│  类型: LRU              key: filePath      value: *content.Page│
│  失效: mtime变更 / 文件删除 / 全量Clear                         │
├──────────────────────────────────────────────────────────────┤
│  L3: ContextCache (新增)                                      │
│  类型: LRU              key: pagePtr       value: *tmpl.Context│
│  失效: page版本号变更 / 依赖的聚合页级联失效 / 全量Clear         │
├──────────────────────────────────────────────────────────────┤
│  L4: FuncCache (新增)                                         │
│  类型: LRU + TTL        key: func+argsHash  value: result     │
│  失效: 全量Clear / TTL过期                                     │
└──────────────────────────────────────────────────────────────┘
```

### 2.2 渲染管线并行化

```go
// 顺序 → 并发
// before:
for _, pg := range p.site.Pages { render → write }

// after:
sem := make(chan struct{}, GOMAXPROCS)
for _, pg := range p.site.Pages { go renderAndWrite(sem, pg) }
wg.Wait()
```

### 2.3 JIT 极致路径

```go
// before: JIT = BuildSite(loadContent(N) + renderMarkdown(N) + buildContexts(N) + render(1))
// after:  JIT = ContentCache.Get(1) + MarkdownCache.Get(1) + ContextCache.Get(1) + render(1)
```

---

## 三、ContentCache 详细设计

### 3.1 数据结构

```go
// internal/build/cache/content.go

type ContentCache struct {
    mu    sync.RWMutex
    items map[string]*cachedPageEntry  // key: content-relative path
    lru   *list.List
    max   int  // default 5000
}

type cachedPageEntry struct {
    key      string
    page     *content.Page
    mtime    time.Time
    element  *list.Element
}
```

### 3.2 核心方法

| 方法 | 签名 | 行为 |
|------|------|------|
| GetOrLoad | `(path, mtime, loadFn) → *Page` | 缓存命中 + mtime 匹配 → 返回；否则调 loadFn 加载并缓存 |
| Invalidate | `(path)` | 逐条失效 |
| InvalidateByPrefix | `(prefix)` | 目录级失效（文件新增/删除） |
| Clear | `()` | 全量清空 |

### 3.3 集成点

```go
// pipeline.go loadContent() 改造
func (p *pipeline) loadContent() error {
    // ...原有逻辑...
    for _, path := range discoveredPaths {
        pg, err := p.contentCache.GetOrLoad(path, fileMtime, func() (*content.Page, error) {
            return content.LoadFile(path)  // 只加载这一个文件
        })
        pages = append(pages, pg)
    }
}
```

### 3.4 失效策略

```
文件变更事件:
  ├── 内容文件修改 → Invalidate(relPath)
  ├── 内容文件新增 → InvalidateByPrefix(sectionPath)
  ├── 内容文件删除 → InvalidateByPrefix(sectionPath)
  └── 模板/配置变更 → Clear()
```

---

## 四、ContextCache 详细设计

### 4.1 数据结构

```go
// internal/build/cache/context.go

type ContextCache struct {
    mu    sync.RWMutex
    items map[*content.Page]*cachedContextEntry
    max   int
}

type cachedContextEntry struct {
    ctx         *tmpl.Context
    pageVersion uint64  // page 内容变更时递增
}
```

### 4.2 关键设计：版本号驱动

每个 `content.Page` 实例增加一个 `Version uint64` 字段。当 page 的 Content/Title/Date/Tags 等元数据变更时，版本号递增。

```go
// content/page.go 新增
type Page struct {
    // ...现有字段...
    Version uint64  // 内容版本号，变更时递增
}
```

### 4.3 失效传播

```
page 内容变更
  └── page.Version++
  └── ContextCache 中该 page 的 entry 失效
  └── DAG 查找 DependedBy → 所有依赖此 page 的聚合页
      └── 聚合页的 Context 也失效（版本号不同）
```

### 4.4 集成点

```go
// pipeline_setup.go buildContexts() 改造
func (p *pipeline) buildContexts() {
    // siteCtx: 模板/i18n 未变则复用
    // 遍历 pages:
    for _, pg := range p.site.Pages {
        if cached := ctxCache.Get(pg); cached != nil && cached.pageVersion == pg.Version {
            lookup[pg] = cached.ctx  // 复用
            continue
        }
        ctx := tmpl.NewContext(pg, siteCtx, p.cfg)
        lookup[pg] = ctx
        ctxCache.Set(pg, ctx, pg.Version)
    }
    // LinkPageRelationships: 只对变更的 page 重连
    // PopulateSitePages: 复用已有 siteCtx.Pages
}
```

---

## 五、FuncCache 详细设计

### 5.1 数据结构

```go
// internal/template/func_cache.go

type FuncCache struct {
    mu    sync.RWMutex
    items map[string]*funcCacheEntry
    max   int
    ttl   time.Duration  // default 60s
}

type funcCacheEntry struct {
    result    any
    expiresAt time.Time
    hits      int64
}
```

### 5.2 缓存的函数

| 函数 | Key 模式 | 说明 |
|------|---------|------|
| `i18n` | `"i18n:" + lang + ":" + key` | 翻译查找，纯函数 |
| `relref` | `"ref:" + pageURL` | 页面引用解析 |
| `urlize` | `"urlize:" + input` | URL 化 |
| `plainify` | `"plainify:" + md5(html)` | HTML 转纯文本 |

### 5.3 集成点

```go
// template/funcs.go 改造
func FuncMap(baseURL string) template.FuncMap {
    fc := templateFuncCache
    return template.FuncMap{
        "i18n": func(key string) string {
            return fc.GetOrCompute("i18n:" + lang + ":" + key, func() any {
                return originalI18nFunc(key)
            }).(string)
        },
        // ...
    }
}
```

---

## 六、渲染并行化

### 6.1 Worker Pool

```go
// pipeline_render.go 改造
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
            // ...render + write...
        }(pg)
    }
    wg.Wait()
}
```

### 6.2 Writer 改造为协程安全

```go
// output/writer.go 改造
type Writer struct {
    // ...原有字段...
    mu sync.Mutex  // 新增，保护 Write 方法
}

func (w *Writer) Write(relPath, content string) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    // ...原有逻辑...
}
```

### 6.3 安全分析

| 组件 | 协程安全 | 说明 |
|------|---------|------|
| `html/template.Execute` | ✅ 安全 | Go 文档明确说明 |
| `p.lookup` map 读 | ✅ 安全 | 只读不写 |
| `p.site.Pages` 迭代 | ✅ 安全 | 只读不写 |
| `output.Writer.Write` | ✅ 安全 | 新增 mutex 保护 |
| `content.Page` 读 | ✅ 安全 | 每 goroutine 持有独立指针 |
| `FuncCache` | ✅ 安全 | RWMutex 保护 |

---

## 七、JIT 极致路径

### 7.1 新入口函数

```go
// internal/build/jit_fast.go (新增)

// JITRenderFast 是 JIT 渲染的极致优化路径。
// 与 RenderPageWithCache 不同，它不走完整 7 阶段管线，
// 而是只加载/渲染/构建目标单页，从 O(N) 降到 O(1)。
//
// 前提：缓存已 warm（经过至少一次全量构建）。
func JITRenderFast(opts Options, cache *PipelineCache, pageURL string) (string, error) {
    // 1. L1 缓存命中检查
    if html := cache.JITCache.Get(pageURL); html != "" {
        return html, nil
    }

    // 2. 单文件加载（跳过全量 loadContent）
    sourceRel := resolveSourceFromURL(pageURL)
    sourcePath := filepath.Join(opts.SourceDir, "content", sourceRel)
    pg, err := cache.ContentCache.GetOrLoad(sourceRel, fileMtime, func() (*content.Page, error) {
        return content.LoadFile(sourcePath)
    })

    // 3. 单页 Markdown 渲染（跳过全量 renderMarkdownAndTree）
    if pg.Content == "" {
        if err := renderSinglePageMarkdown(pg, cache.MDRenderer, cache.SCRegistry); err != nil {
            return "", err
        }
    }

    // 4. 单页 Context 构建（跳过全量 buildContexts）
    ctx := cache.ContextCache.Get(pg)
    if ctx == nil {
        ctx = tmpl.NewContext(pg, siteCtx, cfg)
        cache.ContextCache.Set(pg, ctx, pg.Version)
    }

    // 5. 单页渲染
    html, err := cache.Renderer.Render(tmplName, ctx)
    if err != nil {
        return "", err
    }

    // 6. 写入 L1 缓存
    cache.JITCache.Set(pageURL, html)
    return html, nil
}
```

### 7.2 集成到 daemon

```go
// daemon/builder.go RenderPageJIT() 改造
func (b *Builder) RenderPageJIT(ctx context.Context, pageURL string) (string, error) {
    // 优先走极致路径
    if b.opts.PipelineCache != nil && b.opts.PipelineCache.HasContentCache() {
        return build.JITRenderFast(opts, b.opts.PipelineCache, pageURL)
    }
    // 回退到标准路径
    return b.legacyJITRender(ctx, pageURL)
}
```

---

## 八、PipelineCache 扩展

```go
// build/cache.go 扩展

type PipelineCache struct {
    // 原有字段
    Templates  *template.Template
    I18nBundle *i18n.Bundle
    SCRegistry *shortcode.Registry
    MDRenderer *markdown.Renderer
    SiteCfg    *config.Config
    Writer     *output.Writer
    BuiltAt    time.Time

    // 新增缓存字段
    ContentCache  *ContentCache    // L2 页面内容缓存
    ContextCache  *ContextCache    // L3 上下文缓存
    FuncCache     *FuncCache       // L4 模板函数缓存 (由 template 包持有)
    JITCache      *cache.JITCache  // L1 页面输出缓存 (已有)
    Renderer      *tmpl.Renderer   // 新增：缓存 renderer 避免每次重建
}
```

---

## 九、实现优先级

| 阶段 | 模块 | 文件 | 预期提速 | 风险 |
|------|------|------|---------|------|
| **P0** | ContentCache | `cache/content.go`, `pipeline.go` | 全量 ~2x, 增量 ~5x, JIT ~20x | 低 |
| **P0** | JIT 极致路径 | `jit_fast.go`, `builder.go` | JIT ~10x | 中 |
| **P1** | ContextCache | `cache/context.go`, `pipeline_setup.go`, `content/page.go` | 增量 ~2x | 中 |
| **P1** | 渲染并行化 | `pipeline_render.go`, `writer.go` | 全量 ~2-4x | 中 |
| **P2** | FuncCache | `template/func_cache.go`, `template/funcs.go` | 全量 ~1.2x | 低 |

---

## 十、验收标准

- [ ] 全量构建（~2000 页 zhurongshuo）< 5s
- [ ] 增量构建（单内容变更）< 500ms
- [ ] JIT 首次渲染 < 5ms（缓存 warm 后 < 1ms）
- [ ] `go test -race ./...` 全通过
- [ ] 增量构建行为与全量构建输出一致（`diff-build.sh` 验证）
- [ ] JIT 渲染输出与全量构建输出一致