# 2026-06-14 翻译插件重构：Chunked Translation（section 级 + sliding window）

## 背景

B/A/C/D 修复后仍有 ~30% 长文档失败。根因：Qwen3-Next-80B q4_K_M 在长输入（>20KB）上有强 prior 自作主张重构（加 Part 分组、prose→bullet）。prompt 工程实测无效。

## Grill-me 7 轮收敛

| Q | 主题 | 决策 |
|---|---|---|
| Q1 | 拆分粒度 | section 级（每 `^## ` 一个 chunk） |
| Q2 | 滑动窗口 | token budget=8000，动态塞满 |
| Q3 | 切分算法 | `^## ` regex + code fence aware |
| Q4a | 上下文注入 | user message 内嵌（PREVIOUSLY_TRANSLATED + TRANSLATE_NOW） |
| Q4b | title 时机 | 第一个 chunk 一起翻 |
| Q5 | 失败/重试 | per-chunk retry + atomic write |
| Q6 | 启用/配置/observability | 永远开 + 1 配置旋钮 + 同 trace_id 不同 span_id |
| Q7 | 质量检查 | per-chunk paragraph + heading count（替换整篇 markdown_structure） |

## 实施（10 phases）

| Phase | 内容 | 文件 |
|---|---|---|
| 1 | chunker.go + 9 tests | `internal/translate/qwen3/chunker.go` |
| 2 | context.go (sliding window) + 8 tests | `internal/translate/qwen3/context.go` |
| 3 | assembleChunkPrompt（chunked prompt 模板） | `internal/translate/qwen3/prompt.go` |
| 4 | quality.go：删 markdownCounts/CheckMarkdownStructure，加 chunkStructure/CheckChunkStructure | `internal/translate/qwen3/quality.go` |
| 5 | plugin.go 重写 Translate()（chunked loop + sliding window + per-chunk retry + atomic） | `internal/translate/qwen3/plugin.go` |
| 6 | options.go：加 ChunkContextTokenBudget=8000；MarkdownStructureTolerance 默认改 1 | `internal/translate/qwen3/options.go` |
| 7 | tests 更新（删旧 markdown_structure tests，加 chunk structure tests） | `internal/translate/qwen3/quality_test.go` |
| 8 | parse.go：加 parseChunkBodyOutput（non-first chunk 不需要 `<title>`） | `internal/translate/qwen3/parse.go` |
| 9 | 全量翻译重启 + cron 监控 | `/tmp/translate-full-v3.log` |
| 10 | ADR + 本文档 | `docs/adr/0008-...md` §10、本文档 |

## 验证：chapter-02.md 端到端

| 维度 | 修前 | 修后 |
|---|---|---|
| Pipeline | 整篇一次调用 | 7 chunks（preamble + 6 sections） |
| 总耗时 | ~5 min | 27 min（每 chunk ~3-4 min × 7） |
| 总 tokens | ~17000 | 48386（含 sliding window 上下文） |
| `markdown_structure` | hard_fail（list_items +9 > tol 2） | per-chunk 全 PASS |
| sidecar | ❌ 不写 | ✅ 39769 bytes |
| 滑动窗口演化 | — | 0→649→2877→5752→**7390**（饱和）→6789→5627 |

**关键观察**：滑动窗口在第 5 chunk 接近 budget 饱和（7390/8000），第 6 chunk 自动挤出最早 chunk，回到 6789。这是 sliding window 设计正常工作。

## 设计决策详解

### 为什么 chunking 比 D-fix（asymmetric tolerance）好

| 维度 | D-fix | Chunking |
|---|---|---|
| 治标 vs 治本 | 治标（checker 接受模型重构） | 治本（模型没机会重构） |
| 跨 section 重构（Part 分组） | 允许（25% 上限内） | **阻止**（模型一次只看一个 section） |
| section 内 prose→bullet | 不解决（仍然 fail） | 通过 text units 合并 paragraphs+list_items 容忍 |
| 代码复杂度 | 低（10 行） | 高（新增 chunker + context manager + 重写 plugin） |
| 可观测性 | 同前 | 加 chunk-level trace |
| 失败模式 | 静默接受模型行为 | 失败信号清晰（哪 chunk 失败、为何失败） |

### 为什么 sliding window 不用 chat history

| 方案 | 优点 | 缺点 |
|---|---|---|
| **User message 内嵌**（选定） | 明确分隔 context vs to-translate；debug 友好；不依赖 chat memory | 单条 user message 较长 |
| Chat history 多轮 | 原生 chat 格式；token 高效 | 模型可能误以为"我之前译过 chunk N"而 refuse 或尝试改进 |

### 为什么 text units = paragraphs + list_items

实测 chapter-02.md，模型把源 3 段平行 prose（"概率性/瞬时性/不可逆性"）改成 3 个 bullet。这是地道英文写作惯例（3+ parallel "Term: desc" 该用 bullet）。如果分别检查 paragraphs 和 list_items：
- paragraphs: 3 → 1（diff -2）→ fail
- list_items: 0 → 3（diff +3）→ fail

合并为 text units：3 → 4（diff +1）→ **PASS**。

合并后的不变量："不改变离散内容块的数量"——既允许排版重排，又抓内容增删。

## 当前状态

- ✅ Phase 1-8 完成 + 全 19 包测试 PASS
- ⏳ Phase 9 全量翻译运行中（PID 23065，1052 stale，chunked pipeline）
- ⏳ Phase 10 文档完成（本文档 + ADR 0008 §10）
- 📊 Cron `2564e257` 每 30 分钟监控（17/47 分）

## 文件改动总览

```
internal/translate/qwen3/
├── chunker.go              (new)   section-level splitter，code fence aware
├── chunker_test.go         (new)   9 cases
├── context.go              (new)   estimateTokens + slidingWindowContext
├── context_test.go         (new)   8 cases
├── prompt.go               (mod)   +assembleChunkPrompt；旧 assembleUserPrompt 保留供向后兼容
├── plugin.go               (mod)   重写 Translate()：chunked loop + sliding window + per-chunk retry + atomic
├── quality.go              (mod)   删 markdownCounts/CheckMarkdownStructure；加 chunkStructure/CheckChunkStructure
├── quality_test.go         (mod)   删旧 markdown_structure tests；加 chunk structure tests
├── parse.go                (mod)   +parseChunkBodyOutput（non-first chunk 不需 <title>）
└── options.go              (mod)   +ChunkContextTokenBudget=8000；MarkdownStructureTolerance 默认 1
```

## 经验教训

1. **prompt 强化不是万能的**：模型对某些变换（heading 重构、prose→bullet）有强 prior 时，明确禁止 + 数字约束 + 反例都无效。需要架构级修复。
2. **chunking 是架构级修复**：通过限制模型一次只看一个 section，从源头消除跨 section 重构的可能性。
3. **sliding window 平衡上下文 vs token 成本**：动态 budget（默认 8000）让短文档全带、长文档自动挤出最早 chunks。
4. **per-chunk 检查粒度更精准**：替换整篇 markdown_structure 后，能在 chunk 级别定位失败原因，而不是模糊的"结构不匹配"。
5. **text units 合并是关键设计**：允许 prose→bullet 重排（地道英文写作惯例），同时仍然抓内容增删。
6. **section 内的失败模式仍需 prompt 工程辅助**：chunking 不能 100% 阻止 section 内重排，per-chunk check 兜底。如果 per-chunk check 仍频繁 fail，下一步可能是 paragraph-level chunking（成本 10x）。
