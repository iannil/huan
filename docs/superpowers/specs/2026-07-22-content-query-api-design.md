# 设计文档：内容查询 REST API

- **日期**：2026-07-22
- **状态**：Implemented
- **关联**：[JIT 渲染](2026-07-22-jit-rendering-design.md)、[增量构建](2026-07-21-incremental-build-design.md)、contentapi.go（`internal/output/contentapi.go`）
- **实现阶段**：v0.11.0

## 1. 背景

daemon 当前暴露的路由全是运维/管理 API（`/health`、`/metrics`、`/admin/api/*`），**终端用户只能拿到预构建静态页面**。daemon 作为持久化服务端内容引擎，缺少面向终端用户的运行时内容查询能力。

已有基础：构建时 `GenerateContentAPI` 生成 `/api/{section}.json`（AI 消费者用的静态 JSON 导出，按 section 分文件，已过滤 draft/future/expired）。

**缺口：** 没有动态查询能力（过滤/分页/聚合/全文搜索）。前端要做内容发现（按 tag/section/关键词查询）只能全量加载静态 JSON 再客户端过滤，大数据量下性能差。

## 2. 设计目标

1. **面向终端用户**：公开只读 API，无需 token
2. **动态查询**：支持 section/tag/全文/分页/排序
3. **复用数据源**：包裹预构建 `/api/{section}.json`，不重新造数据源
4. **与构建同步**：构建后自动刷新内存索引
5. **安全**：只返回已发布内容（draft/future/expired 已在数据源过滤）

## 3. 架构总览

```
构建管线（已有）
  │
  └─ GenerateContentAPI → /api/{section}.json（预构建，已排除 draft）
                              │
                              ▼
daemon 启动 / 构建后
  ┌───────────────────────────────────────────┐
  │  ContentIndex（daemon 内存，新增）         │
  │  加载所有 /api/{section}.json → []Item    │
  └──────────────────┬────────────────────────┘
                     │ 查询
                     ▼
  GET /api/v1/pages?section=posts&tag=go&q=key&page=1
  GET /api/v1/pages/{url}
  GET /api/v1/tags
  GET /api/v1/sections
                     │
                     ▼
              JSON 响应（公开只读）
```

### 路径冲突处理

预构建 `/api/{section}.json`（如 `/api/posts.json`）与运行时 API 共存：
- `/api/posts.json` → serving 的 fileServer 服务静态文件（落在 `/` catch-all）
- `/api/v1/*` → mux 精确注册的 API handler（在 `/` catch-all 之前匹配）

版本前缀 `/v1/` 天然隔离，两者不冲突。

## 4. 核心组件

### 4.1 ContentIndex

`internal/daemon/contentindex/index.go`（新增）：

```go
// Item 是查询 API 返回的单条内容（脱敏：默认不含完整正文）。
type Item struct {
    Title       string   `json:"title"`
    URL         string   `json:"url"`          // 相对路径，如 /posts/hello/
    Section     string   `json:"section"`      // 来自 JSON 文件名
    Date        string   `json:"date"`
    Description string   `json:"description,omitempty"`
    Summary     string   `json:"summary,omitempty"`
    Tags        []string `json:"tags,omitempty"`
}

// ContentIndex 是 daemon 内存中的内容查询索引。
// 从预构建的 /api/{section}.json 加载，构建后刷新。
type ContentIndex struct {
    mu    sync.RWMutex
    items []Item
    baseURL string // 用于绝对 URL → 相对 URL 转换
}

func NewContentIndex(baseURL string) *ContentIndex

// LoadFromDir 从 outputDir/api/*.json 加载所有 section 文件。
// 跳过损坏的 JSON 文件（log warn），不中断整体加载。
func (ci *ContentIndex) LoadFromDir(outputDir string) error

// Query 按过滤条件查询，返回分页结果。
func (ci *ContentIndex) Query(filter Filter) Result

// GetByURL 按 URL 查询单个 item。
func (ci *ContentIndex) GetByURL(url string) (Item, bool)

// Tags 返回所有 tag 及其页面数。
func (ci *ContentIndex) Tags() map[string]int

// Sections 返回所有 section 及其页面数。
func (ci *ContentIndex) Sections() map[string]int
```

### 4.2 Filter / Result

```go
type Filter struct {
    Section string // 按 section 过滤
    Tag     string // 按 tag 过滤
    Query   string // 全文搜索（Title/Summary/Description，大小写不敏感）
    Page    int    // 页码，1-based
    Limit   int    // 每页数量，默认 10，上限 50
    Sort    string // date（默认 desc）
}

type Result struct {
    Data  []Item `json:"data"`
    Total int    `json:"total"`
    Page  int    `json:"page"`
    Limit int    `json:"limit"`
}
```

