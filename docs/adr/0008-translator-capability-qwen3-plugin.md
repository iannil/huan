# ADR 0008：Translator capability + Qwen3 首发 plugin

- **状态**：Proposed（待 PR1 落地后转 Accepted）
- **日期**：2026-06-14
- **决策者**：用户（owner）+ Claude（grill-me 收敛）
- **依赖**：[ADR 0003](0003-unified-plugin-system.md)（统一插件系统）
- **被引用**：[ADR 0007](0007-i18n-build-system.md)（i18n build core 消费本插件产出的 `.en.md`）

## 背景

[ADR 0003](0003-unified-plugin-system.md) 把 plugin 做成 huan 的一等扩展机制，首期只画 `Deployer` 一个 capability。i18n 多语言需求（[ADR 0007](0007-i18n-build-system.md)）要求把"中文 → 英文"的翻译过程做成 plugin 产出物（`.en.md` sidecar）。

**为什么 Translator 是 capability 而非 build pipeline 的一部分**：翻译涉及外部依赖（LLM API / Ollama HTTP），与 build pipeline 的"纯函数"特性冲突。把翻译隔离在 plugin 里：
- build pipeline 仍可纯函数式运行（无 LLM 依赖、无网络调用）
- 翻译产物（`.en.md`）作为静态输入进入 build pipeline
- 用户可以选择不运行 plugin（手工创建 `.en.md` 或仅发布中文版）

**用户具体场景**：zhurongshuo 默认中文，deploy 时翻译为英文。用户硬件：Apple M5 Max + 128GB unified memory（详见 [memory/daily/2026-06-14.md](../../memory/daily/2026-06-14.md) grill-me 记录）。

## 决策

### 1. Capability 接口

```go
// internal/translate/types.go
package translate

import (
    "context"
    "github.com/iannil/huan/internal/plugin"
)

// Translator is the capability interface for plugins that translate content
// between languages. It is the second capability in huan's unified plugin
// system (after deploy.Deployer).
type Translator interface {
    plugin.Plugin

    // Translate converts source content to the target language.
    // Implementations should:
    //   - Honor ctx for cancellation.
    //   - Return a Response with the translated content even on partial
    //     success (quality check warnings don't fail the call; only hard
    //     errors like XML parse failure do).
    //   - Return a non-nil error only when translation cannot proceed at all
    //     (e.g. LLM unreachable, invalid config). Per-file quality issues
    //     go into Response.QualityChecks, not the error return.
    Translate(ctx context.Context, req Request) (*Response, error)
}

type Request struct {
    SourceLang  string   // e.g. "zh-cn"
    TargetLang  string   // e.g. "en"
    Title       string   // source title (translated separately, same LLM call)
    Content     string   // source markdown body
    ContentType string   // "markdown" (future: "plain" / "html")
    Hints       []string // user-supplied prompt hints
    Glossary    map[string]string  // term → translation (from i18n/terms.yaml)
}

type Response struct {
    Title       string           // translated title
    Body        string           // translated body
    Model       string           // model identifier used
    TokensUsed  int              // total tokens (input + output)
    DurationMs  int64            // LLM call duration
    QualityChecks QualityResult  // pass/fail per check
}

type QualityResult struct {
    XMLParse            bool    // output parsed as <title>...</title><body>...</body>
    LanguageDetection   bool    // output ≥ 80% target language
    MarkdownStructure   bool    // heading/list/link/image counts match ±2
    LengthRatio         float64 // body_words / source_words (warn if outside [0.5, 2.5])
    GlossaryCompliance  bool    // all glossary terms correctly applied
    RetryCount          int     // retries triggered by quality failures
}
```

**为什么 Request 是值类型而非指针**：Request 是数据传输对象，调用方填充后传给 plugin；不可变语义清晰，避免 plugin 内部修改影响调用方。

### 2. 首发实现：`internal/translate/qwen3/`

通过 Ollama HTTP API（`http://localhost:11434/api/chat`）调用本地 Qwen3 模型。

**默认模型**：`qwen3-next:80b-a3b-instruct-q4_K_M`

