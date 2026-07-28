# huan-plugin-qwen3

Qwen3 翻译插件，用于通过本地 Ollama 运行的 Qwen3 模型将 Markdown 内容翻译为多语言侧边栏文件（如 `.en.md`）。

## 前提

- Go 1.26.2（必须与 huan 主二进制使用相同的 Go 版本）
- [Ollama](https://ollama.ai/) 运行中，且已拉取 Qwen3 模型
- 系统提示文件（如 `i18n/translate-prompt-zh-en.md`）
- 术语表文件（如 `i18n/terms.yaml`）

## 编译

推荐用统一脚本编译所有插件到发布目录 `release/plugins/`：

```bash
scripts/build-plugins.sh
```

单独编译本插件（产物名须遵循「配置名下划线→连字符」约定，见 `internal/plugin.SoFileName`）：

```bash
cd plugins/qwen3
go build -buildmode=plugin -o ../../release/plugins/qwen3-translate.so .
```

产物：`release/plugins/qwen3-translate.so`（配置名 `qwen3_translate` → 文件名 `qwen3-translate.so`）。

运行时 `huan` 优先在 `$HUAN_HOME`（默认 `~/.huan`）查找插件，其次是 `<项目>/plugins`。

## 配置

在 `huan.yaml` 的 `plugins.qwen3_translate` 下配置：

```yaml
plugins:
  qwen3_translate:
    endpoint: http://localhost:11434
    model: qwen3-next:80b-a3b-instruct-q4_K_M
    fallback_model: ""          # 可选，主模型不可用时回退
    timeout_seconds: 120
    system_prompt_file: i18n/translate-prompt-zh-en.md
    glossary_file: i18n/terms.yaml
    quality:
      length_ratio_min: 0.5
      length_ratio_max: 3.5
      target_language_threshold: 0.8
      markdown_structure_tolerance: 1
      chunk_context_token_budget: 8000
      enforce_glossary: true
      retry_on_violation: 1
      max_residual_cjk: 0
    site_translations:  # 可选，站点元数据翻译
      en:
        subTitle: "English Subtitle"
        description: "Site description in English"
        keywords: ["keyword1", "keyword2"]
        footerSlogan: "Footer slogan in English"
```

插件配置通过 `${VAR}` 环境变量插值，由 huan 的 config 层处理。

## 使用

```bash
# 翻译整个站点
huan translate qwen3

# 仅翻译指定语言对
huan translate qwen3 --source-lang zh-cn --target-lang en

# 翻译特定文件
huan translate qwen3 content/posts/my-post.md

# 指定自定义插件目录
huan translate qwen3 --plugin-dir ./plugins
```

## 能力

该插件实现 `translate.Translator` 接口，翻译流程：

1. 按二级标题（`##`）拆分源内容为片段
2. 对每个片段（按文档顺序）：
   - 构建带滑动窗口上下文的提示
   - 调用 Ollama 的 Qwen3 模型
   - 逐片段质量检查（格式纯度、语言检测、结构一致性、长度比、术语合规）
   - 软检失败时自动重试（可配置次数）
3. 拼接翻译后的片段为最终正文
4. 输出 `.en.md` 侧边栏文件

## 质量检查

| 检查项 | 类型 | 说明 |
|--------|------|------|
| `xml_parse` | 硬检 | LLM 输出是否为合法的 `<title><body>` XML |
| `language_detection` | 硬检 | 输出是否 ≥80% 为目标语言 |
| `markdown_structure` | 硬检 | 标题/列表/图片数量是否与源匹配 |
| `format_purity` | 硬检 | 输出是否不含 HTML 块级标签 |
| `length_ratio` | 软检 | 输出字符数 / 源字符数是否在 [0.5, 3.5] |
| `glossary_compliance` | 软检 | 术语表中的词汇是否被正确翻译 |
| `residual_cjk` | 软检 | 输出中残留的中日韩字符数是否在限制内 |

## 本地开发

插件仓库是自包含的，复制了 huan 的 `translate`、`plugin`、`observability`、`i18n/langdetect` 接口类型。

当这些接口变更时，需要手动同步 `plugins/qwen3/translate/`、`plugins/qwen3/plugin/`、`plugins/qwen3/observability/`、`plugins/qwen3/i18n/` 下的文件。

## 依赖

- `gopkg.in/yaml.v3` — 配置解析