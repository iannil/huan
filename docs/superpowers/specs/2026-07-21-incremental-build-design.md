# 设计文档：增量构建（Incremental Build）

- **日期**：2026-07-21
- **状态**：Implemented
- **关联**：[ADR 0001](docs/adr/0001-redefine-equivalence.md)、DAG（`internal/daemon/dag/`）
- **实现阶段**：v0.9.0

## 1. 背景

当前 `huan daemon` 模式下的文件变更处理流程：

```
文件变更 → Watcher → EventBus → HandleContentChanged
                                    ↓
                              DAG.AffectedBy()
                                    ↓
                     IncrementalBuild() → 直接回退到 FullBuild()
```

**问题：** 每次文件变更都触发全量构建（8-stage pipeline）。对于 zhurongshuo 这样 2000+ 页面的站点，全量构建耗时明显，影响开发体验。

**已有的基础设施：**
- DAG（依赖图）— 可以计算受影响的页面路径
- `renderSinglePage` — 可以单页渲染
- 构建管线已分为 8 个 stage，结构清晰
- `QueueBuild` — 已有构建队列 + 去重

## 2. 设计目标

1. **性能**：内容变更时，增量构建耗时 < 全量构建的 10%
2. **正确性**：增量构建的输出与全量构建完全一致
3. **自动降级**：缓存失效或模板变更时自动回退到全量构建
4. **可观测**：增量构建的状态（跳过/命中/降级）可通过日志观察
5. **兼容现有代码**：不改动已有的 build pipeline stage 结构

## 3. 架构总览

```
全量构建（首次/模板变更时）:
  build.BuildSite() → 8 个 stage → 输出 + DAG
                                    ↓
                            AfterBuild 回调
                                    ↓
                          PipelineCache ← 缓存可复用状态

增量构建（内容变更时）:
  ┌─────────────────────────────────────────────────────┐
  │  IncrementalBuild(changedFiles)                      │
  │                                                      │
  │  ① hasTemplateChanges() → 是 → FullBuild()          │
  │  ② PipelineCache == nil → FullBuild()               │
  │  ③ DAG.AffectedBy() → 受影响页面列表                │
  │  ④ 只重新加载变更的 .md 文件（不复用缓存内容）        │
  │  ⑤ 渲染 Markdown（复用 MDRenderer + SCRegistry）    │
  │  ⑥ 按依赖顺序渲染页面（复用 Templates + Context）    │
  │  ⑦ 写入输出文件（复用 Writer）                       │
  │  ⑧ 更新 DAG + KnownSources                          │
  └─────────────────────────────────────────────────────┘
```

## 4. 核心组件

### 4.1 PipelineCache

```go
// internal/build/cache.go

// PipelineCache holds reusable build state across incremental builds.
// Populated by the AfterBuild callback after a full build completes.
// Incremental builds reuse this state to avoid re-parsing templates,
// reloading all content, and rebuilding contexts for every change.
type PipelineCache struct {
    // 可复用状态（模板未变时一直有效）
    Templates    *template.Template
    I18nBundle   *i18n.Bundle
    SCRegistry   *shortcode.Registry
    MDRenderer   *markdown.Renderer

    // 站点上下文（内容不变时一直有效）
    SiteCfg       *config.Config
    SiteCtx       *tmpl.SiteContext
    PageLookup    map[string]*content.Page   // URL → Page
    ContextLookup map[string]*tmpl.Context   // URL → Template Context

    // 输出写入器
    Writer        *output.Writer

    // 已知源文件跟踪
    KnownSources  map[string]string          // 源文件路径 → 页面 URL
    BuiltAt       time.Time                  // 上次全量构建时间
}

func NewPipelineCache() *PipelineCache
```

> **实现说明**：PipelineCache 只缓存渲染基础设施（模板/i18n/markdown/writer），
> 不缓存 PageLookup/ContextLookup。因为内容变更时 context 引用的 page 指针
> 会失效，缓存它们会导致列表页输出过期。增量构建时重建内容+context（开销小），
> 但只渲染受 DAG 影响的页面（跳过 ~95% 的模板渲染）。

### 4.2 模板变更检测