**为什么 Qwen3-Next-80B 是首发而非 API**：基于用户硬件（M5 Max + 128GB）实测——80B-A3B MoE 反超 14B dense 速度（MoE 稀疏激活 + Apple Silicon 高带宽内存甜点），翻译产物无 thinking 污染（instruct 变体），零边际成本，翻译可重现（避免 API 厂商静默升级破坏 byte-deterministic）。详见 [memory/daily/2026-06-14.md](../../memory/daily/2026-06-14.md) 实测数据。

**包结构**：

```
internal/translate/
├── types.go              # Translator interface + Request/Response/QualityResult
└── qwen3/
    ├── plugin.go         # Plugin struct + New() + Name() + Translate()
    ├── client.go         # Ollama HTTP client wrapper
    ├── prompt.go         # System prompt assembly + glossary injection
    ├── quality.go        # Quality check implementations
    ├── parse.go          # XML tag parser (<title>...</title><body>...</body>)
    ├── options.go        # Config struct + ParseConfig(raw)
    ├── plugin_test.go    # Unit tests
    ├── client_test.go    # HTTP client tests with mock server
    ├── quality_test.go   # Quality check tests
    └── parse_test.go     # XML parse tests
```

### 3. 配置（yaml）

```yaml
plugins:
  qwen3_translate:
    endpoint: http://localhost:11434          # Ollama HTTP base URL
    model: qwen3-next:80b-a3b-instruct-q4_K_M # default model
    fallback_model: qwen3:14b                  # used if default unavailable
    timeout_seconds: 120                       # per-call LLM timeout
    concurrency: 1                             # Ollama single-instance = 1
    system_prompt_file: i18n/translate-prompt-zh-en.md  # user-editable prompt
    glossary_file: i18n/terms.yaml             # manual term dictionary
    examples_dir: ""                           # optional few-shot examples (v1 empty)
    quality:
      length_ratio_min: 0.5
      length_ratio_max: 2.5
      target_language_threshold: 0.8           # 80% output must be English
      markdown_structure_tolerance: 2          # ±2 count diff
      enforce_glossary: true
      retry_on_violation: 1                    # retry once on soft warning
    site_translations:                         # site-level metadata cache (ADR 0007 §6)
      en:
        subTitle: "..."
        description: "..."
        keywords: ["..."]
        footerSlogan: "..."
```

**所有路径相对项目根目录**（即 `huan.yaml` 所在目录）。

### 4. CLI

```bash
# 翻译所有 stale 文章（source_hash 不匹配）
huan translate qwen3

# 翻译单篇
huan translate qwen3 --file content/posts/2026/foo.md

# 强制重译全部（忽略 source_hash cache）
huan translate qwen3 --all --force

# Dry-run（列出待译文件清单，不调 LLM）
huan translate qwen3 --dry-run

# 用 fallback 模型（如默认模型未拉到本地）
huan translate qwen3 --model qwen3:14b

# 维护术语字典：扫描未字典化 tag，调 LLM 建议
huan translate terms --propose

# 翻译状态报告
huan translate status
```

**与 plugin list/info 的关系**：
```bash
huan plugin list                          # 列出所有 plugin（含 qwen3_translate）
huan plugin info qwen3_translate          # 显示 effective config + last run summary
```

### 5. 输出契约：`.en.md` sidecar

每篇译文的 frontmatter：

```yaml
---
translation_of: posts/2026/foo.md         # 源文件相对路径
source_lang: zh-cn
target_lang: en
source_hash: 5f3e7a2b...                  # sha256(源 markdown body)
model: qwen3-next:80b-a3b-instruct-q4_K_M
model_hash: a1b2c3...                     # 可选，模型权重 hash（升级时强制重译）
translated_at: 2026-06-14T12:34:56Z
translator: qwen3
quality_checks:
  xml_parse: true
  language_detection: true
  markdown_structure: true
  length_ratio: 0.89
  glossary_compliance: true
  retry_count: 0
tokens_used: 1234
---
```

Body 是翻译后的 markdown 正文。

### 6. 增量翻译机制

`huan translate qwen3` 流程：

