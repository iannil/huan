# huan 项目状态（CURRENT STATE）

> **当前版本**：v0.6.0+ · **分支**：master · **最后更新**：2026-07-25
> **定位**：huan daemon — 持久化运行的服务端内容引擎
> **入口**：新会话先读 `CLAUDE.md` + `docs/INDEX.md`
>
> 本文档替代 `docs/technical-plan.md`（已归档到 `docs/archived/technical-plan.md`），反映 daemon 时代的最新项目状态。

---

## 一、一句话定位

**huan** 是用 Go 编写的 **local-first single-user content engine with built-in admin**，当前核心能力是 **daemon 持久化服务端模式**。

- `huan daemon` — 持久化进程，提供：静态文件服务 + JIT 按需渲染 + 内容查询 REST API + SSE 实时推送 + Admin 管理后台 + 插件热插拔
- `huan build` / `huan serve` / `huan deploy` / `huan release` / `huan translate` — 传统 SSG 模式仍完整保留
- **版本节奏**：v0.5.0（2026-06-30 交付 v1.0 hard gate 1-5）→ v0.6.0+（2026-07-20 起 daemon 时代）→ v1.0.0（gate 6 90 天稳定性，2026-09-11 后）

---

## 二、版本演进

| 版本 | 时间 | 关键变更 |
|------|------|---------|
| **v0.6.0+** | 2026-07-20~ | **Daemon 时代**：持久化服务端内容引擎。四大运行时能力全部就绪（静态服务 + JIT 按需渲染 + 内容查询 REST API + SSE 实时推送） |
| **v0.5.0** | 2026-06-30 | **v1.0 hard gate 1-5 全交付**：定位拆段 / no-op funcs 三档 / I/O 包测试 / Admin 安全边界 / BuildSite 6 文件重构 |
| **v0.4.x** | 2026-06-14~27 | i18n 翻译系统 + Stage 4 Admin Panel v0.4.0~v0.4.2 增量演进 |
| **v0.3.0** | 2026-06-14~26 | i18n 多语言系统（Translator 插件 + MultiSite + 双语上线）+ Admin Panel（Go API + React SPA CRUD） |
| **v0.2.x** | 2026-06-13 | v0.2.0 移除 encrypt/redact → v0.2.1 toc/export/sync → v0.2.2 CI Release → v0.2.3 encryptGroups cleanup |
| **v0.1.0** | 2026-06-13 | `huan release` 首发版本 |

---

## 三、Daemon 时代已完成功能

### 3.1 Daemon 核心骨架（2026-07-20）

- `huan daemon` 命令（替代原 `huan serve` → 重命名为 `huan dev`）
- HTTP 服务 + 静态文件服务 + 全量/增量构建 + 文件监听（fsnotify）
- `internal/daemon/` 完整包结构：`builder.go` / `serving.go` / `watcher.go` / `lifecycle.go` / `metrics.go` / `health.go`
- EventBus（`internal/daemon/eventbus/`）：ChannelBus 实现，事件类型覆盖 build/content/plugin
- DAG 依赖图（`internal/daemon/dag/`）：`OrderByDependency` 拓扑排序
- JITCache（`internal/daemon/cache/jit.go`）：LRU + TTL 过期
- Prometheus 指标（`/metrics`）+ HealthCheck（`/health`）+ TLS + systemd notify + 优雅关闭
- `huan dev` 命令（原 serve 的重命名，保持兼容）

### 3.2 热插拔插件系统（2026-07-21）

- **`internal/plugin/`** 新增：`Loader`（.so 文件加载）、`LifecycleManager`（生命周期管理）、`GRPCStub`（gRPC 预留骨架）
- **独立 .so 插件仓库**：`plugins/cloudflare/`、`plugins/qwen3/`、`plugins/image-pipeline/`。自包含类型复制模式
- **Admin API**：`/admin/api/plugins/*`（list/load/unload/reload）
- **CLI**：`huan plugin load/unload/reload/list`
- **EventBus 插件事件**：Loaded/Unloaded/Reloaded/Error

### 3.3 图片管线插件（2026-07-21）

- `plugins/image-pipeline/`：扫描输出目录 HTML → 解析 `<img>` → 压缩/缩放/格式转换 → 注入 srcset/picture
- `internal/image/processor.go`：`ImageProcessor` capability 接口
- 构建管线集成（`cmd/huan/build_image.go`）：`AfterBuild` 调用

### 3.4 增量构建（2026-07-21）

- `internal/build/cache.go`：`PipelineCache`（缓存渲染基础设施）+ `HasTemplateChanges`（模板变更检测 → 全量回退）
- `internal/build/build.go`：`IncrementalRender`（DAG 驱动部分重渲染）
- `internal/daemon/builder.go`：`IncrementalBuild` 完整实现
- DAG 依赖方向：聚合页 DependsOn 文章（编辑文章时沿 DependedBy BFS 到达所有聚合页）

