# 完成报告：废弃 qwen3-translate 插件与 Translator 翻译基建

- **日期**：2026-08-31
- **决策**：[ADR 0015](../../adr/0015-deprecate-qwen3-translate.md)
- **范围**：huan 仓库（代码删除）+ zhurongshuo 站点（配置/部署脚本同步清理）

## 修改内容

### huan 仓库（21 个文件删除 + 若干更新）

**删除**：

| 路径 | 内容 |
|---|---|
| `plugins/qwen3/`（17 文件） | qwen3 翻译插件（独立 module `huan-plugin-qwen3`） |
| `cmd/huan/translate_cmd.go` | `huan translate qwen3` 子命令 + sidecar 写入 |
| `cmd/huan/translate_audit.go` | `huan translate audit` |
| `cmd/huan/translate_backfill.go` + test | `huan translate backfill`（source_hash 补齐） |
| `cmd/huan/translate_glossary.go` | 术语字典加载 |
| `cmd/huan/translate_helpers.go` | sha256 / 时间工具 |
| `internal/translate/` | Translator capability 内部别名 |
| `pkg/translate/types.go` | Translator capability 跨 .so 契约 |
| `internal/build/inject_translations.go` | site_translations 注入（被 languages.params 取代） |

**更新**：

- `cmd/huan/main.go`：移除 `newTranslateCmd` 注册
- `internal/daemon/daemon.go`、`cmd/huan/plugins.go`：移除 translate capability 探测
- `internal/config/languages.go`：新增 `LanguageConfig.Params`（subTitle/footerSlogan/keywords/description）
- `internal/build/multisite.go`：新增 `applyLanguageParams`，per-language Params 覆盖移到 BuildMultiSite 克隆处
- `internal/build/pipeline.go`、`i18n_strict.go`：移除 `huan translate qwen3/backfill` 提示语
- `huan.example.yaml`：删除 qwen3_translate 块；languages.en 增加 params 示例
- `scripts/build-plugins.sh`、`internal/plugin/loader.go`、`internal/i18n/langdetect`：注释清理
- 测试：`plugins_test.go` 移除 translator stub；fixture 中 qwen3 字样中性化

### zhurongshuo 站点

- `huan.yaml`：删除 `qwen3_translate` 块（30 行）；`site_translations.en` 迁移到 `languages.en.params`
- `deploy.sh`：10 步收敛为 8 步（删除 step 5 增量翻译、step 8 hash 补齐），移除 `--skip-translate`
- `i18n/translate-prompt-zh-en.md`：删除（LLM system prompt）
- `i18n/terms.yaml`：维护注释改为人工维护
- `~/.huan/plugins/qwen3-translate.so`：删除
- `content/hidden/kachuai/dao/1.en.md`：删除（当天 qwen3 运行残留；hidden section 本被 en 构建排除，其存在导致 hreflang 指向 404）

## 保留（不变）

- 多语言构建全套（BuildMultiSite / sidecar / hreflang / exclude·neutral sections）
- strict i18n 检查（source_hash 比对 + HUAN_STRICT_I18N 门禁）——982 个 .en.md 全部通过
- `internal/i18n/`（collator / langdetect / audit）
- 1028 个已有 `.en.md` sidecar 原样保留，继续参与构建

## 验证

1. `go build ./...` / `go vet ./...` 通过
2. `go test ./...` 31 个包全部 ok
3. zhurongshuo 实际构建：`built 2 languages: zh-cn=1162 pages en=1109 pages`，i18n stale check 982 checked / 0 stale / 0 missing
4. 英文站 Params 注入验证：`docs/en/index.html` subTitle=「FORM NOT VOID, MIND NO CORE」，无中文 params 泄漏
5. 构建产物与 git HEAD byte 级一致（docs/ 无 diff），迁移无行为变化
