# 2026-06-14 翻译插件质量门修复（format_purity + length_ratio + prompt 强化）

## 起因

用户报告：`huan translate qwen3` 跑 zhurongshuo 长文时，所有文件 `hard_fail: [markdown_structure]`，sidecar 全部不落盘。`length_ratio` 一直 0.48-0.56。

用户最初的诉求：「修改翻译插件，要考虑单文本文件长度很长，超过本地大模型的上下文的情况」（暗示要做切块）。

## 实证根因（不走间接证据，直接 probe Ollama）

跑 `chapter-04.md`（35KB / 12k CJK chars）直击 Ollama `/api/chat`，绕开 huan 的 hard_fail 短路：

| 维度 | 值 | 结论 |
|---|---|---|
| `done` | true | **没有截断** |
| `done_reason` | stop | **正常终止** |
| `eval_count` | 7831 | **完整生成** |
| 输出字符数 | 36389 | 比源（11994）**多 3 倍** |
| 源 `## / ###` 数 | 6 / 6 | — |
| 输出 `## / ###` 数 | **0 / 0** | markdown_structure 必挂 |

**真正的失败模式**：模型把 markdown 翻译成了 HTML。

```html
<h1>Chapter 4: ...</h1>
<p>Thus far, we have painted ...</p>
<ol><li>...</li></ol>
<h2>4.1 The Illusion of Subject and Object ...</h2>
```

所有 6 个 h2 + 6 个 h3 都还在，内容完整翻译，只是格式从 markdown 变成 HTML。

**根因彻底反转**：既不是「上下文窗口超限」（输入 7646 tokens，输出 7831 tokens，模型家有空间），也不是「输出截断」，也不是「模型压缩」。**根本不需要做切块**。

`length_ratio 0.48-0.56` 是伪信号：原 metric `out_latin_words / src_cjk_chars`，对中→英翻译天然偏低（一句中文 30 字 → 英文 10 词，ratio ≈ 0.33）。

## 修复方案（B → A → C）

### B：新增 `format_purity` 硬检查（严格派）

**决策**：严格派，不做 HTML 等价识别（宽容派被否决）。
- `.en.md` 扩展名就是 markdown 契约
- HTML 进 build pipeline 会让 goldmark 渲染、内链重写、summary、toc、hreflang 每一处都要兼容判断
- 治本：强制 markdown 契约；A 治根：让模型真的输出 markdown

**实现**：
- `internal/translate/qwen3/quality.go::CheckFormatPurity`：regex 黑名单匹配 markdown 等价块级 HTML 标签
- 黑名单：`h1-h6, p, ul, ol, li, pre, blockquote, table, thead, tbody, tfoot, tr, td, th, dl, dt, dd, section, article, header, footer, nav, aside, div`
- 不在黑名单：`<span>`/`<em>`/`<strong>`/`<a>`/`<br>`（goldmark unsafe=true 允许源里有，模型保留合规）
- `internal/translate/types.go::QualityResult.FormatPurity`：新字段
- `HardCheckFailures()`：把 `!FormatPurity` 加入硬失败列表
- `plugin.go`：组装 `qr.FormatPurity`
- `translate_cmd.go`：sidecar frontmatter 加 `format_purity: %t`

### A：强化 prompt suffix

**决策**：(iii) 分层——代码 suffix 放硬约束，user 的 `system_prompt_file` 放风格。

**实现**：
- `internal/translate/qwen3/prompt.go::assembleUserPrompt`：末尾 suffix 从一行改为 8 行 `CRITICAL FORMAT RULES` 块
- 显式禁止 HTML 标签 + 列出 markdown 等价物（`#`/`##`/`###`、blank-line paragraphs、`-/*/+` lists、`[text](url)`、` ``` ` fences）
- 强约束 "Preserve ALL source markdown structure 1:1"
- 先不加 few-shot（保留为下一轮子弹）

### C：length_ratio 改字符膨胀比

**决策**：(i) 字符数比，阈值 `[0.5, 3.5]`。

**实现**：
- `quality.go::CheckLengthRatio`：分子分母都改成 `utf8.RuneCountInString`
- `options.go::defaults()`：`LengthRatioMin=0.5, LengthRatioMax=3.5`
- 删除 `countRoughTokens`（无人引用）
- 保留 `countLatinWords`（测试还在用）

## 实证验证（修完代码后重跑 chapter-04.md）

| 维度 | 修前 | 修后 |
|---|---|---|
| format_purity | — | **PASS**（0 HTML 命中） |
| `## / ###` 数（src→out） | 6→0 / 6→0 | **6→6 / 6→6** |
| char_ratio | 0.519（伪信号） | **2.819**（在 [0.5, 3.5] 内） |
| XML 解析 | OK | OK |
| done_reason | stop | stop |
| eval_count | 7831 | 7267 |

