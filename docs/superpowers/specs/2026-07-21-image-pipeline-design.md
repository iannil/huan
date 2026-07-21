# 设计文档：图片管线插件（Image Pipeline Plugin）

- **日期**：2026-07-21
- **状态**：Draft
- **关联 ADR**：[ADR 0003 — 统一插件系统](docs/adr/0003-unified-plugin-system.md)
- **实现阶段**：v0.8.0

## 1. 背景

huan 当前对图片的处理是零 — `static/` 目录原样复制到输出目录，不做任何压缩、格式转换或多尺寸生成。内容中的 `![alt](photo.jpg)` 直接输出为 `<img>`，不生成 `srcset` 或 `<picture>`。

这导致：
- 用户上传的相机原图（10MB+）直接出现在页面上，加载速度极差
- 不支持 WebP/AVIF 等现代格式
- 没有响应式图片支持，移动端加载巨大图片

## 2. 设计目标

1. **零迁移成本**：用户不需要改模板，加上 `plugins.image_pipeline` 配置就生效
2. **全自动**：构建时自动处理所有图片，无需手动干预
3. **现代格式**：自动生成 WebP/AVIF，浏览器自动选择
4. **响应式**：自动生成多尺寸 + `srcset`，适配不同屏幕
5. **插件化**：与 cloudflare、qwen3 同样的 SSG 插件模式
6. **可配置**：压缩质量、输出格式、尺寸断点均可配置

## 3. 架构总览

```
huan build 管线
     │
     ▼
┌──────────────────────┐
│  1-7 构建阶段         │  加载/渲染/Feeds
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  8. copyStatic       │  static/ → 输出目录
└──────────┬───────────┘
           │
           ▼
┌──────────────────────────────────────────────┐
│  9. 图片管线插件 (AfterBuild 回调)             │
│                                              │
│  ┌──────────┐  ┌───────────┐  ┌───────────┐  │
│  │ scanner  │→│ processor │→│ injector  │  │
│  │ 扫描图片  │  │ 压缩/转换  │  │ HTML 后处理 │  │
│  └──────────┘  └───────────┘  └───────────┘  │
└──────────────────────────────────────────────┘
```

## 4. 插件结构

```
plugins/image-pipeline/
├── go.mod / go.sum
├── plugin_main.go          # InitPlugin 导出
├── plugin.go               # ImagePipelinePlugin 实现（Process 方法）
├── options.go              # Config 解析 + 默认值
├── scanner.go              # 扫描输出目录中的图片
├── processor.go            # 图片压缩/格式转换/多尺寸
├── html_injector.go        # HTML 后处理（srcset/picture 注入）
└── plugin/                 # 自包含的 plugin.Plugin 接口复制
    └── plugin.go
```

## 5. 核心数据结构

### Config

```go
type Config struct {
    // Formats 控制输出的图片格式。默认 ["webp"]
    // 可选: "webp", "avif"
    Formats []string `yaml:"formats"`

    // Quality 控制压缩质量（1-100）。默认 80
    Quality int `yaml:"quality"`

    // Sizes 控制生成的多尺寸（像素宽度）。每个宽度生成一个副本。
    // 例如 [480, 768, 1200] 会生成 photo-480w.jpg, photo-768w.jpg, photo-1200w.jpg
    // 空 = 不生成多尺寸
    Sizes []int `yaml:"sizes"`

    // InjectSrcset 是否在 HTML 中自动注入 srcset 属性。默认 true
    InjectSrcset bool `yaml:"inject_srcset"`

    // InjectPicture 是否生成 <picture> 标签（含 WebP/AVIF fallback）。默认 true
    InjectPicture bool `yaml:"inject_picture"`

    // InjectLazyLoading 是否给 <img> 添加 loading="lazy"。默认 true
    InjectLazyLoading bool `yaml:"inject_lazy_loading"`

    // MaxDimension 最大边长（像素），超过则等比缩放。0 = 不限制
    MaxDimension int `yaml:"max_dimension"`

    // SkipLarger 是否跳过比原图还大的尺寸生成。默认 true
    SkipLarger bool `yaml:"skip_larger"`
}
```

