# ADR 0016: ebook-exporter 插件（Exporter 能力）

- **状态**: Accepted
- **日期**: 2026-09-02
- **决策者**: 用户 + Claude
- **依赖**: [ADR 0003](0003-unified-plugin-system.md)（统一插件系统）、[ADR 0013](0013-plugin-contract-convergence.md)（插件契约收敛）、[ADR 0014](0014-hook-contract-split.md)（Hook 契约拆分）
- **被引用**: [ebook-exporter 插件设计](../superpowers/specs/2026-09-02-ebook-exporter-plugin-design.md)

## 背景

zhurongshuo 曾有一套基于 Node + bash + pandoc/xelatex 的电子书生成流水线，于 2026-06-13 作为 Hugo 时代遗留整体删除（-10,148 行）。该能力曾支持 books/practices/posts × 单本/合卷/合集 × pdf/epub/docx，带封面生成、增量构建与并行。

现需求：在 huan 中以纯 Go 插件形态重新实现该能力——不恢复旧脚本，不依赖外部进程（pandoc/xelatex 均不再调用），不进部署链路。

## 决策

### 1. 能力接口：`pkg/plugin.Exporter`

- `Exporter` 接口嵌入 `Plugin`，添加 `Export(ctx, ExportRequest) (ExportResult, error)`
- 与 Deployer/Translator/ImageProcessor/ThemePlugin 并列，成为插件体系的新能力类型
- `ExportRequest` 携带 CLI flags；`ExportResult` 携带成功/失败清单与跳过计数
- CLI 按能力接口从 registry 选插件（`plugin.Find[Exporter]`），不硬编码插件名；找不到插件时用现有 `diagnoseCapabilityGap` 报告诊断

### 2. 插件架构：内容 / 渲染 / 样式 / 增量四层

```
plugins/ebook-exporter/          # .so，独立 go.mod（自包含类型副本，同其他插件）
├── plugin_main.go               # InitPlugin 入口（cfg 注入）
├── plugin.go                    # Exporter 实现（批次调度、中英配对、错误聚合）
├── content/
│   ├── discover.go              # 遍历 content/{books,practices,posts}：卷→书→部→章
│   └── model.go                 # Book/Practice/Part/Chapter 结构
├── render/
│   ├── ast.go                   # frontmatter 剥离 + goldmark 解析 + 标题层级归一化
│   ├── epub.go / pdf.go / docx.go  # 三后端薄封装
│   └── inline.go                # 行内节点映射
├── style/font.go                # 系统字体扫描 + fonts_dir 覆盖
└── manifest.go                  # 增量 manifest（kind-scoped key → 内容 hash）
```

渲染管线：剥 frontmatter → goldmark（CJK 扩展）→ 标题层级归一化（单本书 chapter 为 `#`，卷/季合集再降级）→ 装配（封面 → 版权页 → 目录 → part 分隔页 → 章节）。`guide/` 目录与 ` ```guide ` 代码块跳过——Web 可视化专属，离线文档无意义。

### 3. 第三方库选型（全部 MIT）

| 环节 | 库 | 理由 |
|---|---|---|
| Markdown 解析 | `yuin/goldmark`（huan 已依赖） | CommonMark 合规、CJK 扩展、AST 单次遍历喂三后端 |
| EPUB | `go-shiori/go-epub` | bmaupin/go-epub 官方后继；EPUB 3.0 + 2.0 NCX 兼容 |
| PDF | `gpdf-dev/gpdf` | 纯 Go 零依赖、CJK-first、TrueType 子集嵌入 |
| DOCX | `mmonterroca/docxgo` | 内置 Heading1-9 样式、TOC 字段、多 section |

排除项：`carmel/gooxml`（AGPL）、`unidoc/unioffice`（商业许可）。

### 4. CLI：`huan export ebook`

```
huan export ebook [--type books|practices|posts|all]
              [--format pdf|epub|docx|all]
              [--level individual|volumes|complete|all]
              [--slug xxx] [--volume N] [--season N] [--year N]
              [--force] [--jobs N]
```

- `huan export`（posts → CSV）现状保持，deploy.sh 零改动；`huan export csv` 为显式别名
- `--level volumes` 对 practices 语义等同 seasons（两词互为别名）
- `--volume N` / `--season N` 隐含 `--level volumes` 并限定 type

### 5. 增量：manifest + kind-scoped key

- manifest 记录内容 hash，key 按 kind（type×level×slug×语言×格式）隔离，避免不同导出维度互相污染
- 未变跳过；`--force` 全量
- 仅本地工具：不进 deploy.sh、不进 CI、不上站

### 6. 错误处理与双语

- 单本书失败不中断批次（collection-not-interruption）；结束时输出成功/失败清单，非零退出码当且仅当有失败项
- `.en.md` 存在 → 生成 `*-en.*`；缺英文侧 → 跳过并 warn，不算错误

## 架构

见上文第 2 节目录树。配置声明（下游仓库 huan.yaml）：

```yaml
plugins:
  ebook_exporter:
    output_dir: "developer/export"   # 默认，可省略
    fonts_dir: ""                    # 可选
    cover: true                      # 简单文字封面页，默认开
```

## 影响

### 新增

- `pkg/plugin` 的 `Exporter` 能力接口 + `cmd/huan` 的 `export ebook` 子命令
- `plugins/ebook-exporter/`（.so 插件）
- 产物目录 `developer/export/{epub,pdf,docx}/...`（沿用旧层级）

### 改造

- 插件 registry / CLI 装配处注册 Exporter 能力类型

### 未来扩展路径

- posts 年度合集（`--year`，P2）
- PDF 大纲书签/页码目录（待 gpdf 上游支持）
- EPUBCheck 集成（CI 无 Java，暂不引入）