**A 单发命中**——prompt 改完后 HTML 输出立即消失，没需要 few-shot 子弹。

## 新发现的失败模式（不在本次 PR 范围，记下来追踪）

修完 B/A/C 后，跑 chapter-04.md 暴露新问题：

- **模型跳过开头 4 段不翻译**——开篇引子段保留中文原文，从 `## 4.1` 才开始翻译
- 段落中间夹中文片段（如 "enabling冷静的、数学化的分析。"）
- 总 CJK 字符 733 / letters 27073 = 2.7% ——**远低于 language_detection 阈值 20%**，会溜过 quality gate

**当前 quality 检查抓不住**：
- `format_purity` PASS（格式正确）
- `markdown_structure` PASS（结构数对得上）
- `length_ratio` PASS
- `language_detection` PASS（CJK 占比低）

**潜在修复方向**（未实施，留下一轮 grill）：
- 加段落级 language 检查（每段独立验语言比例，而非整体平均）
- 加"输出含源文片段"检测（src 句子前 10 字符出现在 out 里即 fail）
- prompt 加更强约束（"Translate EVERY paragraph, including opening"）
- 模型层换 fallback（`qwen3:14b` 非 MoE，prior 可能更弱）

---

## D：非对称 heading tolerance（2026-06-14 增补）

### 触发：长附录 / 长章节的 markdown_structure hard_fail

跑全量翻译时发现：`books/volume-1/advancement-of-reality/` 目录下的 `appendix.md`、`chapter-02.md`、`chapter-03.md` 全部 `hard_fail: [markdown_structure]`，但 `chapter-01.md` / `chapter-04.md` / `chapter-05.md` / `chapter-06.md` PASS。

### Probe 实证（appendix.md）

| Probe 版本 | prompt suffix | heading diff |
|---|---|---|
| v1 | 原始 B/A/C prompt | src 46 → out 54 (**+8**) |
| v2 | 加 `HEADING INVARIANTS` 块 + 明确禁止 "Part One" 分组 + "MUST have exactly 46 heading lines" | src 46 → out 54 (**+8**) |

**Prompt 强化 0 效果**——模型在长附录上"加 Part One/Two/... 中间分组"的 prior 极强，明确禁止 + 数字约束都无效。但 list items 在 v2 里 0→0（模型听话），证明这不是疏忽，是 heading 的认知偏好。

### 设计决策（grill-me Q2 收敛）

走 (b-i) 非对称 tolerance：

```
旧规则（对称）：  abs(out - src) > tol        → fail
新规则（非对称）：(src - out) > tol           → fail  # 不允许丢超过 tol 个
                  out > src * (1 + 0.25)      → fail  # 不允许加超过 25%
```

**只对 heading 非对称**，list/link/image/codefence 维持对称/严格。

### 实现

- `internal/translate/qwen3/quality.go::CheckMarkdownStructure`：heading 检查从 `abs(out-src) > tol` 拆成两个比较
- `internal/translate/qwen3/options.go`：新增 `MarkdownHeadingExpansionMax float64`（默认 0.25），defaults() 注入
- `internal/translate/qwen3/quality_test.go`：6 个新测试 case（appendix 场景 + 边界 + 激进扩张 fail）
- `docs/adr/0008-translator-capability-qwen3-plugin.md`：新增 §9.3 决策记录 + §9 表格更新（heading 非对称说明）