1. 遍历 `content/**/*.md`（排除 `*.en.md` 与 `*.zh-cn.md`）
2. 对每篇中文原文算 `sha256(body)`
3. 查对应 `.en.md`：
   - 不存在 → 加入待译队列
   - 存在但 `source_hash` 不匹配 → 加入待译队列（原文变了）
   - 存在但 `model` 与当前 default 不匹配 → 加入待译队列（模型升级）
   - 存在且全部匹配 → 跳过（cached）
4. 待译队列交给 translator，按 `concurrency` 限流
5. 翻译完成写 `.en.md` + 更新 frontmatter

**触发重译的条件**：
- 中文原文变了（`source_hash` 不匹配）
- 文章新增（`.en.md` 不存在）
- 模型升级（`model` 字段不匹配 default config）
- 用户强制 `--force --file <path>` 或 `--force --all`

### 7. Prompt 架构

**单次 LLM 调用产出 title + body**（XML 标签结构化输出）：

```
[System Prompt]
（用户维护，i18n/translate-prompt-zh-en.md）

[User Prompt]
GLOSSARY:
专注 → focus
觉察 → awareness
法 → Dharma
道 → the Way

SOURCE_TITLE: 法不净空，觉无性也。

SOURCE_BODY:
<markdown 正文>

Translate now. Output ONLY <title>...</title><body>...</body>.
```

**System prompt 内容**（user 可编辑的初稿）：
- 角色：资深中文哲学/冥想文学翻译家
- 风格指南：文学语域、保留歧义、短句节奏、具体意象优先
- 术语策略：严格用 GLOSSARY；专有名词保留拼音；关键哲学概念首次出现带拼音
- Markdown 保真：标题层级、列表缩进、链接 URL、图片 URL、代码块、引用块、表格结构
- 输出契约：XML 标签 `<title>` 与 `<body>`，无 chain-of-thought，无解释

**为什么 XML 标签不是 JSON**：
- LLM（尤其 Qwen3 系列）对 XML 风格的结构化输出更可靠（训练数据 XML 多）
- JSON 对引号/换行/特殊字符的 escaping 是 LLM 出错高发区
- XML 标签内容是 raw text，不需要 escape

### 8. Glossary 双层防护

- **预注入**：所有 `i18n/terms.yaml` 条目在每次调用时塞入 `GLOSSARY:` 块（即使本篇 body 不含这些词——保持 prompt 形态稳定）
- **后验证**：翻译完成后扫描输出：
  - GLOSSARY 左侧的中文词是否仍然出现（漏译检查）
  - GLOSSARY 右侧英文译名是否在输出中出现（自作主张换译名检查）

后验证失败 → 重试 1 次，prompt 加提示"GLOSSARY 中的『专注』必须译为『focus』，不要译为其他词"。仍失败 → 标记为失败，写入失败日志，跳过此篇。

### 9. 质量守门（plugin 内置检测器）

| 检查 | 类型 | 阈值 | 失败动作 |
|---|---|---|---|
| XML 解析 | **硬** | `<title>` 与 `<body>` 都成功解析 | 标记失败，不写盘 |
| 语言检测 | **硬** | 输出 80%+ 是英文 | 标记失败，不写盘 |
| Markdown 结构计数 | **硬** | heading/list/link/image 数量与源一致 ±2 | 标记失败，不写盘 |
| **Format purity** | **硬** | 输出不含 markdown 等价 HTML 块级标签（h1-h6/p/ul/ol/li/pre/blockquote/table/...） | 标记失败，不写盘 |
| 长度比 | 软警告 | `out_chars / src_chars ∈ [0.5, 3.5]`（字符膨胀比） | 重试 1 次 |
| GLOSSARY 违规 | 软警告 | 输出含未译中文术语或自换译名 | 重试 1 次 |
| 重复短语检测 | 软警告 | 输出含连续重复短语（hallucination 模式） | 重试 1 次 |

**四项硬检查全部通过才允许写盘**；其他检查可配置为 warn-only。

**为什么四项硬检查是足够的**：
- XML 解析：确保输出格式可消费
- 语言检测：确保输出确实是目标语言（防 LLM 偶尔"懒得翻"返回原文）
- Markdown 结构：确保渲染不会破坏（防 LLM 丢列表/丢链接）
- Format purity：防 LLM 把 markdown 转成 HTML（Qwen3-Next-80B q4_K_M 在长 zh→en 输入上的已知 prior；详见 §9.1）

