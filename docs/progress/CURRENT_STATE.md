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

## Stage 2 候选工作清单（2026-06-12 stage 1 收尾 + grill-me 复核修订）

stage 1 收尾跑 diff-build.sh 时发现的差异。原 3 项遗留经 grill-me 全量复核后修订为本清单。

> **修订记录（2026-06-12 grill-me 复核）**：原清单 3 项里 #1「meta description 换行压缩」方向描述反了（实际是 huan 多行、Hugo 折叠），#2「RSS items 数量差」与 #3「lastBuildDate 格式差」均不存在（前者是 grep 命令误用、后者实证 byte-identical）。下列为全量调查（1265 个 differing .html/.xml 文件）归纳的真实差异。

> **修订记录（2026-06-12 stage 2 phase 2 启动前调查）**：原候选 #2「RSS 中文 URL 编码（464 文件）」全量复核后发现是误判——实际 0 个文件有单纯 URL 编码差。204 个 RSS differing 文件分类：187 个 items 顺序差（中文排序根因）+ 17 个 items 内容差（独立问题）。原 #3 books part 顺序与 items 顺序同源，合并为新 #2/#3。

### Stage 2 进度

| Phase | 项 | 状态 | 完成日期 |
|---|---|---|---|
| 1 | meta description plainify | ✅ 已完成 | 2026-06-12 |
| 2 | RSS items 顺序（中文排序） | ✅ 已完成 | 2026-06-13 |
| 3 | books section 顺序（同 #2） | ✅ 已完成（与 #2 合并） | 2026-06-13 |
| **4** | **RSS items 内容差**（3 子项） | **✅ 已完成** | 2026-06-13 |
| 5 | body 渲染细节 | 待启动 | — |
| 6 | minify artifacts | 待启动 | — |

---

1. **meta description 多行换行**（原影响 565 文件）→ ✅ **已修（stage 2 phase 1，2026-06-12）**
   - 根因（修正后）：`internal/template/funcs.go:28` 的 `plainify` 是 `stripTags(toString(v))`，未实现 Hugo `tpl/template.go:StripHTML` 的完整算法（placeholder 保留 `</p>` / `<br>` 边界 + 连续 whitespace 去重）
   - 修复：plainify 提取为 named function，Port Hugo 完整 StripHTML 算法（pre-replacer + stripTags + placeholder 还原 + unicode.IsSpace 去重）；不 trim、不 collapseWhitespace
   - **教训**：第一轮修复（collapseWhitespace + TrimSpace）方向反了——实证发现 Hugo 实际保留 `\n`（来自 `</p>` placeholder），不是折叠为单空格。reset 后用 Hugo 源码（`tpl/template.go`）Port 完整算法才 byte-match
   - 验证：3 篇典型页面（general/practices/books）meta name=description / og:description 全部 byte-identical；diff-build.sh 4 模式都下降（seo 983 → 699，ai 323 → 36）

2. **RSS items 顺序差**（原影响 187 文件）→ ✅ **已修（stage 2 phase 2，2026-06-12~13）**
   - 根因：huan 用 `strings.ToLower(Title)` 字节级 UTF-8 比较，Hugo 用 `golang.org/x/text/collate` 库做 locale-aware 排序（zh-cn = 拼音序）
   - 修复：Port Hugo `resources/page/pages_sort.go:DefaultPageSort` 完整链（Weight / Date desc / Collator Title asc / Path asc）；引入 `golang.org/x/text` 依赖；`internal/i18n` 加 `BuildCollator(langCode)`；`build.go:173` 改为传 `site.RegularPages` 给 `taxonomy.BuildAll`（让 tags RSS 也走排序后的 pages）
   - 验证：books/volume-3 RSS items 顺序 byte-identical Hugo；home RSS 全 title 顺序 byte-match；3 个抽样 books RSS byte-match；tags/道 等 3 个抽样 tags RSS byte-match

3. **books section 顺序差**（原影响 104 文件）→ ✅ **已修（与 #2 同根因，stage 2 phase 2 一并修复）**
   - 与 #2 共享修复：Port Hugo DefaultPageSort 后，list page section 顺序 + chapter 顺序 + RSS items 顺序全部对齐

4. **RSS items 内容差**（原影响 17 文件）→ ✅ **已修（stage 2 phase 3a/3b/3c，2026-06-13）**
   - 拆分为 3 个子项，分别修复：
   - **3a (hidden 过滤)**：zhurongshuo `hidden/_index.md` 用 `cascade.build.list: never`，huan 解析了 cascade 但没在 site.RegularPages 过滤。修复：实现 Hugo-style cascade inheritance + 在 BuildTree 过滤 `Build.List == "never"`
   - **3b (posts RSS 缺 items)**：zhurongshuo `posts/` 按 year 嵌套且无 `_index.md`，huan auto-create section 时机太晚 + section.RegularPages 只含直接子。修复：auto-create 提前到 parent 分配前 + section context 的 `.RegularPages` 用 `RegularPagesRecursive`
   - **3c (tags/index.xml 内容定义)**：Hugo 的 taxonomy list RSS 列 term stubs，huan 错误列了 site.RegularPages；同时 auto-create section title 用 dir name，Hugo title-cases。修复：BuildTaxonomyContext 构建 term stubs + 接入 site collator 排序 + CJK permalink percent-encode；新增 `makeSectionTitle` helper（`cases.Title(language.English)` + 连字符转空格）
   - 验证：hidden/posts/tags index.xml 全部 byte-identical Hugo

5. **body 内容渲染细节**（影响 ~30 文件）
   - 现象：少量 practices/books 章节 body HTML 有细微差异（`<p>` / `<h2>` / `<h3>` / `<li>` / `<code>` 标签的属性或内容）
   - 根因：待逐个调查（可能是 goldmark 配置、shortcode 输出、HTML 转义）
   - 修复方向：每篇差异单独定位
   - 三维度影响：肉眼可能可见，SEO/AI minor

6. **minify artifacts**（影响 ~30 文件）
   - 现象：attribute 引号风格、void 元素自闭合形式、entity 编码差异
   - 根因：huan minify 与 Hugo minify 算法不完全一致
   - 修复方向：升级 diff-build.sh Step 5 的 normalized 模式做更激进的 normalize（吸收这些差异），或对齐 minify 行为
   - 三维度影响：肉眼不可见，SEO/AI 不可见（purely byte-level）

---

## Stage 2 候选工作清单（2026-06-13 phase 3 后更新）

phase 3 实施过程中发现的新差异（不在原 5 类中）：

7. **tags/{cjk}/index.xml link/guid URL 编码**（影响 ~150 个 term RSS 文件）：huan 输出原始 CJK（如 `tags/专注/`），Hugo 输出 percent-encoded（`tags/%E4%B8%93%E6%B3%A8/`）。这是 stage 1 收尾报告里"RSS 中文 URL 编码（464 文件）"的**真正根因**——之前误判不存在，实际存在于单个 term RSS（而非 tags/index.xml）。
8. **空 tag RSS 文件未生成**（影响 ~11 个 tags）：Hugo 为空 tag 生成空 RSS 文件，huan 不生成。`/tmp/huan-output` 缺 22 个 `.xml` 文件。

---

## Stage 2 待新增（架构层）

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
