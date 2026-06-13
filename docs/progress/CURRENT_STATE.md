# 当前实际进展

> 最后更新：2026-06-13（grill-me 审计后更新） · 分支：master
> 本文档替代 `docs/technical-plan.md` 第 8.4 节"剩余差异"——后者作为冻结的设计参考，**实际最新状态以此文档为准**。

## Stage 3 — grill-me 完成度确认（2026-06-13，最新）

按用户要求"对 100% 还原进行完成度确认（肉眼 / SEO / AI 三维度）"启动 grill-me 流程。**结果颠覆了 stage 2 主要完成 的结论**——之前的 baseline 一开始就 broken。

### 三维度最终状态 ✅

| 维度 | 数字 | 状态 |
|---|---|---|
| **SEO** | **0 differing** | ✅ **PASS** |
| **AI** | **0 differing** | ✅ **PASS** |
| normalized | 6 differing | 4 个 chroma lexer 版本差 + 2 个非可见 artifact |
| byte | 6 differing | 同上 |
| Files only in Hugo / huan | 0 / 0 | ✅ |
| Identical files | **2026** / 2032 | 99.7% |

剩余 6 个 diff 全部是 chroma v2.26.1（huan）vs Hugo bundled chroma 版本差，或非可见 artifact（products/index.xml RSS 描述缩进、sitemap.xml URL 排序）。**肉眼 / SEO / AI 三维度全部无差异**，详见 [equivalence.md §4](../standards/equivalence.md#4-接受为永久差异的项)。

### Stage 3 关键发现 + 修复

#### 发现 1：zhurongshuo 本地 layout 被截断 → stage 1/2 数字全失真

- `layouts/_default/single.html` working tree 被截断到 12 行（HEAD 是 67 行完整的）
- 导致本地 Hugo 跑出来 96% 页面缺 body，huan 读同样 layout 也缺 body
- 之前所有 stage 1/2 的 diff 数字（"905 same" 等）都是基于 broken baseline 的"假性相同"
- 修复：`git checkout HEAD -- layouts/_default/single.html`（在 zhurongshuo 仓库）
- **教训**：任何对比工作开始前必须实证 baseline 自身正确（对比线上 / HEAD / 已知 good 状态）

#### 发现 2：4 个 slug collision 全部是源内容 typo

- `posts/2025/02/2203.md`（slug=2202，应为 2203）
- `posts/2023/04/1402.md`（slug=1401，应为 1402）
- `practices/season-5/.../epilogue.md`（slug=introduction，应为 epilogue）
- `practices/season-4/the-transformation-of-traffic-stations/chapter-07.md`（slug=chapter-06，应为 chapter-07）

修法：改源 content frontmatter，不是 port Hugo 的 collision resolution（Hugo 是"默默挑一个不报错"，没有公开算法）。

#### Stage 3 完成的工作

| 任务 | 文件 | 备注 |
|---|---|---|
| B1/B2 slug collision（4 文件） | zhurongshuo content | 改源 frontmatter |
| C entity encoding 双逃逸 bug | `internal/template/funcs.go::plainify` | 改返回 `template.HTML` 避免 Go template auto-escape |
| A chroma port 到 huan | `internal/markdown/renderer.go` + go.mod | 加 `github.com/alecthomas/chroma/v2`，自定义 goldmark NodeRenderer |
| search.json 生成（额外发现） | `internal/template/funcs.go::replaceREFunc` | 改签名为 `interface{}` 以接受 template.HTML |
| E1 canonify 跳过 code/pre | `internal/output/canonify.go::applyCanonifyOutsideCode` | regex split 区段 skip |
| E2 hugoSlugify 边界修复 + emoji 短码 | `internal/markdown/renderer.go` | `_` 保留 / Unicode letter 保留 / 不 Trim trailing dash / 多行 heading 取最后段 / emoji 短码跳 code 区 |

### 待办（commit + 推送）

1. **zhurongshuo 仓库**：commit 5 个源文件改动（4 slug fix + CLAUDE.md）—— layout 已 restored to HEAD 不在 modified 列表
2. **huan 仓库**：commit 7 个文件改动（chroma + plainify + replaceRE + canonify + renderer + tests）

### 历史

stage 1/2 的 phase-by-phase 进度（包括 phase 5d/e/f 等细节）保留在下方作为历史记录，但其"stage 2 主要等价工作完成"的结论**已被 stage 3 推翻**——基于 broken baseline 的完成度判定无效。stage 3 才是真正的完成度确认。

---

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
| 10. chroma 语法高亮 port | ✅ | `internal/markdown/renderer.go`（stage 3） |
| 11. 三维度 PASS（SEO + AI 全 0 diff） | ✅ | stage 3 grill-me 完成度确认 |

---

## 历史记录（stage 1/2，已被 stage 3 推翻）

> ⚠️ 以下"stage 2 主要等价工作完成"等结论基于 broken baseline，实际完成度以 stage 3 为准。

### Stage 1 已完成（2026-06-12）

stage 1 收尾时三维度等价标准（[ADR 0001](../adr/0001-redefine-equivalence.md)）落地，4 项必修/应修差异（#1 WordCount / #2 RSS items 顺序 / #3 RSS description 截断 / #5 general summary 截断）全部解决，#4 products 换行接受为永久差异。三维度验证管线建立并 gate 通过。

### Stage 2 历史 phase 进度

stage 2 各 phase 的具体进展详见下方原记录。stage 3 grill-me 后重新审视，发现这些 phase 大多是在修真问题但被 broken baseline 掩盖了实际效果——直到 layout 修复后才暴露真实差异规模。

> **修订记录（2026-06-12 stage 2 phase 2 启动前调查）**：原候选 #2「RSS 中文 URL 编码（464 文件）」全量复核后发现是误判——实际 0 个文件有单纯 URL 编码差。204 个 RSS differing 文件分类：187 个 items 顺序差（中文排序根因）+ 17 个 items 内容差（独立问题）。原 #3 books part 顺序与 items 顺序同源，合并为新 #2/#3。

### Stage 2 进度

| Phase | 项 | 状态 | 完成日期 |
|---|---|---|---|
| 1 | meta description plainify | ✅ 已完成 | 2026-06-12 |
| 2 | RSS items 顺序（中文排序） | ✅ 已完成 | 2026-06-13 |
| 3 | books section 顺序（同 #2） | ✅ 已完成（与 #2 合并） | 2026-06-13 |
| 4 | RSS items 内容差（3 子项） | ✅ 已完成 | 2026-06-13 |
| 7 | CJK URL 编码（term RSS） | ✅ 已完成 | 2026-06-13 |
| 8 | 空 tag RSS 未生成 | ✅ 已完成 | 2026-06-13 |
| 5 | body 渲染细节（5d/5e/5f） | ✅ 已完成 | 2026-06-13 |
| **6** | **minify artifacts（chroma）** | **⚪ 接受为永久差异** | 2026-06-13 |

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

5. **body 渲染细节**（原 ~30 文件）→ ✅ **已修（stage 2 phase 5d/5e/5f，2026-06-13）**
   - **5d WordCount 差 100 字**（原估 81 实际 1 文件）：根因是 huan 缺 footnote 渲染（appendix 少 ~106 字）+ 代码块内 `&quot;` vs `&#34;` entity 编码差异。修复：goldmark 加 Footnote extension；新增 `normalizeCodeEntities` 后处理（限 code/pre 内部）
   - **5e list page part 顺序差**（86 文件）：根因是 huan `sort` 模板函数无 field 参数时不排序（缺 else 分支），导致 `sort ($scratch.Get "partSlugs")` 返回 slice 不变。修复：`sortFunc` 重构为 `keyOf` closure（无 field 时 identity，有 field 时 field-extractor），共享 stable insertion sort
   - **5f-link-text**（3 文件）：根因是 `GroupByDate` 组内未排序，same-Date slug-collided posts 渲染顺序与 Hugo 不一致。修复：`GroupByDate` 组内按 Date desc → Title desc (collator) → File.Path desc 排序
   - **5f-home-rss**（1 文件）：根因是 `TruncateHTMLToBlockBoundary` 是 Hugo `ExtractSummaryFromHTML` 的近似 Port，漏掉 3 个 quirk：(a) 段尾 word 算法是 `s[wi:i]` 其中 `i` 是最后 rune 的 byte offset（每段少算 1 字）；(b) `</p>` 之间的 `<h2>` 等中间 block tag 算到下一段 word count；(c) HTML-tag-shaped tokens 算 0 word。修复：byte-faithful Port，新增 stateful `stripHTMLTagsInWord` helper
6. **minify artifacts**（原估 ~30 实际 17 文件）→ **接受为永久差异**
   - 根因：products 代码块用 chroma 语法高亮（`<div class=highlight><pre class=chroma>`），huan 当前用纯 goldmark `<pre><code>`，引入 chroma 库成本高
   - 决策：登记到 `docs/standards/equivalence.md` §4 永久差异表
   - 三维度影响：肉眼不可见（HTML 渲染等价），SEO/AI 不可见

---

## Stage 2 候选工作清单（2026-06-13 phase 3 后更新）

phase 3 实施过程中发现的新差异（不在原 5 类中）：

7. **tags/{cjk}/index.xml link/guid URL 编码**（原影响 ~150 文件）→ ✅ **已修（stage 2 phase 4，2026-06-13）**
   - 根因：URLEscape 同时用于文件路径（CJK 保留）和 URL（需 percent-encode），未区分
   - 修复：新增 `URLEscapeForURL` helper（percent-encode CJK + 其他非 ASCII），用于 BuildTermContext 的 permalink；URLEscape 仍用于文件路径
   - 验证：tags/专注/index.xml link byte-identical Hugo；~150 个 term RSS 文件全部对齐

8. **空 tag RSS 未生成**（原影响 22 文件）→ ✅ **已修（stage 2 phase 4，2026-06-13）**
   - 根因：phase 3a 过滤 hidden 后某些 tag 的 pages 全空，但 taxonomy map 用 site.RegularPages 构建 → 空 tag 不在 map 里 → 不生成 RSS
   - 修复：新增 `BuildAllWithOriginalCase(site.Pages)`，从 site.Pages 构建 taxonomy（hidden pages 仍在 map keys 里，只是 Pages 空）；build.go 生成所有 tag 的 RSS（即使 items 空）
   - 同时修复 4 个相邻问题（必需才能 byte-match）：(a) TaxonomyOriginalCase 保留 tag 原始大小写（FANFAN vs fanfan）；(b) `lastBuildDate` 空 RegularPages 时输出空（vs 零时间字符串）；(c) term page Section 字段设为 "tags"；(d) SortDefault（Hugo DefaultPageSort Port）保证 term items 顺序

### Stage 2 重大里程碑（2026-06-13 phase 4 后）

`./scripts/diff-build.sh` **byte mode 首次 PASS**：1815 identical / 170 differing（之前 976/1031）。这是 stage 1 收尾以来 byte-diff 首次低于 normalized-diff 阈值，意味着 huan 输出在 byte 维度已与 Hugo 整体一致（剩余 170 多个文件分布在 normalized/seo/ai 维度）。

剩余差异类型（待 stage 2 phase 5+ 处理）：
- 13 个 tag HTML list page：same-date items 排序差（Phase 5 候选）
- ~30 practices/books chapter body 内容渲染细节
- ~30 minify artifacts（attribute 引号、entity 编码）

---

## Stage 2 候选工作清单（2026-06-13 phase 5d/e/f 后更新）

phase 5d/e/f 实施过程中发现 / 解决的差异，以及剩余接受为永久差异的项：

9. **sitemap.xml items 顺序 + lastmod**（1 文件）→ **接受为永久差异**
   - 影响：SEO 微小
10. **robots.txt**（1 文件）→ **接受为永久差异**
11. **search.json**（1 文件）→ **接受为永久差异**
12. **practices description entity encoding 边缘 case**（1 文件）→ **接受为永久差异**
    - chapter-07 of data-as-the-boundary 的 meta description entity encoding 差，与 chroma 类似根因

### Stage 2 重大里程碑（2026-06-13 phase 5d/e/f 后）

**byte mode 持续 PASS**：1965 identical / 20 differing。剩余 20 个全部登记为永久差异（chroma 17 + entity 边缘 case + search.json + sitemap.xml + robots.txt）。**stage 2 主要等价工作完成**——所有肉眼可见 / SEO 主要 / AI 主要差异全部解决。

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