软警告通过 retry 后即写盘（即便仍 warn）。

#### 9.1 Format purity 检查（2026-06-14 增补）

**背景**：首次实测 zhurongshuo `books/volume-1/advancement-of-reality/part-01/chapter-04.md`（35KB / 12k CJK chars）时，模型输出把 markdown 全部转成 HTML（`<h1>`/`<h2>`/`<p>`/`<ol>`/`<li>`）。`markdown_structure` 检查间接抓到（heading 数 6→0），但错误归因误导。新增 `format_purity` 直接命名该失败模式。

**检查规则**（`internal/translate/qwen3/quality.go::CheckFormatPurity`）：regex 命中 markdown 等价块级 HTML 标签即 fail。黑名单：

```
h1-h6, p, ul, ol, li, pre, blockquote, table, thead, tbody, tfoot, tr, td, th,
dl, dt, dd, section, article, header, footer, nav, aside, div
```

**为什么是黑名单而不是零容忍**：
- goldmark `unsafe: true` 允许源 markdown 含合法 inline HTML（`<span>`/`<em>`/`<a>`/`<br>`），模型保留它们是合规的
- 块级 HTML 才是"格式转换"的特征信号
- 黑名单方法误报低、漏报可控（仅当模型用冷门标签如 `<dl>` 时漏报，已在黑名单内）

**为什么不宽容派（接受 HTML 等价）**：
- `.en.md` sidecar 扩展名就是契约——必须 markdown
- build pipeline（goldmark 渲染、内链重写、summary 抽取、toc 生成、hreflang）每处都假设 markdown AST
- 严格化反而是真实信号：HTML 漏出率能反馈 prompt 工程质量

#### 9.2 Length ratio: 字符膨胀比（2026-06-14 修正）

**原 metric**：`out_latin_words / src_rough_tokens`，其中 `src_rough_tokens` 用 CJK 字符 + Latin 词。zh→en 正常翻译此值 ≈ 0.5（一句中文 30 字 → 英文 10 词），落在原 `[0.5, 2.5]` 下界，**对所有正常翻译都触发软警告**。

**新 metric**：`out_chars / src_chars`（字符膨胀比）。zh→en 实测典型 1.5-2.5；chapter-04.md 实测 2.82（12000 src CJK chars → 33810 out chars）。

**新阈值**：`[0.5, 3.5]`（zh→en 上限留 0.5 余量给更长 Philosophical prose）。

**为什么字符比稳定**：跨语言通用（en→zh 反向比例 ~0.4-0.7；同语言 ~1.0），不需要按语言对调阈值。

### 10. 失败处理

**per-file 失败不阻断整体**（collection-not-interruption 模式，与 ADR 0002 §9 一致）：
- 单篇翻译失败（API 超时 / Ollama crash / 输出格式错）→ 写日志 + 继续下一篇
- 失败的文件不留 `.en.md`，下次 `huan translate` 自动重试
- 全部完成后输出 Report：translated=N / skipped=M / failed=K + 失败文件清单
- 失败 > 阈值（如 > 50%）才阻断后续步骤，否则允许继续

### 11. Observability（CLAUDE.md "全链路可观测性" mandate）

复用 `internal/observability/` 包（与 deploy/release 共享 Logger）。

每篇翻译事件结构化日志：

```json
{
  "timestamp": "2026-06-14T12:34:56.789Z",
  "trace_id": "translate-2026-06-14T12-00-00-abc123",
  "span_id": "file-posts-2026-foo-md",
  "event_type": "Function_End",
  "payload": {
    "file": "content/posts/2026/foo.md",
    "source_lang": "zh-cn",
    "target_lang": "en",
    "source_hash": "5f3e...",
    "cached": false,
    "model": "qwen3-next:80b-a3b-instruct-q4_K_M",
    "tokens_input": 1234,
    "tokens_output": 1100,
    "duration_ms": 7100,
    "quality_checks": {
      "xml_parse": true,
      "language_detection": true,
      "markdown_structure": true,
      "length_ratio": 0.89,
      "glossary_compliance": true,
      "retry_count": 0
    },
    "status": "success"
  }
}
```

