# 当前实际进展

> 最后更新：2026-06-30（v0.5.0 发版，6 hard gate 全交付，等 gate 6 90 天稳定性） · 分支：master · 当前版本：**v0.5.0**
> 本文档替代 `docs/technical-plan.md` 第 8.4 节"剩余差异"——后者作为冻结的设计参考，**实际最新状态以此文档为准**。

## v1.0 Release Tracking（2026-06-30 新增）

**定位**：v0.x = local-first single-user content engine with built-in admin；v1.x+ = path toward all-CMS replacement（详见 [ADR 0010](../adr/0010-v1-0-scope-and-positioning-split.md)）。

**6 Hard Gate**（详见 [ADR 0010](../adr/0010-v1-0-scope-and-positioning-split.md) §2）：

| # | 标准 | 状态 | 交付版本 |
|---|------|------|---------|
| 1 | 文档定位一致（撤回"替代所有 CMS"过度宣称） | ✅ 完成（`00a92dc`） | v0.5.0 |
| 2 | 无静默 no-op 模板函数（三档策略） | ✅ 完成（`a5cc31b`） | v0.5.0 |
| 3 | I/O 包（admin/output/i18n）有测试 | ✅ 完成（`b11bb23` `94b5b75`） | v0.5.0 |
| 4 | Admin 安全边界（[ADR 0011](../adr/0011-admin-security-boundary.md)，L1+L2+L4） | ✅ 完成（`5fb2d56`） | v0.5.0 |
| 5 | BuildSite 拆 ≤80 行 stage（纯抽取 6 文件） | ✅ 完成（`2f490ed`） | v0.5.0 |
| 6 | zhurongshuo 生产稳定 90 天 + 自己满意 | ⏳ 等待期 2026-06-13 → **2026-09-11**（已 17 天） | v1.0.0 |

**版本节奏**：v0.5.0 交付 gate 1-5（2026-06-30）→ 等 73 天 → **v1.0.0（2026-09-11 后）**。

详细 release notes 详见 [`docs/reports/completed/2026-06-30-v0.5.0-v1-0-gates.md`](../reports/completed/2026-06-30-v0.5.0-v1-0-gates.md)。

---

## v0.2.x 系列（2026-06-13 夜 ~ 今）

v0.1.0 发版后连续多个版本，详见 [ADR 0005](../adr/0005-remove-encrypt-and-v02-feature-batch.md)、[ADR 0006](../adr/0006-remove-encryptgroups-dead-config.md) 与 [`memory/daily/2026-06-13-v02.md`](../../memory/daily/2026-06-13-v02.md)。

| 版本 | commit | 关键变更 | 验证 |
|---|---|---|---|
| **v0.2.3** | _(本 commit)_ | 移除 `huan.yaml` 的 `encryptGroups` dead config 11 行 + 全文档同步（反转 ADR 0005 §1.2）；ADR 0006 新建 | `go test ./...` 全 PASS；`huan build -s zhurongshuo` 3032 文件（与 stage 3 基线一致）；`huan version` 输出 `0.2.3`。**注**：`diff-build.sh` 因 zhurongshuo `800b67a59` 删除 `config.toml` 而无法运行（预先存在状态，与本 commit 无关） |
| **v0.3.0** | 2026-06-14~26 | `6364086f`~`f877b2a` | **i18n 多语言系统**（7 PRs 全完成：Translator 插件 + MultiSite 核心 + 模板 helpers + hreflang/sitemap + strict CI；zhurongshuo.com/en/ 生产上线）；**Stage 4 Admin Panel**（Go API + React SPA：ContentList/ContentEdit/ContentNew/Settings/Dashboard 完整功能）；**v0.2.3** encryptGroups cleanup | `go test ./...` 全 PASS；双语 build 零回归；admin UI 前端 TypeScript 零错误；Vite build 成功；zhurongshuo.com/en/ 实测 HTTP 200 |
| **v0.2.1** | `afe89a9` | `huan toc/export/sync` 子命令 + multi-archetype `huan new`（zhurongshuo Hugo→huan 迁移 Phase 1） | toc byte-identical `generate-toc.js`；export md5-identical `export.sh`（i18n collator 复现 zh_CN 排序） |
| **v0.2.0** | `5c220e2` | 移除未启用的 `internal/encrypt/` + `shortcode/redact.go`（-593 行）；`huan.yaml` `params.encryptGroups` 保留为 dead config | `./scripts/diff-build.sh` 三维度 PASS（无回归），证明 zhurongshuo 输出不受影响 |

