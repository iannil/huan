# ebook-exporter 插件设计（Exporter 能力）

- **日期**: 2026-09-02
- **状态**: Proposed
- **决策者**: 用户 + Claude
- **依赖**: [ADR 0003](../../../docs/adr/0003-unified-plugin-system.md)（统一插件系统）、[ADR 0013](../../../docs/adr/0013-plugin-contract-convergence.md)（插件契约收敛）、[ADR 0014](../../../docs/adr/0014-hook-contract-split.md)（Hook 契约拆分）
- **关联**: 本设计在 huan 仓库实现；zhurongshuo 仓库仅新增 huan.yaml 插件声明

## 背景

zhurongshuo 曾有一套基于 Node + bash + pandoc/xelatex 的电子书生成流水线（`scripts/generate-ebook.sh` 2213 行 + `scripts/ebook/` + `scripts/templates/`），于 2026-06-13 在 commit `6f630ca4a "remove outdated"` 中作为 Hugo 时代遗留被整体删除（-10,148 行）。该能力曾支持 books/practices/posts × 单本/合卷/合集 × pdf/epub/docx，带封面生成、增量构建与并行。

现需求：在 huan 中以纯 Go 插件形态重新实现该能力，不恢复旧脚本，不依赖外部进程（pandoc/xelatex 均不再调用）。

## 决策

### 1. 能力接口：`pkg/plugin.Exporter`

```go
// Exporter transforms site content into offline document formats.
type Exporter interface {
    Plugin
    Export(ctx context.Context, req ExportRequest) (ExportResult, error)
}
```

- 与 Deployer/ThemePlugin/PostBuildHook 并列，成为插件体系的又一能力类型
- `ExportRequest` 携带 CLI flags；`ExportResult` 携带成功/失败清单与跳过计数

### 2. 插件：`plugins/ebook-exporter/`（.so，独立 go.mod）

```
plugins/ebook-exporter/
├── plugin_main.go            # InitPlugin 入口（cfg 注入，同 seo-injector 模式）
├── plugin.go                 # Exporter 实现
├── content/
│   ├── discover.go           # 遍历 content/{books,practices,posts}：卷→书→部→章
│   └── model.go              # Book/Practice/Part/Chapter 结构
├── render/
│   ├── ast.go                # frontmatter 剥离 + goldmark 解析 + 标题层级归一化
│   ├── epub.go               # → go-shiori/go-epub
│   ├── pdf.go                # → gpdf-dev/gpdf
│   └── docx.go               # → mmonterroca/docxgo
├── style/                    # CSS、模板、字体解析
└── go.mod                    # 独立模块
```

### 3. 第三方库选型（全部 MIT）

| 环节 | 库 | 理由 |
|---|---|---|
| Markdown 解析 | `github.com/yuin/goldmark`（huan 已依赖 v1.8.2） | CommonMark 合规、CJK 扩展、AST 单次遍历喂三后端 |
| EPUB | `github.com/go-shiori/go-epub` | bmaupin/go-epub 官方指定后继；EPUB 3.0 + 2.0 NCX 兼容；CSS/字体/图片支持 |
| PDF | `github.com/gpdf-dev/gpdf` | 纯 Go 零依赖、CJK-first、TrueType 子集嵌入、页码/页眉页脚 |
| DOCX | `github.com/mmonterroca/docxgo` | 40+ 内置样式（Heading1-9）、TOC、多 section、页眉页脚 |

排除项：`carmel/gooxml`（AGPL）、`unidoc/unioffice`（商业许可）。

### 4. CLI：`huan export ebook`

```
huan export            # 现状保持（posts → CSV），deploy.sh 第 5 步不受影响
huan export csv        # 同上的显式别名
huan export ebook [--type books|practices|posts|all]
                  [--format pdf|epub|docx|all]
                  [--level individual|volumes|complete|all]
                  [--slug xxx] [--volume N] [--season N] [--year N]
                  [--force] [--jobs N]
```

- `--level volumes`：对 books 是卷合集（4 卷），对 practices 语义等同 `seasons`（7 季）；两词互为别名
- `--volume N` / `--season N`：选定单个卷/季，隐含 `--level volumes` 并限定 type

- 按能力接口从 registry 选插件（`plugin.Find[pkgplugin.Exporter]`），不硬编码插件名
- 找不到插件时用现有 `diagnoseCapabilityGap` 报告诊断

### 5. 渲染管线

