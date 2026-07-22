# JIT 渲染 + 内容查询 REST API 完成报告

> 完成日期：2026-07-22  ·  对标：daemon 运行时能力补齐（按需渲染 + 面向终端用户的内容 API）
> 设计原文：
> - [JIT 渲染设计](../../superpowers/specs/2026-07-22-jit-rendering-design.md)
> - [内容查询 REST API 设计](../../superpowers/specs/2026-07-22-content-query-api-design.md)

## 1. 概述

本次会话围绕 daemon 的运行时能力缺口，连续交付两个功能：

1. **JIT 渲染（Just-In-Time Page Rendering）** — 把 `Builder.RenderPageJIT` 的 stub 替换为真实实现，daemon 能按需渲染未预构建的页面（draft、构建时跳过的、增量未触及的），复用 PipelineCache 保证性能
2. **内容查询 REST API（`/api/v1/*`）** — daemon 暴露面向终端用户的公开只读内容查询 API，从预构建 `/api/{section}.json` 加载内存索引，支持 section/tag/全文/分页查询

这两个功能共同把 daemon 从"只服务预构建静态页 + 运维 API"升级为"预构建 + 按需渲染 + 面向终端用户的动态查询"的服务端内容平台。

## 2. 新增依赖

无新增外部依赖。全部基于标准库（`net/http`、`encoding/json`、`unicode/utf8`）+ 已有内部包。

## 3. 新增 / 修改的包

### 3.1 JIT 渲染

| 路径 | 职责 |
|---|---|
| `internal/build/jit.go` | `RenderPageWithCache`（复用 PipelineCache 渲染单页返回 HTML）+ `ResolveSourceFromURL`（URL→源文件推导） |
| `internal/daemon/dag/graph.go` | `SourceFromPagePath`（URL→源文件反向查找） |
| `internal/daemon/builder.go` | `RenderPageJIT` 重写（去 stub）+ `resolveSourceFile`（DAG+URL 组合）+ JITCache 失效钩子 |

### 3.2 内容查询 REST API

| 路径 | 职责 |
|---|---|
| `internal/daemon/contentindex/index.go` | `ContentIndex`（内存索引）+ `Item`/`Filter`/`Result` + `LoadFromDir`/`Query`/`GetByURL`/`Tags`/`Sections` |
| `internal/daemon/contentindex/handler.go` | HTTP Handler（`/api/v1/*` 路由） |
| `internal/daemon/serving.go` | 注册 `/api/v1/` handler |
| `internal/daemon/builder.go` | ContentIndex 刷新钩子（全量+增量构建后） |
| `internal/daemon/daemon.go` | 创建 ContentIndex + Handler，按 `cfg.AI.ContentAPI` 开关注入 |

## 4. CLI / API 变更

### 新增运行时 API（公开只读，无 token）

| 端点 | 方法 | 说明 |
|---|---|---|
| `/api/v1/pages` | GET | 内容列表 + 过滤（section/tag/q）+ 分页（page/limit）+ 排序 |
| `/api/v1/pages/{url}` | GET | 单页详情（404 若不存在） |
| `/api/v1/tags` | GET | tag 聚合（tag → 页面数） |
| `/api/v1/sections` | GET | section 聚合（section → 页面数） |

**路径冲突处理：** `/api/v1/*`（运行时 API）与 `/api/{section}.json`（预构建静态文件）通过版本前缀 `/v1/` 共存，mux 精确匹配优先于 `/` catch-all。

**JIT 渲染**无新增端点——复用 serving 层已有的 `jitFallback`，只是把 stub 换成真实实现。

## 5. 关键设计决策

按重要性排序：

1. **JIT 复用 PipelineCache（不重新造渲染管线）** —— JIT 是高频路径，必须复用增量构建同一套 `PipelineCache`（模板/i18n/markdown/writer）。与 `IncrementalRender` 共享底层（重建内容+context），职责分离：一个写文件、一个返回 HTML。

2. **URL→源文件双层查找（DAG + URL 推导）** —— DAG `SourceFromPagePath` 精确查找已构建页面；未命中时按 huan URL 规则推导（`/posts/hello/` → `content/posts/hello.md`）作为 fallback，覆盖新建 draft 等场景。

3. **JITCache 自动失效** —— 全量构建 `Clear()`，增量构建对受影响 URL `Remove()`。保证内容变更后不返回过期缓存。