```go
// hasTemplateChanges checks whether any changed file invalidates the
// pipeline cache (templates, i18n, config). Returns true = need full rebuild.
func hasTemplateChanges(changedFiles []string, sourceDir string) bool {
    for _, f := range changedFiles {
        rel, err := filepath.Rel(sourceDir, f)
        if err != nil {
            continue
        }
        switch {
        case strings.HasPrefix(rel, "layouts"):
            return true
        case strings.HasPrefix(rel, "i18n"):
            return true
        case rel == "huan.yaml":
            return true
        case strings.HasPrefix(rel, "themes"):
            return true
        }
    }
    return false
}
```

### 4.3 DAG 增强

在 `internal/daemon/dag/graph.go` 中新增方法：

```go
// OrderByDependency returns the page paths in topological order
// (leaf pages first, pages that depend on them later).
// This ensures correct rendering order during incremental builds:
// a section page is rendered after its child pages are updated.
func (dg *DependencyGraph) OrderByDependency(pagePaths []string) []string
```

### 4.4 Builder 增强

```go
// internal/daemon/builder.go — BuilderOptions 新增
type BuilderOptions struct {
    // ... 原有字段
    PipelineCache *build.PipelineCache
}

// IncrementalBuild 的完整实现（不再回退到 FullBuild）
func (b *Builder) IncrementalBuild(ctx context.Context, changedFiles []string) error {
    // 1. 模板变更检测 → 全量构建
    if hasTemplateChanges(changedFiles, b.opts.SourceDir) {
        b.opts.Logf("builder: template change, falling back to full build")
        return b.executeFullBuild(ctx)
    }

    // 2. PipelineCache 不存在 → 全量构建
    cache := b.opts.PipelineCache
    if cache == nil {
        b.opts.Logf("builder: no pipeline cache, falling back to full build")
        return b.executeFullBuild(ctx)
    }

    // 3. DAG 计算受影响的页面
    affected := b.opts.DAG.AffectedBy(changedFiles)
    if len(affected) == 0 {
        return nil
    }

    // 4. 按依赖顺序排列
    ordered := b.opts.DAG.OrderByDependency(affected)

    // 5. 只重新加载变更的源文件
    reloadedPages := reloadChangedPages(changedFiles, cache, b.opts.SourceDir)

    // 6. 更新 PageLookup 中的变更页面
    for _, pg := range reloadedPages {
        cache.PageLookup[pg.URL] = pg
    }

    // 7. 按顺序渲染受影响的页面
    for _, pageURL := range ordered {
        if err := renderSinglePageIncremental(pageURL, cache); err != nil {
            b.opts.Logf("builder: incremental render error for %s: %v", pageURL, err)
        }
    }

    // 8. 更新 DAG（如有新增/删除页面）
    b.syncDAG(changedFiles, reloadedPages)

    // 9. 发布构建完成事件
    _ = b.opts.Bus.Publish(ctx, eventbus.Event{
        Type:      eventbus.EventBuildCompleted,
        Timestamp: time.Now(),
        Payload:   map[string]interface{}{"incremental": true, "pages": len(ordered)},
    })

    return nil
}
```

## 5. 增量构建流程详解

### 5.1 内容变更 → 增量构建

```
1. Watcher 检测到 content/posts/hello.md 变更
2. HandleContentChanged 收到 EventContentChanged
3. IncrementalBuild 被调用
4. hasTemplateChanges → false（不是模板）
5. PipelineCache 存在 → 继续
6. DAG.AffectedBy(["content/posts/hello.md"]) → ["/posts/hello/", "/posts/", "/tags/hello/", "/"]
7. OrderByDependency → ["/posts/hello/", "/posts/", "/tags/hello/", "/"]
8. 重新加载 content/posts/hello.md → 新的 Page 对象
9. 更新 PageLookup["/posts/hello/"] = 新 Page
10. 渲染 /posts/hello/ → 写入输出
11. 渲染 /posts/ → 写入输出（section 列表页）
12. 渲染 /tags/hello/ → 写入输出
13. 渲染 / → 写入输出（home 页）
```

### 5.2 数据文件变更 → 增量构建

