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

- **huan** = Go 静态站点生成器，阶段一目标：替代 Hugo 构建 zhurongshuo.com，输出与 Hugo 在「肉眼 / SEO / AI 三维度」无差异（甚至更好），详见 [`docs/standards/equivalence.md`](../docs/standards/equivalence.md) 与 [ADR 0001](../docs/adr/0001-redefine-equivalence.md)
- 关联内容项目：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`），当前仍由 Hugo 构建
- 当前分支：`master`；阶段一里程碑 1–9 已全部落地（详见 [`docs/progress/CURRENT_STATE.md`](../docs/progress/CURRENT_STATE.md)）
- `huan build` 与 Hugo byte-diff 一致率：905/2028 共有文件完全相同（44.6%，噪声 ±75，详见经验教训），剩余 5 类差异已按三维度尺子重新归类（详见 [ADR 0001](../docs/adr/0001-redefine-equivalence.md)）
- `huan serve`（HTTP + fsnotify + LiveReload）已于 2026-06-12 完成，17 commits 落地
- 仓库整洁度：无 TODO/FIXME 标记，无 backup/tmp/CI/Makefile；`pkg/` 与 `internal/{pipeline,plugin,search}/` 已删除（stage 2 时再建）

## 关键决策

- 模板引擎阶段一用 `html/template`，阶段二可插件替换
- Markdown 用 goldmark（与 Hugo 同源库）
- 配置格式 `huan.yaml`（YAML），非 drop-in 替换 Hugo
- 验证方式：`./scripts/diff-build.sh` 多模式对比（byte 雷达 + normalized / seo / ai 三维度门禁），三维度任一失败则阻断合并
- 插件架构阶段一**只预留文档**（接口未落地，连占位空目录都已删除），stage 2 增量扩展
- 加密密文仍由外部 Node.js `scripts/encrypt-content.js` 生成，huan 只负责读取与嵌入
- serve 模式用临时目录 `os.MkdirTemp("", "huan-serve-*")`，绝不污染 `docs/` 生产输出
- rebuild 用原子 swap（sibling staging dir + rename），保证 rebuild 期间无 404
- `BuildSite` 非并发安全，rebuild 通过 `atomic.Bool` 串行化 + pending 合并
- **三维度等价标准（2026-06-12，ADR 0001）**：stage 1 验收从「逐字节 100% 一致」改为「肉眼 / SEO / AI 三维度与 Hugo 输出无差异（甚至更好）」。byte-diff 保留作回归雷达，三维度对比作为门禁。允许修正型 + 扩展型「更好」（不破坏基线即可）。

## 经验教训

- **huan build 非确定性**（2026-06-12 发现）：连续两次跑相同代码的 `./huan build`，输出有约 75 个文件不同（占 3.7%）。来源未确认——可能涉及 map iteration、文件扫描顺序、或时间戳。**含义**：单次 diff-build.sh 的 same/diff 计数有 ±75 噪声。判断"改动让 diff 增/减 N 个"时，N 在 75 以内**不可信**——必须用同一份 huan-baseline 跟新 huan-output 直接对比，而不是依赖绝对数字。
- **Go template + Scratch 引用语义 bug**（2026-06-12 撞墙，未解）：模板 `{{ $x := sort ($scratch.Get "key") }}{{ range $x }}` 中，`range $x` 实际遍历的是 `$scratch.data["key"]` 的**原始 slice**，不是 sort 返回值。证据：把 sort 改成 in-place mutate input slice 也不能改变 HTML 输出。试图给 `internal/template/funcs.go` 的 `sortFunc` 加 PageSlice/[]interface{} mutation 都失败。**含义**：补 Hugo 兼容排序的"在 sortFunc 里动手脚"路径走不通。下次尝试应该从 **Scratch.Add/Set 的 slice 共享** 或 **Go template 变量赋值在 pipe + variadic + 多返回值场景下的实际行为** 入手，并配合最小复现单元测试。
- **grill 实证必须 fresh build**（2026-06-12 教训）：早期基于 `/tmp/huan-output`（几天前的旧 build）做实证，误判 huan 有"effective-constraints 缺 15 章"的 bug。重新 build 后该 bug 消失——是旧输出造成的假象。**含义**：任何关于 huan 输出的实证，必须**当场重新跑** `./huan build`，不能信任 `/tmp` 里已有的输出。
- **CURRENT_STATE.md 的「5 类差异」描述部分过期**（2026-06-12 更新）：第 2 类「Hugo date 相同时顺序不稳定」是错的——实证发现 Hugo 有稳定 tiebreaker（推断为 `Date desc → lower(Title) asc → RelPath asc`）。stage 1 收尾后所有 5 类差异已按三维度尺子重新归类，详见 [`docs/standards/equivalence.md`](../docs/standards/equivalence.md) §4 与 §5。

## 文档与导航

- 入口索引：[`docs/INDEX.md`](../docs/INDEX.md)
- 总图：[`docs/technical-plan.md`](../docs/technical-plan.md)
- 当前进展：[`docs/progress/CURRENT_STATE.md`](../docs/progress/CURRENT_STATE.md)
- 已完成报告：[`docs/reports/completed/`](../docs/reports/completed/)
- 文档规范：[`docs/standards/documentation.md`](../docs/standards/documentation.md)
- 项目根指南：[`CLAUDE.md`](../CLAUDE.md)
