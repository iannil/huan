# MEMORY — huan 项目长期记忆

> 维护规则：当检测到有意义信息（用户偏好 / 关键决策 / 经验教训 / 项目上下文变化）时智能合并；过期信息主动更新或删除。
> 最近更新：2026-06-12

## 用户偏好

- 交流与文档使用**中文**，生成的代码使用英文
- 文档放在 `docs/` 下，使用 Markdown
- 数据库 / 消息队列 / 缓存等基础设施尽量用 Docker 部署，配独立网络避免冲突
- 全链路可观测性：JSON 结构化日志（`timestamp` / `trace_id` / `span_id` / `event_type` / `payload`），装饰器 / 切面与业务逻辑解耦
- 发布产物固定在 `/release`，必须包含全量 + 增量发布所需全部文件
- 面向 LLM 的可改写性：一致分层、单一职责、显式类型、声明式配置、统一命名（`parseXxx` / `assertNever` / `safeJsonParse` 等）、小步提交
- 批量程序性改动前先备份到 `/backup`，错误数异常上升立即回滚

## 项目上下文

- **huan** = Go 静态站点生成器，阶段一目标：替代 Hugo 构建 zhurongshuo.com，输出 100% 一致
- 关联内容项目：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`），当前仍由 Hugo 构建
- 当前分支：`master`；阶段一里程碑 1–9 已全部落地（详见 [`docs/progress/CURRENT_STATE.md`](../docs/progress/CURRENT_STATE.md)）
- `huan build` 与 Hugo 一致率：905/2028 共有文件完全相同（44.5%），剩余 5 类边缘差异
- `huan serve`（HTTP + fsnotify + LiveReload）已于 2026-06-12 完成，17 commits 落地
- 仓库整洁度：无 TODO/FIXME 标记，无 backup/tmp/CI/Makefile；`pkg/` 与 `internal/{pipeline,plugin,search}/` 已删除（stage 2 时再建）

## 关键决策

- 模板引擎阶段一用 `html/template`，阶段二可插件替换
- Markdown 用 goldmark（与 Hugo 同源库）
- 配置格式 `huan.yaml`（YAML），非 drop-in 替换 Hugo
- 验证方式：`./scripts/diff-build.sh` 逐字节对比 Hugo 输出，**零回归才允许合并**
- 插件架构阶段一**只预留文档**（接口未落地，连占位空目录都已删除），stage 2 增量扩展
- 加密密文仍由外部 Node.js `scripts/encrypt-content.js` 生成，huan 只负责读取与嵌入
- serve 模式用临时目录 `os.MkdirTemp("", "huan-serve-*")`，绝不污染 `docs/` 生产输出
- rebuild 用原子 swap（sibling staging dir + rename），保证 rebuild 期间无 404
- `BuildSite` 非并发安全，rebuild 通过 `atomic.Bool` 串行化 + pending 合并

## 经验教训

（待积累 —— 出现踩坑、回归、用户纠正时记到这里）

## 文档与导航

- 入口索引：[`docs/INDEX.md`](../docs/INDEX.md)
- 总图：[`docs/technical-plan.md`](../docs/technical-plan.md)
- 当前进展：[`docs/progress/CURRENT_STATE.md`](../docs/progress/CURRENT_STATE.md)
- 已完成报告：[`docs/reports/completed/`](../docs/reports/completed/)
- 文档规范：[`docs/standards/documentation.md`](../docs/standards/documentation.md)
- 项目根指南：[`CLAUDE.md`](../CLAUDE.md)
