# MEMORY — huan 项目长期记忆

> 维护规则：当检测到有意义信息（用户偏好 / 关键决策 / 项目上下文变化）时智能合并；过期信息主动更新或删除。
> 最近更新：2026-07-28（**插件契约主动收敛**：ImageProcessor 迁 pkg/image、diagnoseCapabilityGap 诊断 + TestCapabilityContractsLiveInPkg 护栏，见 ADR 0013）

## 用户偏好

- 交流与文档使用**中文**，生成的代码使用英文
- 文档放在 `docs/` 下，使用 Markdown
- 数据库 / 消息队列 / 缓存等基础设施尽量用 Docker 部署，配独立网络避免冲突
- 全链路可观测性：JSON 结构化日志（`timestamp` / `trace_id` / `span_id` / `event_type` / `payload`），装饰器 / 切面与业务逻辑解耦
- 发布产物固定在 `/release`，必须包含全量 + 增量发布所需全部文件
- 面向 LLM 的可改写性：一致分层、单一职责、显式类型、声明式配置、统一命名（`parseXxx` / `assertNever` / `safeJsonParse` 等）、小步提交
- 批量程序性改动前先备份到 `/backup`，错误数异常上升立即回滚

## 文档规范执行

- 文档体系已按 `docs/standards/documentation.md` 规范整理，CURRENT_STATE.md 在 `docs/progress/`（未完成但持续更新）
- 已完成报告在 `docs/reports/completed/`（按时间倒序）
- 设计文档在 `docs/superpowers/specs/`
- 已归档内容统一在 `docs/archived/`（仅保留 `technical-plan.md` 一份历史参考）
- 文档冗余清理完成：`docs/archived/daily/` / `docs/archived/reports/` / `docs/archived/progress/` 均已删除（数据在 `memory/daily/` 和 `reports/completed/` 中有完整副本）
- `archived/hugo/`（44MB Hugo 源码）使用 `git filter-repo` 从仓库历史中永久删除

## 项目上下文

### 当前定位（2026-07-20 起）

**huan daemon** — 持久化运行的服务端内容引擎。核心能力是 daemon 模式下的持久化服务，所有功能围绕 daemon 展开。

### 旧定位已归档

- 之前的 SSG / 替代 Hugo / 三维度等价 / v1.0 hard gate / 9 步执行计划 / stage 1-4 等全部开发计划已归档到 `docs/archived/`
- `docs/technical-plan.md`（Hugo 等价时代的设计参考）已归档到 `docs/archived/technical-plan.md`

### 当前版本：v0.6.0+

最新版本标签：v0.5.0（2026-06-30）。v0.5.0 之后 ~80 个 commit 均为 daemon 时代功能（v0.6.x 系列，未发 tag）。

### 已存在的代码资产（仍有效）

- `huan build` / `huan serve` / `huan deploy` / `huan release` / `huan translate` / `huan daemon` 等 CLI 子命令（16 个）
- `internal/build/`、`internal/template/`、`internal/markdown/`、`internal/output/`、`internal/admin/`、`internal/i18n/`、`internal/translate/`、`internal/plugin/`、`internal/deploy/`、`internal/observability/`、`internal/image/`、`internal/daemon/`、`internal/seo/` 等内部包
- Admin Panel（Go API + React SPA）：ContentList / ContentEdit / ContentNew / Settings / Dashboard + 插件管理
- i18n 多语言系统（zh-cn + en /en/）
- huan daemon — 核心定位

### 2026-07-21 新增：插件化架构 + 增量构建

- **热插拔插件系统**：`internal/plugin/` 新增 `Loader`（.so 加载）、`LifecycleManager`（生命周期）、`GRPCStub`（gRPC 预留骨架）；Admin API `/admin/api/plugins/*`（list/load/unload/reload）；CLI `plugin load/unload/reload`
- **独立 .so 插件仓库**：`plugins/cloudflare/`（deploy 能力）、`plugins/qwen3/`（translate 能力）、`plugins/image-pipeline/`（image 能力）。自包含类型复制模式
- **图片管线插件**：构建时自动压缩/多尺寸/HTML srcset 注入；`internal/image/processor.go` 的 `ImageProcessor` capability 接口
- **增量构建**：`internal/build/cache.go` 的 `PipelineCache`（缓存渲染基础设施）；`IncrementalRender`（DAG 驱动部分重渲染）；`HasTemplateChanges`（模板变更检测 → 全量回退）；`dag.OrderByDependency`（拓扑排序）。daemon 文件变更走增量路径

### 2026-07-22 新增：JIT 渲染 + 内容查询 REST API + SSE 实时推送