4. **内容查询 API 包裹预构建 JSON（不重新造数据源）** —— 复用已有 `GenerateContentAPI` 生成的 `/api/{section}.json`，daemon 只做查询层（过滤/分页/聚合）。与增量构建天然协同（构建后 JSON 更新 → ContentIndex reload）。

5. **ContentIndex 不缓存 page/context（只缓存查询索引）** —— 数据源是静态 JSON 文件，构建后整体 reload，无引用失效问题。区别于 PipelineCache（缓存渲染基础设施）。

6. **draft 强制排除（不依赖 BuildDrafts 标志）** —— `GenerateContentAPI` 始终 `includeDrafts=false`，公开 API 永不泄露 draft，即使 daemon 用 `--buildDrafts` 启动。

7. **查询长度按 rune 计数（CJK 友好）** —— `utf8.RuneCountInString` 而非字节计数，避免误杀中文查询。

## 6. 验收记录

```
$ go build ./... && go vet ./...
（无输出，构建成功）

$ go test ./... -count=1
27 个包全部通过，0 失败
```

### JIT 渲染验收
- `TestDaemon_JIT_RenderUnbuiltPage`：渲染 draft 页面 + 验证 JITCache 填充 + 二次命中
- `TestDaemon_JIT_PageNotFound`：不存在 URL 返回 error → 404
- `TestJITCache_InvalidatedOnFullBuild`：全量构建后缓存清空

### 内容查询 API 验收
- `TestDaemon_ContentAPI_AfterBuild`：构建后 API 可查询
- `TestDaemon_ContentAPI_RefreshOnBuild`：内容变更→构建→API 刷新
- `TestDaemon_ContentAPI_ExcludesDraft`：draft 不出现
- `TestDaemon_ContentAPI_ExcludesDraftEvenWithBuildDrafts`：BuildDrafts=true 时 draft 仍不泄露

### Final review 发现并修复的关键缺陷

| 缺陷 | 严重度 | 修复 |
|---|---|---|
| JIT 路径归一化（watcher 绝对路径 vs DAG 相对路径） | Critical | builder.go `normalizeChangedFiles`（上个功能已修，JIT 受益） |
| Query 整数溢出 → panic（公开端点 DoS） | Critical | page 上限 100000 + 负值边界守卫 |
| daemon `--buildDrafts` 时 draft 泄露到公开 API | Important | GenerateContentAPI 强制 `includeDrafts=false` |
| 查询长度按字节计数误杀 CJK 查询 | Important | 改用 `utf8.RuneCountInString` |
| 查询长度无上限（资源耗尽） | Medium | rune 上限 200 → 400 |
| 无 Cache-Control 头 | Medium | `Cache-Control: no-store` |

## 7. 已知限制

- **JIT 全文正文**：`RenderPageWithCache` 不返回完整正文（API 也不返回 `Plain`），完整内容走预构建页面或 JIT HTML
- **Query 全文搜索**：MVP 用 `strings.Contains`（Title/Summary/Description），无倒排索引/中文分词
- **Query 排序**：仅 `date desc`，`Sort` 参数的其他值回退到 date desc
- **ContentIndex 性能**：`sortItemsByDateDesc` O(n²) 插入排序，`GetByURL` O(n) 线性扫描——当前规模可接受，大规模需优化（预排序 + map 索引）
- **JIT `renderPageFn` 字段**：历史遗留 dead field（Task 4 review 标注，未移除）
- **缓存策略**：内容查询 API 当前 `no-store`，未来可改 `max-age` + ETag
- **安全头**：仅 Content-Type + Cache-Control，无 `X-Content-Type-Options`/CORS（跨域消费需代理）

## 8. 后续优化项

- **全文搜索引擎**：接入 bleve 倒排索引（中文分词、相关性排序）
- **ContentIndex 性能**：LoadFromDir 预排序 + `map[string]int` URL 索引（O(1) 查找）
- **缓存**：`Cache-Control: max-age` + ETag（基于 index 长度 + 最新日期）
- **字段选择**：`?fields=plain` 返回完整正文
- **JIT 缓存预热**：daemon 启动后预渲染高频页面
- **JIT 指标**：命中率/渲染耗时接入 Prometheus
- **认证分层**：protected/private 内容需 token（会员/付费场景）
- **实时推送**：WebSocket/SSE 推送内容变更给前端

## 9. 提交历史

本次会话共 13 个提交：

- JIT 渲染：`cc25723..b5d7e2d`（6 commits）
- 内容查询 API：`7aa7417..76bbf97`（6 commits）+ 修复 `3b40202`（1 commit）