### 验证（推理验算，未跑 Ollama）

appendix.md 源 46 headings，实测输出 54：
- loss = 46 - 54 = -8（负数，不算 loss）→ 第一条规则 PASS
- max allowed = 46 + int(46 * 0.25) = 46 + 11 = 57
- out 54 ≤ 57 → 第二条规则 PASS

→ appendix.md 在新规则下应该 PASS。等翻译进程跑到该文件实际验证。

### 经验教训

1. **prompt 强化不是万能的**：当模型对某类变换有强 prior（如 heading 重构），明确禁止、显式数字约束、列举反例都可能无效。两次 probe 对照是关键证据。
2. **区分「丢内容」和「加结构」**：翻译 quality 的真实目标是"内容没丢"。模型加 intermediate 分组对最终阅读无害甚至有益，应该接受而非惩罚。
3. **非对称 tolerance 是 checker 层的实用工具**：当 prompt 解不了、又不能换模型时，区分正负偏差是务实的兜底。
4. **失败率监控的偏差**：早期"0% 成功"的报告是抽样偏差（前 5 篇恰好都是长附录/章节）。等 chapter-04/05/06 跑完才发现实际成功率 ~50%。需要更长窗口判断真实失败率。

## 文件改动清单

- `internal/translate/types.go`：加 `FormatPurity bool`，更新 `HardCheckFailures`
- `internal/translate/types_test.go`：更新硬失败计数（3→4）
- `internal/translate/qwen3/quality.go`：加 `htmlBlockTagRe` + `CheckFormatPurity`；改 `CheckLengthRatio` 用 `utf8.RuneCountInString`；删 `countRoughTokens`
- `internal/translate/qwen3/quality_test.go`：加 8 个 format_purity 测试 + 4 个 length_ratio 测试（适配新 metric）
- `internal/translate/qwen3/options.go`：`LengthRatioMax` 默认 2.5→3.5；注释改字符比语义
- `internal/translate/qwen3/prompt.go`：suffix 从 1 行改为 8 行 `CRITICAL FORMAT RULES`
- `internal/translate/qwen3/parse_test.go`：加 prompt suffix 内容断言
- `internal/translate/qwen3/plugin.go`：组装 `qr.FormatPurity`；XML 失败分支补 `FormatPurity: false`
- `internal/translate/qwen3/plugin_test.go`：响应状态测试加 `FormatPurity` 字段
- `cmd/huan/translate_cmd.go`：sidecar frontmatter 加 `format_purity: %t`
- `docs/adr/0008-translator-capability-qwen3-plugin.md`：§9 更新（4 项硬检查）+ 新增 §9.1（format_purity 背景）+ §9.2（length_ratio 字符比修正）

## 测试结果

```
ok  github.com/iannil/huan/internal/translate       0.121s
ok  github.com/iannil/huan/internal/translate/qwen3 0.178s
ok  github.com/iannil/huan/cmd/huan                 0.196s
（全 22 个包全部 PASS）
```

## 经验教训（沉淀到 MEMORY.md）

1. **间接证据不够，要直接 probe 中间层**。用户报告"length_ratio 低 = 截断"看似合理，但直接 curl Ollama 才发现是 HTML 转换，与上下文长度无关。
2. **伪科学 metric 比没 metric 更糟**。原 `length_ratio` 用 mixed token 估算，对所有正常翻译都报警，遮蔽了真问题。换成字符比后立即清晰。
3. **hard_fail 的错误归因很重要**。原 `markdown_structure` 间接抓到 HTML（heading 数 6→0），但报告"markdown_structure"让用户以为是"模型丢了标题"。新增 `format_purity` 直接命名失败模式。
4. **长文上下文问题在这个 scale 下不是问题**。80B MoE 在 35KB / 12k CJK 上跑得稳，输出未截断。切块（chunking）暂时不需要。`appendix.md` 80KB 是潜在风险点，但实测之后再决定。
5. **Qwen3-Next-80B q4_K_M 的两个已知 prior**：(a) 把 markdown 转 HTML（已修）；(b) 长文跳译开头段落（未修，需段落级检查）。
