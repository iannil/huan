# 当前实际进展

> 最后更新：2026-06-12  ·  分支：master
> 本文档替代 `docs/technical-plan.md` 第 8.4 节"剩余差异"——后者作为冻结的设计参考，**实际最新状态以此文档为准**。

## 阶段一进度总览

| 里程碑 | 状态 | 备注 |
|---|---|---|
| 1. 项目骨架 + 配置系统 | ✅ | `cmd/huan` + `internal/config` |
| 2. 内容加载 + Markdown 渲染 | ✅ | `internal/content` + `internal/markdown` (goldmark) |
| 3. 模板系统 | ✅ | `internal/template`，自定义函数注册完整 |
| 4. Shortcode + 加密系统 | ✅ | `internal/shortcode` (redact/audio/img) + `internal/encrypt` |
| 5. 列表页 + Taxonomy + 分页 | ✅ | `internal/taxonomy` + `internal/pagination` |
| 6. 辅助输出（RSS/sitemap/search.json） | ✅ | `internal/output` |
| 7. Minify + 输出优化 | ✅ | `internal/output/minify.go` |
| 8. 验证 + 修正（Hugo diff 管线） | ✅ | `scripts/diff-*.sh` |
| 9. 开发服务器（serve） | ✅ | `internal/serve`（17 commits，2026-06-12 完成） |

**Hugo 输出一致性快照**（来自 `technical-plan.md` 第 8.2 节，作为基线参考）：
- Hugo 总文件数：2029  ·  huan 总文件数：2036
- 共有文件：2028  ·  完全一致：905（44.5%）
- 仅 Hugo（缺失）：0  ·  仅 huan（多余）：8
- 内容差异：1124（含上方"剩余差异"5 类的批量影响）

---

## 当前活跃工作

无。`huan serve` 已于 2026-06-12 收尾，下一步方向待用户决定（见下方"待办"）。

---

## 待办 — 剩余 Hugo 兼容差异

这 5 类是 stage 1 阶段允许保留的边缘差异（详见 `docs/technical-plan.md` 第 8.4 节）。修复任一项需配套新增 diff 用例。

1. **字数统计精度**
   - 现象：Hugo 用专门 CJK word segmenter（基于 dictionary），huan 用简单字符计数，差距约 25%
   - 影响范围：列表页 `WordCount` / `ReadingTime` / 摘要的 FuzzySummary 字段
   - 建议方向：引入分词库或对齐 Hugo 算法；先评估收益（这些字段是否影响 zhurongshuo 实际页面）

2. **RSS items 顺序**
   - 现象：Hugo 内部多字段排序（date desc → LinkTitle asc → path asc），date 相同时顺序不稳定
   - 影响范围：所有 RSS 文件
   - 建议方向：在 `internal/output` 的 RSS 生成路径补一个稳定 tiebreaker

3. **RSS item description 截断**
   - 现象：Hugo summary 在 word 边界截断，huan 在 `</p>` 边界截断
   - 影响范围：RSS 文件
   - 建议方向：复用 `internal/build/summary.go` 的 word-boundary 逻辑

4. **products page description 换行**
   - 现象：summary 中 block-level 换行（`</h2>\n<p>`）Hugo 转为空格，huan 保留换行
   - 影响范围：products 列表页
   - 建议方向：在 summary 后处理增加 block-level whitespace 折叠

5. **general page summary 截断位置**
   - 现象：summary 截断位置与 Hugo 略有不同
   - 影响范围：general section 页面
   - 建议方向：与 (3)(4) 一起在 summary 后处理中统一对齐

---

## Stage 2 待新增（当前为空）

下列目录与能力属于 stage 2 范围，stage 1 期间**未保留任何占位代码**（避免空目录垃圾）。启动 stage 2 时按 `docs/technical-plan.md` 第 4.11 节定义从零创建。

| 路径 / 能力 | 用途 | 接口参考 |
|---|---|---|
| `internal/pipeline/` | 构建管线编排（如需插件化重构） | technical-plan §4.8 |
| `internal/plugin/` | 插件接口（AuthPlugin/PaymentPlugin/MemberPlugin/DynamicRenderPlugin 等） | technical-plan §4.11 |
| `internal/search/` | 服务端全文搜索 | technical-plan §4.10 |
| `pkg/` | 可导出的公共库（阶段二用） | — |

---

## 仓库整洁度自检（2026-06-12）

| 项 | 结论 |
|---|---|
| `.gitignore` 排除 `/huan` 二进制 | ✅ |
| 代码内 `TODO/FIXME/XXX/HACK` 标记 | 0 个 |
| `backup/` `tmp/` `*.bak` `*_old.go` | 无 |
| `Makefile` / `.github/` CI | 无（暂不需要） |
| `scripts/diff-*.sh` | 4 份均在用 |
| `cmd/huan/main.go` | 67 行（< 80，符合 serve plan 验收） |