Execution Trace Report（每次 `huan translate` 完成后输出 summary）：

```
Translation Report
==================
Trace ID: translate-2026-06-14T12-00-00-abc123
Started: 2026-06-14T12:00:00Z
Ended:   2026-06-14T15:23:45Z
Duration: 3h 23m 45s

Files:
  Total scanned:    1500
  Already cached:   1450  (source_hash match)
  Newly translated: 38
  Retried:          7
  Failed:           5
    - content/posts/2025/bar.md (XML parse failed)
    - content/posts/2024/baz.md (length ratio 0.31 - suspicious)
    - ...

Quality:
  Avg length ratio:  0.92
  Avg tokens/post:   1150
  Glossary violations: 3 (retried, 2 passed, 1 still failing)
```

### 12. 注册

编译期 hardcoded 在 `cmd/huan/plugins.go`（composition root，与 ADR 0003 §7 一致）：

```go
case "qwen3_translate":
    qCfg, err := qwen3.ParseConfig(raw)
    if err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
    if err := r.Register(qwen3.New(qCfg)); err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
```

`capabilityLabels`（plugin list 输出用）扩展：

```go
if _, ok := p.(translate.Translator); ok {
    labels = append(labels, "translate")
}
```

### 13. Stale 检测（与 ADR 0007 §10.1 联动）

`huan build` 启动时遍历所有 `.en.md`，重算对应中文源 sha256，与 frontmatter `source_hash` 对比：
- mismatch + strict mode（CI 设 `HUAN_STRICT_I18N=true`）→ fail-fast 阻断 build
- mismatch + non-strict mode（本地）→ warn 到 stderr 但继续 build

配合 git pre-commit hook（zhurongshuo 仓库）：扫描 staged 中文 `.md` 文件，对应 `.en.md` 不存在或 hash 不匹配 → 警告（不阻断 commit）。

### 14. v1 不在范围（YAGNI 显式声明）

- **第三种语言**（日文 / 韩文）：等真正需要时再扩
- **翻译记忆库（TM）跨文章复用**：Qwen3-Next 一次翻译足够好，TM 是企业级特性
- **译者协作工作流**（多人 review / approve）：个人博客不需要
- **A/B 测试不同翻译版本**：研究级特性
- **Few-shot 示例自动注入**：v1 用户手工编辑 system prompt 已足够
- **Anthropic Claude provider 实现**：保留接口契约，等真有"质量补强"需求时实施
- **OpenAI / DeepL provider 实现**：同上

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| 首发 provider | Anthropic Claude Sonnet 4.6 API | 用户硬件（M5 Max + 128GB）支持本地 80B MoE；本地零成本 + 数据不外流 + 翻译可重现 |
| 首发模型 | Qwen3-14B（已有）| 实测 80B-A3B 反超速度 + 翻译质量明显提升（"not inherently empty" vs "not pure emptiness"） |
| 首发模型 | Qwen3-235B-A22B | Q4 量化 ~140GB，128GB 内存 + 系统/IDE 占用后不够；thrash 风险高 |
| 首发模型 | DeepSeek-R1-32B | reasoning 模型容易过度思考翻译任务；CoT 难剥离 |
| 首发模型 | Llama 3.3 70B | 中文能力弱于 Qwen3-Next |
| 首发模型 | GLM-4 / GLM-4.5 | Ollama registry 仅 9B（小于 14B）；4.5/4.6 不在标准 library |
| Provider 抽象 | 单一 provider 硬编码（YAGNI 极端）| Translator 接口极小，做抽象边际成本低；翻译场景 provider 差异大（成本/质量/速度/合规），用户换 provider 是高概率事件 |
| 输出格式 | JSON `{title, body}` | LLM escaping 引号/换行易错；XML 标签 raw text 更可靠 |
| 输出格式 | 分离两次 LLM 调用（title + body）| 双倍 API 开销；title 短单独调用质量提升不显著 |
| Tag 翻译 | LLM 自由翻译 | tag 是站点架构（URL/taxonomy/RSS），术语一致性必须强约束；手工字典是标准做法 |
| Glossary | 仅预注入 | 漏译检测靠后验证；预注入仅是预防 |
| Glossary | 仅后验证 | LLM 不知道术语表会自作主张，预防比修复好 |
| 翻译产物存放 | 并行目录树 `content/en/` | Hugo `.en.md` sidecar 是既有约定；文件成对便于 review |
| 翻译产物存放 | 外部缓存目录（不入 git）| CI 无 GPU，路线不通 |
| 增量机制 | 每次重译 | $22 × N deploy 累积成本；source_hash 是无脑增量 |
| 质量检查 | 只做 XML 解析 | 语言检测防 LLM 偷懒返回原文；Markdown 结构防丢列表 |
| 质量检查 | 全部硬阻断 | 软警告通过 retry 解决更宽容；不阻断 deploy |
| 失败处理 | 单文件失败阻断 | collection-not-interruption（与 ADR 0002 一致）；用户重跑只翻失败篇 |
| Observability | 复用 deploy Logger | 已是 cross-cutting 基础设施（ADR 0004 §9 提取）；避免重复造轮子 |

