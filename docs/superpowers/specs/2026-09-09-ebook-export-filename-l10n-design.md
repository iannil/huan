# ebook-exporter 导出文件名随语言本地化 — 设计文档

- 日期：2026-09-09
- 状态：已确认（方案 A）
- 关联：`docs/superpowers/specs/2026-09-02-ebook-exporter-plugin-design.md`

## 背景与目标

当前 `plugins/ebook-exporter` 的输出文件名主干（`baseName`）为 slug：
`<outRoot>/<format>/<kind>/<dirName>/<baseName>[-en].<ext>`，英文版仅追加
`-en` 后缀。目标：中文版文件名用中文书名（`data/<kind>.yaml` 的
`title`），英文版用英文书名（`subtitle`），提升导出产物的可读性。

## 需求决策（已与用户确认）

1. **纯标题**：文件名主干只用书名，不保留 slug。
2. **非法字符**：半角冒号 `:` 转全角 `：`；其余 Windows 非法字符一并清洗。
3. **`-en` 后缀**：默认去掉；仅当英文书名缺失回退到中文书名、导致与
   中文版同名时追加 `-en`。
4. **旧文件**：自动清理。渲染成功新名后删除同单元旧 slug 命名文件。

## 设计（方案 A：输出路径层本地化，manifest 身份不变）

### fileStem(u *unit, lang) string

三类单元（individual / volumes / complete）统一从 `u.agg` 取标题，无需
按类型特判：

- zh → `u.agg.TitleZH`
- en → `u.agg.TitleEN`（individual 来自 `subtitle`；卷/全集为
  `expandUnits` 已合成的聚合标题，如 `Collected Books: Volume 1`）
- en 回退 `TitleZH` 时，若清洗后与 zh 主干相同，则追加 `-en`

### sanitizeFilename(s string) string

- 半角 `:` → 全角 `：`
- 删除 `\ / * ? " < > |` 与控制字符
- 连续空白折叠为单个空格
- 去除首尾空格与句点
- 结果为空 → 回退 `u.baseName`（slug），导出永不因命名失败

### 路径与旧文件清理

- `outPath` 签名改为接收 `*unit`：
  `<outRoot>/<format>/<kind>/<dirName>/<stem>.<ext>`
- 每个文件渲染成功后，尝试删除其旧 slug 路径（`<baseName>[-en].<ext>`，
  由旧命名规则直接推出）；文件不存在则忽略；删除失败仅记入
  `res.Warnings`，不影响导出结果。

### 增量与 manifest

- `manifestKey` 保持 slug 语义（`baseName` 不变），单元身份跨改名稳定。
- 内容哈希前缀 `publication-v3-20260905` → `publication-v4-20260909`，
  强制所有已缓存单元重导一次，以完成改名与旧文件清理。manifest 无需
  迁移。

## 错误处理

- 清洗后主干为空：回退 slug。
- 旧文件删除失败：Warning，不失败。
- 旧名与新名相同（理论上仅在 slug 恰好等于书名时）：删除旧路径前比对
  新路径，相同则跳过删除。

## 测试计划

- `sanitizeFilename` 表驱动测试：冒号转全角、非法字符删除、空白折叠、
  空串回退。
- `fileStem`：zh / en / EN 回退加 `-en`、卷与全集聚合标题。
- `outPath` 新路径格式。
- 旧文件清理：渲染成功后旧 slug 文件被删除；`export_flow_test` 覆盖。

## 非目标（V1 不做）

- `filename_mode` 配置开关。
- 对历史 manifest 的批量迁移或旧哈希清理。
