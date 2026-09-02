# 电子书出版级审计与修复设计

- **日期**: 2026-09-02
- **状态**: Proposed
- **决策者**: 用户 + Claude
- **依赖**: [ebook-exporter 插件设计](2026-09-02-ebook-exporter-plugin-design.md)（审计对象是其产物）
- **关联仓库**: 审计脚本与报告在 zhurongshuo；修复改 huan 的 `plugins/ebook-exporter/render/`

## 背景

ebook-exporter V1 已全量生成 311 个产物（epub 1.4G / pdf 1.6G / docx 33M，`developer/export/`）。初步抽检发现产物与"出版标准"存在系统性差距：EPUB 正文直引号 `"` 而非中文弯引号、半角标点紧邻 CJK；PDF 无 `/Outlines` 书签、TOC 无页码、无页眉；DOCX 无 TOC 字段、未声明 eastAsia 字体、无页码。本设计定义一套可重复的出版级审计（自动化全量 + 抽样深审）与分批修复流程。

## 标准基线：双标准叠加

1. **中文图书出版规范**（排版主导）：字号级标题体系、正文首行缩进 2 字、中文弯引号 `“”‘’` 与全角标点、章起换页、页眉页码、目录与页码对齐
2. **数字出版最佳实践**（结构主导）：EPUB 3 合规（epubcheck 通过）、OPF 元数据完整、语义化结构（`epub:type`）、无障碍属性、字体子集化

## 决策

### 1. 审计脚本：`zhurongshuo/scripts/audit_ebooks.py`

风格对齐现有 `check_translation_quality.py`（`--json` / `--fail-on <level>` / 分级退出码）。工具：Python 3 + PyMuPDF 1.26.5（PDF 结构/文本/渲染）+ zipfile（epub/docx 解包）+ 正则（标点扫描）。

**检查项分级**：

| 级别 | 格式 | 检查 |
|---|---|---|
| P0 结构 | epub | mimetype 首位且未压缩；OPF 元数据（title/author/language/identifier）；nav.xhtml + toc.ncx 齐全；所有 XHTML well-formed；css/字体相对引用无死链 |
| P0 结构 | pdf | `%PDF-` 头；页数 > 0；每页有字体资源引用 |
| P0 结构 | docx | document.xml/styles.xml 存在且可解析；引用的 pStyle 均在 styles.xml 定义 |
| P1 出版 | epub | 正文中文弯引号（无直引号 `"` 残留）；CSS 含首行缩进与避头尾（`line-break`）；字体子集化（嵌入字体 < 全量字体大小的 50%）；orphans/widows 声明 |
| P1 出版 | pdf | `/Outlines` 书签存在且含章级；页面有页码文本；有页眉；目录条目与页码对应 |
| P1 出版 | docx | TOC 字段存在；styles.xml 声明 eastAsia 中文字体；section 含页码 |
| P1 出版 | 全部 | 半角标点紧邻 CJK 的密度（`（` 前后紧邻 CJK 等）低于阈值 |
| P2 润色 | 全部 | 标题层级不跳级；OPF description/keywords 非空；封面页含版本与日期信息 |

**输出**：`developer/export/AUDIT-REPORT.md`（人读）+ `--json`（机器）；退出码 0/1 按 `--fail-on P0|P1|P2` 阈值。

### 2. 抽样深审（子代理执行，产物落 `developer/export/audit/`）

**样本矩阵**：3 格式 × {单本 `reality-construction`、卷 `volume-1`、全集 `books-complete`} × {zh, en} = 18 样本。

**手段**：
- PDF：PyMuPDF 300dpi 渲染封面/版权/目录/章首/正文/末页 → PNG，视觉检查字体渲染（豆腐块）、行距缩进、标题视觉层级、页边距、页眉页码、表格代码块溢出
- EPUB：解包审 `epub:type` 语义标注、标题层级、CSS 出版属性
- DOCX：解包审 theme/styles 中文字体、TOC 字段、页面设置
- epubcheck：jar 放 `developer/audit-tools/`（gitignore，不入库），Java 21 运行，18 个 epub 样本合规检查

### 3. 修复批次（改 huan render 层，zhurongshuo 只接收产物）

每批流程：huan worktree 改码 → `--force` 全量重新生成 → 复跑 `audit_ebooks.py` 验证该级归零 → commit（huan 侧）。

| 批次 | 内容 | 改动位置 |
|---|---|---|
| P0 | 结构错误（如有） | 视报告 |
| P1-a 标点文本 | zh 内容弯引号/全角标点转换（内存中，不回写 content/）；EPUB CSS 避头尾 | `render/ast.go` 归一化层 + `render/epub.go` CSS |
| P1-b PDF 出版件 | 书签 outline、页码、页眉；目录页码（两遍渲染收集页码，或链接式目录） | `render/pdf.go`（gpdf 无 outline 则 PyMuPDF 后处理作为备选方案，优先 gpdf 原生） |
| P1-c DOCX 出版件 | TOC 字段、eastAsia 字体声明（styles/Normal + Heading 系）、页码 section | `render/docx.go` |
| P1-d EPUB 体积 | 字体子集化（fontTools 子集 Noto CJK 后嵌入；pip 依赖一次性装） | `render/epub.go` 或 `style/font.go` |
| P2 | orphans/widows、OPF 元数据、封面页信息 | 各 render 文件 |

### 4. 边界

- 不修改中文源内容（content/）——标点转换仅在导出管线内
- epubcheck jar、fontTools 等工具不入库
- 修复在 huan `feature/ebook-exporter` 分支续作（该分支尚未合并，审计修复并入同一 PR）
- 审计脚本本身是长期资产：以后每次重新生成后可回归

## 测试与验收

- 审计脚本单元可验证：构造最小坏样本（直引号 epub、无书签 pdf、无 TOC docx）断言报告命中
- 修复验收：`audit_ebooks.py --fail-on P1` 退出码 0（P1 全清）；epubcheck 18 样本 0 error
- 最终人工抽查：每格式 1 本打开确认视觉达标

## 明确不做

- 不重写排版引擎或换 gpdf/xelatex
- 不做印刷级（CMYK/出血/拼版）——数字出版范畴
- posts 产物不在本轮审计范围（books/practices 的 311 个文件）
