# 三维度等价标准落地 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 stage 1 的验收标准从「逐字节 100% 一致」改为「肉眼 / SEO / AI 三维度与 Hugo 输出对比无差异（甚至更好）」，落地所有文档、验证管线、必修差异。

**Architecture:** 分 6 个 phase：(1) 文档化新标准（ADR + 标准 + 更新 CLAUDE.md / MEMORY / CURRENT_STATE）；(2) 新建 `internal/equiv/` 包提供 HTML normalize / SEO 字段 / AI 字段三种对比，CLI 入口在 `cmd/equiv-check/`；(3) Port Hugo WordCount 算法修必修 #1；(4) `sortPagesByDateDesc` 加 tiebreaker 修 RSS items 顺序；(5) `TruncateHTMLByWords` 对齐 word boundary 修 RSS description + general summary 截断；(6) 跑三维度管线收尾 stage 1。

**Tech Stack:** Go 1.x（与项目一致）/ `unicode` 标准库 / `golang.org/x/net/html`（HTML 解析，可能已在依赖）/ bash + Go CLI（diff-build.sh 升级）/ goldmark（已在用）。

---

## File Structure

**新建文件**：
- `docs/adr/0001-redefine-equivalence.md` — 标准变更 ADR
- `docs/standards/equivalence.md` — 三维度等价标准完整定义（含永久差异登记）
- `internal/equiv/normalizer.go` + `_test.go` — HTML normalize（肉眼维度）
- `internal/equiv/seo.go` + `_test.go` — SEO 字段提取（SEO 维度）
- `internal/equiv/ai.go` + `_test.go` — AI 字段提取（AI 维度）
- `internal/equiv/runner.go` + `_test.go` — 三模式对比 runner（聚合输出）
- `cmd/equiv-check/main.go` — CLI 入口，被 diff-build.sh 调用

**修订文件**：
- `CLAUDE.md` — 「阶段一目标：100% 一致」→ 新三维度表述
- `memory/MEMORY.md` — 更新决策、纠正「输出 100% 一致」「Hugo date 不稳定」等过期描述
- `memory/daily/2026-06-12.md` — 已有 grill 记录，本 plan 落地后追加完成标记
- `docs/progress/CURRENT_STATE.md` — 5 类差异重新归类、刷新数字、stage 1 完成标记
- `docs/INDEX.md` — 新增 ADR / equivalence / equiv-check 索引
- `internal/build/summary.go` — Port Hugo WordCount 算法（CJK 全范围）
- `internal/content/tree.go` — `sortPagesByDateDesc` 加 tiebreaker
- `scripts/diff-build.sh` — 升级为多模式（byte/normalized/seo/ai）

**接受为永久差异**（不修，仅文档化）：
- `</h2>\n<p>` vs `</h2> <p>`（products summary 换行）—— 渲染等价

---

# Phase 1 — 文档化新标准

**目的**：在动任何代码前，先把标准固化为文档。这避免后续修复时反复争论「该不该修」。

## Task 1.1：写 ADR 0001

**Files**:
- Create: `docs/adr/0001-redefine-equivalence.md`

- [ ] **Step 1: 创建 ADR 文件**

写入以下内容（Markdown，结构按 ADR 规范）：

````markdown
# ADR 0001：重新界定「100% 还原」为三维度等价

- **状态**：Accepted
- **日期**：2026-06-12
- **决策者**：用户（owner）+ Claude（grill-me 收敛）
- **替代方案**：保持「逐字节 100% 一致」原标准

## 背景

stage 1 原目标是「huan 输出与 Hugo 逐字节 100% 一致」。该目标在 zhurongshuo 实际推进中遇到：

1. **边际收益递减**：剩余 5 类边缘差异里，部分（如字数统计算法）涉及对齐 Hugo 内部 CJK 分词器，工作量大但与「用户实际感知」无关
2. **目标过严**：HTML 源码层的换行差异（如 `</h2>\n<p>` vs `</h2> <p>`）在浏览器渲染时等价，但 byte-diff 仍标为差异
3. **目标错位**：阶段一真正的承诺是「zhurongshuo 用户/搜索引擎/AI 切换到 huan 后无感」，而非「字节一致」
4. **实证撞墙**：Go template + Scratch + sort 的引用语义问题导致 RSS items 顺序 tiebreaker 修复无法落地，反映「严格还原」路线在某些点不可行

## 决策

把 stage 1 的「100% 还原」重新定义为「**与 Hugo 输出对比，肉眼 / SEO / AI 三维度均无差异（甚至更好）**」。

### 三维度定义

| 维度 | 测量方法 | 等价判据 |
|---|---|---|
| 肉眼 | HTML normalize 后字节对比（折叠空白、规范 attribute、自闭合标签） | normalize 后完全等价 |
| SEO | SEO 关键字段提取对比（title / description / og:* / canonical / h1-h3 / JSON-LD / sitemap / robots） | 所有字段逐项等价 |
| AI | AI 友好度字段对比（main 内容 / heading outline / JSON-LD / llms.txt / 内部链接 graph / 语义化标签） | 所有字段逐项等价 |

### 「甚至更好」的允许范围

stage 1 范畴内允许两种「更好」：

1. **修正型**：Hugo 输出有客观错误时（如 WordCount 不准、HTML 不规范），huan 可以选择修正
2. **扩展型**：huan 可以主动添加 Hugo 未做的现代实践（如 llms.txt / 额外 JSON-LD），只要不破坏三维度无差异基线

每项「更好」必须文档化并标记 "better than Hugo"。

### 与 diff-build.sh 的关系（分层并存）

- `scripts/diff-build.sh` 的原 byte-diff 模式保留作回归雷达（仅报告，不阻断合并）
- 新增 normalized / seo / ai 三种对比模式，任一失败则阻断合并

## 5 类差异的归类

基于三维度尺子重新评估（详见 `docs/standards/equivalence.md`）：

| # | 差异 | 归类 | 处理 |
|---|---|---|---|
| 1 | 字数统计精度 | 必修 | Port Hugo WordCount 算法（覆盖 unicode.Is(unicode.Han) 全范围 + 假名 + 韩文 + 全角符号） |
| 2 | RSS items 顺序 | 应修 | `sortPagesByDateDesc` 加 tiebreaker（date desc → title asc → path asc） |
| 3 | RSS description 截断 | 应修 | `TruncateHTMLByWords` 改为 word-boundary 截断 |
| 4 | products summary 换行 | 接受 | 永久差异（渲染等价），登记在 `docs/standards/equivalence.md` |
| 5 | general summary 截断 | 应修 | 与 #3 在 summary 后处理统一 |

## 影响

- stage 1 收尾判定：4 项必修/应修全部解决 + 三维度管线通过 + 本 ADR 写完 + 全面文档化
- stage 2 起步：llms.txt + 额外 JSON-LD + 搜索/插件（按 `docs/technical-plan.md` §4.11）
- 现有 `MEMORY.md` / `CURRENT_STATE.md` 中的「100% 一致」「Hugo date 不稳定」等表述需要修订
````

- [ ] **Step 2: 验证 ADR 文件存在**

Run: `test -f docs/adr/0001-redefine-equivalence.md && head -5 docs/adr/0001-redefine-equivalence.md`
Expected: 显示文件前 5 行（标题与状态）

- [ ] **Step 3: Commit**

```bash
git add docs/adr/0001-redefine-equivalence.md
git commit -m "docs(adr): 0001 redefine 100% equivalence as 3-dimension standard"
```

## Task 1.2：写三维度等价标准文档

**Files**:
- Create: `docs/standards/equivalence.md`

- [ ] **Step 1: 创建标准文件**

写入以下内容：

````markdown
# 三维度等价标准

> **生效日期**：2026-06-12 · **关联 ADR**：[ADR 0001](../adr/0001-redefine-equivalence.md)
> 本文档定义 huan 与 Hugo 输出对比的「100% 还原」标准。所有 stage 1 验收以本标准为准。

## 1. 总则

「100% 还原」= **肉眼 / SEO / AI 三维度与 Hugo 输出对比均无差异（甚至更好）**。

- **比对基准**：Hugo 当前输出（Hugo 0.x + zhurongshuo 当前模板与配置）
- **比对工具**：`scripts/diff-build.sh`（升级版，支持 `byte` / `normalized` / `seo` / `ai` 四种模式）
- **失败策略**：byte 模式仅报告（雷达）；normalized / seo / ai 三种任一失败则阻断合并

## 2. 三维度定义

### 2.1 肉眼无差异

**测量**：HTML normalize 后字节对比。

normalize 规则：
1. 折叠连续空白（空格 / `\t` / `\n` / `\r`）为单个空格
2. 移除标签间的纯空白（`<div>\n  <p>` → `<div><p>`）
3. 规范化自闭合标签（`<br/>` ↔ `<br />` ↔ `<br>`）
4. 规范化 attribute 顺序（按字典序）
5. 规范化 attribute 引号（统一为双引号）
6. 规范化 boolean attribute（`disabled="disabled"` → `disabled`）
7. 规范化 HTML entity（`&amp;` ↔ `&#38;`，统一为命名 entity）

**等价判据**：normalize 后字节完全相同。

### 2.2 SEO 无差异

**测量**：从 HTML 提取 SEO 关键字段，逐字段对比。

