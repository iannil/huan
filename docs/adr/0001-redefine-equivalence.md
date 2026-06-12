# ADR 0001：重新界定「100% 还原」为三维度等价

- **状态**：Accepted
- **日期**：2026-06-12
- **决策者**：用户（owner）+ Claude（grill-me 收敛）
- **替代方案**：保持「逐字节 100% 一致」原标准

## 背景

stage 1 原目标是「huan 输出与 Hugo 逐字节 100% 一致」。该目标在 zhurongshuo 实际推进中遇到：

1. **边际收益递减**：剩余 5 类边缘差异里，部分（如字数统计算法）涉及对齐 Hugo 内部 CJK 分词器，工作量大但与「用户实际感知」无关
2. **目标过严**：HTML 源码层的换行差异（如 `</h2>\n<p>` vs `</h2> <p>`）在浏览器渲染时等价，但 byte-diff 仍标为差异
3. **目标错位**：阶段一真正的承诺是「zhurongshuo 用户/搜索引擎/AI 切换到 huan 后无感」，而非「字节一致」
4. **实证撞墙**：Go template + Scratch + sort 的引用语义问题导致 RSS items 顺序 tiebreaker 修复无法落地，反映「严格还原」路线在某些点不可行

## 决策

把 stage 1 的「100% 还原」重新定义为「**与 Hugo 输出对比，肉眼 / SEO / AI 三维度均无差异（甚至更好）**」。

### 三维度定义

| 维度 | 测量方法 | 等价判据 |
|---|---|---|
| 肉眼 | HTML normalize 后字节对比（折叠空白、规范 attribute、自闭合标签） | normalize 后完全等价 |
| SEO | SEO 关键字段提取对比（title / description / og:* / canonical / h1-h3 / JSON-LD / sitemap / robots） | 所有字段逐项等价 |
| AI | AI 友好度字段对比（main 内容 / heading outline / JSON-LD / llms.txt / 内部链接 graph / 语义化标签） | 所有字段逐项等价 |

### 「甚至更好」的允许范围

stage 1 范畴内允许两种「更好」：

1. **修正型**：Hugo 输出有客观错误时（如 WordCount 不准、HTML 不规范），huan 可以选择修正
2. **扩展型**：huan 可以主动添加 Hugo 未做的现代实践（如 llms.txt / 额外 JSON-LD），只要不破坏三维度无差异基线

每项「更好」必须文档化并标记 "better than Hugo"。

### 与 diff-build.sh 的关系（分层并存）

- `scripts/diff-build.sh` 的原 byte-diff 模式保留作回归雷达（仅报告，不阻断合并）
- 新增 normalized / seo / ai 三种对比模式，任一失败则阻断合并

## 5 类差异的归类

基于三维度尺子重新评估（详见 `docs/standards/equivalence.md`）：

| # | 差异 | 归类 | 处理 |
|---|---|---|---|
| 1 | 字数统计精度 | 必修 | Port Hugo WordCount 算法（覆盖 unicode.Is(unicode.Han) 全范围 + 假名 + 韩文 + 全角符号） |
| 2 | RSS items 顺序 | 应修 | `sortPagesByDateDesc` 加 tiebreaker（date desc → lower(title) asc → relpath asc） |
| 3 | RSS description 截断 | 应修 | `TruncateHTMLByWords` 改为 word-boundary 截断 |
| 4 | products summary 换行 | 接受 | 永久差异（渲染等价），登记在 `docs/standards/equivalence.md` |
| 5 | general summary 截断 | 应修 | 与 #3 在 summary 后处理统一 |

## 影响

- stage 1 收尾判定：4 项必修/应修全部解决 + 三维度管线建立并通过 + 本 ADR 写完 + 全面文档化
- stage 2 起步：llms.txt + 额外 JSON-LD + 搜索/插件（按 `docs/technical-plan.md` §4.11）
- 现有 `MEMORY.md` / `CURRENT_STATE.md` 中的「100% 一致」「Hugo date 不稳定」等表述需要修订
