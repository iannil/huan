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

## V1 限制清单

- PDF 无页码目录、无大纲书签（gpdf 暂不支持；目录页为章标题列表）
- PDF 表格降级为文本行渲染
- DOCX 表格降级为 tab 连接的文本段
- EPUB 列表统一渲染为 `<ul>`
- 英文导出在英文侧元数据缺失时，使用中文侧章节标题/标签
- posts 年度合集（`--year`）未实现（P2）