- **JIT 渲染**：`build.RenderPageWithCache`（复用 PipelineCache 渲染单页返回 HTML，不写文件）；`DAG.SourceFromPagePath` + `ResolveSourceFromURL`（URL→源文件双层查找）；`Builder.RenderPageJIT` 真实实现
- **内容查询 REST API**：`/api/v1/pages`、`/pages/{url}`、`/tags`、`/sections`（公开只读，无 token）；`internal/daemon/contentindex/` 的 `ContentIndex`（从预构建 `/api/{section}.json` 加载内存索引）+ `Handler`
- **SSE 实时推送**：`GET /api/v1/events`（text/event-stream，公开只读）；`internal/daemon/sse/` 的 `SSEHub`（非阻塞广播 + 15s 心跳 + maxClients 上限）+ `SubscribeBus`（桥接 EventBus 5 类事件：build/content/plugin）
- daemon 四大核心运行时能力全部就绪：静态服务 + JIT 按需渲染 + 内容查询 + 实时推送

### 2026-07-23~25 新增：插件管理后台 + SEO 编译期插件 + 事件订阅

- **插件管理后台 + 配置验证**：`ValidateConfig` + schema 类型 + Admin 插件管理页面（list/load/unload/reload）
- **SEO 编译期插件**（4 个）：SEO 注入器（meta/JSON-LD/OpenGraph）、Sitemap 增强器（优先级/变更频率/图片/视频）、HTML 注入器（自定义 head/body HTML）、MetadataProvider（插件元数据 → Admin 插件列表增强）
- **插件事件订阅**：`EventSubscriber` 接口，插件可订阅 daemon EventBus 事件，生命周期自动注册/注销

## 关键决策

- 模板引擎用 `html/template`，阶段二可插件替换
- Markdown 用 goldmark（与 Hugo 同源库）
- 配置格式 `huan.yaml`（YAML）
- 插件架构：统一插件系统（`internal/plugin/` + capability 接口），编译期 hardcoded + 运行时 .so 热插拔两条路径并存
- 全链路可观测性：`internal/observability/` 共享包
- **插件自包含类型复制**（2026-07-21）：独立插件仓库复制所需的 Plugin 接口/capability 类型，而非 go.mod replace —— Go internal 包可见性规则禁止外部 module replace 导入
- **PipelineCache 只缓存渲染基础设施**（2026-07-21）：缓存模板/i18n/markdown/writer，不缓存 page/context
- **DAG 依赖方向：聚合页 DependsOn 文章**（2026-07-21）：section/home/tag 显示文章内容所以依赖文章
- **增量构建自动降级**（2026-07-21）：模板/i18n/config/themes 变更时回退全量构建
- **JIT 复用 PipelineCache**（2026-07-22）：JIT 与增量构建共享 PipelineCache
- **公开 API 契约独立于开发标志**（2026-07-22）：`GenerateContentAPI` 强制 `includeDrafts=false`
- **预构建数据源 + 运行时查询层**（2026-07-22）：内容查询 API 复用 `/api/{section}.json` 数据源
- **SSE 优先于 WebSocket**（2026-07-22）：单向推送匹配需求，HTTP 原生，浏览器 EventSource 原生支持
- **插件查找顺序 `$HUAN_HOME`→项目**（2026-07-28）：`internal/plugin.HuanHome()`（默认 `~/.huan`）优先于 `<项目>/plugins`；同名插件高优先级目录胜出。`Loader.Resolve`/`Scan*` 统一走 `searchDirs`
- **命令按能力发现插件，不硬编码插件名**（2026-07-28）：deploy/translate/image 用 `plugin.Find[Deployer/Translator/ImageProcessor]` + `loadConfiguredPlugins`（加载 yaml 声明插件）挑选；插件名只存在于 huan.yaml
- **插件 .so 命名/发布约定**（2026-07-28）：文件名 = 配置名下划线→连字符（`internal/plugin.SoFileName`，如 `qwen3_translate`→`qwen3-translate.so`）；编译产物发布到 `release/plugins/`（gitignore，`scripts/build-plugins.sh` 重建）
- **`.Site.Data` 不按语言作用域**（2026-07-28）：huan 递归加载 `data/`，`data/en/*.yaml` 挂在 `.Site.Data.en.*`；模板需自行按 `$.Site.LanguageCode` 覆盖（见 `practices/list.html`）
- **catalogSections 会丢弃全部内容页**（2026-07-28）：仅渲染 `_index.<lang>` 索引，用于未翻译章节占位；若章节已翻译并要显示目录，须从 catalogSections 移除该 section
- **能力契约一律置于 `pkg/`**（2026-07-28，ADR 0013）：所有 embed `plugin.Plugin` 且跨 `.so` 边界的能力契约必须在 `pkg/`（Deployer/Translator/ImageProcessor/ThemePlugin），`internal/` 仅用类型别名回填以保调用点零改动。护栏二道：`TestCapabilityContractsLiveInPkg`（go/ast 扫描，`internal/` 新增同类契约即 CI 红）+ `diagnoseCapabilityGap`（`Find[T]` 落空且 registry 非空时枚举已加载插件、指向 contract mismatch，替代裸 `no X available`）。插件侧加编译期断言（如 image-pipeline `var _ pkgimage.ImageProcessor`）实现 host+plugin 同源锁步。白名单例外 3 项：`EventSubscriber`（引用 internal eventbus、无 .so 用者、不 embed Plugin，YAGNI 推迟、纯留痕）、`GRPCPlugin`（gRPC 跨进程，不受 .so 类型同一性约束）、`Hook`（`internal/build.Hook` 用 `[]*content.Page` 无法迁 pkg/，.so 安全版 `pkg/plugin.Hook` 用 `[]any` 已存在）。**后续待办（同类 bug，未修）**：`internal/build/pipeline.go:75` 断言 `internal/build.Hook`（`[]*content.Page`），而已发布 .so 插件实现 `pkg/plugin.Hook`（`[]any`）——结构不同接口、无桥接 adapter，这些插件 hook 很可能从未被构建管线调用；候选独立后续计划（加桥接 adapter 或统一两个 Hook 接口）。→ **已由 ADR 0014 解决**（见下条）。
- **`.so` 构建钩子用 `PostBuildHook`**（2026-07-28，ADR 0014）：`pkg/plugin.Hook`（`interface{}` 三方法）拆为 `PostBuildHook`（embed `Plugin` + 仅 `OnOutputWritten(ctx, outputDir string) error`，唯一可跨 `.so`）；`internal/build.Hook`（`*content.Page` 三方法）保留为编译期富页面钩子，`internal/plugin` 加别名回填。pipeline `runOnOutputWritten` 在 `h.(build.Hook)` 之外 `else if h.(PostBuildHook)` 桥接调用（build.Hook 优先、互斥不重复、错误只 warn）；`runOnContentLoaded`/`runOnPageRendered` 仍 build.Hook-only。三个 .so 插件（seo/sitemap/html）删 no-op 页面钩子、仅实现 `OnOutputWritten` 据此生效——此前因契约分叉（pipeline 断言 build.Hook、插件实现 pkg Hook、无桥接）钩子从未被调用，SEO meta/sitemap 增强/自定义 HTML 注入在真实站点上静默失效。为何拆分而非双向 adapter：页面级钩子跨 .so 本质无用（.so 只拿到不透明装箱值）。回归测试 `internal/build/hook_test.go`（`TestRunOnOutputWritten_InvokesPostBuildHook` 红→绿直击 bug、`_InvokesBuildHook` 防回归）。提交 Task1 `9f08581`/Task2 `c9f7c60`/Task3 `f715b14`。

