# ebook-exporter 字体自动解析（纯 Go 回退链）设计

日期：2026-09-09
状态：已批准（用户确认三个关键决策：回退制 / 保持 STHeiti/Times / 不做 python 子集化）

## 背景与问题

电子书导出依赖两类机器本地的预生成字体，缺失时 `huan export ebook` 硬失败：

1. **PDF 正文 + 封面字体**（`developer/audit-tools/publication-fonts/{body-cover-cjk.ttf, cover-latin.ttf}`）
   由 huan 仓库 `plugins/ebook-exporter/tools/prepare_publication_fonts.py`（python3 + fontTools）从
   macOS 系统字体（STHeiti Light.ttc index 1、Times New Roman.ttf）提取子集生成。
   zhurongshuo 的 huan.yaml 显式配置了 `pdf_font` / `cover_font` / `cover_latin_font` 指向这两个文件，
   **缺失时逐书失败**——本次事故的根因。
2. **EPUB 正文字体子集**（`developer/audit-tools/NotoSansCJKsc-Regular.otf`，15.7MB → 1.9MB）
   由 zhurongshuo 仓库 `scripts/gen_subset_font.sh` 生成。缺失时插件已回退全量系统字体，仅体积问题。

两个脚本都游离在导出路径之外，导致"换机器/清空 audit-tools 后必须先跑脚本才能导出"。
字体准备应内聚在插件里，导出开箱即用。

## 目标

在一台**零预生成文件、无 python/fontTools**（但装有系统 CJK 字体）的机器上，
`huan export ebook` 直接产出正确的书。全程纯 Go，不 exec 任何外部进程。

## 字体解析链（三级）

每个字体槽位独立走链，任何一级命中即用；全链失败才报错，错误信息列出尝试过的路径：

```
显式配置路径（存在且可读）
  → 已知系统路径（macOS 出版设计源，含 TTC 提取）
  → 系统字体目录扫描回退（现有 FindCJKFont 家族，PDF 槽位加 glyf 过滤）
```

| 槽位 | 第二级（已知系统路径） | 第三级回退 | 全失败时 |
|---|---|---|---|
| PDF 正文 CJK | `/System/Library/Fonts/STHeiti Light.ttc` #1 → `STHeiti Medium.ttc` → Hiragino/PingFang TTC | `FindCJKFont` + glyf 过滤（CFF 的 NotoSansCJKsc.otf 不得落到 PDF 正文） | 逐书失败（现有行为） |
| 封面 CJK | 同上 | 同上 | 同上 |
| 封面拉丁 | `/System/Library/Fonts/Supplemental/Times New Roman.ttf` → Georgia → 任意系统衬线 ttf | — | 空值降级：封面拉丁字用 CJK 字体渲染（可接受降质，不算失败） |
| EPUB 正文（不变） | — | `FindCJKFont(fonts_dir)`（fonts_dir 为空时扫系统目录） | 逐书失败（现有行为） |

## 组件设计（均在 plugins/ebook-exporter）

### `style/ttc.go`（新）

纯 Go TTC→TTF 提取：

- 解析 TTC header（`ttcf` tag），读取第 N 个 font 的 offset table，重写为独立 .ttf；
  重算 `head.checkSumAdjustment`。
- 结果缓存到 `<output_dir>/.font-cache/`，缓存键 = sha256(源路径 + 源 mtime + fontNumber)；
  源字体未变则直接复用缓存文件。
- 函数签名：`ExtractTTC(sourcePath string, index int, cacheDir string) (string, error)`。

### `style/font.go`（扩展）

- `FindPublicationCJKFont(fontsDir, cfgPath string) (string, error)`：按上表链解析，
  回退 `FindCJKFont` 后用 `hasTrueTypeOutlines` 过滤（候选里无 TrueType 则报错）。
- `FindPublicationLatinFont(cfgPath string) string`：Times New Roman → Georgia →
  任意系统衬线 ttf；找不到返回 ""（调用方降级）。
- `hasTrueTypeOutlines(path string) bool`：解析 sfnt 表目录，`glyf` 表存在即真。
- 自动回退发生时插件输出一条 info 日志，例如
  `pdf_font not found at <path>, derived from /System/Library/Fonts/STHeiti Light.ttc`，
  保证机器差异可见、可预测。

### `plugin.go`（改）

- `Export()` 中 `pdf_font` / `cover_font` / `cover_latin_font` 三处：配置路径存在且可读 → 用；
  否则走上述自动链。
- manifest 的 assetHash 继续用**最终解析到的字体路径**参与（现状即如此）；换机器解析结果
  不同 → hash 不同 → 自动重建，无需额外失效逻辑。

## 配套清理（zhurongshuo 仓库，一次性）

- `huan.yaml` 删除 `fonts_dir` / `pdf_font` / `cover_font` / `cover_latin_font` 四键。
  **`fonts_dir` 必须一并删**：它指向空目录时 EPUB 正文字体查找只扫该目录、必然失败。
- 删除 `scripts/gen_subset_font.sh`（若存在于工作区）。
- `developer/audit-tools/` 此后无存在必要（现存文件可清可留，gitignored）。

## 体积影响（接受的代价）

EPUB 回退全量字体：15.7MB × 52 本 ≈ +500MB 本地产物体积。`developer/export/` 本就
gitignored、纯本地消费。若未来在意，可单独设计 opt-in 子集化，不影响本次。

## 测试

- **TTC 提取**：构造双 font 的 fixture ttc → 提取 index 0/1 → 断言 `hasTrueTypeOutlines`
  与表完整性（head/hhea/maxp/cmap 可解析）；缓存二次调用复用同一文件。
- **回退链**：temp 目录伪造三种场景——配置路径缺失 / 配置文件损坏（不可解析）/ 系统源存在——
  断言解析顺序与结果。
- **导出冒烟**：`export_flow_test.go` 补"无任何预生成字体时导出不失败"（CI 无 STHeiti 的
  Linux 环境走第三级回退）。

## 明确不做

- python fontTools 子集化（运行时或工具路径，均不做）
- OFL 字体替换出版设计（STHeiti/Times 保持）
- 封面视觉设计改动
- CI 电子书导出