**CLI 子命令当前总数**：14（build/serve/deploy/plugin/release/version/env/config/list/new/sync/toc/export/translate）。

**`internal/` 包当前总数**：22（v0.3.0 新增 i18n/translate/qwen3/admin/observability 等）

---

## v0.3.0 — i18n 多语言系统（2026-06-14，生产已上线）

**设计来源**：[ADR 0007](../adr/0007-i18n-build-system.md) + [ADR 0008](../adr/0008-translator-capability-qwen3-plugin.md)，grill-me 详见 [`memory/daily/2026-06-14.md`](../../memory/daily/2026-06-14.md)。

**定位**：双语化（zh-cn 默认 + en /en/），Cloudflare Worker 自动检测浏览器语言路由。所有 7 PR 已全部完成并在生产运行。

### 7 PR 完成情况

| PR | 标题 | 状态 | 范围 |
|----|------|------|------|
| PR1 | `internal/translate/` plugin + CLI | ✅ | Translator capability 接口 + Qwen3-Next-80B 实现 + `huan translate` CLI 命令树 |
| PR2 | i18n multilingual build core | ✅ | `LanguageConfig` + `BuildMultiSite` + `detectLanguageFromFilename()` + per-lang 输出路径 |
| PR3 | 模板 helpers + i18n bundle + site_translations | ✅ | `hreflang`/`translationLinks`/`langPrefix` 模板函数 + per-lang i18n bundle 加载 + site_translations 注入 |
| PR4 | zhurongshuo i18n 模板改造 | ✅ | nav/search/header/comments/404/shortcodes 全部 `{{ i18n "key" }}` 化 + `{{ hreflang . }}` + 语言切换 UI |
| PR5 | hreflang 正确性 + sitemap i18n | ✅ | `AvailableTranslations` 过滤 + sitemap xhtml:link annotation（仅实际有 sidecar 的语言） |
| PR6 | zhurongshuo 模板 i18n 收尾 | ✅ | gallery JS I18N 注入 + practice 序数词 i18n + books/practices 万字单位 i18n |
| PR7 | strict_i18n stale 检测 + CI | ✅ | `checkStaleTranslations()` + HUAN_STRICT_I18N env + CI 阻断 |
| PR8 | i18n-router Worker 部署 | ✅ | Cloudflare Worker 线上、zhurongshuo.com/en/ 已可访问 |

### 翻译质量保障体系

translator 插件经过 3 轮迭代：

1. **format_purity + length_ratio 修复**（2026-06-14）：根因发现是模型输出 HTML 而非 Markdown，修复包括：(a) `format_purity` 硬检查（黑名单 HTML 标签），(b) prompt suffix 从 1 行强化为 8 行 `CRITICAL FORMAT RULES`，(c) length_ratio 改字符膨胀比 `[0.5, 3.5]`。详见 [`docs/reports/completed/2026-06-14-translate-format-purity-fix.md`](../docs/reports/completed/2026-06-14-translate-format-purity-fix.md)

2. **Chunked Translation**（2026-06-14）：长文档（>20KB）Qwen3 强 prior 重构问题 → section 级切分 + sliding window（8000 token budget）+ per-chunk retry + atomic write。10 phases 完成。详见 [`docs/reports/completed/2026-06-14-chunked-translation.md`](../docs/reports/completed/2026-06-14-chunked-translation.md)

3. **非对称 heading tolerance**（2026-06-14）：长附录 heading 扩展问题 → heading 非对称检查（src - out > tol 才 fail，扩展 ≤25% 接受），list/link/image 保持对称。

详见 [`docs/reports/completed/2026-06-14-i18n-plugin-implementation.md`](../docs/reports/completed/2026-06-14-i18n-plugin-implementation.md)。

### v0.3.0 上线（Docker image + zhurongshuo 生产 deploy）

