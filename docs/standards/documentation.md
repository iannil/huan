# 文档规范

> 抽取自 `CLAUDE.md`「项目指南 · 文档约定」一节，便于独立引用与 LLM 遵循。

## 目录用途与放置规则

| 目录 | 用途 | 何时放这里 |
|---|---|---|
| `docs/` 根 | 项目蓝图与入口索引 | `INDEX.md`（导航）、`technical-plan.md`（总图） |
| `docs/progress/` | 进行中工作 | 任何**未完成**的实施 / 调研 / 修正任务 |
| `docs/reports/` | 验收文档 | 对已完成工作的验收 / 评审记录 |
| `docs/reports/completed/` | 已完成报告 | commit 已落地、测试已通过、验收已确认 |
| `docs/standards/` | 规范 | 命名 / 目录 / 编码 / 发布等长期约定 |
| `docs/templates/` | 模板 | progress / report / spec 等文档骨架 |

## 命名规范

文件名格式：`YYYY-MM-DD-{kebab-case-name}.md`

- 日期取**实际修改时间**，可用 `git log -1 --format=%ai -- <path>` 查询
- 主题用全小写 kebab-case，例：`2026-06-12-serve-implementation-report.md`
- 永久性文档（INDEX、规范、模板）不带日期前缀

## 归档时机

```
新建任务 → docs/progress/YYYY-MM-DD-{name}.md
              │
              │  commit 已合并 + 测试通过 + 验收完成
              ▼
         docs/reports/completed/YYYY-MM-DD-{name}.md
```

归档时同时：
- 把 progress 文档里的 `- [ ]` 全部勾选为 `- [x]`
- 在归档文档头部加完成日期与对标对象（如对标 `hugo serve`）
- 必要时另写一份精简「完成报告」，与原计划文档并列存放

## 进展随时保存

- **执行修改过程中**：进展随时记录到 `docs/progress/` 下对应文档，带上实际修改时间
- **每次修改必须延续上一次的进展**：先读已有文档，再追加 / 修订，禁止覆盖式重写
- **跨会话可恢复**：任何中断点都应有可执行的下一步指引

## 文档维护

对**重复 / 冗余 / 不能体现实际情况**的文档或文档内容，保持主动更新与调整：

- 同一主题只留一份权威文档，其他重定向或删除
- 过期内容直接修订，不保留"历史版本"（git 已记录历史）
- 矛盾内容以**最新日期 + 实际代码状态**为准，旧文档让位

## 关联规范

- `CLAUDE.md` —— 项目根指南，包含语言约定（中文交流 / 英文代码）、发布约定、可观测性、记忆系统
- `docs/templates/progress-template.md` —— 进行中工作模板
- `docs/templates/report-template.md` —— 完成报告模板