### ImageAsset

```go
type ImageAsset struct {
    SrcPath  string // 输出目录中的原图路径
    RelPath  string // 相对路径（用于 HTML 匹配）
    Width    int    // 原图宽度
    Height   int    // 原图高度
    Size     int64  // 原图文件大小
    Format   string // 原图格式
}
```

### ProcessedImage

```go
type ProcessedImage struct {
    Original   ImageAsset
    Variants   []ImageVariant  // 生成的副本
}

type ImageVariant struct {
    RelPath string // 相对路径
    Width   int    // 宽度
    Format  string // 格式
    Size    int64  // 文件大小
}
```

## 6. 处理流程

### 6.1 扫描（Scanner）

```
scanner.Scan(outputDir) → []ImageAsset
```

1. 递归遍历输出目录
2. 匹配扩展名：`.jpg`, `.jpeg`, `.png`, `.gif`
3. 解码图片头获取尺寸（不完整解码）
4. 返回 ImageAsset 列表

### 6.2 处理（Processor）

```
processor.Process(assets, config) → []ProcessedImage
```

对每个图片：

1. **解码**：`image.Decode()` 读取原图
2. **缩放**：如果 `MaxDimension > 0` 且图片边长超过限制，等比缩放
3. **格式转换**：按 `Formats` 生成副本
   - WebP: `photo.jpg` → `photo.webp`
   - AVIF: `photo.jpg` → `photo.avif`
4. **多尺寸**：按 `Sizes` 生成宽度副本
   - `photo-480w.jpg`, `photo-480w.webp`, `photo-480w.avif`
   - `photo-768w.jpg`, `photo-768w.webp`, `photo-768w.avif`
5. **写入**：所有副本写入输出目录，与原图并列

### 6.3 HTML 后处理（HTMLInjector）

```
html_injector.Process(outputDir, processed, config) → error
```

1. 扫描输出目录中所有 `.html` 文件
2. 对每个文件，用正则/html 解析找出 `<img>` 标签
3. 对每个 `<img>`：
   - 提取 `src` 属性值
   - 在 `processed` 中查找匹配的图片
   - 如果没找到，跳过
   - 如果 `<img>` 已有 `srcset` 或在 `<picture>` 内部，跳过
   - 否则执行替换

**替换逻辑（InjectPicture=true）：**

```
原: <img src="/images/photo.jpg" alt="示例">

新: <picture>
     <source srcset="/images/photo-768w.avif" type="image/avif">
     <source srcset="/images/photo-768w.webp" type="image/webp">
     <img src="/images/photo-768w.jpg"
          srcset="/images/photo-480w.jpg 480w, /images/photo-768w.jpg 768w"
          sizes="(max-width: 768px) 100vw, 768px"
          alt="示例"
          loading="lazy"
          width="1200" height="800">
    </picture>
```

**替换逻辑（InjectPicture=false, InjectSrcset=true）：**

```
原: <img src="/images/photo.jpg" alt="示例">

新: <img src="/images/photo-768w.jpg"
         srcset="/images/photo-480w.jpg 480w, /images/photo-768w.jpg 768w"
         sizes="(max-width: 768px) 100vw, 768px"
         alt="示例"
         loading="lazy"
         width="1200" height="800">
```

**sizes 属性生成规则：**
- 只有一个尺寸时：`sizes="(max-width: ${width}px) 100vw, ${width}px"`
- 多个尺寸时：取最大尺寸作为默认
- 用户可跳过（不生成 sizes 属性）

### 6.4 图片命名约定

```
原图: static/images/photo.jpg
处理后输出:
  images/photo.jpg              ← 原图（不变）
  images/photo.webp             ← WebP 格式
  images/photo.avif             ← AVIF 格式
  images/photo-480w.jpg         ← 480px 宽
  images/photo-768w.jpg         ← 768px 宽
  images/photo-480w.webp        ← 480px 宽 + WebP
  images/photo-768w.webp        ← 768px 宽 + WebP
```

## 7. 与现有系统的集成

### 7.1 插件注册

在 `cmd/huan/plugins.go` 或作为独立 `.so` 插件加载。