### 3.5 JIT 按需渲染（2026-07-22）

- `internal/build/jit.go`：`RenderPageWithCache`（复用 PipelineCache 渲染单页返回 HTML）
- URL→源文件双层查找：DAG `SourceFromPagePath` + `ResolveSourceFromURL`（URL 推导 fallback）
- JITCache 失效：全量 Clear / 增量 Remove

### 3.6 内容查询 REST API（2026-07-22）

- `internal/daemon/contentindex/`：`ContentIndex`（从预构建 JSON 加载内存索引）+ HTTP Handler
- 端点：`GET /api/v1/pages`、`/pages/{url}`、`/tags`、`/sections`（公开只读，无 token）
- 构建后自动刷新索引

### 3.7 SSE 实时推送（2026-07-22）

- `internal/daemon/sse/`：`SSEHub`（非阻塞广播 + 15s 心跳 + maxClients=1000）
- 端点：`GET /api/v1/events`（text/event-stream，公开只读）
- 事件类型：build_completed / build_failed / content_changed / plugin_loaded / plugin_unloaded

### 3.8 插件管理后台 + 配置验证（2026-07-23）

- Admin 插件管理页面：列表 + 加载/卸载/热重载
- 配置验证：`ValidateConfig` + schema 类型 + 集成到 `newPluginRegistry`
- 修复：路径验证、共享 registry、热重载安全

### 3.9 SEO 编译期插件（2026-07-24）

四个 SEO 增强插件作为 compiled-in 插件集成：

1. **SEO 注入器**：`internal/seo/injector/` — 构建时注入 meta/JSON-LD/OpenGraph，配置驱动
2. **Sitemap 增强器**：`internal/seo/sitemap/` — 构建时增强 sitemap 条目（优先级/变更频率/图片/视频）
3. **HTML 注入器**：`internal/seo/htmlinjector/` — 构建时注入自定义 head/body HTML
4. **MetadataProvider**：`internal/plugin/` — 插件元数据接口（版本/作者/标签/描述），Admin 插件列表增强

### 3.10 插件事件订阅（2026-07-25）

- `EventSubscriber` 接口：插件可订阅 daemon EventBus 事件
- 生命周期集成：加载时自动注册/卸载时自动注销

---

## 四、Daemon 运行时能力栈

```
┌─────────────────────────────────────────────┐
│              面向终端用户                      │
│  /api/v1/events              SSE 实时推送     │
│  /api/v1/pages /tags /sections  内容查询 API  │
│  /                             静态服务+JIT   │
├─────────────────────────────────────────────┤
│              运维/管理                        │
│  /admin/api/*  /health  /metrics            │
│  /admin/plugins/*        插件管理            │
└─────────────────────────────────────────────┘
```

---

## 五、CLI 子命令一览

| 子命令 | 用途 | 状态 |
|--------|------|------|
| `build` | 构建静态站点 | ✅ 稳定 |
| `daemon` | 持久化服务端模式 | ✅ v0.6+ 核心 |
| `dev` | 开发服务器（原 serve） | ✅ 稳定 |
| `serve` | 已废弃（由 dev 替代） | ⚠️ 保留兼容 |
| `deploy` | 部署到 Cloudflare | ✅ 稳定 |
| `plugin` | 插件管理（load/unload/reload/list） | ✅ v0.6+ 新增 |
| `release` | 本地打包发布 | ✅ 稳定 |
| `translate` | 翻译内容（qwen3 插件） | ✅ 稳定 |
| `version` | 输出版本 + git SHA | ✅ 稳定 |
| `env` | 打印环境信息 | ✅ 稳定 |
| `config` | 打印解析后配置 | ✅ 稳定 |
| `list` | 列出内容 | ✅ 稳定 |
| `new` | 新建内容 | ✅ 稳定 |
| `sync` | 同步资源（gallery scaffold） | ✅ 稳定 |
| `toc` | 生成 TOC | ✅ 稳定 |
| `export` | 导出 CSV | ✅ 稳定 |

---

## 六、项目结构

