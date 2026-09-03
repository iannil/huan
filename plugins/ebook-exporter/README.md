# ebook-exporter

huan Exporter 能力插件：将站点内容（books / practices / posts）导出为 EPUB / PDF / DOCX 离线文档。纯 Go 实现，无外部进程依赖（不调用 pandoc/xelatex）。设计背景见 [ADR 0016](../../docs/adr/0016-ebook-exporter-plugin.md)。

## 用法

### 1. huan.yaml 声明（下游站点仓库）

```yaml
plugins:
  ebook_exporter:
    output_dir: "developer/export"   # 默认，可省略
    fonts_dir: ""                    # 可选：自定义字体目录（默认扫描系统字体目录）
    cover: true                      # 简单文字封面页，默认开
```

### 2. 安装 .so

```bash
mkdir -p ~/.huan/plugins   # 或项目根 plugins/
cp release/plugins/ebook-exporter.so ~/.huan/plugins/
```

注意：Go plugin 机制要求 .so 与 huan 二进制由**同一工具链 + 同一模块状态**构建，版本不匹配会在加载时告警并被跳过。

### 3. CLI

```bash
# 单本全格式
huan export ebook --slug reality-construction --format all

# 全量（所有类型 × 三格式 × 全层级）
huan export ebook --type all --format all --level all

# 单卷 / 单季（隐含 --level volumes 并限定 type）
huan export ebook --volume 1
huan export ebook --season 3

# 强制全量重建（忽略增量 manifest）
huan export ebook --type all --format all --force
```

产物落 `developer/export/{epub,pdf,docx}/books|practices|posts/{individual|volumes|complete}/`，英文版带 `-en` 后缀。重跑时未变化的书自动跳过（增量 manifest）。

## 依赖

- **系统 CJK 字体**：PDF 渲染需要 TrueType 字体。推荐安装 [Noto Sans CJK SC](https://github.com/notofonts/noto-cjk)（macOS 可 `brew install font-noto-sans-cjk-sc`）。默认扫描 `~/Library/Fonts`、`/Library/Fonts`、`/System/Library/Fonts`（Linux: `/usr/share/fonts`）；也可用 `fonts_dir` 配置指定目录。找不到 CJK 字体时报明确错误。
- OTF 字体：gpdf v1.0.11 **已实测接受 OTF**，无需转换为 TTF。
- **字体子集化（fonts_dir 预子集路线）**：EPUB 内嵌字体走"预子集"——`fonts_dir` 指向一个已用 pyftsubset 子集化的字体文件（15.7 MB 全量 → ~1.9 MB 子集），插件按 `notosanscjk`+`sc` 文件名匹配直接嵌入。子集字体由 zhurongshuo 仓库 `scripts/gen_subset_font.sh` 生成（字符集 = books/practices 全部 zh + .en.md 全文 + 元数据 yaml），不入库、机器本地；换机器或新增生僻字内容后须先重跑该脚本再 `huan export --force`。PDF 侧 gpdf 嵌入时自行子集，不受影响。

## V1 限制清单

- ~~PDF 无页码目录、无大纲书签~~（已修复）：PDF 现含 /Outlines 大纲书签（渲染后增量更新注入，gpdf `pdf` 包 Reader+Modifier）、每页页眉（书名小字）与页脚纯数字页码、带页码的目录页。章起始页通过"逐章独立渲染测页数 + 目录独立渲染测页数"确定——每章以新页开始（AddPage），独立测量与成书内分页一致；目录条目页码放在独立网格列，其宽度不影响换行，测量版与最终版分页相同。
- 页眉/页脚出现在封面与目录页：gpdf 的页眉页脚是 Document-level API，无首页/节级开关，无法只对正文启用；封面顶部会重复一行书名小字、封面页脚带页码 1。titlePg（首页不同）留作后续可选优化。
- DOCX 目录为双列表（TOC field + plain list）：Task 7 起目录页同时含 F9 TOC field（Word 内可刷新页码）与静态 plain list——部分阅读器/预览器不渲染 field，双列保证到处可见；代价是 Word 中刷新 field 后目录会重复出现一次。
- EN 合集封面显示中文标题：英文侧没有书系级（complete）标题元数据，合集封面 fallback 到中文标题（ZH-fallback 政策）；章节级标题/标签缺失时同样回退中文侧。
- PDF 导出耗时 ~3m20s/本：逐章双渲染（测量 + 成书）保证目录页码正确，正确性优先于速度；epub/docx 为秒级。
- 子集字体是机器本地产物（见"依赖"节）：其他机器未先跑 `gen_subset_font.sh` 时，EPUB 会回退嵌入 15.7 MB 全量字体（体积问题，非渲染问题）。
- PDF 表格降级为文本行渲染
- DOCX 表格降级为 tab 连接的文本段
- EPUB 列表统一渲染为 `<ul>`
- 英文导出在英文侧元数据缺失时，使用中文侧章节标题/标签
- posts 年度合集（`--year`）未实现（P2）