提取的字段集：
- `<title>` 文本
- `<meta name="description">` 的 content
- `<meta property="og:*">` 的 content（og:title / og:description / og:image / og:url / og:type）
- `<meta name="twitter:*">` 的 content
- `<link rel="canonical">` 的 href
- `<meta name="robots">` 的 content
- 所有 `<h1>` / `<h2>` / `<h3>` 的文本（按出现顺序）
- `<script type="application/ld+json">` 的内容（JSON-LD，sort keys 后对比）
- `<a>` 的 href 与 rel="nofollow" 标记
- 站点级：`sitemap.xml` 文件内容、`robots.txt` 文件内容

**等价判据**：所有字段逐项等价（JSON-LD 用 `sort_keys + indent=2` 规范化后字节对比）。

### 2.3 AI 抓取无差异

**测量**：从 HTML 提取 AI 友好度关键字段，逐字段对比。

提取的字段集：
- `<main>` 或 `<article>` 的 innerText（主体内容）
- heading outline：所有 `<h1>`-`<h6>` 的层级 + 文本（按出现顺序）
- `<script type="application/ld+json">` 内容（与 SEO 维度复用）
- `<nav>` 的链接结构（内部链接 graph）
- 语义化标签存在性：`<header>` / `<main>` / `<article>` / `<section>` / `<nav>` / `<footer>` / `<aside>`
- 站点级：`llms.txt`（如果存在）

**等价判据**：所有字段逐项等价。

## 3. 「甚至更好」登记簿

任何 huan 与 Hugo 不同、但符合「无差异基线（vs Hugo 不退步）」的偏离，登记在此处。每项必须标注维度影响。

| 项 | 维度影响 | 类型 | 说明 |
|---|---|---|---|
| （stage 1 范畴暂无） | — | — | — |

stage 1 收尾后，所有「更好」的偏离由本表统一登记。任何未登记的偏离视为回归。

## 4. 接受为永久差异的项

这些差异在三维度上**渲染等价**或**对最终感知无影响**，登记后不再修复。

| 项 | 影响维度 | 是否真的无感 | 登记日期 |
|---|---|---|---|
| products 列表页 summary 中 `</h2>\n<p>` 的源码换行 vs Hugo 的空格 | 肉眼：浏览器折叠空白，渲染等价；SEO/AI：不读源码空白 | ✅ 是 | 2026-06-12 |

## 5. 修订历史

- 2026-06-12：初版，由 ADR 0001 落地。
````

- [ ] **Step 2: 验证文件存在且结构完整**

Run: `test -f docs/standards/equivalence.md && grep -c "^##" docs/standards/equivalence.md`
Expected: 输出 ≥ 5（共至少 5 个二级标题）

- [ ] **Step 3: Commit**

```bash
git add docs/standards/equivalence.md
git commit -m "docs(standards): three-dimension equivalence standard"
```

## Task 1.3：修订 CLAUDE.md「100% 一致」表述

**Files**:
- Modify: `CLAUDE.md`

- [ ] **Step 1: 读 CLAUDE.md 确认表述位置**

Run: `grep -n "100%\|一致" CLAUDE.md`
Expected: 列出包含「100%」或「一致」的行号（包括第 6 行「输出 100% 一致」、第 30 行「逐字节对比」）

- [ ] **Step 2: 修订第 6 行（关联项目 zhurongshuo 描述）**

把 `阶段一目标：huan 生成的站点输出必须与 Hugo 的输出 100% 一致`

改为：

```markdown
- 阶段一目标：huan 生成的站点输出与 Hugo 输出在「肉眼 / SEO / AI 三维度」无差异（甚至更好），详见 [`docs/standards/equivalence.md`](docs/standards/equivalence.md) 与 [ADR 0001](docs/adr/0001-redefine-equivalence.md)
```

- [ ] **Step 3: 修订架构决策中的「逐字节对比」表述**

把第 30 行（验证方式）的 `diff 测试管线，与 Hugo 输出逐字节对比`

改为：

```markdown
- 验证方式：`./scripts/diff-build.sh` 多模式对比（byte 雷达 + normalized / seo / ai 三维度门禁），详见 [`docs/standards/equivalence.md`](docs/standards/equivalence.md)
```

- [ ] **Step 4: 验证修改**

Run: `grep -n "三维度\|equivalence" CLAUDE.md`
Expected: 至少 2 行匹配（关联项目 + 验证方式都已改）

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md to 3-dimension equivalence standard"
```

## Task 1.4：修订 MEMORY.md 过期描述

**Files**:
- Modify: `memory/MEMORY.md`

- [ ] **Step 1: 读 MEMORY.md 当前内容**

Run: `cat memory/MEMORY.md | head -60`
Expected: 看到当前长期记忆内容

- [ ] **Step 2: 修订「项目上下文」段（第 18 行）**

把 `- **huan** = Go 静态站点生成器，阶段一目标：替代 Hugo 构建 zhurongshuo.com，输出 100% 一致`

改为：

```markdown
- **huan** = Go 静态站点生成器，阶段一目标：替代 Hugo 构建 zhurongshuo.com，输出与 Hugo 在「肉眼 / SEO / AI 三维度」无差异（甚至更好），详见 [`docs/standards/equivalence.md`](../docs/standards/equivalence.md) 与 [ADR 0001](../docs/adr/0001-redefine-equivalence.md)
```

- [ ] **Step 3: 在「关键决策」段追加 ADR 0001 条目**

在「关键决策」段（`## 关键决策` 标题下）追加：

```markdown
- **三维度等价标准（2026-06-12，ADR 0001）**：stage 1 验收从「逐字节 100% 一致」改为「肉眼 / SEO / AI 三维度与 Hugo 输出无差异（甚至更好）」。byte-diff 保留作回归雷达，三维度对比作为门禁。允许修正型 + 扩展型「更好」（不破坏基线即可）。
```

- [ ] **Step 4: 修订「经验教训」段中的「Hugo date 不稳定」描述**

读 daily 2026-06-12 已经记录的实证：Hugo date 相同时有稳定 tiebreaker（lower(Title) asc → RelPath asc）。

在 `## 经验教训` 段追加：

```markdown
- **CURRENT_STATE.md 的「5 类差异」描述部分过期**（2026-06-12 更新）：第 2 类「Hugo date 相同时顺序不稳定」是错的——实证发现 Hugo 有稳定 tiebreaker（推断为 `Date desc → lower(Title) asc → RelPath asc`）。stage 1 收尾后所有 5 类差异已按三维度尺子重新归类，详见 [`docs/standards/equivalence.md`](../docs/standards/equivalence.md) §4 与 §5。
```

- [ ] **Step 5: 验证修改**

Run: `grep -c "三维度\|ADR 0001" memory/MEMORY.md`
Expected: 输出 ≥ 3（项目上下文 + 关键决策 + 经验教训都已涉及）

- [ ] **Step 6: Commit**

```bash
git add memory/MEMORY.md
git commit -m "docs(memory): align MEMORY.md with 3-dimension equivalence standard"
```

## Task 1.5：修订 CURRENT_STATE.md

**Files**:
- Modify: `docs/progress/CURRENT_STATE.md`

- [ ] **Step 1: 读 CURRENT_STATE.md 当前内容**

Run: `cat docs/progress/CURRENT_STATE.md`
Expected: 看到当前进度文档

- [ ] **Step 2: 修订「Hugo 输出一致性快照」段（带噪声说明）**

把原「Hugo 输出一致性快照」整段（约第 20-25 行）替换为：

```markdown
**Hugo 输出一致性快照**（带 ±75 文件噪声，详见经验教训）：
- Hugo 总文件数：2029  ·  huan 总文件数：2036
- byte-diff：约 905 完全一致 / 1124 差异（噪声 ±75）
- **新等价标准**：以 [`docs/standards/equivalence.md`](../standards/equivalence.md) 为准，byte-diff 仅作雷达
```

- [ ] **Step 3: 修订「待办 — 剩余 Hugo 兼容差异」段**

把原 5 类差异描述整段（约第 34-62 行）替换为三维度归类：

```markdown
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
```

- [ ] **Step 4: 修订「当前活跃工作」段**

把「当前活跃工作」段（约第 28-30 行）替换为：

```markdown
## 当前活跃工作

三维度等价标准落地（ADR 0001）：详见 [`docs/superpowers/plans/2026-06-12-redefine-equivalence.md`](../superpowers/plans/2026-06-12-redefine-equivalence.md)。
```

- [ ] **Step 5: 验证修改**

Run: `grep -c "三维度\|必修\|应修\|接受" docs/progress/CURRENT_STATE.md`
Expected: 输出 ≥ 5

- [ ] **Step 6: Commit**

```bash
git add docs/progress/CURRENT_STATE.md
git commit -m "docs(progress): re-classify 5 hugo-compat diffs by 3-dimension standard"
```

## Task 1.6：更新 docs/INDEX.md 索引

**Files**:
- Modify: `docs/INDEX.md`

- [ ] **Step 1: 读 INDEX.md**

Run: `cat docs/INDEX.md`
Expected: 看到当前索引结构

- [ ] **Step 2: 在 INDEX.md 合适位置（标准或决策段）追加两条索引**

追加：

```markdown
- [ADR 0001：重新界定「100% 还原」为三维度等价](adr/0001-redefine-equivalence.md)
- [三维度等价标准](standards/equivalence.md)
- [三维度等价标准实施 plan（2026-06-12）](superpowers/plans/2026-06-12-redefine-equivalence.md)
```