### 4.3 关键设计决策

1. **URL 规范化**：预构建 JSON 的 URL 是绝对 URL（`cfg.BaseURL + path`），ContentIndex 加载时转成相对路径（去掉 BaseURL），便于 API 消费和 `GetByURL` 匹配。
2. **不返回完整正文**：Item 默认不含 `Plain`（纯文本正文），避免响应过大；完整内容走预构建页面或 JIT。如需全文，未来扩展 `?fields=plain`。
3. **draft 已排除**：数据源 `GenerateContentAPI` 已过滤 draft/future/expired，ContentIndex 无需重复过滤。
4. **全文搜索**：在 Title/Summary/Description 子串匹配，大小写不敏感。MVP 用 `strings.Contains`；未来可接入倒排索引（bleve 等）。

## 5. 查询端点详细设计

所有端点注册在 daemon serving 的 mux 上，前缀 `/api/v1/`，公开只读（无 token）。

```
GET /api/v1/pages           列表 + 过滤 + 分页
GET /api/v1/pages/{path}    单页详情（按 URL）
GET /api/v1/tags            所有 tag + 页面数
GET /api/v1/sections        所有 section + 页面数
```

### GET /api/v1/pages 查询参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `section` | 空 | 按 section 过滤（如 `posts`） |
| `tag` | 空 | 按 tag 过滤（如 `go`） |
| `q` | 空 | 全文搜索（匹配 Title/Summary/Description，大小写不敏感） |
| `page` | 1 | 页码，1-based |
| `limit` | 10 | 每页数量，上限 50 |
| `sort` | date | 排序字段，默认 date desc |

响应：

```json
{
  "data": [
    {"title": "Hello", "url": "/posts/hello/", "section": "posts", "date": "2026-01-01", "summary": "...", "tags": ["go"]}
  ],
  "total": 42,
  "page": 1,
  "limit": 10
}
```

### GET /api/v1/pages/{path} 单页详情

```
GET /api/v1/pages/posts/hello/   → 返回该 URL 的 Item（404 若不存在）
```

Handler 从 `/api/v1/pages/` 之后截取剩余作为页面 URL（补回前导 `/`）。

### GET /api/v1/tags / sections

```json
// /api/v1/tags
{"go": 5, "rust": 3, "huan": 8}

// /api/v1/sections
{"posts": 120, "books": 15, "gallery": 30}
```

## 6. ContentIndex 刷新机制

| 触发事件 | 行为 |
|---------|------|
| daemon 启动 | 首次 `LoadFromDir(outputDir)` |
| 全量构建完成 | `LoadFromDir(outputDir)`（重新加载所有 section） |
| 增量构建完成 | `LoadFromDir(outputDir)`（增量改变了内容，JSON 已更新） |

```go
// executeFullBuild 成功后
if b.opts.ContentIndex != nil {
    if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
        b.opts.Logf("builder: content index reload: %v", err)
    }
}

// IncrementalBuild 成功后（同上）
```

增量构建已重新生成 contentapi JSON（`IncrementalRender` 的 `renderAIOutputs`），所以构建后 ContentIndex reload 即可拿到最新数据。

## 7. HTTP Handler

`internal/daemon/contentindex/handler.go`（新增）：

```go
// Handler 暴露 /api/v1/* 查询端点。公开只读，无需 token。
type Handler struct {
    index *ContentIndex
}

func NewHandler(index *ContentIndex) *Handler

// ServeHTTP 路由 /api/v1/pages, /api/v1/tags, /api/v1/sections。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

**路径匹配逻辑：**
- `/api/v1/pages`（无尾部路径）→ 列表查询
- `/api/v1/pages/posts/hello/`（有尾部路径）→ 单页详情
- `/api/v1/tags` → tag 聚合
- `/api/v1/sections` → section 聚合

### daemon serving 集成

在 `internal/daemon/serving.go` 的 `Start()` 中注册（admin handler 之前，`/` catch-all 之前）：

```go
if s.opts.ContentAPI != nil {
    mux.Handle("/api/v1/", s.opts.ContentAPI)
}
```

mux 精确前缀匹配 `/api/v1/` 优先于 `/` catch-all；`/api/posts.json`（无 v1）继续走 fileServer 静态服务。

## 8. BuilderOptions / ServingOptions / Daemon 集成

```go
// BuilderOptions 新增
ContentIndex *contentindex.ContentIndex