```
huan/
├── cmd/
│   ├── huan/                # CLI 入口（16 子命令）
│   └── equiv-check/         # 三维度等价检查工具
├── internal/
│   ├── build/               # 构建管线（BuildSite / PipelineCache / IncrementalRender / JIT）
│   ├── config/              # huan.yaml 解析
│   ├── content/             # 内容加载 + frontmatter + 内容树
│   ├── markdown/            # goldmark + chroma 语法高亮
│   ├── shortcode/           # shortcode（audio/img）
│   ├── template/            # 模板加载 + 函数注册
│   ├── taxonomy/            # 标签/分类
│   ├── pagination/          # 分页器
│   ├── output/              # 写入 / minify / canonify / contentapi / llmstxt
│   ├── i18n/                # i18n bundle + collator + audit + langdetect
│   ├── admin/               # Admin API（Go API + 嵌入式 React SPA）
│   ├── daemon/              # Daemon 核心（builder / serving / watcher / lifecycle / metrics）
│   │   ├── eventbus/        # 事件总线（build / content / plugin 事件）
│   │   ├── dag/             # DAG 依赖图 + 拓扑排序
│   │   ├── cache/           # JITCache（LRU + TTL）
│   │   ├── contentindex/    # 内容查询索引
│   │   └── sse/             # SSE 实时推送
│   ├── deploy/              # Deployer 接口 + Report
│   ├── translate/           # Translator 接口 + types
│   ├── plugin/              # 统一插件宿主（Loader / LifecycleManager / GRPCStub / MetadataProvider / EventSubscriber）
│   ├── image/               # ImageProcessor 接口
│   ├── seo/                 # SEO 编译期插件（injector / sitemap / htmlinjector）
│   ├── observability/       # JSON Logger
│   ├── release/             # 跨平台打包
│   ├── version/             # VCS info
│   └── equiv/               # 三维度等价算法
├── plugins/                 # 独立 .so 插件仓库
│   ├── cloudflare/          # Cloudflare deploy 插件
│   ├── qwen3/               # Qwen3 翻译插件
│   └── image-pipeline/      # 图片管线插件
├── web/admin/               # Admin React SPA
├── scripts/                 # diff-build / diff-summary / diff-patterns
├── docs/                    # 文档
├── memory/                  # 双层记忆（MEMORY.md + daily/）
├── release/                 # 发布产物
├── .github/workflows/       # CI（release.yml）
└── go.mod / go.sum / huan.yaml / README.md / README.zh-CN.md / CLAUDE.md
```

---

## 七、Hugo 等价状态（历史参考）

三维度等价已于 **2026-06-13 stage 3 grill-me** 确认达成：

- **SEO 维度**：0 differing ✅
- **AI 维度**：0 differing ✅
- **肉眼维度**：normalized 6 differing（chroma 版本差 4 + 非可见 artifact 2）
- **字节一致率**：99.7%（2026/2032 files）

Hugo 等价是 v0.x 早期阶段的目标，**当前 v0.6+ daemon 时代已不再以 Hugo 等价为核心约束**。`scripts/diff-*.sh` 工具仍保留，但不 gate daemon 开发。

> 详细历史记录已归档到 `docs/archived/`（progress / reports / daily）。

---

## 八、v1.0 Release Tracking

| # | 标准 | 状态 | 交付版本 |
|---|------|------|---------|
| 1 | 文档定位一致 | ✅ 完成 | v0.5.0 |
| 2 | 无静默 no-op 模板函数 | ✅ 完成 | v0.5.0 |
| 3 | I/O 包有测试 | ✅ 完成 | v0.5.0 |
| 4 | Admin 安全边界（L1+L2+L4） | ✅ 完成 | v0.5.0 |
| 5 | BuildSite 拆 ≤80 行 stage | ✅ 完成 | v0.5.0 |
| 6 | zhurongshuo 生产稳定 90 天 + 自己满意 | ⏳ 2026-06-13 → **2026-09-11**（已 42 天） | v1.0.0 |

v0.5.0 交付 gate 1-5（2026-06-30）→ 等 73 天 → **v1.0.0（2026-09-11 后）**。Daemon 时代的功能（v0.6.x 系列）在等待期内正常迭代。

---

## 九、已知限制 & 后续方向

### 限制
- JIT 不返回完整正文（仅 HTML）
- 内容查询 API 全文搜索仅 `strings.Contains`（无中文分词/倒排索引）
- 图片管线仅生成 JPEG（WebP/AVIF 跳过）
- 增量构建的 renderEmptyCategories/render404 未重新生成
- Watcher 跳过 layouts/static/themes 目录（daemon 下需手动触发 rebuild）
- SSE 全量广播（无服务端类型过滤）

### 后续方向
- 全文搜索引擎（bleve 倒排索引）
- ContentIndex 性能优化（预排序 + map 索引）
- 图片管线 WebP/AVIF 编码
- gRPC 插件路径落地（跨语言）
- 插件市场：`huan plugin install <name>`
- JIT 缓存预热 + 指标
- SSE 类型过滤 + Last-Event-ID 恢复
- 热模板重载
- 并行渲染优化