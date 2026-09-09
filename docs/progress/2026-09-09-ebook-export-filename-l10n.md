# ebook-exporter 导出文件名随语言本地化 — 进展

- 日期：2026-09-09
- 状态：已完成开发，待验收
- 规格：`docs/superpowers/specs/2026-09-09-ebook-export-filename-l10n-design.md`
- 计划：`docs/superpowers/plans/2026-09-09-ebook-export-filename-l10n.md`

## 修改内容

- 新增 `plugins/ebook-exporter/filename.go`：`sanitizeFilename`（非法字符清洗，半角冒号转全角）与 `fileStem`（按语言取书名主干，EN 回退时加 `-en`）。
- `outPath` 改用 `fileStem`，输出文件名变为语言对应的书名；`legacyPath` 保留旧 slug 规则。
- 内容哈希前缀升级 `publication-v3-20260905` → `publication-v4-20260909`，全量重导一次完成改名。
- 渲染成功后自动删除旧 slug 命名文件；删除失败仅记 Warning。

## 验证

- `cd plugins/ebook-exporter && go test ./...` 全部通过（需 CJK 字体的集成测试在无字体环境 SKIP）。
- 集成测试 `TestExportRenamesSlugArtifacts` 覆盖：旧文件清理 + 新名落盘。

## 待办

- 在 zhurongshuo 仓库实际执行 `huan export ebook --type all --format all --level all` 验收产物文件名。
