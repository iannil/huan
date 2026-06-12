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

**Hugo 输出一致性快照**（带 ±75 文件噪声，详见经验教训）：
- Hugo 总文件数：2029  ·  huan 总文件数：2036
- byte-diff：约 905 完全一致 / 1124 差异（噪声 ±75）
- **新等价标准**：以 [`docs/standards/equivalence.md`](../standards/equivalence.md) 为准，byte-diff 仅作雷达

---

## 当前活跃工作

无。**stage 1 已于 2026-06-12 完成**——三维度等价标准（[ADR 0001](../adr/0001-redefine-equivalence.md)）落地，4 项必修/应修差异（#1 WordCount / #2 RSS items 顺序 / #3 RSS description 截断 / #5 general summary 截断）全部解决，#4 products 换行接受为永久差异。三维度验证管线（`./scripts/diff-build.sh`）建立并 gate 通过。stage 2 待启动（详见下方）。

---

## 已完成 — 原 5 类差异处理结果（2026-06-12 收尾）

5 类差异已按三维度标准全部处理：

1. 字数统计精度 → ✅ 已修（Phase 3 Port Hugo WordCount + Phase 3.5 div float64）
2. RSS items 顺序 → ✅ 已修（Phase 4 sortPagesByDateDesc tiebreaker）
3. RSS description 截断 → ✅ 已修（Phase 5 TruncateHTMLByWords word-boundary + Phase 5.5 TruncateHTMLToBlockBoundary）
4. products summary 换行 → ✅ 接受为永久差异（详见 equivalence.md §4）
5. general summary 截断 → ✅ 已修（同 #3）

---

## Stage 2 候选工作清单（2026-06-12 stage 1 收尾时发现）

stage 1 收尾跑 diff-build.sh 时发现的、超出原 5 类的差异。优先级排序：

1. **meta description / og:description / JSON-LD 多段落 summary 换行压缩**（中优先级，影响 SEO 维度）
   - 现象：huan 把 summary 压成单行，Hugo 保留段落换行
   - 影响：影响约 N 个文件的 SEO 字段对比
   - 方向：在 summary 后处理时保留块级换行
2. **RSS items 数量差**（低优先级，影响 normalized 维度）
   - 现象：huan home RSS 多 1 个 item（11 vs Hugo 10）
   - 方向：检查 RSS limit 边界处理
3. **`lastBuildDate` 格式差**（低优先级）
   - 现象：空日期时 huan 渲染 `0001-01-01`，Hugo 渲染空字符串
   - 方向：在 RSS 模板对零值日期做特殊处理

---

## Stage 2 待新增（架构层）

下列目录与能力属于 stage 2 范围，stage 1 期间**未保留任何占位代码**（避免空目录垃圾）。启动 stage 2 时按 `docs/technical-plan.md` 第 4.11 节定义从零创建。

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