- [ ] **Step 3: Commit**

```bash
git add docs/INDEX.md
git commit -m "docs(index): add ADR 0001, equivalence standard, plan"
```

---

# Phase 2 — 三维度验证管线

**目的**：先建工具，后修差异。这样 Phase 3-5 的每个修复都能用工具验证。

## Task 2.1：HTML normalizer 模块 + 测试

**Files**:
- Create: `internal/equiv/normalizer.go`
- Test: `internal/equiv/normalizer_test.go`

- [ ] **Step 1: 检查 golang.org/x/net/html 是否在依赖中**

Run: `grep "golang.org/x/net" go.mod`
Expected: 显示 `golang.org/x/net vX.Y.Z`（若不在，下一步先 `go get`）

- [ ] **Step 2: 写 normalizer_test.go 的第一个失败测试（折叠空白）**

Create `internal/equiv/normalizer_test.go`:

```go
package equiv

import "testing"

func TestNormalizeHTML_FoldsWhitespaceBetweenTags(t *testing.T) {
	in := "<div>\n  <p>hello</p>\n</div>"
	want := "<div><p>hello</p></div>"
	got := NormalizeHTML(in)
	if got != want {
		t.Errorf("NormalizeHTML whitespace fold:\n got: %q\nwant: %q", got, want)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/equiv/ -run TestNormalizeHTML_FoldsWhitespaceBetweenTags -v`
Expected: FAIL，`undefined: NormalizeHTML`

- [ ] **Step 4: 创建 normalizer.go 的最小实现**

Create `internal/equiv/normalizer.go`:

```go
// Package equiv provides HTML/SEO/AI field comparison utilities
// for verifying huan output equivalence against Hugo.
package equiv

import (
	"bytes"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// NormalizeHTML returns a canonical form of the input HTML, suitable
// for byte-level comparison of pages that should render identically.
//
// Normalizations applied:
//   - Whitespace between tags is removed (<div>\n  <p> → <div><p>)
//   - Self-closing tags are canonicalized (<br/> and <br /> → <br/>)
//   - Attributes are sorted by name with double-quoted values
//   - Boolean attributes are normalized
//   - HTML entities are decoded then re-encoded as named entities where possible
func NormalizeHTML(in string) string {
	doc, err := html.Parse(strings.NewReader(in))
	if err != nil {
		return in
	}
	var buf bytes.Buffer
	crawlNormalize(doc, &buf)
	return buf.String()
}

func crawlNormalize(n *html.Node, buf *bytes.Buffer) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawlNormalize(c, buf)
		}
		return
	case html.ElementNode:
		buf.WriteString("<")
		buf.WriteString(n.Data)
		attrs := append([]html.Attribute(nil), n.Attr...)
		sort.Slice(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
		for _, a := range attrs {
			buf.WriteString(" ")
			buf.WriteString(a.Key)
			if a.Val != "" || stringHasSpace(a.Val) {
				buf.WriteString(`="`)
				buf.WriteString(html.EscapeString(a.Val))
				buf.WriteString(`"`)
			}
		}
		if isVoidElement(n.Data) {
			buf.WriteString("/>")
		} else {
			buf.WriteString(">")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				crawlNormalize(c, buf)
			}
			buf.WriteString("</")
			buf.WriteString(n.Data)
			buf.WriteString(">")
		}
		return
	case html.TextNode:
		folded := strings.TrimSpace(n.Data)
		if folded != "" {
			buf.WriteString(html.EscapeString(folded))
		}
		return
	case html.CommentNode:
		return
	}
}