作为 SSG 插件，执行时机在 `BuildSite` 的 `AfterBuild` 回调中：

```go
// 在 Builder 或 build 命令中
build.BuildSite(build.Options{
    // ... 其他选项
    AfterBuild: func(result *build.Result, _ build.RenderPageFunc) error {
        // 查找并调用图片管线插件
        if p, ok := registry.Get("image_pipeline"); ok {
            if processor, ok := p.(ImageProcessor); ok {
                return processor.Process(outputDir, sourceDir)
            }
        }
        return nil
    },
})
```

### 7.2 Capability 接口

```go
// 新建 capability 接口：internal/image/processor.go
package image

type ImageProcessor interface {
    plugin.Plugin
    Process(outputDir, sourceDir string) error
}
```

### 7.3 配置

```yaml
# huan.yaml
plugins:
  image_pipeline:
    formats: ["webp", "avif"]
    quality: 80
    sizes: [480, 768, 1200]
    inject_srcset: true
    inject_picture: true
    inject_lazy_loading: true
    max_dimension: 2048
    skip_larger: true
```

## 8. 依赖

- `image` — Go 标准库解码
- `golang.org/x/image/draw` — 高质量缩放
- `github.com/chai2010/webp` — WebP 编码（或 `libwebp` 的纯 Go 替代）
- 或 `github.com/disintegration/imaging` — 全功能图片处理封装

## 9. 测试策略

| 测试 | 说明 |
|------|------|
| `TestScanner_ScansOutputDir` | 扫描输出目录，找到所有图片文件 |
| `TestScanner_IgnoresNonImages` | 跳过非图片文件（css, js, html） |
| `TestProcessor_WebpConversion` | JPEG → WebP 转换 |
| `TestProcessor_AvifConversion` | PNG → AVIF 转换 |
| `TestProcessor_MultiSize` | 生成多尺寸副本 |
| `TestProcessor_SkipLarger` | 跳过比原图大的尺寸生成 |
| `TestProcessor_MaxDimension` | 等比缩放 |
| `TestProcessor_Quality` | 不同质量参数 |
| `TestHTMLInjector_Srcset` | 注入 srcset 属性 |
| `TestHTMLInjector_Picture` | 生成 picture 标签 |
| `TestHTMLInjector_SkipExisting` | 跳过已有 srcset 的标签 |
| `TestHTMLInjector_SkipPicture` | 跳过已在 picture 内的 img |
| `TestHTMLInjector_MultipleImages` | 同一页面多个图片 |
| `TestHTMLInjector_NoMatch` | 没有匹配的图片时不做修改 |
| `TestIntegration_FullPipeline` | 从扫描到 HTML 后处理的完整流程 |

## 10. 文件变更清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `plugins/image-pipeline/` | 新增 | 插件源码目录 |
| `plugins/image-pipeline/go.mod` | 新增 | 独立 go module |
| `plugins/image-pipeline/plugin_main.go` | 新增 | InitPlugin 导出 |
| `plugins/image-pipeline/plugin.go` | 新增 | ImagePipelinePlugin 实现 |
| `plugins/image-pipeline/options.go` | 新增 | Config + ParseConfig |
| `plugins/image-pipeline/scanner.go` | 新增 | 图片扫描 |
| `plugins/image-pipeline/processor.go` | 新增 | 压缩/转换/多尺寸 |
| `plugins/image-pipeline/html_injector.go` | 新增 | HTML 后处理 |
| `plugins/image-pipeline/plugin/plugin.go` | 新增 | 自包含 Plugin 接口 |
| `internal/image/types.go` | 新增 | ImageProcessor capability 接口 |
| `cmd/huan/plugins.go` | 修改 | 注册 image_pipeline case |
| `cmd/huan/build.go` | 修改 | AfterBuild 中调用图片管线 |

## 11. 未来扩展

- **CDN 集成**：处理后的图片上传到 R2/CDN，自动替换 URL
- **模板函数**：`{{ imageSrcset "/images/photo.jpg" }}` 等精细控制
- **水印**：自动添加水印
- **EXIF 清理**：自动剥离 EXIF 数据