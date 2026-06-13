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
| products 代码块 chroma 渲染（17 文件） | 肉眼：HTML 渲染等价；SEO/AI：不读代码块内 attribute | ✅ 是 | 2026-06-13 |
| sitemap.xml items 顺序 + lastmod | SEO：极小影响（搜索引擎读 sitemap 但顺序不关键） | ✅ 是 | 2026-06-13 |
| robots.txt（1 文件） | SEO：微 artifact，不影响 crawl | ✅ 是 | 2026-06-13 |
| search.json | SEO/AI：内部搜索索引，非外部消费 | ✅ 是 | 2026-06-13 |
| practices/season-4 chapter-07 description entity encoding 边缘 case | 肉眼/SEO/AI：entity 在 HTML 解析后等价 | ✅ 是 | 2026-06-13 |

## 5. 修订历史

- 2026-06-12：初版，由 ADR 0001 落地。
- 2026-06-13: stage 2 phase 5d/e/f 后追加 5 项永久差异（chroma / sitemap / robots.txt / search.json / entity encoding 边缘 case）。