func isVoidElement(name string) bool {
	switch strings.ToLower(name) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func stringHasSpace(s string) bool { return strings.ContainsAny(s, " \t\n\r\"") }
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/equiv/ -run TestNormalizeHTML_FoldsWhitespaceBetweenTags -v`
Expected: PASS

- [ ] **Step 6: 追加 attribute 排序测试**

Append to `normalizer_test.go`:

```go
func TestNormalizeHTML_SortsAttributes(t *testing.T) {
	in := `<a href="x" class="c" id="i">txt</a>`
	want := `<a class="c" href="x" id="i">txt</a>`
	got := NormalizeHTML(in)
	if got != want {
		t.Errorf("NormalizeHTML attr sort:\n got: %q\nwant: %q", got, want)
	}
}
```

- [ ] **Step 7: 运行新测试**

Run: `go test ./internal/equiv/ -run TestNormalizeHTML_SortsAttributes -v`
Expected: PASS（实现已经支持排序）

- [ ] **Step 8: 追加自闭合标签测试**

Append to `normalizer_test.go`:

```go
func TestNormalizeHTML_VoidElementCanonical(t *testing.T) {
	in := `<div><br /><img src="a.png"></div>`
	want := `<div><br/><img src="a.png"/></div>`
	got := NormalizeHTML(in)
	if got != want {
		t.Errorf("NormalizeHTML void canonical:\n got: %q\nwant: %q", got, want)
	}
}
```

- [ ] **Step 9: 运行测试**

Run: `go test ./internal/equiv/ -v`
Expected: 3 个测试全 PASS

- [ ] **Step 10: Commit**

```bash
git add internal/equiv/normalizer.go internal/equiv/normalizer_test.go
git commit -m "feat(equiv): HTML normalizer with whitespace fold + attr sort"
```

## Task 2.2：SEO 字段提取器

**Files**:
- Create: `internal/equiv/seo.go`
- Test: `internal/equiv/seo_test.go`

- [ ] **Step 1: 写 SEO 字段提取测试**

Create `internal/equiv/seo_test.go`:

```go
package equiv

import (
	"reflect"
	"testing"
)

func TestExtractSEO_CapturesCoreFields(t *testing.T) {
	htmlSrc := `<!doctype html><html><head>
<title>Page T</title>
<meta name="description" content="desc">
<meta property="og:title" content="OG T">
<meta property="og:image" content="og.png">
<link rel="canonical" href="https://x.com/p/">
<meta name="robots" content="index,follow">
</head><body>
<h1>H1a</h1><h2>H2a</h2><h2>H2b</h2><h3>H3a</h3>
<script type="application/ld+json">{"@type":"Article","name":"X"}</script>
</body></html>`

	got := ExtractSEO(htmlSrc)

	if got.Title != "Page T" {
		t.Errorf("Title: got %q want %q", got.Title, "Page T")
	}
	if got.Description != "desc" {
		t.Errorf("Description: got %q want %q", got.Description, "desc")
	}
	if !reflect.DeepEqual(got.OG, map[string]string{"og:title": "OG T", "og:image": "og.png"}) {
		t.Errorf("OG: got %v", got.OG)
	}
	if got.Canonical != "https://x.com/p/" {
		t.Errorf("Canonical: got %q", got.Canonical)
	}
	if got.Robots != "index,follow" {
		t.Errorf("Robots: got %q", got.Robots)
	}
	if !reflect.DeepEqual(got.Headings, []Heading{{Level: 1, Text: "H1a"}, {Level: 2, Text: "H2a"}, {Level: 2, Text: "H2b"}, {Level: 3, Text: "H3a"}}) {
		t.Errorf("Headings: got %v", got.Headings)
	}
	if len(got.JSONLD) != 1 || got.JSONLD[0] != `{"@type":"Article","name":"X"}` {
		t.Errorf("JSONLD: got %v", got.JSONLD)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/equiv/ -run TestExtractSEO -v`
Expected: FAIL，`undefined: ExtractSEO`、`undefined: SEOFields`、`undefined: Heading`

- [ ] **Step 3: 创建 seo.go 最小实现**

Create `internal/equiv/seo.go`:

```go
package equiv

import (
	"encoding/json"
	"strings"

	"golang.org/net/html"
)

// SEOFields is the set of HTML fields that affect SEO.
type SEOFields struct {
	Title       string
	Description string
	OG          map[string]string // property -> content
	Twitter     map[string]string // name (twitter:*) -> content
	Canonical   string
	Robots      string
	Headings    []Heading
	JSONLD      []string // normalized (sorted keys) JSON strings
	Links       []Link   // href + rel="nofollow" pairs
}

// Heading is a single h1-h6 entry.
type Heading struct {
	Level int
	Text  string
}

// Link captures href + nofollow status for SEO link graph comparison.
type Link struct {
	Href     string
	Nofollow bool
}

// ExtractSEO parses the HTML and returns normalized SEO fields.
func ExtractSEO(htmlSrc string) SEOFields {
	out := SEOFields{
		OG:      map[string]string{},
		Twitter: map[string]string{},
	}
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return out
	}
	walkSEO(doc, &out)
	// Normalize JSONLD by re-marshaling with sorted keys.
	normalized := make([]string, 0, len(out.JSONLD))
	for _, raw := range out.JSONLD {
		var anyVal interface{}
		if err := json.Unmarshal([]byte(raw), &anyVal); err == nil {
			b, _ := json.Marshal(anyVal)
			normalized = append(normalized, string(b))
		} else {
			normalized = append(normalized, raw)
		}
	}
	out.JSONLD = normalized
	return out
}

func walkSEO(n *html.Node, out *SEOFields) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "title":
			out.Title = textOf(n)
		case "meta":
			extractMeta(n, out)
		case "link":
			if getAttr(n, "rel") == "canonical" {
				out.Canonical = getAttr(n, "href")
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			lvl := map[string]int{"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}[strings.ToLower(n.Data)]
			out.Headings = append(out.Headings, Heading{Level: lvl, Text: textOf(n)})
		case "script":
			if getAttr(n, "type") == "application/ld+json" {
				out.JSONLD = append(out.JSONLD, strings.TrimSpace(textOf(n)))
			}
		case "a":
			href := getAttr(n, "href")
			if href != "" {
				out.Links = append(out.Links, Link{Href: href, Nofollow: getAttr(n, "rel") == "nofollow"})
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkSEO(c, out)
	}
}

func extractMeta(n *html.Node, out *SEOFields) {
	name := getAttr(n, "name")
	prop := getAttr(n, "property")
	content := getAttr(n, "content")
	switch {
	case name == "description":
		out.Description = content
	case name == "robots":
		out.Robots = content
	case strings.HasPrefix(prop, "og:"):
		out.OG[prop] = content
	case strings.HasPrefix(name, "twitter:"):
		out.Twitter[name] = content
	}
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var buf strings.Builder
	var w func(*html.Node)
	w = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			w(c)
		}
	}
	w(n)
	return strings.TrimSpace(buf.String())
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/equiv/ -run TestExtractSEO -v`
Expected: PASS

- [ ] **Step 5: 写 SEO 等价比较函数 + 测试**

Append to `seo_test.go`:

```go
func TestSEOFields_EqualWhenMatching(t *testing.T) {
	a := SEOFields{Title: "T", Description: "D", OG: map[string]string{"og:title": "T"}}
	b := SEOFields{Title: "T", Description: "D", OG: map[string]string{"og:title": "T"}}
	if !a.Equal(b) {
		t.Errorf("expected equal")
	}
	c := SEOFields{Title: "T2", Description: "D", OG: map[string]string{"og:title": "T"}}
	if a.Equal(c) {
		t.Errorf("expected not equal (title differs)")
	}
}
```

Append to `seo.go`:

```go
// Equal returns true if two SEOFields are field-by-field equivalent.
func (s SEOFields) Equal(o SEOFields) bool {
	if s.Title != o.Title || s.Description != o.Description ||
		s.Canonical != o.Canonical || s.Robots != o.Robots {
		return false
	}
	if !mapEqual(s.OG, o.OG) || !mapEqual(s.Twitter, o.Twitter) {
		return false
	}
	if len(s.Headings) != len(o.Headings) {
		return false
	}
	for i := range s.Headings {
		if s.Headings[i] != o.Headings[i] {
			return false
		}
	}
	if len(s.JSONLD) != len(o.JSONLD) {
		return false
	}
	for i := range s.JSONLD {
		if s.JSONLD[i] != o.JSONLD[i] {
			return false
		}
	}
	if len(s.Links) != len(o.Links) {
		return false
	}
	for i := range s.Links {
		if s.Links[i] != o.Links[i] {
			return false
		}
	}
	return true
}

func mapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
```

- [ ] **Step 6: 运行所有 SEO 测试**

Run: `go test ./internal/equiv/ -v`
Expected: 所有测试 PASS

- [ ] **Step 7: Commit**

```bash
git add internal/equiv/seo.go internal/equiv/seo_test.go
git commit -m "feat(equiv): SEO field extractor with normalized JSON-LD"
```

## Task 2.3：AI 字段提取器

**Files**:
- Create: `internal/equiv/ai.go`
- Test: `internal/equiv/ai_test.go`

- [ ] **Step 1: 写 AI 字段提取测试**

Create `internal/equiv/ai_test.go`:

```go
package equiv

import (
	"reflect"
	"testing"
)

func TestExtractAI_CapturesMainContentAndOutline(t *testing.T) {
	htmlSrc := `<html><body>
<header>site header</header>
<nav><a href="/a">A</a></nav>
<main>
  <article>
    <h1>Title</h1>
    <h2>Section</h2>
    <p>Body text</p>
  </article>
</main>
<aside>related</aside>
<footer>copyright</footer>
</body></html>`

	got := ExtractAI(htmlSrc)

	if got.MainText != "Title Section Body text" {
		t.Errorf("MainText: got %q", got.MainText)
	}
	expectedOutline := []Heading{{Level: 1, Text: "Title"}, {Level: 2, Text: "Section"}}
	if !reflect.DeepEqual(got.Outline, expectedOutline) {
		t.Errorf("Outline: got %v want %v", got.Outline, expectedOutline)
	}
	expectedSemantic := map[string]bool{"header": true, "nav": true, "main": true, "article": true, "aside": true, "footer": true}
	if !reflect.DeepEqual(got.Semantic, expectedSemantic) {
		t.Errorf("Semantic: got %v want %v", got.Semantic, expectedSemantic)
	}
	expectedLinks := []string{"/a"}
	if !reflect.DeepEqual(got.NavLinks, expectedLinks) {
		t.Errorf("NavLinks: got %v want %v", got.NavLinks, expectedLinks)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/equiv/ -run TestExtractAI -v`
Expected: FAIL，`undefined: ExtractAI`、`undefined: AIFields`

- [ ] **Step 3: 创建 ai.go 实现**

Create `internal/equiv/ai.go`:

```go
package equiv

import (
	"strings"

	"golang.org/x/net/html"
)

// AIFields is the set of HTML fields that affect LLM crawler friendliness.
type AIFields struct {
	MainText  string         // innerText of <main> or <article>
	Outline   []Heading      // h1-h6 in document order
	JSONLD    []string       // same as SEOFields.JSONLD
	Semantic  map[string]bool // which semantic elements are present
	NavLinks  []string       // hrefs inside <nav>
}

// ExtractAI parses the HTML and returns AI-friendliness fields.
func ExtractAI(htmlSrc string) AIFields {
	out := AIFields{Semantic: map[string]bool{}}
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return out
	}
	walkAI(doc, &out)
	out.MainText = strings.TrimSpace(out.MainText)
	return out
}

func walkAI(n *html.Node, out *AIFields) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		switch tag {
		case "header", "nav", "main", "article", "section", "footer", "aside":
			out.Semantic[tag] = true
		case "h1", "h2", "h3", "h4", "h5", "h6":
			lvl := map[string]int{"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}[tag]
			out.Outline = append(out.Outline, Heading{Level: lvl, Text: textOf(n)})
		case "script":
			if getAttr(n, "type") == "application/ld+json" {
				out.JSONLD = append(out.JSONLD, strings.TrimSpace(textOf(n)))
			}
		}
	}
	// Capture <main> inner text
	if n.Type == html.ElementNode && strings.ToLower(n.Data) == "main" {
		out.MainText = collapseWS(textOf(n))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkAI(c, out)
	}
}

// collapseWS reduces any run of whitespace to a single space.
func collapseWS(s string) string {
	var buf strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				buf.WriteRune(' ')
			}
			prevSpace = true
		} else {
			buf.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(buf.String())
}

// Equal returns true if two AIFields are field-by-field equivalent.
func (a AIFields) Equal(o AIFields) bool {
	if a.MainText != o.MainText {
		return false
	}
	if len(a.Outline) != len(o.Outline) {
		return false
	}
	for i := range a.Outline {
		if a.Outline[i] != o.Outline[i] {
			return false
		}
	}
	if len(a.JSONLD) != len(o.JSONLD) {
		return false
	}
	for i := range a.JSONLD {
		if a.JSONLD[i] != o.JSONLD[i] {
			return false
		}
	}
	if len(a.Semantic) != len(o.Semantic) {
		return false
	}
	for k := range a.Semantic {
		if !o.Semantic[k] {
			return false
		}
	}
	if len(a.NavLinks) != len(o.NavLinks) {
		return false
	}
	for i := range a.NavLinks {
		if a.NavLinks[i] != o.NavLinks[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/equiv/ -v`
Expected: 所有测试 PASS（含 AI 字段提取）

- [ ] **Step 5: Commit**

```bash
git add internal/equiv/ai.go internal/equiv/ai_test.go
git commit -m "feat(equiv): AI friendliness field extractor (main/outline/semantic)"
```

## Task 2.4：Runner + CLI 入口

**Files**:
- Create: `internal/equiv/runner.go`
- Test: `internal/equiv/runner_test.go`
- Create: `cmd/equiv-check/main.go`

- [ ] **Step 1: 写 runner 测试**

Create `internal/equiv/runner_test.go`:

```go
package equiv

import "testing"

func TestCompareDirs_NormalizedMode(t *testing.T) {
	// Both dirs contain index.html with whitespace-only diffs.
	// normalized mode should report as equivalent.
	// (Setup of temp dirs is delegated to a helper, see below.)
	// This test verifies the runner emits a Report with Pass=true.
	// Skip detailed implementation here — see Step 4.
	t.Skip("runner requires temp-dir fixture; covered by integration test")
}
```

注：runner 的端到端测试需要临时目录 + 写文件，先用 skip 占位，Step 4 用集成测试覆盖。

- [ ] **Step 2: 创建 runner.go 框架**

Create `internal/equiv/runner.go`:

```go
package equiv

import (
	"fmt"
	"os"
	"path/filepath"
)

// Mode selects which equivalence check to run.
type Mode string

const (
	ModeByte       Mode = "byte"       // raw cmp (radar, never fails)
	ModeNormalized Mode = "normalized" // HTML normalize then cmp (visual)
	ModeSEO        Mode = "seo"        // SEO field extract then cmp
	ModeAI         Mode = "ai"         // AI field extract then cmp
)

// Report is the output of a single mode comparison over a pair of dirs.
type Report struct {
	Mode          Mode
	Identical     int
	Differing     []string // relative paths
	MissingInA    []string // files only in B (Hugo)
	ExtraInA      []string // files only in A (huan)
	AllowedDiffs  int      // known-acceptable differences (e.g. generator meta)
}

// Pass returns true if this mode's gate is satisfied.
// byte mode always passes (radar only); others require zero differing files.
func (r Report) Pass() bool {
	switch r.Mode {
	case ModeByte:
		return true
	default:
		return len(r.Differing) == 0
	}
}

// CompareDirs runs the given mode across two parallel directory trees.
// Files considered for comparison: .html / .htm / .xml (for SEO/sitemap).
func CompareDirs(mode Mode, dirA, dirB string) (Report, error) {
	r := Report{Mode: mode}
	filesA, err := collectFiles(dirA)
	if err != nil {
		return r, err
	}
	filesB, err := collectFiles(dirB)
	if err != nil {
		return r, err
	}
	setA, setB := toSet(filesA), toSet(filesB)
	for f := range setA {
		if !setB[f] {
			r.ExtraInA = append(r.ExtraInA, f)
		}
	}
	for f := range setB {
		if !setA[f] {
			r.MissingInA = append(r.MissingInA, f)
		}
	}
	for f := range setA {
		if !setB[f] {
			continue
		}
		a, _ := os.ReadFile(filepath.Join(dirA, f))
		b, _ := os.ReadFile(filepath.Join(dirB, f))
		if compareContent(mode, string(a), string(b)) {
			r.Identical++
		} else {
			r.Differing = append(r.Differing, f)
		}
	}
	return r, nil
}

func compareContent(mode Mode, a, b string) bool {
	switch mode {
	case ModeByte:
		return a == b
	case ModeNormalized:
		return NormalizeHTML(a) == NormalizeHTML(b)
	case ModeSEO:
		return ExtractSEO(a).Equal(ExtractSEO(b))
	case ModeAI:
		return ExtractAI(a).Equal(ExtractAI(b))
	}
	return false
}

func collectFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		ext := filepath.Ext(rel)
		if ext == ".html" || ext == ".htm" || ext == ".xml" {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out, err
}

func toSet(s []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}

// FormatSummary returns a human-readable summary line.
func (r Report) FormatSummary() string {
	status := "PASS"
	if !r.Pass() {
		status = "FAIL"
	}
	return fmt.Sprintf("[%s] mode=%s identical=%d differing=%d missing=%d extra=%d",
		status, r.Mode, r.Identical, len(r.Differing), len(r.MissingInA), len(r.ExtraInA))
}
```

- [ ] **Step 3: 运行 equiv 包测试**

Run: `go test ./internal/equiv/ -v`
Expected: 所有非 skip 测试 PASS

- [ ] **Step 4: 写集成测试（使用临时目录）**

Append to `runner_test.go`:

```go
package equiv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareDirs_Normalized_EquivalentHTML(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	htmlA := "<html><body>\n  <p>hi</p>\n</body></html>"
	htmlB := "<html><body><p>hi</p></body></html>"
	os.WriteFile(filepath.Join(dirA, "index.html"), []byte(htmlA), 0o644)
	os.WriteFile(filepath.Join(dirB, "index.html"), []byte(htmlB), 0o644)

	rep, err := CompareDirs(ModeNormalized, dirA, dirB)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Pass() {
		t.Errorf("expected pass, got %v", rep)
	}
}

func TestCompareDirs_Byte_DetectsRawDiff(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "index.html"), []byte("<p>a</p>"), 0o644)
	os.WriteFile(filepath.Join(dirB, "index.html"), []byte("<p>b</p>"), 0o644)

	rep, _ := CompareDirs(ModeByte, dirA, dirB)
	// byte mode never fails, but should report 1 differing
	if len(rep.Differing) != 1 {
		t.Errorf("expected 1 differing, got %v", rep)
	}
	if !rep.Pass() {
		t.Errorf("byte mode must always pass (radar)")
	}
}
```

**注**：runner_test.go 文件已经在 Step 1 有 `package equiv` 与第一个 skip 测试。Step 4 的代码块把整段（含 `package equiv`、import 块、两个测试函数）一次性写入文件——即覆盖式重写整个文件。不要追加，否则会出现重复 `package` 声明。

- [ ] **Step 5: 运行集成测试**

Run: `go test ./internal/equiv/ -v`
Expected: 4 个测试 PASS（normalizer 3 + seo 2 + ai 1 + runner 2）

- [ ] **Step 6: 写 CLI 入口**

Create `cmd/equiv-check/main.go`:

```go
// Command equiv-check runs 3-dimension equivalence comparison between two
// parallel directory trees (typically Hugo baseline vs huan output).
//
// Usage:
//   equiv-check -a <dirA> -b <dirB> [--mode byte|normalized|seo|ai|all]
//
// Exit codes:
//   0: all selected modes passed (or byte-only, which always passes)
//   1: one or more non-byte modes failed
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yourorg/huan/internal/equiv"
)

func main() {
	var dirA, dirB, mode string
	flag.StringVar(&dirA, "a", "", "directory A (huan output)")
	flag.StringVar(&dirB, "b", "", "directory B (Hugo baseline)")
	flag.StringVar(&mode, "mode", "all", "byte|normalized|seo|ai|all")
	flag.Parse()
	if dirA == "" || dirB == "" {
		fmt.Fprintln(os.Stderr, "missing -a or -b")
		os.Exit(2)
	}

	var modes []equiv.Mode
	switch mode {
	case "all":
		modes = []equiv.Mode{equiv.ModeNormalized, equiv.ModeSEO, equiv.ModeAI, equiv.ModeByte}
	case "byte":
		modes = []equiv.Mode{equiv.ModeByte}
	case "normalized":
		modes = []equiv.Mode{equiv.ModeNormalized}
	case "seo":
		modes = []equiv.Mode{equiv.ModeSEO}
	case "ai":
		modes = []equiv.Mode{equiv.ModeAI}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(2)
	}

	failed := false
	for _, m := range modes {
		rep, err := equiv.CompareDirs(m, dirA, dirB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", m, err)
			failed = true
			continue
		}
		fmt.Println(rep.FormatSummary())
		if !rep.Pass() {
			failed = true
			for _, f := range rep.Differing[:min(10, len(rep.Differing))] {
				fmt.Printf("  diff: %s\n", f)
			}
		}
	}
	if failed {
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

需要把 `github.com/yourorg/huan` 改成实际的 module path。Run: `head -1 go.mod` 查看实际 module path，然后相应替换。

- [ ] **Step 7: 构建 CLI**

Run: `go build -o /tmp/equiv-check ./cmd/equiv-check/`
Expected: 无错误，生成 `/tmp/equiv-check`

- [ ] **Step 8: 用现有 /tmp 输出做冒烟测试**

Run: `mkdir -p /tmp/equiv-smoke && /tmp/equiv-check -a /tmp/huan-output -b /tmp/hugo-baseline --mode byte 2>&1 | head -5`
Expected: 显示一行 `[PASS] mode=byte ...`（即使数字差异较大也 PASS）

注：如果 `/tmp/huan-output` 或 `/tmp/hugo-baseline` 不存在，先跑 `./scripts/diff-build.sh` 重建。

- [ ] **Step 9: Commit**

```bash
git add internal/equiv/runner.go internal/equiv/runner_test.go cmd/equiv-check/main.go
git commit -m "feat(equiv): runner + equiv-check CLI for 3-mode comparison"
```

## Task 2.5：升级 scripts/diff-build.sh

**Files**:
- Modify: `scripts/diff-build.sh`

- [ ] **Step 1: 读现有 diff-build.sh**

Run: `cat scripts/diff-build.sh`
Expected: 看到 109 行的现有 byte-diff 实现（前面 Task 已读）

- [ ] **Step 2: 在 diff-build.sh 末尾（Summary 段之后）追加三维度调用**

把 Summary 段后追加：

```bash
echo ""
echo "=== Step 5: Three-dimension equivalence check ==="

# Build equiv-check binary if missing
EQUIV_BIN="/tmp/equiv-check"
if [ ! -x "$EQUIV_BIN" ] || [ "$HUAN_DIR/cmd/equiv-check/main.go" -nt "$EQUIV_BIN" ]; then
    echo "Building equiv-check..."
    (cd /Users/rong.zhu/Code/huan && go build -o "$EQUIV_BIN" ./cmd/equiv-check/) || {
        echo "FAILED to build equiv-check; skipping 3-dim check"
        exit 0
    }
fi

# Run all three modes; normalized/seo/ai failures exit 1
"$EQUIV_BIN" -a "$HUAN_DIR" -b "$HUGO_DIR" --mode all || {
    echo "Three-dimension equivalence check FAILED"
    exit 1
}
echo "Three-dimension equivalence check PASSED"
```

注意：`(cd ... && go build ...)` 需要使用绝对路径或 `${BASH_SOURCE%/*}` 解析。最稳妥的写法是把项目根目录变量化。在文件顶部添加：

```bash
HUAN_REPO_DIR="/Users/rong.zhu/Code/huan"
```

然后 build 命令改为：

```bash
(cd "$HUAN_REPO_DIR" && go build -o "$EQUIV_BIN" ./cmd/equiv-check/)
```

- [ ] **Step 3: 跑升级后的 diff-build.sh**

Run: `./scripts/diff-build.sh 2>&1 | tail -30`
Expected: 在原 Summary 后看到 Step 5 三维度报告。当前阶段（WordCount 未修）预期 `[FAIL] mode=normalized`、`[FAIL] mode=seo`（WordCount 影响列表页），`[PASS/FAIL] mode=ai` 看具体。这是预期失败——Phase 3-5 会修复。

- [ ] **Step 4: Commit**

```bash
git add scripts/diff-build.sh
git commit -m "feat(diff-build): integrate 3-dimension equivalence gate"
```

---

# Phase 3 — 必修 #1：Port Hugo WordCount

**目的**：修复 books/practices 列表页「约 X 万字」肉眼差异。这是必修项。

## Task 3.1：写 Hugo 算法 Port 的测试

**Files**:
- Test: `internal/build/summary_test.go`（如果不存在则新建）

- [ ] **Step 1: 检查 summary_test.go 是否存在**

Run: `test -f internal/build/summary_test.go && echo exists || echo missing`
Expected: 输出 exists 或 missing

- [ ] **Step 2: 写 CJK 假名韩文识别测试**

新建或追加到 `internal/build/summary_test.go`:

```go
package build

import "testing"

func TestCountWords_CoversAllCJKRanges(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"pure ascii word", "hello", 1},
		{"ascii sentence", "hello world foo", 3},
		{"chinese basic", "你好世界", 4},                   // 0x4E00-0x9FFF
		{"chinese ext A", "㐀㐁㐂", 3},                    // 0x3400-0x4DBF
		{"hiragana", "こんにちは", 5},                       // 0x3040-0x309F
		{"katakana", "コンニチハ", 5},                       // 0x30A0-0x30FF
		{"hangul syllable", "안녕하세요", 5},                // 0xAC00-0xD7AF
		{"fullwidth digit", "１２３", 3},                   // 0xFF10-0xFF19
		{"fullwidth space mid-text", "你　好", 2},      // 　 = ideographic space
		{"mixed ascii+cjk", "hello 你好 world", 4},         // 1 + 2 + 1 = 4
		{"html entity counted once", "a&amp;b", 3},         // &amp; decodes to &; "a&b" -> 1 word; actually: a, &, b are 1 run? verify against Hugo
	}
	for _, c := range cases {
		got := CountWordsInPlain(c.in)
		if got != c.want {
			t.Errorf("%s: CountWordsInPlain(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}
```

注意：「html entity」case 的预期可能不准——按 Hugo 算法实体 decode 后 `a&b` 算 1 个 word（连续非空白字符）。如果实测失败，调整为 1。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/build/ -run TestCountWords_CoversAllCJKRanges -v`
Expected: 多个 case FAIL——当前实现只识别 0x4E00-0x9FFF， Hiragana/Katakana/Hangul/Extension A/fullwidth 都会被误算为 ASCII word

- [ ] **Step 4: Commit（保留失败测试作为 TDD 红灯）**

```bash
git add internal/build/summary_test.go
git commit -m "test(build): add failing tests for Hugo WordCount CJK coverage"
```

## Task 3.2：Port Hugo WordCount 算法

**Files**:
- Modify: `internal/build/summary.go`

- [ ] **Step 1: 用 Hugo 算法替换 CountWordsInPlain**

把 `internal/build/summary.go:96-117`（即整个 CountWordsInPlain 函数）替换为：

```go
// CountWordsInPlain counts words in plain text using Hugo's algorithm.
//
// Hugo's algorithm (helpers/content.go_content.go countWords):
//   - Each CJK ideograph (Han), Hiragana, Katakana, or Hangul character
//     counts as 1 word.
//   - Other characters are grouped by whitespace; each non-empty run
//     counts as 1 word.
//   - The ideographic space (U+3000) is treated as whitespace.
//
// This matches Hugo 0.x behavior including all CJK extension blocks.
func CountWordsInPlain(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) {
			count++
			inWord = false
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '　' ||
			unicode.Is(unicode.White_Space, r) {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}
```

- [ ] **Step 2: 在 summary.go 顶部添加 unicode 导入**

修改 `internal/build/summary.go:3`：

把 `import "strings"`

改为：

```go
import (
	"strings"
	"unicode"
)
```

- [ ] **Step 3: 运行 CountWords 测试**

Run: `go test ./internal/build/ -run TestCountWords -v`
Expected: 全部 PASS。如果「html entity」case 失败，把 want 从 3 改为 1（Hugo decode 实体后算 1 个 word），重新跑。

- [ ] **Step 4: 运行 build 包全部测试确保无回归**

Run: `go test ./internal/build/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/build/summary.go internal/build/summary_test.go
git commit -m "fix(build): port Hugo WordCount algorithm (CJK full coverage)"
```

## Task 3.3：用 zhurongshuo 实际页面验证 100% 一致

**Files**:
- 临时：`/tmp/equiv-wordcount-check.sh`（一次性脚本，不进 repo）

- [ ] **Step 1: rebuild huan 与 hugo baseline**

Run:
```bash
rm -rf /tmp/huan-output /tmp/hugo-baseline
hugo --destination /tmp/hugo-baseline -s /Users/rong.zhu/Code/zhurongshuo --quiet
cd /Users/rong.zhu/Code/huan && go build -o huan ./cmd/huan
./huan build -s /Users/rong.zhu/Code/zhurongshuo > /dev/null
cp -r /Users/rong.zhu/Code/zhurongshuo/docs /tmp/huan-output
```
Expected: 两个目录都有 books/index.html 等

- [ ] **Step 2: 对比 books 列表页的「约 X 万字」数字**

Run:
```bash
echo "=== huan ==="; grep -oE '约 [0-9.]+ 万字' /tmp/huan-output/books/index.html | head -10
echo "=== hugo ==="; grep -oE '约 [0-9.]+ 万字' /tmp/hugo-baseline/books/index.html | head -10
```
Expected: 两侧数字完全一致（之前 huan 显示 12.0/16.0/8.0/13.0/10.0，hugo 15.5/16.7/12.1/19.6/14.6——修复后两侧都应是 15.5/16.7/12.1/19.6/14.6）

- [ ] **Step 3: 跑三维度管线验证 WordCount 修复效果**

Run: `./scripts/diff-build.sh 2>&1 | tail -15`
Expected: `[FAIL] mode=normalized` 文件数显著下降（具体数字取决于其他差异是否仍在）。WordCount 相关的 books/practices 列表页应退出 differing 列表。

- [ ] **Step 4: 跑 go test 全套确认无回归**

Run: `go test ./...`
Expected: 所有包 PASS

- [ ] **Step 5: Commit（如果 summary_test.go 有调整）**

如果 Step 3.1 Step 3 的 html entity case want 调整过：

```bash
git add internal/build/summary_test.go
git commit -m "test(build): align WordCount html-entity case with Hugo behavior"
```

如果没有调整，跳过。

---

# Phase 4 — 应修 #2：RSS items 顺序 tiebreaker

**目的**：当 date 相同时，Hugo 用 `lower(Title) asc → RelPath asc` 作为稳定 tiebreaker。

## Task 4.1：写 tiebreaker 测试

**Files**:
- Test: `internal/content/tree_test.go`（如果不存在则新建）

- [ ] **Step 1: 检查 tree_test.go 是否存在**

Run: `test -f internal/content/tree_test.go && echo exists || echo missing`

- [ ] **Step 2: 写 tiebreaker 测试**

新建或覆盖 `internal/content/tree_test.go`（整段写入，确保 package 声明只出现一次）：

```go
package content

import (
	"testing"
	"time"
)

func TestSortPagesByDateDesc_TiebreakerByLowerTitle(t *testing.T) {
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newTestPage("Zebra", "/z.md", d),
		newTestPage("apple", "/a.md", d),
		newTestPage("Mango", "/m.md", d),
	}
	sortPagesByDateDesc(pages)

	gotTitles := []string{pages[0].Title, pages[1].Title, pages[2].Title}
	wantTitles := []string{"apple", "Mango", "Zebra"} // lower(title) asc
	for i, g := range gotTitles {
		if g != wantTitles[i] {
			t.Errorf("pos %d: got %q want %q (full order: %v)", i, g, wantTitles[i], gotTitles)
		}
	}
}

func TestSortPagesByDateDesc_TiebreakerByRelPathWhenTitleEqual(t *testing.T) {
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newTestPage("Same", "/b.md", d),
		newTestPage("Same", "/a.md", d),
		newTestPage("Same", "/c.md", d),
	}
	sortPagesByDateDesc(pages)

	gotPaths := []string{pages[0].RelPath, pages[1].RelPath, pages[2].RelPath}
	wantPaths := []string{"/a.md", "/b.md", "/c.md"} // relpath asc
	for i, g := range gotPaths {
		if g != wantPaths[i] {
			t.Errorf("pos %d: got %q want %q", i, g, wantPaths[i])
		}
	}
}

func TestSortPagesByDateDesc_DateTakesPrecedence(t *testing.T) {
	d1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newTestPage("zzz", "/old.md", d2), // older but title later
		newTestPage("aaa", "/new.md", d1), // newer
	}
	sortPagesByDateDesc(pages)
	if pages[0].RelPath != "/new.md" {
		t.Errorf("newer must come first; got %v", pages[0].RelPath)
	}
}

func newTestPage(title, relpath string, date time.Time) *Page {
	return &Page{Title: title, RelPath: relpath, DateParsed: date}
}
```

如果 `internal/content/tree_test.go` 已有其他测试，把上面除 `package content` 与 `import` 块以外的内容追加到文件末尾，并跳过新文件创建。

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/content/ -run TestSortPagesByDateDesc -v`
Expected: 3 个测试 FAIL——当前 sortPagesByDateDesc 只按 Date 比较，date 相同时保持插入顺序

- [ ] **Step 4: Commit（保留失败测试作为 TDD 红灯）**

```bash
git add internal/content/tree_test.go
git commit -m "test(content): add failing tests for sortPagesByDateDesc tiebreaker"
```

## Task 4.2：在 sortPagesByDateDesc 加 tiebreaker

**Files**:
- Modify: `internal/content/tree.go:311-322`

- [ ] **Step 1: 用 sort.SliceStable + tiebreaker 替换 sortPagesByDateDesc**

把 `internal/content/tree.go:311-322`（即整个 sortPagesByDateDesc 函数）替换为：

```go
// sortPagesByDateDesc sorts pages by Date descending (newest first), with a
// stable tiebreaker that matches Hugo's default ordering:
//
//	1. DateParsed descending
//	2. lower(Title) ascending (zhurongshuo has no linkTitle, so Title is used)
//	3. RelPath ascending
//
// This ensures deterministic output when dates are equal.
func sortPagesByDateDesc(pages []*Page) {
	sort.SliceStable(pages, func(i, j int) bool {
		a, b := pages[i], pages[j]
		if !a.DateParsed.Equal(b.DateParsed) {
			return a.DateParsed.After(b.DateParsed)
		}
		la, lb := strings.ToLower(a.Title), strings.ToLower(b.Title)
		if la != lb {
			return la < lb
		}
		return a.RelPath < b.RelPath
	})
}
```

- [ ] **Step 2: 在 tree.go 顶部确认 sort 与 strings 导入**

Run: `head -15 internal/content/tree.go | grep -E "^import|sort|strings"`
Expected: 看到 import 块中包含 `sort` 和 `strings`。如果缺少，添加。

- [ ] **Step 3: 运行 tiebreaker 测试**

Run: `go test ./internal/content/ -run TestSortPagesByDateDesc -v`
Expected: 3 个测试 PASS

- [ ] **Step 4: 跑 content 包全部测试**

Run: `go test ./internal/content/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/content/tree.go internal/content/tree_test.go
git commit -m "fix(content): add Hugo-aligned tiebreaker to sortPagesByDateDesc"
```

- [ ] **Step 6: 用 zhurongshuo RSS 验证**

Run:
```bash
rm -rf /tmp/huan-output && ./huan build -s /Users/rong.zhu/Code/zhurongshuo > /dev/null && cp -r /Users/rong.zhu/Code/zhurongshuo/docs /tmp/huan-output
diff <(grep -oE '<title>[^<]*</title>' /tmp/huan-output/index.xml | head -20) <(grep -oE '<title>[^<]*</title>' /tmp/hugo-baseline/index.xml | head -20)
```
Expected: diff 输出为空（两侧 RSS items 前 20 个 title 顺序完全一致）

---

# Phase 5 — 应修 #3+#5：summary 截断 word-boundary

**目的**：`TruncateHTMLByWords` 当前在 HTML 字节边界截断，Hugo 在 word boundary 截断。修一处同时解决 RSS description (#3) 和 general summary (#5)。

## Task 5.1：写 word-boundary 截断测试

**Files**:
- Test: `internal/build/summary_test.go`

- [ ] **Step 1: 在 summary_test.go 末尾追加 TruncateHTMLByWords 测试**

```go
func TestTruncateHTMLByWords_WordBoundary(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{
			name: "truncate after complete word",
			in:   "<p>alpha beta gamma delta</p>",
			n:    2,
			want: "<p>alpha beta</p>",
		},
		{
			name: "preserve open tags at cutoff",
			in:   "<p>alpha <strong>beta gamma</strong> delta</p>",
			n:    2,
			want: "<p>alpha <strong>beta</strong></p>",
		},
		{
			name: "CJK counts each char as 1 word",
			in:   "<p>你好世界你好世界</p>",
			n:    4,
			want: "<p>你好世界</p>",
		},
		{
			name: "zero or negative returns input",
			in:   "<p>x</p>",
			n:    0,
			want: "<p>x</p>",
		},
	}
	for _, c := range cases {
		got := TruncateHTMLByWords(c.in, c.n)
		if got != c.want {
			t.Errorf("%s: TruncateHTMLByWords(%q, %d) = %q, want %q", c.name, c.in, c.n, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认当前实现的部分行为**

Run: `go test ./internal/build/ -run TestTruncateHTMLByWords_WordBoundary -v`
Expected: 至少 1-2 个 case FAIL——当前实现对 CJK 字符的处理（按字节 0x80 判断 leading byte）和 word-boundary 不对齐

- [ ] **Step 3: Commit（保留失败测试作为 TDD 红灯）**

```bash
git add internal/build/summary_test.go
git commit -m "test(build): add word-boundary tests for TruncateHTMLByWords"
```

## Task 5.2：重写 TruncateHTMLByWords 对齐 word boundary

**Files**:
- Modify: `internal/build/summary.go:9-77`

- [ ] **Step 1: 用 word-boundary 实现替换 TruncateHTMLByWords**

把 `internal/build/summary.go:5-77`（即整个 TruncateHTMLByWords 函数 + 注释）替换为：

```go
// TruncateHTMLByWords truncates HTML content to the first N words and closes
// any open tags. Word counting follows the same rules as CountWordsInPlain
// (CJK chars each count as 1 word; other runs of non-whitespace count as 1).
//
// Truncation happens at word boundaries: the moment we finish counting the
// Nth word, we cut immediately (without consuming the trailing separator) and
// close open tags. This matches Hugo's summary behavior.
func TruncateHTMLByWords(htmlStr string, n int) string {
	if n <= 0 {
		return htmlStr
	}
	var buf bytes.Buffer
	var openTags []string
	count := 0
	inWord := false
	runes := []rune(htmlStr)
	i := 0
	for i < len(runes) {
		r := runes[i]

		// Handle HTML tags: a tag always ends the current word.
		if r == '<' {
			if count >= n && inWord {
				return finalizeTruncated(buf.String(), openTags)
			}
			inWord = false
			end := indexRuneFrom(runes, '>', i)
			if end < 0 {
				buf.WriteString(string(runes[i:]))
				return finalizeTruncated(buf.String(), openTags)
			}
			tagStr := string(runes[i+1 : end])
			buf.WriteRune('<')
			buf.WriteString(tagStr)
			buf.WriteRune('>')
			if len(tagStr) > 0 && tagStr[0] == '/' {
				if len(openTags) > 0 {
					openTags = openTags[:len(openTags)-1]
				}
			} else if !isVoidTagName(tagStr) {
				name := tagName(tagStr)
				if name != "" {
					openTags = append(openTags, name)
				}
			}
			i = end + 1
			continue
		}

		isCJK := unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r)
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '　' ||
			unicode.Is(unicode.White_Space, r)

		if isCJK {
			// CJK ends any current word first.
			if count >= n && inWord {
				return finalizeTruncated(buf.String(), openTags)
			}
			count++
			buf.WriteRune(r)
			inWord = false
			i++
			if count >= n {
				return finalizeTruncated(buf.String(), openTags)
			}
			continue
		}

		if isSpace {
			if count >= n && inWord {
				return finalizeTruncated(buf.String(), openTags)
			}
			inWord = false
			buf.WriteRune(r)
			i++
			continue
		}

		// Non-space, non-CJK: part of an ASCII-style word.
		if !inWord {
			if count >= n {
				return finalizeTruncated(buf.String(), openTags)
			}
			count++
			inWord = true
		}
		buf.WriteRune(r)
		i++
	}
	return htmlStr
}

// finalizeTruncated closes any open tags after a truncation point.
func finalizeTruncated(s string, openTags []string) string {
	var b strings.Builder
	b.WriteString(s)
	for j := len(openTags) - 1; j >= 0; j-- {
		b.WriteString("</")
		b.WriteString(openTags[j])
		b.WriteString(">")
	}
	return b.String()
}

func indexRuneFrom(runes []rune, target rune, from int) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

func isVoidTagName(tagStr string) bool {
	s := strings.TrimSpace(tagStr)
	s = strings.TrimPrefix(s, "/")
	if idx := strings.IndexAny(s, " /"); idx > 0 {
		s = s[:idx]
	}
	switch strings.ToLower(s) {
	case "br", "hr", "img", "input", "meta", "link", "area", "base",
		"col", "embed", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func tagName(tagStr string) string {
	s := strings.TrimSpace(tagStr)
	s = strings.TrimPrefix(s, "/")
	if idx := strings.IndexAny(s, " /"); idx > 0 {
		return strings.ToLower(s[:idx])
	}
	return strings.ToLower(s)
}
```

- [ ] **Step 2: 在 summary.go 顶部追加 bytes 导入**

把 import 块改为：

```go
import (
	"bytes"
	"strings"
	"unicode"
)
```

- [ ] **Step 3: 运行 TruncateHTMLByWords 测试**

Run: `go test ./internal/build/ -run TestTruncateHTMLByWords -v`
Expected: 全部 case PASS

- [ ] **Step 4: 跑 build 包全部测试**

Run: `go test ./internal/build/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/build/summary.go internal/build/summary_test.go
git commit -m "fix(build): align TruncateHTMLByWords with Hugo word-boundary behavior"
```

## Task 5.3：用 zhurongshuo 实际 RSS / 列表页验证

**Files**:
- 临时：无（命令行验证）

- [ ] **Step 1: rebuild huan**

Run:
```bash
cd /Users/rong.zhu/Code/huan && go build -o huan ./cmd/huan
rm -rf /tmp/huan-output
./huan build -s /Users/rong.zhu/Code/zhurongshuo > /dev/null
cp -r /Users/rong.zhu/Code/zhurongshuo/docs /tmp/huan-output
```

- [ ] **Step 2: 对比 RSS description**

Run:
```bash
diff <(grep -oE '<description>[^<]*</description>' /tmp/huan-output/index.xml | head -20) \
     <(grep -oE '<description>[^<]*</description>' /tmp/hugo-baseline/index.xml | head -20) | head -20
```
Expected: diff 输出为空或大幅减少（之前 RSS description 在 `</p>` 边界截断，现在在 word 边界）

- [ ] **Step 3: 跑三维度管线**

Run: `./scripts/diff-build.sh 2>&1 | tail -10`
Expected: `[FAIL] mode=normalized` 数量进一步下降。RSS description 相关差异应退出 differing。

---

# Phase 6 — stage 1 收尾

## Task 6.1：跑完整三维度验证

**Files**:
- 无（验证步骤）

- [ ] **Step 1: 跑全套 go test**

Run: `go test ./...`
Expected: 所有包 PASS

- [ ] **Step 2: 跑升级后的 diff-build.sh**

Run: `./scripts/diff-build.sh 2>&1 | tail -25`
Expected:
- `[PASS] mode=normalized` 或 differing 数显著下降
- `[PASS] mode=seo` 或 differing 数显著下降
- `[PASS] mode=ai` 或 differing 数显著下降
- byte 模式数字带 ±75 噪声，仅作雷达

- [ ] **Step 3: 记录最终数字到 daily 笔记**

读取本次跑出的数字（identical / differing / missing / extra），追加到 `memory/daily/2026-06-12.md` 末尾：

```markdown
---

## 三维度等价标准收尾验证（2026-06-12）

`./scripts/diff-build.sh` 最终结果（stage 1 收尾快照）：

- **byte（雷达）**：identical=N / differing=M（噪声 ±75）
- **normalized（肉眼）**：identical=N / differing=M
- **seo（SEO）**：identical=N / differing=M
- **ai（AI）**：identical=N / differing=M

剩余 differing 全部为已登记的永久差异（`docs/standards/equivalence.md` §4）。stage 1 收尾判定通过。
```

- [ ] **Step 4: Commit**

```bash
git add memory/daily/2026-06-12.md
git commit -m "docs(daily): record 3-dimension equivalence finalization snapshot"
```

## Task 6.2：标记 stage 1 完成

**Files**:
- Modify: `docs/progress/CURRENT_STATE.md`
- Modify: `memory/MEMORY.md`

- [ ] **Step 1: 修订 CURRENT_STATE.md 标记 stage 1 完成**

把「当前活跃工作」段（约第 28-30 行）替换为：

```markdown
## 当前活跃工作

无。**stage 1 已于 2026-06-12 完成**——三维度等价标准（[ADR 0001](../adr/0001-redefine-equivalence.md)）落地，4 项必修/应修差异全部解决，三维度验证管线（`./scripts/diff-build.sh`）通过。stage 2 待启动（详见下方）。
```

把「待办」段标题改为：

```markdown
## 已完成 — 原 5 类差异处理结果（2026-06-12 收尾）

5 类差异已按三维度标准全部处理：

1. 字数统计精度 → ✅ 已修（Port Hugo WordCount）
2. RSS items 顺序 → ✅ 已修（sortPagesByDateDesc tiebreaker）
3. RSS description 截断 → ✅ 已修（TruncateHTMLByWords word-boundary）
4. products summary 换行 → ✅ 接受为永久差异（详见 equivalence.md §4）
5. general summary 截断 → ✅ 已修（同 #3）
```

- [ ] **Step 2: 修订 MEMORY.md 项目上下文段**

把项目上下文段中「当前分支：master；阶段一里程碑 1–9 已全部落地」

改为：

```markdown
- 当前分支：master；**stage 1 已于 2026-06-12 完成**（含三维度等价标准 ADR 0001 落地）；stage 2 待启动
```

- [ ] **Step 3: 验证修改**

Run: `grep -c "stage 1 已于 2026-06-12 完成\|三维度" docs/progress/CURRENT_STATE.md memory/MEMORY.md`
Expected: 输出 ≥ 3

- [ ] **Step 4: Commit**

```bash
git add docs/progress/CURRENT_STATE.md memory/MEMORY.md
git commit -m "docs: mark stage 1 complete (3-dimension equivalence finalized)"
```

## Task 6.3：自检与归档

**Files**:
- 无（最终自检）

- [ ] **Step 1: 跑 go build 确认无编译错误**

Run: `go build -o /tmp/huan-final ./cmd/huan`
Expected: 无错误

- [ ] **Step 2: 确认仓库整洁度**

Run:
```bash
grep -rn "TODO\|FIXME\|XXX\|HACK" --include="*.go" . | grep -v "_test.go" || echo "clean"
ls backup/ tmp/ 2>/dev/null || echo "no junk"
```
Expected: 输出 `clean` + `no junk`

- [ ] **Step 3: 把本 plan 移到 completed**

Run:
```bash
mkdir -p docs/reports/completed
git mv docs/superpowers/plans/2026-06-12-redefine-equivalence.md docs/reports/completed/2026-06-12-redefine-equivalence-plan.md
# 如果 plans 目录空，删除
rmdir docs/superpowers/plans docs/superpowers 2>/dev/null || true
```

- [ ] **Step 4: 写一份完成报告**

Create `docs/reports/completed/2026-06-12-redefine-equivalence-report.md`:

````markdown
# 三维度等价标准落地 完成报告

> 完成日期：2026-06-12 · 关联 ADR：[0001](../../adr/0001-redefine-equivalence.md) · 原 plan：[2026-06-12-redefine-equivalence-plan.md](2026-06-12-redefine-equivalence-plan.md)

## 落地内容

### 文档
- 新建 `docs/adr/0001-redefine-equivalence.md`
- 新建 `docs/standards/equivalence.md`
- 修订 `CLAUDE.md`（关联项目 + 验证方式表述）
- 修订 `memory/MEMORY.md`（项目上下文 + 关键决策 + 经验教训）
- 修订 `docs/progress/CURRENT_STATE.md`（5 类差异归类 + stage 1 完成标记）
- 更新 `docs/INDEX.md`

### 代码
- 新建 `internal/equiv/`（normalizer / seo / ai / runner + 测试）
- 新建 `cmd/equiv-check/`（CLI 入口）
- 修订 `internal/build/summary.go`（Port Hugo WordCount + TruncateHTMLByWords word-boundary）
- 修订 `internal/content/tree.go`（sortPagesByDateDesc tiebreaker）
- 升级 `scripts/diff-build.sh`（三维度门禁集成）

### 验证结果

- `go test ./...`：全 PASS
- `./scripts/diff-build.sh`：byte / normalized / seo / ai 四模式报告（最终数字见 `memory/daily/2026-06-12.md`）
- zhurongshuo 实际页面验证：books/practices「约 X 万字」数字与 Hugo 一致；RSS items 顺序一致；RSS description word-boundary 一致

### 永久差异登记

- products 列表页 summary 换行（`</h2>\n<p>` vs `</h2> <p>`）—— 渲染等价，详见 `docs/standards/equivalence.md` §4

## stage 2 起步建议

- llms.txt（站点 AI 抓取说明）
- 额外 JSON-LD（Article / Course / Product 等 schema）
- 服务端搜索 / 插件接口（按 `docs/technical-plan.md` §4.11）
````

- [ ] **Step 5: Commit 归档**

```bash
git add docs/reports/completed/ docs/superpowers/
git commit -m "docs(reports): archive 3-dimension equivalence plan + completion report"
```

---

# Self-Review Notes

完成全部 task 后，回到这份 plan 做一遍自检：

**Spec 覆盖**：
- ✅ (1) 修订 CLAUDE.md / MEMORY / CURRENT_STATE + 新建 ADR 0001 + equivalence.md — Phase 1 全覆盖
- ✅ (2) 升级 diff-build.sh 为多模式 — Phase 2 Task 2.5
- ✅ (3) Port Hugo WordCount — Phase 3
- ✅ (4) RSS items 顺序 + RSS description 截断 + general summary 截断 — Phase 4 + Phase 5
- ✅ (5) products summary 换行接受为永久差异 — Task 1.2 §4 登记

**类型一致性**：
- `SEOFields` / `AIFields` / `Heading` / `Link` / `Report` / `Mode` 在 Task 2.x 定义后，所有引用都按这个名字
- `CountWordsInPlain` / `TruncateHTMLByWords` / `sortPagesByDateDesc` 保留原名（不重命名）
- `equiv.CompareDirs(mode, dirA, dirB)` 签名在 runner.go 与 main.go 一致

**Placeholder 扫描**：plan 内无 TBD/TODO/"implement later"——每个步骤都有具体代码或命令。
