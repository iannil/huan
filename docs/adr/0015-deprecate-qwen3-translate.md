# ADR 0015：废弃 qwen3-translate 插件与 Translator 翻译基建

- **状态**：Accepted
- **日期**：2026-08-31
- **决策者**：用户（owner）+ Claude
- **依赖**：[ADR 0008](0008-translator-capability-qwen3-plugin.md)（被本 ADR 废弃）、[ADR 0007](0007-i18n-build-system.md)（i18n build core 保留）
- **关联**：[ADR 0009](0009-self-contained-downstream-deploys.md)（自包含插件部署）

## 背景

[ADR 0008](0008-translator-capability-qwen3-plugin.md) 定义了 Translator capability 并以 qwen3 插件（本地 Ollama LLM 翻译）作为唯一实现。运行两个多月后，用户决定废弃该插件与整套翻译基建：

- 翻译不再由 huan 内置工具链驱动（后续翻译流程移至 huan 之外，`.en.md` sidecar 作为静态输入继续参与构建，见 [ADR 0007](0007-i18n-build-system.md)）
- qwen3 是 Translator capability 的唯一实现方，删除实现后保留 capability 接口没有意义

## 决策

### 1. 完全删除（非 deprecation 标记）

- 删除 `plugins/qwen3/`（独立 Go module `huan-plugin-qwen3`）
- 删除 `huan translate` 命令树（qwen3 / status / audit / backfill 子命令）
- 删除 `internal/translate/`、`pkg/translate/`（Translator capability 契约）
- 删除 daemon / plugin list 中的 `translate` capability 探测
- 删除 zhurongshuo 的 `i18n/translate-prompt-zh-en.md`（LLM system prompt）与 `~/.huan/plugins/qwen3-translate.so`

### 2. site_translations 迁移到 `languages.<code>.params`

原 `plugins.qwen3_translate.site_translations` 承载"英文站点元数据"（subTitle / description / keywords / footerSlogan），与翻译无关，属多语言构建配置。迁移为通用配置：

```yaml
languages:
  en:
    weight: 2
    baseURL: /en
    params:              # 空字段回退顶层 params（部分覆盖语义与原行为一致）
      subTitle: ...
      description: ...
      keywords: [...]
      footerSlogan: ...
```

实现：`internal/config/languages.go` 新增 `LanguageParamsConfig`；`internal/build/multisite.go::applyLanguageParams` 在克隆 master cfg 时覆盖 `Params`（替代原 `pipeline.go::injectSiteTranslations`）。

### 3. i18n 构建侧保留（不变）

以下能力与 LLM 翻译无关，全部保留：

- 多语言构建（`BuildMultiSite`、sidecar 加载、hreflang、语言切换）
- strict 模式 stale 检查（`source_hash` 比对，`HUAN_STRICT_I18N=true`）——已有的 1028 个 `.en.md` sidecar 继续受其保护
- `internal/i18n/`（collator、langdetect、audit 包）
- `i18n/terms.yaml` 术语字典（维护方式注释更新为人工维护）

### 4. 部署脚本同步收敛

zhurongshuo `deploy.sh` 从 10 步收敛为 8 步：移除 step 5（`huan translate qwen3`）与 step 8（`huan translate backfill`），同时移除 `--skip-translate` 参数。strict i18n 构建门禁保留。

## 影响

- 下游站点（zhurongshuo）需同步迁移配置：`site_translations` → `languages.en.params`，验证英文站 Params 注入与中文 params 无泄漏（本次已验证 byte 级一致）
- 后续翻译工作流由用户在 huan 之外自行选择工具；`.en.md` 的新增/更新需人工维护 `source_hash`（或容忍 strict 模式报错提示）
- 未来若重新引入 LLM 翻译，应参考本 ADR 与 ADR 0008 的教训重新设计，而非恢复已删代码（git 历史可查）