## 影响

### 文档

- 新增本 ADR（0008）
- 引用 [ADR 0007](0007-i18n-build-system.md)（i18n build core）
- 更新 [ADR 0003](0003-unified-plugin-system.md) §1：Translator 加入 capability 清单（与"未来 MultiLanguageProvider"对齐）
- 更新 `docs/INDEX.md`：加 translate 命令索引
- 更新 `README.md` / `README.zh-CN.md`：translate 能力介绍

### 代码（huan）

**新增**：
```
internal/translate/
├── types.go                  # Translator interface + Request/Response/QualityResult
├── types_test.go             # interface conformance tests
└── qwen3/
    ├── plugin.go             # Plugin struct + New() + Name() + Translate()
    ├── client.go             # Ollama HTTP client wrapper
    ├── prompt.go             # System prompt assembly + glossary injection
    ├── quality.go            # Quality check implementations
    ├── parse.go              # XML tag parser
    ├── options.go            # Config struct + ParseConfig
    ├── plugin_test.go
    ├── client_test.go        # with mock Ollama server
    ├── quality_test.go
    └── parse_test.go

cmd/huan/
├── translate_cmd.go          # `huan translate` subcommand tree
└── plugins.go (改造)         # 加 qwen3_translate case + capabilityLabels "translate"
```

**改造**：
- `cmd/huan/plugins.go`：`switch name` 加 `case "qwen3_translate"`，`capabilityLabels` 加 `translate.Translator` 判断

### 代码（zhurongshuo 仓库）

- 新增 `i18n/translate-prompt-zh-en.md`：system prompt（用户可编辑）
- 新增 `i18n/terms.yaml`：手工术语字典（初版前 50-100 高频 tag）
- 新增 `i18n/translation-examples/`（可选，v1 空）：few-shot 示例
- 改造 `huan.yaml`：加 `plugins.qwen3_translate` 块

### 风险

1. **LLM 幻觉**（添加原文没有的内容）：缓解——长度比 check + 抽样人工 review + glossary 强制
2. **Tag 术语表维护拖累作者**：缓解——`huan translate terms --propose` 自动建议；CI 阻断提示
3. **Ollama 不稳定**（崩溃 / hang）：缓解——per-file 失败不阻断 + 重跑只翻失败篇
4. **Qwen3-Next 模型升级后翻译风格变**：缓解——`model_hash` 字段提示；用户决定是否全量重译
5. **首次全量翻译耗时长**（~3 小时 for 1500 篇）：可接受（一次性）；后续增量每篇 ~7s
6. **本地硬件依赖**（M5 Max + 128GB）：缓解——保留 Claude provider 接口，硬件不济时切 API
7. **首次翻译产物 commit 体积大**（1500 文件 × ~2KB = ~3MB markdown）：可接受；可分批 commit 按 section / year
8. **CI 必须设 dummy 模型 endpoint**（实际不调，只验证 build pipeline 不报错）：与现有 deploy 凭证 dummy 同模式（详见 MEMORY.md 经验教训）