- GitHub Release 自动建 release.yml → `ghcr.io/iannil/huan:v0.3.0`（41.1MB linux/amd64，debian:bookworm-slim 基础镜像）
- zhurongshuo CI 用 `container: ghcr.io/iannil/huan:latest` 替代手工安装 huan（ADR 0009）
- Dockerfile 三次修复经验教训：`USER huan` → EACCES issues，Alpine → bash/tar 缺失，最终用 debian:bookworm-slim
- 生产验证：`https://zhurongshuo.com/en/` HTTP 200，菜单全英文，hreflang 三标签，sitemap xhtml:link

### 后续工作（v1.1）

- 首次全量翻译 zhurongshuo ~1075 篇（`huan translate qwen3` ~3h on M5 Max）
- gallery JS 字符串 i18n（需 JS-side i18n 机制）

---

## v0.3.0 配套功能（2026-06-15 ~ 2026-06-16）

### 英文站三态栏目（2026-06-15）

huan core 新增 `LanguageConfig` 三态语义：`ExcludeSections`（完全隐藏）、`CatalogSections`（显示英文目录，内容页不发布）、`NeutralSections`（显示同样图片，内容取默认语言）。

- `internal/config/languages.go`：新增 `ExcludeSections` / `CatalogSections` / `NeutralSections` 字段 + 纯函数 `TopSection()` + `IsSectionExcludedForLang()`
- `internal/build/multisite.go`：非默认语言 PageFilter 按栏目分类过滤
- `internal/template/funcs.go`：新增 `sectionExcluded` 模板函数
- zhurongshuo 主题：nav/header 用 `sectionExcluded` 包裹入口；books/practices en 版显示英文目录（disabled）+ gallery 显示图片

zhurongshuo 侧修复：英文站 RSS 空链接（`OutputFormats.Get "RSS"` 条件渲染）+ `/en/products/` 残留中文（`data/en/products.yaml` + list.html 语言分支）。

### `huan translate audit` 全站审计（2026-06-15）

新增子命令，对 zhurongshuo 每个页面审计中英文对等存在 + 语言正确性。

- `internal/i18n/langdetect/`：抽取共享语言检测包（CJKFraction/CJKRunesOutsideCode/CountLatinWords/CountCJKRunes）
- `internal/i18n/audit/`：HTML 解析（x/net/html）+ 语言检测 + 对等性检查
- `cmd/huan/translate_audit.go`：`huan translate audit` 子命令（--base-url / --allow / --report / --cjk-threshold / --concurrency / --fail）
- 端到端结果：EnglishHasChinese=0、ChineseLooksEnglish=0、MissingEN=333、OrphanEN=0

### `huan serve` 多语言修复（2026-06-15）

`huan serve` 在多语言配置下修复只构建默认语言的问题，`/en/` 路由正确工作。

### 盘古之白 CSS（2026-06-16）

zhurongshuo CSS 层 `text-autospace: normal`（CJK↔西文自动间距），huan core 无改动。

---

## Stage 2 — AI 友好输出（2026-06-13，最新）

huan 定位从"zhurongshuo 专用 Hugo 替代"扩展为"AI 友好优先的通用 SSG"。stage 2 聚焦三个 AI 消费者需要的新增输出：

| 功能 | 路径 | 配置 | 默认 |
|---|---|---|---|
| Markdown 镜像 | `{url}/index.md` | `ai.markdownMirror` | true |
| llms.txt | `/llms.txt` | `ai.llmsTxt` | true |
| 内容 API | `/api/{section}.json` | `ai.contentAPI` | true |

**设计决策（grill-me 共识）**：
- 不做插件架构（YAGNI）——三个功能和 RSS/sitemap/search.json 同类，直接加到 `internal/output/`
- llms.txt 混合模式：auto 从 config 生成 + `layouts/llms.txt` override
- 内容 API 扁平挂载（`/api/{section}.json`），完整字段（title/url/date/summary/plain/tags）
- Markdown 镜像同 HTML draft/future/expired 过滤，只镜像 page kind
- 不做：认证 / 支付 / 会员 / 动态渲染 / 搜索系统 / pipeline 重构

