# Hugo DefaultPageSort Port（CJK 拼音排序）完成报告

> 完成日期：2026-06-13 · 关联 plan：[2026-06-12-cjk-sort-plan.md](2026-06-12-cjk-sort-plan.md)
> 关联上一阶段：[meta description plainify 完成报告](2026-06-12-meta-plainify-report.md)

## 落地内容

### 代码（7 commits since b7fa008）

**Phase 1：基础设施**
- 新增依赖 `golang.org/x/text v0.38.0`（提供 `collate` + `language` 包）
- 新建 `internal/i18n/collator.go`：`BuildCollator(langCode string) *collate.Collator`，Hugo-aligned fallback（无效 lang → English）

**Phase 2：Port DefaultPageSort**
- `internal/content/tree.go`：`sortPagesByDateDesc` 重命名为 `sortPagesDefault(pages, coll)`
- Port Hugo `resources/page/pages_sort.go:DefaultPageSort` 4 层链：Weight（0 排最后）/ Date desc / Collator Title asc / Path asc
- `BuildTree` 内构建一次 Collator（按 `cfg.LanguageCode`），传递给所有 sortPagesDefault 调用

**Phase 2.5：补漏 tags RSS**
- `internal/build/build.go:173`：`taxonomy.BuildAll(pages)` 改为 `taxonomy.BuildAll(site.RegularPages)`
- 让 tags RSS 也走排序后的 pages（之前用未排序的扫描顺序，154 个 tags RSS 文件错序）

### 测试（11 个单元测试全 PASS）

i18n 包（4 个）：
- `TestBuildCollator_ReturnsCollator`
- `TestBuildCollator_ZhCN_PinyinOrder`（13 数字拼音序 byte-verify）
- `TestBuildCollator_FallbackToEnglishForInvalidLang`
- `TestBuildCollator_EmptyStringDefaultsToEnglish`

content 包（6 个 sortPagesDefault）：
- `TestSortPagesDefault_CJKPinyinOrder`
- `TestSortPagesDefault_DateDescTakesPrecedence`
- `TestSortPagesDefault_WeightNonZeroBeforeZero`
- `TestSortPagesDefault_BothWeightZeroFallsBackToTitleThenPath`
- `TestSortPagesDefault_PathTiebreakerIsByteLevel`
- `TestSortPagesDefault_RealWorldZhurongshuoChapters`（7 章 二/六/七/三/四/五/一 byte-verify）

taxonomy 包（1 个）：
- `TestBuild_PreservesInputPageOrder`（契约测试）

### 验证结果

zhurongshuo 实际页面 byte-match：
- `books/volume-3/the-fetters-of-evaluation/index.xml`：byte-identical Hugo（完整文件 diff exit 0）
- home `index.xml`：全部 `<title>` 顺序 byte-match
- 3 个抽样 books RSS byte-match
- tags/道 + tags/专注 + tags/观点：byte-match
- tags/书稿：items 内容差（stage 2 phase 3 范畴，与排序无关）

diff-build.sh 四模式数字（stage 2 phase 1 后 → stage 2 phase 2 后）：

| 模式 | phase 1 后 | phase 2 后 | 增量 |
|---|---|---|---|
| byte | 1031 | 1049 | +18（±75 噪声） |
| normalized | 1031 | 1049 | +18（±75 噪声） |
| seo | 699 | **508** | **-191** |
| ai | 36 | 31 | -5 |

byte/normalized 在 ±75 噪声内（huan build 非确定性 + RSS minify 布局漂移）。seo 显著下降（-191）证明 sort 修复生效。

### Stage 2 路线图进度

| Phase | 项 | 状态 |
|---|---|---|
| 1 | meta description plainify | ✅ 已完成 |
| **2** | **RSS items 顺序（中文排序）** | **✅ 已完成** |
| **3** | **books section 顺序（同 #2）** | **✅ 已完成（与 #2 合并）** |
| 4 | RSS items 内容差 | 待启动 |
| 5 | body 渲染细节 | 待启动 |
| 6 | minify artifacts | 待启动 |

## 关键发现 / 教训

- **Hugo 用 collate 库做 locale-aware 排序**：`resources/page/pages_sort.go:DefaultPageSort` 在 Date 相同时用 `golang.org/x/text/collate` 按 site language CLDR 表排序（zh-cn = 拼音序）。
- **完整 DefaultPageSort 链 4+ 层**：Weight（0 排最后）/ Date desc / Collator LinkTitle asc / Path asc。Hugo 还有更早的 Ordinal / Weight0 层，zhurongshuo 不用所以可省略。
- **Go 标准库不能 locale-aware 排序**：`strings.Compare` 是 byte-level，CJK 字符按 UTF-8 编码排序，与拼音序完全不同。
- **排序修复要覆盖所有调用路径**：phase 2 初版只修了 sortPagesDefault，遗漏了 `taxonomy.BuildAll` 的 pages 参数（build.go:173）。**含义**：grep 修复函数名时必须检查所有数据流路径，不能假设"修了源头就全对"。实证发现 154 个 tags RSS 文件未对齐 → 立刻 BLOCKED + 调查根因，避免错误基础上继续 Phase 4 文档化。