## 经验教训

### 2026-07-21 新增
- **设计阶段难预见引用失效**：spec 设计缓存 PageLookup/ContextLookup，实现时发现 context 引用的 page 指针在内容变更时失效
- **DAG 依赖方向易搞反**：必须明确"谁依赖谁"。聚合页（section/tag/home）显示文章 → 聚合页 DependsOn 文章
- **Watcher 与 DAG 路径格式不匹配是隐蔽 bug**：fsnotify 给绝对路径，DAG 存相对路径 → 需要路径归一化层

### 2026-07-22 新增
- **公开 API 安全边界独立于开发标志**：`GenerateContentAPI` 原用 `p.opts.IncludeDrafts`，导致 daemon `--buildDrafts` 时 draft 泄露到公开 API
- **中文站点的长度限制按 rune 计数**：`utf8.RuneCountInString` 而非字节计数
- **公开端点的整数溢出是 DoS**：`page=MaxInt64` 导致 `(page-1)*limit` 溢出为负值，slice panic

### 2026-07-25 新增
- **文档需定期整理**：不维护的文档会累积大量冗余（daily/ 双份、reports/ 双份、progress/ 空目录）。维护规范 `docs/standards/documentation.md` 已执行，但需定期跟进
- **git filter-repo 有效清理大文件历史**：`archived/hugo/` 44MB 使用 filter-repo 从 Git 历史彻底删除，仓库从 ~50MB 降到 ~8MB

## 文档与导航

- 入口索引：[`docs/INDEX.md`](../docs/INDEX.md)
- 当前项目状态：[`docs/progress/CURRENT_STATE.md`](../docs/progress/CURRENT_STATE.md)
- 已归档的旧计划：[`docs/archived/`](../docs/archived/)