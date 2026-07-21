# MEMORY — huan 项目长期记忆

> 维护规则：当检测到有意义信息（用户偏好 / 关键决策 / 项目上下文变化）时智能合并；过期信息主动更新或删除。
> 最近更新：2026-07-20（**定位变更：huan daemon** — 旧版 SSG / 替代 Hugo / v1.0 hard gate 等全部开发计划已归档到 `docs/archived/`）

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

- `huan build` / `huan serve` / `huan deploy` / `huan release` / `huan translate` 等 14 子命令
- `internal/build/`、`internal/template/`、`internal/markdown/`、`internal/output/`、`internal/admin/`、`internal/i18n/`、`internal/translate/`、`internal/plugin/`、`internal/deploy/`、`internal/observability/` 等 22 内部包
- Admin Panel（Go API + React SPA）：ContentList / ContentEdit / ContentNew / Settings / Dashboard
- i18n 多语言系统（zh-cn + en /en/）
- Cloudflare deploy 插件（Pages / R2 / Worker）
- huan daemon（`huan daemon` 命令 + 持久化运行能力）—— 这是新定位的核心

## 关键决策

- 模板引擎用 `html/template`，阶段二可插件替换
- Markdown 用 goldmark（与 Hugo 同源库）
- 配置格式 `huan.yaml`（YAML）
- 插件架构：统一插件系统（`internal/plugin/` + `internal/deploy/`），编译期 hardcoded 注册
- 全链路可观测性：`internal/observability/` 共享包

## 经验教训

### 已归档

旧定位下的经验教训（Hugo 等价、stage 1-3 diff 修复等）随 `docs/archived/` 一同归档，不再维护。

## 文档与导航

- 入口索引：[`docs/INDEX.md`](../docs/INDEX.md)
- 已归档的旧计划：[`docs/archived/`](../docs/archived/)