// ServingOptions 新增
ContentAPI http.Handler  // 来自 NewHandler(index)

// daemon.go Run()
contentIndex := contentindex.NewContentIndex(cfg.BaseURL)
// 注入 Builder（用于构建后刷新）
// 注入 Serving（作为 ContentAPI handler）
// daemon 启动时首次 LoadFromDir
```

daemon 启动时检查 `cfg.AI.ContentAPI`，若未开启则不注册 handler（log warn：数据源未生成）。

## 9. Error Handling

| 场景 | 响应 |
|------|------|
| 无效 page/limit（<1 或非数字） | 用默认值（1/10），不报错 |
| limit > 50 | 截断为 50 |
| 过滤后无结果 | 200 + `{"data":[], "total":0}` |
| 单页 URL 不存在 | 404 + `{"error":"not found"}` |
| ContentIndex 未加载（启动中） | 503 + `{"error":"index not ready"}` |
| JSON 文件损坏 | LoadFromDir 跳过坏文件，log warn |
| 未开启 contentAPI 配置 | handler 不注册，请求落到静态服务 |

## 10. 测试策略

### 10.1 单元测试

| 测试 | 说明 |
|------|------|
| `TestLoadFromDir_LoadsAllSections` | 多个 section JSON 全部加载 |
| `TestLoadFromDir_RelativeURL` | 绝对 URL 转相对（去掉 BaseURL） |
| `TestLoadFromDir_MalformedJSON` | 坏文件跳过，不中断 |
| `TestLoadFromDir_EmptyDir` | 空目录不报错 |
| `TestQuery_SectionFilter` | section 过滤 |
| `TestQuery_TagFilter` | tag 过滤 |
| `TestQuery_FullTextSearch` | q 在 Title/Summary/Description 匹配，大小写不敏感 |
| `TestQuery_Pagination` | page/limit 分页正确 |
| `TestQuery_LimitCapped` | limit > 50 截断 |
| `TestQuery_SortByDate` | date desc 默认排序 |
| `TestQuery_NoMatch` | 无结果返回空 data |
| `TestGetByURL_Found/NotFound` | URL 查询 |
| `TestTags` | tag 聚合 + 计数 |
| `TestSections` | section 聚合 + 计数 |

### 10.2 Handler 测试

| 测试 | 说明 |
|------|------|
| `TestHandler_PagesList` | GET /api/v1/pages 返回分页 JSON |
| `TestHandler_PagesFilter` | section/tag/q 参数生效 |
| `TestHandler_PageDetail` | GET /api/v1/pages/{path} 返回单页 |
| `TestHandler_PageDetail404` | 不存在 URL 返回 404 |
| `TestHandler_Tags/Sections` | 聚合端点 |
| `TestHandler_NoTokenRequired` | 无 token 也能访问（公开） |

### 10.3 集成测试

| 测试 | 说明 |
|------|------|
| `TestDaemon_ContentAPI_AfterBuild` | 构建后 API 可查询到内容 |
| `TestDaemon_ContentAPI_RefreshOnBuild` | 内容变更 → 构建 → API 返回新内容 |
| `TestDaemon_ContentAPI_ExcludesDraft` | draft 不出现在 API |

## 11. 文件变更清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/daemon/contentindex/index.go` | 新增 | ContentIndex + Item + Filter + Result |
| `internal/daemon/contentindex/index_test.go` | 新增 | 索引 + 查询测试 |
| `internal/daemon/contentindex/handler.go` | 新增 | HTTP Handler |
| `internal/daemon/contentindex/handler_test.go` | 新增 | Handler 测试 |
| `internal/daemon/serving.go` | 修改 | 注册 /api/v1/ handler |
| `internal/daemon/builder.go` | 修改 | ContentIndex 刷新钩子 |
| `internal/daemon/daemon.go` | 修改 | 创建 ContentIndex + Handler，注入 |
| `internal/daemon/daemon_test.go` | 修改 | 集成测试 |

## 12. 未来扩展

- **全文搜索引擎**：MVP 用 `strings.Contains`，未来接入 bleve 倒排索引支持中文分词、相关性排序
- **字段选择**：`?fields=plain` 返回完整正文
- **缓存**：高频查询结果缓存（ETag / Cache-Control）
- **认证分层**：protected/private 内容需 token（会员/付费场景）
- **GraphQL**：复杂查询需求演进
- **实时更新**：WebSocket/SSE 推送内容变更给前端