# meta description plainify 完成报告

> 完成日期：2026-06-12 · 关联 plan：[2026-06-12-meta-plainify-plan.md](2026-06-12-meta-plainify-plan.md)
> 关联上一阶段：[三维度等价标准落地完成报告](2026-06-12-redefine-equivalence-report.md)

## 落地内容

### 代码（3 commits since dba8494）

- `internal/template/funcs.go:28` plainify 提取为 named function，Port Hugo `tpl/template.go:StripHTML` 完整算法
- 算法核心：pre-replacer（`\n`→空格、`</p>`/`<br>`/`<br />`→placeholder）+ stripTags + placeholder→`\n` 还原 + unicode.IsSpace 连续 whitespace 去重
- 删除 dead code `collapseWhitespace`（Port 后无调用点）
- 无新增依赖（`unicode` 已在 imports）

### 测试（8 个 plainify 单元测试全 PASS）

- `TestPlainify_NoTagsShortcut` — 无 tag 直接返回
- `TestPlainify_PBlockBoundaryBecomesNewline` — `</p>` → `\n`
- `TestPlainify_BrBecomesNewline` — `<br>` → `\n`
- `TestPlainify_NonPTagsDoNotGetNewline` — `<h2>` 等不保留 `\n`
- `TestPlainify_DedupsConsecutiveWhitespace` — 连续 whitespace 去重
- `TestPlainify_PreservesLeadingTrailingWhitespace` — 前后 whitespace 保留（去重后）
- `TestPlainify_RealWorldZhurongshuoSummary` — 真实 zhurongshuo summary
- `TestPlainify_HandlesEmptyAndNil` — 空输入 / nil

### 验证结果

zhurongshuo 3 篇典型页面 byte-identical 验证（huan vs Hugo）：
- `general/index.html`：meta name=description / og:description ✅
- `practices/season-4/data-as-the-boundary/part-05/chapter-10/index.html`：✅
- `books/volume-1/god-beyond-observation/index.html`：✅

diff-build.sh 四模式数字（修复前 → 修复后）：

| 模式 | 修复前 differing | 修复后 differing | 增量 |
|---|---|---|---|
| byte（雷达） | 1265 | 1031 | -234 |
| normalized（肉眼） | 1265 | 1031 | -234 |
| seo | 983 | 699 | **-284** |
| ai | 323 | 36 | **-287** |

### Stage 2 路线图进度

| Phase | 项 | 状态 |
|---|---|---|
| **1** | meta description plainify | ✅ 已完成 |
| 2 | RSS 中文 URL 编码 | 待启动 |
| 3 | books section part 顺序 | 待启动 |
| 4 | body 渲染细节 | 待启动 |
| 5 | minify artifacts | 待启动 |

## 关键发现 / 教训

- **第一轮修复方向反了**：假设 Hugo plainify 折叠 `\n` 为空格，实施 collapseWhitespace + TrimSpace 后差异反而增加 300 个文件。reset 后读 Hugo `tpl/template.go:StripHTML` 源码，发现实际算法用 placeholder `___hugonl_` 保留 `</p>` / `<br>` 边界为 `\n`，只对源 `\n` 与连续 whitespace 去重，**不 trim**。
- **Port 上游算法前必须读真实源码**——不能凭直觉。`tpl/template.go` 是 Hugo 实际入口，比想象中的 `helpers/` 更权威。
- **byte-level 实证（`od -c` 或 Python repr）比 grep 更可靠**——前者能看到 `\n` / 空格 / 前导/尾随，后者会因 minified 单行 / regex 转义等假象误导。
