# 设计文档：JIT 渲染（Just-In-Time Page Rendering）

- **日期**：2026-07-22
- **状态**：Draft
- **关联**：[增量构建](2026-07-21-incremental-build-design.md)、[热插拔插件系统](2026-07-20-daemon-hotplug-plugin-design.md)
- **实现阶段**：v0.10.0

## 1. 背景

daemon 的 serving 层已搭好 JIT 渲染骨架：

```
serving.jitFallback(path):
  1. JITCache.Get(path) → 命中返回
  2. Builder.RenderPageJIT(ctx, path) → 渲染并缓存
```

但 `Builder.RenderPageJIT` 当前是 **stub**：

```go
// internal/daemon/builder.go:312
func (b *Builder) RenderPageJIT(ctx context.Context, pagePath string) (string, error) {
    return "", fmt.Errorf("JIT rendering: page lookup from DAG not yet implemented — use full build for now")
}
```

**后果：** daemon 收到未预构建页面的请求时，JIT fallback 永远返回 404。daemon 无法按需渲染 draft 页面、构建时跳过的页面、或增量构建尚未触及的新页面。

## 2. 设计目标

1. **按需渲染**：未预构建的页面能被 daemon 实时渲染返回
2. **复用缓存**：JIT 是高频路径，必须复用 `PipelineCache`（模板/i18n/markdown/writer）才能快
3. **正确性**：JIT 渲染输出与全量构建一致（含 LiveReload、列表页 context）
4. **自动失效**：内容/模板变更时 JITCache 自动失效，不返回过期内容
5. **优雅降级**：PipelineCache 不存在时仍能渲染（慢但正确）

## 3. 架构总览

```
daemon serving 收到请求 GET /posts/new-draft/
    │
    ▼
静态文件查找（pathResolvesToFile）
    │ 未找到
    ▼
jitFallback(path)
    │
    ├─ JITCache.Get(path) 命中？ → 返回（X-Huan-Cache: jit）
    │
    └─ 未命中 → Builder.RenderPageJIT(ctx, path)
                    │
                    ▼
                ① resolveSourceFile: pageURL → 源文件
                   （DAG.SourceFromPagePath 或 URL 推导 fallback）
                ② 验证源文件存在
                ③ build.RenderPageWithCache(opts, cache, pageURL)
                   （复用 PipelineCache + 重建内容/context + 渲染单页）
                ④ 返回 HTML（不写文件）
                    │
                    ▼
                JITCache.Set(path, html) → 返回客户端（X-Huan-Cache: jit-hit）
```

## 4. 核心组件

### 4.1 URL → 源文件推导

**情况 1：页面在 DAG 中**（上次全量构建包含它）

```
pageURL = "/posts/hello/"
→ DAG.SourceFromPagePath("/posts/hello/")
→ Node.SourceFile = "posts/hello.md"
→ 源文件 = <sourceDir>/content/posts/hello.md
```

新增 DAG 反向查找方法（`internal/daemon/dag/graph.go`）：

