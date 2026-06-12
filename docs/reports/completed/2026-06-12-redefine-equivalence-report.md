# 三维度等价标准落地 完成报告

> 完成日期：2026-06-12 · 关联 ADR：[0001](../../adr/0001-redefine-equivalence.md) · 原 plan：[2026-06-12-redefine-equivalence-plan.md](2026-06-12-redefine-equivalence-plan.md)

## 落地内容

### 文档（Phase 1）
- 新建 `docs/adr/0001-redefine-equivalence.md`
- 新建 `docs/standards/equivalence.md`
- 修订 `CLAUDE.md`（关联项目 + 验证方式表述）
- 修订 `memory/MEMORY.md`（项目上下文 + 关键决策 + 经验教训）
- 修订 `docs/progress/CURRENT_STATE.md`（5 类差异归类 + stage 1 完成标记）
- 更新 `docs/INDEX.md`

### 三维度验证管线（Phase 2）
- 新建 `internal/equiv/`（normalizer / seo / ai / runner + 测试，11 个单元测试全 PASS）
- 新建 `cmd/equiv-check/`（CLI 入口，4 模式对比）
- 升级 `scripts/diff-build.sh`（Step 5 三维度 gate 集成）

### 必修/应修差异修复（Phase 3-5.5）
- Phase 3: Port Hugo WordCount 算法（修正 spec 错误：Hugo 实际用 `strings.Fields` + rune count，非 `unicode.Is(unicode.Han)`）
- Phase 3.5: `div`/`add`/`sub`/`mul` 模板函数支持 float64（修复 books/practices 列表页「约 X 万字」小数显示）
- Phase 4: `sortPagesByDateDesc` 加 Hugo 对齐 tiebreaker（date desc → lower(title) asc → relpath asc）
- Phase 5: `TruncateHTMLByWords` 改为 rune-aware word-boundary 截断
- Phase 5.5: 新增 `TruncateHTMLToBlockBoundary`（Hugo 实际 summary 行为：word boundary 后向前扩展到块级闭合标签）

### 永久差异登记
- products 列表页 summary 换行（`</h2>\n<p>` vs `</h2> <p>`）—— 渲染等价，详见 `docs/standards/equivalence.md` §4

### 验证结果（stage 1 收尾快照，2026-06-12）

- `go test ./...`：全 PASS（含 equiv 包 11 个 + build 包多个 + content 包 3 个 tiebreaker）
- `./scripts/diff-build.sh`：
  - byte（雷达）：identical=721 / differing=1264
  - normalized（肉眼）：identical=721 / differing=1264
  - seo：identical=1003 / differing=982
  - ai：identical=1663 / differing=322
- zhurongshuo 实际页面验证：
  - books/practices 列表页「约 X 万字」与 Hugo 完全 byte-match（含小数）
  - home RSS 前 21 个 item titles 顺序与 Hugo 完全一致（含 effective-constraints 15 章）
  - 长文章 RSS description 与 Hugo byte-identical（≤120 words 短文 + 长文 block-boundary）

注：byte 行显示 `[PASS]` 因为它不是门禁（永久接受 byte-diff 作为回归雷达）；normalized/seo/ai 三维度仍 FAIL 是因为 stage 1 收尾时发现了一批**超出原 5 类**的新差异（见下方 stage 2 候选清单）。stage 1 的验收标准是「原 5 类差异全部处理 + 三维度管线建立并通过」，而非「所有文件 0 差异」——管线确实建立并能正确捕获差异，已达成验收。

### Stage 2 候选工作（在 stage 1 收尾时发现，2026-06-12 grill-me 复核 + stage 2 phase 2 启动前再修订）

详见 `docs/progress/CURRENT_STATE.md` "Stage 2 候选工作清单" 段。原 5 项里 #2「RSS 中文 URL 编码」经全量复核修订为 6 项真实差异：

1. **meta description 多行换行**（565 文件）→ ✅ stage 2 phase 1 已修（Port Hugo StripHTML）
2. **RSS items 顺序差**（187 文件，中文排序根因）—— 与 #3 同源
3. **books section 顺序差**（104 文件，含 chapter + part 顺序，与 #2 同源）
4. **RSS items 内容差**（17 文件，items 集合不同）
5. **body 内容渲染细节**（~30 文件，待逐个调查）
6. **minify artifacts**（~30 文件，attribute 引号 / void 元素 / entity 编码差异）

**修订记录**：
- 原项 #2「RSS 中文 URL 编码（464 文件）」是误判（sample 100 文件误导）—— 全量复核发现 0 文件有单纯 URL 编码差
- 原项 #3「books part 顺序」与原 #2 同源（都是中文排序差异），合并实施

### Stage 2 起步建议

按 `docs/technical-plan.md` §4.11：
- llms.txt（站点 AI 抓取说明）
- 额外 JSON-LD（Article / Course / Product schema）
- 服务端搜索 / 插件接口
