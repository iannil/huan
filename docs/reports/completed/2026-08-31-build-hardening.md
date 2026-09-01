# 2026-08-31 构建健壮性批次（v0.7.2 → v0.8.0）完成报告

> 完成日期：2026-09-01（commit c9d3f26，tag v0.8.0）
> 关联：qwen3 移除见 [2026-08-31-deprecate-qwen3-translate.md](2026-08-31-deprecate-qwen3-translate.md) 与 [ADR 0015](../../adr/0015-deprecate-qwen3-translate.md)

## 1. 概述

2026-08-31 晚至 09-01 凌晨，master 上连续落地 6 个 commit，构成 v0.7.2 → v0.8.0 的构建健壮性批次：构建管线四特性（CopyStatic 前缀排除、构建前清理 publish 目录、多语言默认语言先构建、每语言独立 staticExclude/staticRoot）+ zhurongshuo practice/list 模板修复 + 版本号 bump。动机均来自 zhurongshuo 双语生产构建中的实际问题——stale 文件残留、语言目录静态资源串扰、构建顺序不确定。

## 2. 新增依赖

无。

## 3. 新增 / 修改的包

| 路径 | 职责 | 关键文件 | commit |
|---|---|---|---|
| internal/output | CopyStatic 支持前缀排除 | writer.go | 033fdcc |
| internal/config | cleanPublishDir（默认 true）；LanguageConfig 增加 staticExclude/staticRoot | config.go / languages.go | a1ae5c5 / 6bfd2d5 |
| internal/build | 默认语言始终先构建；构建前清理 publish 目录 | multisite.go / pipeline_setup.go / pipeline_write.go | 6015558 / a1ae5c5 / 033fdcc / 6bfd2d5 |
| plugins/zhurongshuo | practice/list 仅匹配根级 introduction | templates/practice/list.html | df2072a |
| internal/version | 版本号 0.7.3 → 0.8.0 | VERSION | c9d3f26 |

## 4. CLI / 配置变更

| 配置 | 默认值 | 说明 |
|---|---|---|
| `cleanPublishDir` | `true` | 构建前清空 publish 目录，消除 stale 产物；`OutputDir == SourceDir` 时自动跳过（daemon 安全护栏）。`huan.example.yaml` 中为顶层配置项（`CleanPublishDir *bool`，`nil` 视为 true） |
| `languages.<code>.staticExclude` | （空） | 该语言的 plain static 拷贝跳过指定 slash 路径前缀 |
| `languages.<code>.staticRoot` | （空） | 将 `static/<staticRoot>/` 子树映射为该语言输出根的本地 static |

`huan.example.yaml` 中的实际示例片段（6bfd2d5）：

```yaml
# 语言级 static 配置（可选）：
#   en:
#     staticExclude: ["en/"]   # 该语言的 plain static 拷贝跳过 static/en/
#     staticRoot: "en"         # static/en/* 映射为该语言输出根的本地 static
```

## 5. 关键设计决策

1. **CopyStatic 以 slash 相对路径前缀做排除（033fdcc）** —— `CopyStatic(srcDir string, excludes []string)`；`nil`/空列表保持原行为（拷贝全部）。为每语言 staticExclude 提供底层原语。
2. **cleanPublishDir 默认开启 + daemon 安全护栏（a1ae5c5）** —— `CleanPublishDir` 原是死代码，本次接入 pipeline setup。默认 true 以消除 stale 产物；`sameDir(OutputDir, SourceDir)` 为真时跳过清理，避免 daemon 场景误删源文件。
3. **默认语言始终先构建（6015558）** —— `SortedLanguages()` 只按 weight/code 排序，不保证默认语言在前；而默认语言构建进 publishDir 根且 cleanPublishDir 会清空该目录，若默认语言不是第一个，会删除其他语言的输出。因此显式将默认语言提升到构建顺序首位（其余保持 weight/code 顺序），与 cleanPublishDir 形成安全组合：后续语言只清理并重建自己的子目录，不会跨语言误删。
4. **staticRoot 映射 + staticExclude 跳过，替代构建后 restore hack（6bfd2d5）** —— plain static 拷贝先排除 `staticRoot + "/"` 前缀与显式 staticExclude 前缀，随后将 `static/<staticRoot>/*` 拷贝到该语言输出根（如 `static/en/llms.txt` → `docs/en/llms.txt`）。修复语言不匹配的 llms.txt 与多余的 `docs/en/en/` 目录，无需构建后恢复脚本。
5. **practice/list 的 introduction 仅匹配根级（df2072a，v0.7.2）** —— 当 practice 根目录与 part-XX 子目录同时存在 introduction.md 时，排序可能使 part 级文件被 `first 1 (where ...)` 选中；改为仅匹配 `File.Dir` 等于 practice 根目录的文件，保证「导论」栏始终显示根级序言。

## 6. 验证

各 commit 均附单元测试（`writer_test.go` +40、`pipeline_setup_test.go` +36、`multisite_test.go` +58 与 +53，合计 +149 行以上），随提交通过。

端到端实测记录（引自 memory/daily/2026-08-31.md 与同日 qwen3 报告的既有记录，本次报告未执行新验证）：

- zhurongshuo 全量构建：`built 2 languages: zh-cn=1162 pages en=1109 pages`
- i18n stale check：982 checked / 0 stale / 0 missing

## 7. 已知限制

- `staticExclude` 为前缀匹配（`strings.HasPrefix`），不做 glob / 精确路径匹配。
- `cleanPublishDir` 清理发生在每次单语言构建的 pipeline setup 中，依赖「默认语言先构建」的顺序保证才不会跨语言误删。

## 8. 后续优化项（不在当前阶段范围）

- staticExclude 支持 glob 模式。
- 文档站（docs/）中各配置项的专题页面。