```go
// SourceFromPagePath returns the source file path (relative to content/)
// for the given page URL. Returns "", false if the URL is not in the graph.
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

**情况 2：页面不在 DAG 中**（新建 draft、构建时跳过的页面）

URL 推导 fallback（`internal/build/jit.go`）。huan 的 URL→源文件规则：

| URL | 推导源文件 |
|-----|-----------|
| `/` (home) | `content/_index.md` |
| `/posts/` (section) | `content/posts/_index.md` |
| `/posts/hello/` | `content/posts/hello.md` |
| `/posts/2026/new-year/` | `content/posts/2026/new-year.md` |
| `/books/v1/ch1/` | `content/books/v1/ch1.md` |

```go
// resolveSourceFromURL derives the source file path (relative to content/)
// from a page URL. Used when the URL is not in the DAG.
// Returns "" if it cannot be derived.
func resolveSourceFromURL(pageURL string) string {
    u := strings.Trim(pageURL, "/")
    if u == "" {
        return "_index.md" // home
    }
    parts := strings.Split(u, "/")
    last := parts[len(parts)-1]
    if last == "_index" {
        // /posts/_index/ → content/posts/_index.md
        return strings.Join(parts, "/") + ".md"
    }
    if len(parts) == 1 {
        // /posts/ → content/posts/_index.md (single-segment = section)
        return parts[0] + "/_index.md"
    }
    // /posts/hello/ → content/posts/hello.md
    return strings.Join(parts, "/") + ".md"
}
```

**组合查找：**

```go
func resolveSourceFile(dg *dag.DependencyGraph, pageURL string) (string, bool) {
    // 1. DAG 反向查找（精确）
    if src, ok := dg.SourceFromPagePath(pageURL); ok {
        return src, true
    }
    // 2. URL 推导（fallback）
    if src := resolveSourceFromURL(pageURL); src != "" {
        return src, true
    }
    return "", false
}
```

### 4.2 RenderPageWithCache

`internal/build/jit.go`（新增）：

```go
// RenderPageWithCache renders a single page on demand, reusing the cached
// rendering infrastructure (templates/i18n/markdown/writer). Unlike
// IncrementalRender, it returns the HTML without writing to disk, and
// targets exactly one page identified by URL.
//
// Used by daemon's JIT fallback when a requested URL is not in the
// pre-built static output. IncludeDrafts is forced true (JIT primarily
// serves draft pages that were skipped at build time).
func RenderPageWithCache(opts Options, cache *PipelineCache, pageURL string) (string, error)
```

内部流程：

1. 配置：复用 `cache.SiteCfg`（应用 serve-mode overrides），fallback `loadConfig`
2. 强制 `IncludeDrafts = true`（JIT 主要服务 draft）
3. 加载内容 + markdown（重建 context 保证列表页正确，与 IncrementalRender 同策略）
4. 复用 `cache.Templates/i18nBundle/scRegistry/md/Writer`（fallback `setupTemplatesAndWriter`）
5. `tmpl.SetI18nBundle` 重新接线全局 i18n
6. 重建 context
7. 按 URL 找到目标 page
8. `renderSinglePage(target)` 返回 HTML（不写文件）
9. serve-mode LiveReload 注入

### 4.3 Builder.RenderPageJIT 重写

```go
func (b *Builder) RenderPageJIT(ctx context.Context, pageURL string) (string, error) {
    cache := b.opts.PipelineCache

    // 1. pageURL → 源文件
    sourceRel, ok := resolveSourceFile(b.opts.DAG, pageURL)
    if !ok {
        return "", fmt.Errorf("JIT: cannot resolve source for %s", pageURL)
    }

    // 2. 验证源文件存在
    sourcePath := filepath.Join(b.opts.SourceDir, "content", sourceRel)
    if _, err := os.Stat(sourcePath); err != nil {
        return "", fmt.Errorf("JIT: source not found: %s", sourcePath)
    }

    // 3. 复用 PipelineCache 渲染
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
```

## 5. JITCache 失效策略

JIT 缓存的 HTML 必须在内容/模板变更时失效。

| 触发事件 | JITCache 行为 |
|---------|--------------|
| 全量构建完成（`executeFullBuild`） | `JITCache.Clear()` |
| 增量构建完成（`IncrementalBuild`） | 受影响 URL：`JITCache.Remove(url)` |
| 模板变更（回退全量） | `JITCache.Clear()` |
| TTL 过期（默认 5 分钟） | 自动逐出（已有机制） |

```go
// executeFullBuild 成功后
b.opts.JITCache.Clear()

// IncrementalBuild 渲染后
for _, url := range ordered {
    b.opts.JITCache.Remove(url)
}
```

## 6. 与现有系统的集成

### 6.1 serving.jitFallback（已存在，无需改）

```go
// internal/daemon/serving.go:106 — 现有代码
func (s *Serving) jitFallback(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Path
    if entry := s.opts.JITCache.Get(path); entry != nil {
        // 命中缓存
        w.Write(entry.HTML)
        return
    }
    html, err := s.opts.Builder.RenderPageJIT(r.Context(), path)
    if err != nil {
        http.NotFound(w, r)  // JIT 失败 → 404
        return
    }
    s.opts.JITCache.Set(path, &cache.JITEntry{...})
    w.Write([]byte(html))
}
```

只需 `RenderPageJIT` 实现，serving 层零改动。

### 6.2 与增量构建协同

`PipelineCache` 同时服务于：
- `IncrementalRender`（写文件，内容变更触发）
- `RenderPageWithCache`（返回 HTML，请求触发）

两者共享底层（重建内容+context，复用缓存模板），职责分离。

## 7. JIT 渲染场景边界

**适用：**
- 未预构建的 draft 页面（`buildDrafts: false` 时被跳过）
- 构建时出错的页面
- 增量构建尚未触及的新页面

**不适用（返回 404）：**
- URL 无法推导源文件
- 源文件不存在于磁盘
- 源文件 frontmatter 解析失败

## 8. Error Handling

| 场景 | 处理 | 客户端响应 |
|------|------|-----------|
| PipelineCache 不存在 | fallback `setupTemplatesAndWriter` | 正常渲染（慢） |
| 源文件不存在 | 返回 error | 404 |
| URL 无法推导 | 返回 error | 404 |
| frontmatter 解析失败 | target 找不到 | 404 |
| 模板渲染失败 | 返回 error | 500 + 日志 |
| 渲染超时 | context 取消 | 503 |

所有 JIT 错误通过 `logf` 记录，不打断 daemon 服务。

## 9. 测试策略

### 9.1 单元测试

| 测试 | 说明 |
|------|------|
| `TestRenderPageWithCache_SinglePage` | 复用缓存渲染单页，返回 HTML |
| `TestRenderPageWithCache_DraftPage` | draft 页面能被 JIT 渲染 |
| `TestRenderPageWithCache_PageNotFound` | 不存在 URL 返回 error |
| `TestRenderPageWithCache_NoCache` | cache=nil fallback 仍能渲染 |
| `TestResolveSourceFromURL` | URL 推导规则覆盖各种路径 |
| `TestSourceFromPagePath_Found` | DAG 反向查找命中 |
| `TestSourceFromPagePath_NotFound` | DAG 反向查找未命中 |

### 9.2 集成测试

| 测试 | 说明 |
|------|------|
| `TestDaemon_JIT_RenderUnbuiltPage` | 渲染未预构建页面 + 验证缓存填充 + 二次命中 |
| `TestDaemon_JIT_PageNotFound` | 不存在 URL 返回 error |
| `TestJITCache_InvalidatedOnFullBuild` | 全量构建后缓存清空 |
| `TestJITCache_InvalidatedOnIncrementalBuild` | 增量构建后受影响 URL 缓存移除 |
| `TestDaemon_JIT_RendersViaHTTP` | HTTP GET 未预构建页面端到端 |

## 10. 文件变更清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/build/jit.go` | 新增 | RenderPageWithCache + resolveSourceFromURL |
| `internal/build/jit_test.go` | 新增 | JIT 渲染 + URL 推导测试 |
| `internal/daemon/dag/graph.go` | 修改 | 新增 SourceFromPagePath |
| `internal/daemon/dag/graph_test.go` | 修改 | SourceFromPagePath 测试 |
| `internal/daemon/builder.go` | 修改 | 重写 RenderPageJIT（去 stub）+ JITCache 失效钩子 |
| `internal/daemon/daemon_test.go` | 修改 | JIT 集成测试 |

## 11. 未来扩展

- **JIT 缓存预热**：daemon 启动后预渲染高频访问页面
- **JIT 缓存指标**：命中率、渲染耗时接入 Prometheus
- **按需渲染受保护内容**：JIT + 认证中间件，动态渲染会员内容
- **JIT 失败降级**：渲染失败时返回预构建的 fallback 页面（如有）