```
1. Watcher 检测到 data/books.yaml 变更
2. DAG 目前不跟踪数据文件依赖 → 全量构建（安全降级）
   （后续可扩展 DAG 以支持数据文件依赖追踪）
```

### 5.3 模板变更 → 全量构建

```
1. Watcher 检测到 layouts/_default/single.html 变更
2. hasTemplateChanges → true
3. 回退到 executeFullBuild()
4. 全量构建完成后，PipelineCache 被重新填充
```

## 6. BuilderOptions 变更

```go
// internal/daemon/builder.go
type BuilderOptions struct {
    SourceDir   string
    OutputDir   string
    Bus         eventbus.EventBus
    DAG         *dag.DependencyGraph
    JITCache    *cache.JITCache
    Metrics     *MetricsCollector
    BuildDrafts bool
    Logf        func(format string, args ...any)

    OnAfterBuild func(*build.Result) error

    // PipelineCache 保存可复用的构建状态
    // 首次全量构建后由 AfterBuild 填充
    // IncrementalBuild 时复用
    PipelineCache *build.PipelineCache
}
```

## 7. Daemon 启动流程变更

```
daemon.Run() 现有流程:
  1. Load config
  2. Init EventBus
  3. Init Cache
  4. Init Health + Metrics
  5. Create temp dir
  6. Init DAG
  7. Init Builder
  8. Init Plugin Lifecycle Manager
  9. Init Serving
  10. Subscribe event handlers
  11. Initial full build  ← 在这里填充 PipelineCache
  12. Start file watcher
  13. Start HTTP server
```

在步骤 11（Initial full build）的 `OnAfterBuild` 回调中填充 `PipelineCache`：

```go
// daemon.go — 创建 Builder 时
d.builder = NewBuilder(BuilderOptions{
    // ... 其他选项
    PipelineCache: build.NewPipelineCache(),
    OnAfterBuild: func(r *build.Result) error {
        // PipelineCache 在 build.BuildSite 的 AfterBuild 中被填充
        return nil
    },
})

// daemon.go — 全量构建完成后
// PipelineCache 已通过 AfterBuild 回调被填充
d.builder.opts.PipelineCache = pipelineCache
```

## 8. 测试策略

### 8.1 单元测试

| 测试 | 说明 |
|------|------|
| `TestPipelineCache_StoreAndRetrieve` | 存储和读取缓存 |
| `TestHasTemplateChanges_Layouts` | 模板变更检测 |
| `TestHasTemplateChanges_Content` | 内容文件不被误判 |
| `TestHasTemplateChanges_Config` | 配置文件变更检测 |
| `TestOrderByDependency` | DAG 拓扑排序 |

### 8.2 集成测试

| 测试 | 说明 |
|------|------|
| `TestDaemon_IncrementalBuild_SinglePage` | 单个页面变更 |
| `TestDaemon_IncrementalBuild_TemplateChange` | 模板变更回退到全量构建 |
| `TestDaemon_IncrementalBuild_NoCache` | 无缓存回退 |
| `TestDaemon_IncrementalBuild_MultipleFiles` | 多个文件同时变更 |
| `TestDaemon_IncrementalBuild_OutputConsistency` | 增量输出与全量输出一致 |

## 9. 文件变更清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/build/cache.go` | 新增 | PipelineCache 结构体 |
| `internal/build/cache_test.go` | 新增 | 缓存测试 |
| `internal/build/build.go` | 修改 | AfterBuild 中填充 PipelineCache |
| `internal/daemon/builder.go` | 修改 | IncrementalBuild 完整实现 |
| `internal/daemon/daemon.go` | 修改 | 传递 PipelineCache |
| `internal/daemon/daemon_test.go` | 修改 | 增量构建集成测试 |
| `internal/daemon/dag/graph.go` | 修改 | 新增 OrderByDependency |

## 10. 未来扩展

- **数据文件依赖追踪**：DAG 扩展以支持 `data/` 文件的依赖
- **静态文件增量**：`static/` 变更时增量复制而非全量
- **并行渲染**：独立的页面可以并行渲染
- **热模板重载**：模板变更时只重新解析模板树，不重新加载内容