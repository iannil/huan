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

三维度等价标准落地（ADR 0001）：详见 [`docs/superpowers/plans/2026-06-12-redefine-equivalence.md`](../superpowers/plans/2026-06-12-redefine-equivalence.md)。

---

## 待办 — 剩余差异（按三维度归类，2026-06-12 重新评估）

详见 [ADR 0001](../adr/0001-redefine-equivalence.md) 与 [`docs/standards/equivalence.md`](../standards/equivalence.md)。

1. **字数统计精度** → **必修**（books/practices 列表页「约 X 万字」肉眼可见，huan 12.0 vs Hugo 15.5）
   - 处理：Port Hugo WordCount 算法（覆盖 `unicode.Is(unicode.Han/Hiragana/Katakana/Hangul)` + 全角符号）
   - 影响：`internal/build/summary.go:CountWordsInPlain`

2. **RSS items 顺序** → **应修**
   - 处理：`sortPagesByDateDesc` 加 tiebreaker（date desc → lower(title) asc → relpath asc）
   - 影响：`internal/content/tree.go:311`

3. **RSS item description 截断** → **应修**
   - 处理：`TruncateHTMLByWords` 改为 word-boundary 截断
   - 影响：`internal/build/summary.go:9`

4. **products page description 换行** → **接受**为永久差异
   - 原因：`</h2>\n<p>` 与 `</h2> <p>` 浏览器渲染等价
   - 已登记于 `docs/standards/equivalence.md` §4

5. **general page summary 截断位置** → **应修**
   - 处理：与 (3) 在 `TruncateHTMLByWords` 统一
   - 影响：`internal/build/summary.go:9`

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