**输入归一化**（render/ast.go）：
1. 剥离 YAML frontmatter（解析前剥掉，避免 `---` 误解析——旧脚本踩过的同类坑）
2. goldmark 解析，启用 `extension.NewCJK()`
3. 标题层级归一化：单本书 chapter 为 `#`、章内标题顺移；卷/季合集再降一级
4. `guide/` 目录与 ` ```guide ` 代码块**跳过**——Web 可视化专属 schema，离线文档无意义

**装配顺序**：封面页 → 版权/版本页（version、last_updated 取自 data yaml）→ 目录 → introduction → part-XX（part_titles 作分隔页）→ chapters → epilogue → appendix。

**三后端策略**：

| | epub | pdf | docx |
|---|---|---|---|
| 章节 | 一章一个 XHTML section | 一章起一页（AddPage） | Heading1 分章 |
| 目录 | go-epub 自动 TOC（3.0 nav + 2.0 NCX） | 插件生成链接式目录页（gpdf 无内置 TOC） | docxgo 内置 TOC 字段 |
| 字体 | 内嵌 Noto CJK | gpdf TrueType 嵌入 + 子集 | 引用样式名，Word 侧解析 |

**字体来源**：默认扫描系统字体目录（macOS: `~/Library/Fonts`、`/System/Library/Fonts`；Linux: `/usr/share/fonts`），中文找 Noto Sans CJK SC、英文找 Noto Sans（缺失时英文版可回退用 CJK 字体的拉丁字形）；`fonts_dir` 配置可覆盖。找不到 CJK 字体时报明确错误（同 deploy 无凭据的报错模式）。

### 6. 中英双语

- `.en.md` 存在 → 生成 `*-en.{epub,pdf,docx}`
- 缺英文侧文件 → 跳过并 warn，不算错误
- 英文版字体走 Noto Sans（拉丁子集）

### 7. 产物与增量

- 输出：`developer/export/{pdf,epub,docx}/books|practices|posts/{individual|volumes|seasons|yearly|complete}/`（沿用旧目录层级）
- 增量：manifest 记录内容 hash，未变跳过；`--force` 全量
- 仅本地工具：不进 deploy.sh、不进 CI、不上站

### 8. 错误处理

- 单本书失败不中断批次（collection-not-interruption，同 build hook 语义）
- 结束时输出成功/失败清单，非零退出码当且仅当有失败项

### 9. 配置（zhurongshuo huan.yaml）

```yaml
plugins:
  ebook_exporter:
    output_dir: "developer/export"   # 默认，可省略
    fonts_dir: ""                    # 可选
    cover: true                      # 简单文字封面页，默认开
```

### 10. ADR

新增 [ADR 0016](../../../docs/adr/0016-ebook-exporter-plugin.md)（实施阶段创建）：Exporter 能力 + ebook-exporter 插件，延续 ADR 0012/0013/0014 的插件能力演进线。

## 测试策略（TDD）

- **单元**：content 发现（卷→书→部→章顺序、中英配对、guide 跳过）、frontmatter 剥离、标题降级、manifest 增量
- **渲染合格性**：
  - epub：zip 结构断言（mimetype 首位、nav.xhtml 存在）
  - pdf：产物头断言（`%PDF-`、页数 > 0）
  - docx：解包断言 document.xml 含标题结构
- **端到端**：zhurongshuo 仓库跑 `huan export ebook --slug reality-construction`，三格式产物人工抽查一次
- **不追求**：像素级排版比对、EPUBCheck 外部依赖（CI 无 Java）

## 验收标准

1. 全量 books（25 本）+ practices（7 季约 33 本）× 中英 × 3 格式跑完无 panic，失败清单为空
2. 产物落 `developer/export/{pdf,epub,docx}/` 层级目录，命名含语言后缀
3. 增量生效：未修改的书重跑被跳过（manifest hash 命中）
4. zhurongshuo `deploy.sh` 零改动

## 已知风险与对策

| 风险 | 对策 |
|---|---|
| gpdf 无 tag 版本、单一维护者 | go.mod pin 具体 commit；端到端抽查 |
| gpdf 无内置 TOC/bookmark | 第一版用"章标题目录页"（无页码链接）；PDF 大纲书签若 gpdf 不支持则记入限制 |
| docxgo 较新（v2.x） | Word 打开抽查即可 |
| 三后端 AST 映射层工作量大 | 映射层独立成 render/ast.go 单一职责，三后端各自薄封装 |

## 明确不做

- 不恢复 zhurongshuo `scripts/` 下任何 Node/bash 电子书脚本
- 不调用 pandoc/xelatex 等外部进程
- 不进 CI / 部署链路
- 不导出 hidden/ 内容
- posts 年度合集（`--year`）为 P2，第一版可只做 books/practices