**验证**：三维度等价 PASS（无回归），新增文件不在 Hugo 对比范围。详见 [equivalence.md §3](../standards/equivalence.md#3-甚至更好登记簿)。

---

## Stage 3 — grill-me 完成度确认（2026-06-13）

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
| 4. Shortcode + 加密系统 | ✅ → v0.2.0 移除 encrypt | `internal/shortcode` (audio/img)；`redact` + `internal/encrypt` 因未启用于 v0.2.0 删除，详见 [ADR 0005](../adr/0005-remove-encrypt-and-v02-feature-batch.md) |
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

## Stage 4 — Admin Panel（一体化内容引擎管理后台）

**定位**：基于 huan 新定位「一体化内容引擎」，构建内置管理后台（`huan serve` 的 `/admin` 路由），实现基于文件系统的内容管理。

### 已实施功能

#### PR1：Go API + React SPA 骨架（2026-06-26）

- **Go API**（`internal/admin/`）：
  - `content.go` — 文件系统 CRUD（list/read/create/update/delete Markdown，5 端点）
  - `settings.go` — 站点配置读/写（JSON + YAML raw 双轨）
  - `media.go` — 媒体文件列表
  - `types.go` — API 请求/响应类型定义
  - `api.go` — HTTP 处理器（5 内容端点 + 2 配置端点）
  - `handler.go` — `//go:embed` SPA 分发 + API 路由注册
- **React SPA**（`web/admin/`）：
  - Vite + TypeScript + Tailwind CSS + Shadcn UI
  - React 19 + React Router DOM
  - 页面：`ContentList` / `ContentEdit` / `ContentNew` / `Settings` / `Dashboard`
- **serve 集成**（`internal/serve/server.go`）：`/admin/` 路由嵌入 huan serve

#### ContentList 完整化（2026-06-26，纯前端）

- **搜索框**：按标题/路径实时过滤
- **排序**：标题/日期/状态 3 列，箭头指示
- **草稿筛选**：三段式（全部/草稿/已发布）
- **内容类型标签**：section Badge
- **分页**：每页 20/50/100 + 页码导航
- **批量操作**：全选 + 批量删除(Dialog) + 批量切换草稿
- **3 轮 UI 重设计**：7列→5列→3列极简布局

#### ContentEdit 完整化（2026-06-26）

- **Markdown 实时预览**：3 种模式（编辑/分屏/预览），`marked.parse()` 渲染
- **分屏同步滚动**：编辑和预览面板滚动联动
- **frontend-design 重设计**：Vercel 单色极简，黑白灰 + Geist 字体
- **多语言支持**：语言切换器（同系列其他语言版本切换）

#### Settings 页面（Phase 1，已实施）

- **表单 + YAML 双轨编辑**：常用字段（标题/副标题/描述/版权/GA/CDN 等）+ 原始 YAML 编辑器
- **后端**：`yaml.Node` 保格式写入 + 自动触发 rebuild
- **导航**：Layout 导航栏「设置」项 → `/admin/settings`

#### Dashboard 概览页（2026-06-26）

- **6 张统计卡片**：总内容/草稿/已发布/分类数/标签数/媒体文件
- **进度条**：已发布/草稿比例
- **内容分布**：按 section 可视化
- **最近内容**：最新 5 条，标题/分类/日期/草稿徽章

#### 多语言管理支持（2026-06-26）

- **ContentList 语言列**：彩色语言徽章（EN=靛蓝，ZH=暖橙，默认=中性灰） + 语言筛选 dropdown
- **ContentEdit 语言切换器**：顶栏 Globe icon + dropdown 切换同系列语言版本
- **ContentNew 语言选择**：新建时自动使用 `{title}.{lang}.md` 文件命名约定
- **后端语言 API**：`GET /admin/api/content/{path}/languages`

### 决策记录

| 决策 | 结论 |
|------|------|
| 技术栈 | Go JSON API + 嵌入式 React SPA |
| 前端框架 | React 19 + Shadcn UI |
| 构建工具 | Vite + TypeScript + Tailwind CSS |
| 项目结构 | `web/admin/`（前端）+ `internal/admin/`（Go） |
| 认证 | 初期无（localhost only） |
| 数据模型 | 基于文件系统（Markdown 文件即内容源） |

### 后续方向

- Admin 认证系统（session/token）
- 从其他 CMS（WordPress / Ghost / Strapi）的迁移工具
- 多用户协作支持
- 媒体库管理增强（在线裁剪/上传）
- Admin 内集成 deploy 配置

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
