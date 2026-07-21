# MEMORY — huan 项目长期记忆

> 维护规则：当检测到有意义信息（用户偏好 / 关键决策 / 项目上下文变化）时智能合并；过期信息主动更新或删除。
> 最近更新：2026-07-21（**插件化架构 + 增量构建交付**：热插拔插件系统、cloudflare/qwen3 解耦为独立 .so、图片管线插件、DAG 驱动的增量构建）

## 用户偏好

- 交流与文档使用**中文**，生成的代码使用英文
- 文档放在 `docs/` 下，使用 Markdown
- 数据库 / 消息队列 / 缓存等基础设施尽量用 Docker 部署，配独立网络避免冲突
- 全链路可观测性：JSON 结构化日志（`timestamp` / `trace_id` / `span_id` / `event_type` / `payload`），装饰器 / 切面与业务逻辑解耦
- 发布产物固定在 `/release`，必须包含全量 + 增量发布所需全部文件
- 面向 LLM 的可改写性：一致分层、单一职责、显式类型、声明式配置、统一命名（`parseXxx` / `assertNever` / `safeJsonParse` 等）、小步提交
- 批量程序性改动前先备份到 `/backup`，错误数异常上升立即回滚

## 项目上下文

### 当前定位（2026-07-20 起）

**huan daemon** — 持久化运行的服务端内容引擎。核心能力是 daemon 模式下的持久化服务，所有功能围绕 daemon 展开。

### 旧定位已归档

- 之前的 SSG / 替代 Hugo / 三维度等价 / v1.0 hard gate / 9 步执行计划 / stage 1-4 等全部开发计划已归档到 `docs/archived/`
- 详见 `docs/archived/` 下的 `progress/`、`reports/`、`daily/`

### 已存在的代码资产（仍有效）

- `huan build` / `huan serve` / `huan deploy` / `huan release` / `huan translate` / `huan daemon` 等 CLI 子命令
- `internal/build/`、`internal/template/`、`internal/markdown/`、`internal/output/`、`internal/admin/`、`internal/i18n/`、`internal/translate/`、`internal/plugin/`、`internal/deploy/`、`internal/observability/`、`internal/image/` 等内部包
- Admin Panel（Go API + React SPA）：ContentList / ContentEdit / ContentNew / Settings / Dashboard + 插件管理
- i18n 多语言系统（zh-cn + en /en/）
- huan daemon（`huan daemon` 命令 + 持久化运行能力）—— 新定位的核心

### 2026-07-21 新增：插件化架构 + 增量构建

- **热插拔插件系统**：`internal/plugin/` 新增 `Loader`（.so 加载）、`LifecycleManager`（生命周期）、`GRPCStub`（gRPC 预留骨架）；Admin API `/admin/api/plugins/*`（list/load/unload/reload）；CLI `plugin load/unload/reload`
- **独立 .so 插件仓库**：`plugins/cloudflare/`（deploy 能力）、`plugins/qwen3/`（translate 能力）、`plugins/image-pipeline/`（image 能力）。自包含类型复制模式（非 go.mod replace）
- **图片管线插件**：构建时自动压缩/多尺寸/HTML srcset 注入；`internal/image/processor.go` 的 `ImageProcessor` capability 接口
- **增量构建**：`internal/build/cache.go` 的 `PipelineCache`（缓存渲染基础设施）；`IncrementalRender`（DAG 驱动部分重渲染）；`HasTemplateChanges`（模板变更检测 → 全量回退）；`dag.OrderByDependency`（拓扑排序）。daemon 文件变更走增量路径

## 关键决策

- 模板引擎用 `html/template`，阶段二可插件替换
- Markdown 用 goldmark（与 Hugo 同源库）
- 配置格式 `huan.yaml`（YAML）
- 插件架构：统一插件系统（`internal/plugin/` + capability 接口），编译期 hardcoded + 运行时 .so 热插拔两条路径并存
- 全链路可观测性：`internal/observability/` 共享包
- **插件自包含类型复制**（2026-07-21）：独立插件仓库复制所需的 Plugin 接口/capability 类型，而非 go.mod replace —— Go internal 包可见性规则禁止外部 module replace 导入
- **PipelineCache 只缓存渲染基础设施**（2026-07-21）：缓存模板/i18n/markdown/writer，不缓存 page/context（引用失效会导致列表页过期）；增量构建重建内容+context，只渲染受 DAG 影响的页面
- **DAG 依赖方向：聚合页 DependsOn 文章**（2026-07-21）：section/home/tag 显示文章内容所以依赖文章；编辑文章时沿 DependedBy BFS 正确到达所有聚合页
- **增量构建自动降级**（2026-07-21）：模板/i18n/config/themes 变更时回退全量构建（HasTemplateChanges 检测），保证正确性优先

## 经验教训

### 2026-07-21 新增

- **设计阶段难预见引用失效**：spec 设计缓存 PageLookup/ContextLookup，实现时发现 context 引用的 page 指针在内容变更时失效。务实调整为只缓存渲染基础设施。
- **DAG 依赖方向易搞反**：必须明确"谁依赖谁"。聚合页（section/tag/home）显示文章 → 聚合页 DependsOn 文章。final review 通过端到端测试（编辑文章 → 检查列表页输出）才发现初版方向反了。
- **Watcher 与 DAG 路径格式不匹配是隐蔽 bug**：fsnotify 给绝对路径，DAG 存相对路径，AffectedBy 查表返回空 → 增量构建静默 no-op。需要路径归一化层（builder.go 的 normalizeChangedFiles）。
- **Subagent-driven development 有效**：每个 task 独立 subagent + review，Critical 缺陷在 final review 被独立 subagent 实际复现，不依赖实现者自检。

### 已归档

旧定位下的经验教训（Hugo 等价、stage 1-3 diff 修复等）随 `docs/archived/` 一同归档，不再维护。

## 文档与导航

- 入口索引：[`docs/INDEX.md`](../docs/INDEX.md)
- 已归档的旧计划：[`docs/archived/`](../docs/archived/)