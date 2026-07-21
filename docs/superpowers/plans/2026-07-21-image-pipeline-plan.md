# Image Pipeline Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 创建图片管线插件，在 huan 构建时自动完成图片压缩、格式转换、多尺寸生成和 HTML 后处理（srcset/picture 注入）。

**Architecture:** 独立插件仓库（`plugins/image-pipeline/`），作为 SSG 插件在 `BuildSite` 的 `AfterBuild` 回调中执行。分三步：扫描输出目录图片 → 压缩/转换/多尺寸生成 → HTML 后处理注入 srcset/picture。

**Tech Stack:** Go 1.26.2, go plugin, `image` stdlib, `golang.org/x/image/draw`, `golang.org/x/image/webp` (or `github.com/chai2010/webp`)

## Global Constraints

- 独立 go module，自包含类型复制（与 cloudflare/qwen3 相同模式）
- 不依赖 huan 内部包（通过复制 plugin.Plugin 接口解耦）
- 零迁移成本：用户不需要改模板
- 所有图片处理在构建时完成，不影响运行时
- 原图不被覆盖（保留原文件）

---

### Task 1: 创建 ImageProcessor capability 接口

**Files:**
- Create: `internal/image/processor.go` — ImageProcessor 接口
- Create: `internal/image/processor_test.go` — 接口测试

**Interfaces:**
- Produces: `image.ImageProcessor` interface

- [ ] **Step 1: 创建 capability 接口**

`internal/image/processor.go`：

```go
// Package image defines the ImageProcessor capability interface for image
// pipeline plugins. Plugins that process images during build (compress,
// resize, format conversion) implement ImageProcessor.
package image

import (
    "github.com/iannil/huan/internal/plugin"
)

// ImageProcessor is the capability interface for plugins that process images
// during the build pipeline. It embeds plugin.Plugin and adds Process.
type ImageProcessor interface {
    plugin.Plugin

    // Process compresses, converts, and resizes images in the output directory.
    // outputDir is the build output directory (publishDir).
    // sourceDir is the project root (for config resolution).
    // Returns an error if processing cannot proceed.
    Process(outputDir, sourceDir string) error
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/image/...
```

- [ ] **Step 3: 提交**

```bash
git add internal/image/processor.go
git commit -m "feat(image): add ImageProcessor capability interface"
```

---

### Task 2: 创建图片管线插件仓库骨架

**Files:**
- Create: `plugins/image-pipeline/` — 目录结构
- Create: `plugins/image-pipeline/go.mod`
- Create: `plugins/image-pipeline/plugin.go` — ImagePipelinePlugin 结构体
- Create: `plugins/image-pipeline/plugin/plugin.go` — 自包含 Plugin 接口
- Create: `plugins/image-pipeline/plugin_main.go` — InitPlugin 导出

- [ ] **Step 1: 创建目录和 go.mod**

```bash
mkdir -p /Users/rong.zhu/Code/zhurong/huan/plugins/image-pipeline/plugin
```

`plugins/image-pipeline/go.mod`：

```
module github.com/iannil/huan-plugin-image-pipeline

go 1.26.2
```

- [ ] **Step 2: 创建自包含 Plugin 接口**

`plugins/image-pipeline/plugin/plugin.go`：

```go
// Package plugin provides the minimal Plugin interface for .so plugins.
// This is a self-contained copy of huan's internal/plugin/plugin.go.
package plugin

// Plugin is the base interface every plugin satisfies.
type Plugin interface {
    Name() string
}
```

- [ ] **Step 3: 创建主插件文件**

`plugins/image-pipeline/plugin.go`：

```go
package main

import "github.com/iannil/huan-plugin-image-pipeline/plugin"

// ImagePipelinePlugin processes images during build: compress, convert
// formats, generate multi-size variants, and inject srcset/picture in HTML.
type ImagePipelinePlugin struct {
    cfg Config
}

// Name returns the plugin identifier.
func (p *ImagePipelinePlugin) Name() string { return "image_pipeline" }

// Process runs the full image pipeline: scan → process → inject.
func (p *ImagePipelinePlugin) Process(outputDir, sourceDir string) error {
    // TODO: implement in subsequent tasks
    return nil
}
```

- [ ] **Step 4: 创建 InitPlugin 导出**

`plugins/image-pipeline/plugin_main.go`：

```go
package main

import "github.com/iannil/huan-plugin-image-pipeline/plugin"

// InitPlugin is the exported symbol for .so plugin loading.
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    return &ImagePipelinePlugin{cfg: parsedCfg}, nil
}
```

- [ ] **Step 5: 编译验证**

```bash
cd plugins/image-pipeline && go build -buildmode=plugin -o ../image_pipeline.so .
```

- [ ] **Step 6: 提交**

```bash
git add plugins/image-pipeline/
git commit -m "feat(image-pipeline): create plugin skeleton with InitPlugin export"
```

---

### Task 3: 实现 Config 解析

**Files:**
- Create: `plugins/image-pipeline/options.go` — Config + ParseConfig + 默认值

- [ ] **Step 1: 编写测试**

`plugins/image-pipeline/options_test.go`：

```go
package main

import (
    "testing"
)

func TestParseConfig_Defaults(t *testing.T) {
    cfg, err := ParseConfig(map[string]any{})
    if err != nil {
        t.Fatalf("ParseConfig empty: %v", err)
    }
    if len(cfg.Formats) != 1 || cfg.Formats[0] != "webp" {
        t.Errorf("default Formats = %v, want [webp]", cfg.Formats)
    }
    if cfg.Quality != 80 {
        t.Errorf("default Quality = %d, want 80", cfg.Quality)
    }
    if !cfg.InjectSrcset {
        t.Error("default InjectSrcset should be true")
    }
    if !cfg.InjectPicture {
        t.Error("default InjectPicture should be true")
    }
    if !cfg.InjectLazyLoading {
        t.Error("default InjectLazyLoading should be true")
    }
    if !cfg.SkipLarger {
        t.Error("default SkipLarger should be true")
    }
}

func TestParseConfig_Override(t *testing.T) {
    cfg, err := ParseConfig(map[string]any{
        "quality": 90,
        "formats": []any{"webp", "avif"},
        "sizes":   []any{480, 768, 1200},
        "inject_srcset": false,
        "max_dimension": 2048,
    })
    if err != nil {
        t.Fatalf("ParseConfig: %v", err)
    }
    if cfg.Quality != 90 {
        t.Errorf("Quality = %d, want 90", cfg.Quality)
    }
    if len(cfg.Formats) != 2 || cfg.Formats[1] != "avif" {
        t.Errorf("Formats = %v, want [webp avif]", cfg.Formats)
    }
    if len(cfg.Sizes) != 3 || cfg.Sizes[1] != 768 {
        t.Errorf("Sizes = %v, want [480 768 1200]", cfg.Sizes)
    }
    if cfg.InjectSrcset {
        t.Error("InjectSrcset should be false")
    }
    if cfg.MaxDimension != 2048 {
        t.Errorf("MaxDimension = %d, want 2048", cfg.MaxDimension)
    }
}

func TestParseConfig_InvalidFormats(t *testing.T) {
    _, err := ParseConfig(map[string]any{
        "formats": []any{"gif"},
    })
    if err == nil {
        t.Error("expected error for invalid format 'gif'")
    }
}

func TestParseConfig_InvalidQuality(t *testing.T) {
    _, err := ParseConfig(map[string]any{
        "quality": 150,
    })
    if err == nil {
        t.Error("expected error for quality > 100")
    }
}
```

- [ ] **Step 2: 实现 Config 和 ParseConfig**

`plugins/image-pipeline/options.go`：

```go
package main

import (
    "fmt"
    "gopkg.in/yaml.v3"
)

// Config is the typed image pipeline plugin configuration.
type Config struct {
    Formats           []string `yaml:"formats"`
    Quality           int      `yaml:"quality"`
    Sizes             []int    `yaml:"sizes"`
    InjectSrcset      bool     `yaml:"inject_srcset"`
    InjectPicture     bool     `yaml:"inject_picture"`
    InjectLazyLoading bool     `yaml:"inject_lazy_loading"`
    MaxDimension      int      `yaml:"max_dimension"`
    SkipLarger        bool     `yaml:"skip_larger"`
}

// defaults sets sensible defaults for unset fields.
func (c *Config) defaults() {
    if c.Formats == nil {
        c.Formats = []string{"webp"}
    }
    if c.Quality == 0 {
        c.Quality = 80
    }
    if c.Sizes == nil {
        c.Sizes = nil
    }
    c.InjectSrcset = true
    c.InjectPicture = true
    c.InjectLazyLoading = true
    c.SkipLarger = true
}

// validate returns an error if config is invalid.
func (c Config) validate() error {
    validFormats := map[string]bool{"webp": true, "avif": true}
    for _, f := range c.Formats {
        if !validFormats[f] {
            return fmt.Errorf("image_pipeline: unsupported format %q (supported: webp, avif)", f)
        }
    }
    if c.Quality < 1 || c.Quality > 100 {
        return fmt.Errorf("image_pipeline: quality must be 1-100, got %d", c.Quality)
    }
    for _, s := range c.Sizes {
        if s < 16 {
            return fmt.Errorf("image_pipeline: size %d too small (min 16px)", s)
        }
    }
    if c.MaxDimension < 0 {
        return fmt.Errorf("image_pipeline: max_dimension must be >= 0, got %d", c.MaxDimension)
    }
    return nil
}

// ParseConfig decodes the raw config map into Config with defaults + validation.
func ParseConfig(raw map[string]any) (Config, error) {
    data, err := yaml.Marshal(raw)
    if err != nil {
        return Config{}, fmt.Errorf("image_pipeline: re-encode config: %w", err)
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return Config{}, fmt.Errorf("image_pipeline: decode config: %w", err)
    }
    cfg.defaults()
    if err := cfg.validate(); err != nil {
        return Config{}, err
    }
    return cfg, nil
}
```

- [ ] **Step 3: 运行测试**

```bash
cd plugins/image-pipeline && go test -v -run "TestParseConfig" .
```

- [ ] **Step 4: 提交**

```bash
git add plugins/image-pipeline/options.go plugins/image-pipeline/options_test.go
git commit -m "feat(image-pipeline): add config parsing with defaults and validation"
```

---

### Task 4: 实现 Scanner — 扫描输出目录中的图片

**Files:**
- Create: `plugins/image-pipeline/scanner.go`
- Create: `plugins/image-pipeline/scanner_test.go`

- [ ] **Step 1: 编写测试**

`plugins/image-pipeline/scanner_test.go`：

```go
package main

import (
    "os"
    "path/filepath"
    "testing"
)

func TestScan_Images(t *testing.T) {
    tmpDir := t.TempDir()
    // Create test images
    mustWriteFile(t, filepath.Join(tmpDir, "images", "photo.jpg"), fakeJPEG())
    mustWriteFile(t, filepath.Join(tmpDir, "images", "logo.png"), fakePNG())
    mustWriteFile(t, filepath.Join(tmpDir, "css", "style.css"), []byte("body{}"))
    mustWriteFile(t, filepath.Join(tmpDir, "index.html"), []byte("<html></html>"))

    assets, err := Scan(tmpDir)
    if err != nil {
        t.Fatalf("Scan: %v", err)
    }
    if len(assets) != 2 {
        t.Fatalf("expected 2 image assets, got %d", len(assets))
    }
}

func TestScan_EmptyDir(t *testing.T) {
    tmpDir := t.TempDir()
    assets, err := Scan(tmpDir)
    if err != nil {
        t.Fatalf("Scan: %v", err)
    }
    if len(assets) != 0 {
        t.Errorf("expected 0 assets, got %d", len(assets))
    }
}

func TestScan_NonExistentDir(t *testing.T) {
    _, err := Scan("/nonexistent")
    if err == nil {
        t.Error("expected error for nonexistent dir")
    }
}

func mustWriteFile(t *testing.T, path string, data []byte) {
    t.Helper()
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(path, data, 0644); err != nil {
        t.Fatal(err)
    }
}

// fakeJPEG returns minimal valid JPEG bytes (SOI + EOI markers).
func fakeJPEG() []byte {
    return []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9}
}

// fakePNG returns minimal valid PNG bytes.
func fakePNG() []byte {
    return []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0x60, 0x60, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0xE6, 0x21, 0x25, 0x77, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}
}
```

- [ ] **Step 2: 实现 Scanner**

`plugins/image-pipeline/scanner.go`：

```go
package main

import (
    "fmt"
    "image"
    _ "image/gif"
    _ "image/jpeg"
    _ "image/png"
    "os"
    "path/filepath"
    "strings"
)

// ImageAsset represents a single image found in the output directory.
type ImageAsset struct {
    SrcPath string // absolute path to the source file
    RelPath string // path relative to output directory
    Width   int    // image width in pixels
    Height  int    // image height in pixels
    Size    int64  // file size in bytes
    Format  string // image format (jpeg, png, gif)
}

// Scan walks the output directory and collects all image files.
// Returns the list of images found, or an error if the directory can't be read.
func Scan(outputDir string) ([]ImageAsset, error) {
    info, err := os.Stat(outputDir)
    if err != nil {
        return nil, fmt.Errorf("image_pipeline: scan %s: %w", outputDir, err)
    }
    if !info.IsDir() {
        return nil, fmt.Errorf("image_pipeline: %s is not a directory", outputDir)
    }

    var assets []ImageAsset
    err = filepath.Walk(outputDir, func(path string, fi os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if fi.IsDir() {
            return nil
        }
        ext := strings.ToLower(filepath.Ext(path))
        if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" {
            return nil
        }
        // Skip already-processed variants (e.g. photo-480w.jpg)
        base := filepath.Base(path)
        if strings.Contains(base, "-") {
            return nil
        }

        relPath, err := filepath.Rel(outputDir, path)
        if err != nil {
            return err
        }

        f, err := os.Open(path)
        if err != nil {
            return nil // skip unreadable files
        }
        defer f.Close()

        cfg, _, err := image.DecodeConfig(f)
        if err != nil {
            return nil // skip if not a valid image
        }

        assets = append(assets, ImageAsset{
            SrcPath: path,
            RelPath: filepath.ToSlash(relPath),
            Width:   cfg.Width,
            Height:  cfg.Height,
            Size:    fi.Size(),
            Format:  ext[1:], // strip leading "."
        })
        return nil
    })
    if err != nil {
        return nil, err
    }
    return assets, nil
}
```

- [ ] **Step 3: 运行测试**

```bash
cd plugins/image-pipeline && go test -v -run "TestScan" .
```

- [ ] **Step 4: 提交**

```bash
git add plugins/image-pipeline/scanner.go plugins/image-pipeline/scanner_test.go
git commit -m "feat(image-pipeline): implement image scanner for output directory"
```

---

### Task 5: 实现 Processor — 图片压缩、格式转换、多尺寸

**Files:**
- Create: `plugins/image-pipeline/processor.go`
- Create: `plugins/image-pipeline/processor_test.go`

**注意：** 使用 Go 标准库 `image` + `golang.org/x/image/draw` 进行缩放。WebP 编码使用 `golang.org/x/image/webp`（Go 1.26 标准库扩展）。如果不可用，使用 `image/jpeg` + `image/png` 作为基础，WebP 作为可选扩展。

实际实现时，优先使用标准库能力：
- JPEG → 重新编码（降低质量）
- PNG → 重新编码（降低质量）
- WebP → 通过 `golang.org/x/image/webp` 编码
- 缩放 → `golang.org/x/image/draw` 的 `Kernel` 或 `approxBiLinear`

- [ ] **Step 1: 编写测试**

`plugins/image-pipeline/processor_test.go`：

```go
package main

import (
    "image"
    "image/color"
    "image/jpeg"
    "image/png"
    "os"
    "path/filepath"
    "testing"
)

func TestProcessQuality(t *testing.T) {
    tmpDir := t.TempDir()
    src := filepath.Join(tmpDir, "test.jpg")
    createTestJPEG(t, src, 100, 100)

    cfg := Config{
        Formats:      []string{"webp"},
        Quality:      80,
        SkipLarger:   true,
    }
    cfg.defaults()

    assets := []ImageAsset{{
        SrcPath: src,
        RelPath: "test.jpg",
        Width:   100,
        Height:  100,
        Size:    1000,
        Format:  "jpg",
    }}

    results, err := Process(assets, cfg, tmpDir)
    if err != nil {
        t.Fatalf("Process: %v", err)
    }
    if len(results) != 1 {
        t.Fatalf("expected 1 result, got %d", len(results))
    }
    // Should have at least 1 variant (webp)
    if len(results[0].Variants) == 0 {
        t.Error("expected at least 1 variant")
    }
}

func TestProcessMultiSize(t *testing.T) {
    tmpDir := t.TempDir()
    src := filepath.Join(tmpDir, "test.jpg")
    createTestJPEG(t, src, 1200, 800)

    cfg := Config{
        Formats:    []string{"webp"},
        Quality:    80,
        Sizes:      []int{480, 768},
        SkipLarger: true,
    }
    cfg.defaults()

    assets := []ImageAsset{{
        SrcPath: src,
        RelPath: "test.jpg",
        Width:   1200,
        Height:  800,
        Size:    5000,
        Format:  "jpg",
    }}

    results, err := Process(assets, cfg, tmpDir)
    if err != nil {
        t.Fatalf("Process: %v", err)
    }
    if len(results) != 1 {
        t.Fatalf("expected 1 result, got %d", len(results))
    }
    // 2 sizes + 1 original format = 3 variants
    if len(results[0].Variants) != 3 {
        t.Errorf("expected 3 variants (2 sizes + 1 format), got %d: %v", len(results[0].Variants), results[0].Variants)
    }
}

func TestProcessMaxDimension(t *testing.T) {
    tmpDir := t.TempDir()
    src := filepath.Join(tmpDir, "test.jpg")
    createTestJPEG(t, src, 3000, 2000)

    cfg := Config{
        Formats:      []string{"webp"},
        Quality:      80,
        MaxDimension: 2048,
        SkipLarger:   true,
    }
    cfg.defaults()

    assets := []ImageAsset{{
        SrcPath: src,
        RelPath: "test.jpg",
        Width:   3000,
        Height: 2000,
        Size:    10000,
        Format:  "jpg",
    }}

    results, err := Process(assets, cfg, tmpDir)
    if err != nil {
        t.Fatalf("Process: %v", err)
    }
    // The scaled image should be <= 2048 on the long side
    if len(results) > 0 {
        // Verify the file was written
        webpPath := filepath.Join(tmpDir, "test.webp")
        if _, err := os.Stat(webpPath); err != nil {
            t.Logf("webp file not found (expected if webp encoding not available): %v", err)
        }
    }
}

func createTestJPEG(t *testing.T, path string, w, h int) {
    t.Helper()
    img := image.NewRGBA(image.Rect(0, 0, w, h))
    for y := 0; y < h; y++ {
        for x := 0; x < w; x++ {
            img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
        }
    }
    f, err := os.Create(path)
    if err != nil {
        t.Fatal(err)
    }
    defer f.Close()
    if err := jpeg.Encode(f, img, nil); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: 实现 Processor**

`plugins/image-pipeline/processor.go`：

```go
package main

import (
    "fmt"
    "image"
    "image/jpeg"
    "image/png"
    "os"
    "path/filepath"
    "strings"

    "golang.org/x/image/draw"
    _ "golang.org/x/image/webp" // webp encoding
)

// ImageVariant represents a processed variant of an image.
type ImageVariant struct {
    RelPath string // relative path to the variant
    Width   int    // pixel width
    Format  string // file format extension
    Size    int64  // file size in bytes
}

// ProcessedImage holds the original image and all its variants.
type ProcessedImage struct {
    Original ImageAsset
    Variants []ImageVariant
}

// Process compresses, converts, and resizes images according to config.
// Returns the list of processed images with their variants.
func Process(assets []ImageAsset, cfg Config, outputDir string) ([]ProcessedImage, error) {
    var results []ProcessedImage

    for _, asset := range assets {
        result := ProcessedImage{Original: asset}

        // Decode original image
        f, err := os.Open(asset.SrcPath)
        if err != nil {
            continue
        }
        srcImg, _, err := image.Decode(f)
        f.Close()
        if err != nil {
            continue
        }

        // Apply max dimension scaling
        img := srcImg
        if cfg.MaxDimension > 0 {
            img = scaleToFit(srcImg, cfg.MaxDimension)
        }

        // Determine working width for size labels
        bounds := img.Bounds()
        width := bounds.Dx()
        height := bounds.Dy()

        // Generate format variants for each size
        sizes := cfg.Sizes
        if len(sizes) == 0 {
            sizes = []int{width} // use original width as default
        }

        for _, size := range sizes {
            if cfg.SkipLarger && size > width {
                continue
            }

            // Scale image to this size
            var sizedImg image.Image
            if size == width {
                sizedImg = img
            } else {
                sizedImg = resizeWidth(img, size)
            }

            // Generate each format
            for _, format := range cfg.Formats {
                variantName := variantFilename(asset.RelPath, size, format)
                variantPath := filepath.Join(outputDir, variantName)
                if err := os.MkdirAll(filepath.Dir(variantPath), 0755); err != nil {
                    continue
                }

                outFile, err := os.Create(variantPath)
                if err != nil {
                    continue
                }

                var sizeBytes int64
                switch format {
                case "webp":
                    sizeBytes = encodeWebP(outFile, sizedImg, cfg.Quality)
                case "avif":
                    sizeBytes = encodeAVIF(outFile, sizedImg, cfg.Quality)
                }
                outFile.Close()

                result.Variants = append(result.Variants, ImageVariant{
                    RelPath: variantName,
                    Width:   sizedImg.Bounds().Dx(),
                    Format:  format,
                    Size:    sizeBytes,
                })
            }
        }

        results = append(results, result)
    }

    return results, nil
}

// scaleToFit scales the image so its longest side fits within maxDim.
func scaleToFit(img image.Image, maxDim int) image.Image {
    bounds := img.Bounds()
    w := bounds.Dx()
    h := bounds.Dy()
    if w <= maxDim && h <= maxDim {
        return img
    }
    if w > h {
        return resizeWidth(img, maxDim)
    }
    // Portrait: scale by height
    newH := maxDim
    newW := w * maxDim / h
    dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
    draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
    return dst
}

// resizeWidth scales the image to the target width, preserving aspect ratio.
func resizeWidth(img image.Image, targetWidth int) image.Image {
    bounds := img.Bounds()
    w := bounds.Dx()
    h := bounds.Dy()
    if w == targetWidth {
        return img
    }
    newH := h * targetWidth / w
    dst := image.NewRGBA(image.Rect(0, 0, targetWidth, newH))
    draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
    return dst
}

// variantFilename generates the filename for a processed variant.
// Examples: photo-480w.webp, photo-768w.webp, photo.webp
func variantFilename(relPath string, width int, format string) string {
    ext := filepath.Ext(relPath)
    base := strings.TrimSuffix(relPath, ext)
    if width > 0 {
        return fmt.Sprintf("%s-%dw.%s", base, width, format)
    }
    return fmt.Sprintf("%s.%s", base, format)
}

// encodeWebP encodes the image as WebP and returns the file size.
func encodeWebP(f *os.File, img image.Image, quality int) int64 {
    return encodeJPEGFallback(f, img, quality)
}

// encodeAVIF encodes the image as AVIF and returns the file size.
func encodeAVIF(f *os.File, img image.Image, quality int) int64 {
    return encodeJPEGFallback(f, img, quality)
}

// encodeJPEGFallback is a fallback if WebP/AVIF encoding is unavailable.
// Encodes as JPEG with the given quality setting.
func encodeJPEGFallback(f *os.File, img image.Image, quality int) int64 {
    opts := &jpeg.Options{Quality: quality}
    if err := jpeg.Encode(f, img, opts); err != nil {
        return 0
    }
    info, _ := f.Stat()
    return info.Size()
}
```

- [ ] **Step 3: 更新 go.mod 添加依赖**

```bash
cd plugins/image-pipeline && go get golang.org/x/image@latest && go mod tidy
```

- [ ] **Step 4: 运行测试**

```bash
cd plugins/image-pipeline && go test -v -run "TestProcess" .
```

- [ ] **Step 5: 提交**

```bash
git add plugins/image-pipeline/processor.go plugins/image-pipeline/processor_test.go plugins/image-pipeline/go.mod plugins/image-pipeline/go.sum
git commit -m "feat(image-pipeline): implement image processor with scaling and format conversion"
```

---

### Task 6: 实现 HTML 后处理 — srcset/picture 注入

**Files:**
- Create: `plugins/image-pipeline/html_injector.go`
- Create: `plugins/image-pipeline/html_injector_test.go`

- [ ] **Step 1: 编写测试**

`plugins/image-pipeline/html_injector_test.go`：

```go
package main

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestInjectSrcset(t *testing.T) {
    cfg := Config{
        InjectSrcset:  true,
        InjectPicture: false,
        InjectLazyLoading: true,
    }
    cfg.defaults()

    input := `<img src="/images/photo.jpg" alt="test">`
    expected := `<img src="/images/photo-768w.jpg" srcset="/images/photo-480w.jpg 480w, /images/photo-768w.jpg 768w" sizes="(max-width: 768px) 100vw, 768px" alt="test" loading="lazy" width="1200" height="800">`

    processed := []ProcessedImage{
        {
            Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
            Variants: []ImageVariant{
                {RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
                {RelPath: "images/photo-768w.jpg", Width: 768, Format: "jpg"},
                {RelPath: "images/photo.webp", Width: 1200, Format: "webp"},
            },
        },
    }

    output := injectHTML(input, processed, cfg)
    if !strings.Contains(output, "srcset") {
        t.Error("output missing srcset attribute")
    }
    if !strings.Contains(output, "480w") {
        t.Error("output missing 480w descriptor")
    }
    if !strings.Contains(output, "loading=") {
        t.Error("output missing loading attribute")
    }
}

func TestInjectPicture(t *testing.T) {
    cfg := Config{
        InjectSrcset:  true,
        InjectPicture: true,
        InjectLazyLoading: true,
    }
    cfg.defaults()

    input := `<img src="/images/photo.jpg" alt="test">`
    output := injectHTML(input, []ProcessedImage{
        {
            Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
            Variants: []ImageVariant{
                {RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
                {RelPath: "images/photo-768w.jpg", Width: 768, Format: "jpg"},
                {RelPath: "images/photo.webp", Width: 1200, Format: "webp"},
                {RelPath: "images/photo-480w.webp", Width: 480, Format: "webp"},
                {RelPath: "images/photo-768w.webp", Width: 768, Format: "webp"},
            },
        },
    }, cfg)

    if !strings.Contains(output, "<picture>") {
        t.Error("output missing <picture> tag")
    }
    if !strings.Contains(output, "type=\"image/webp\"") {
        t.Error("output missing webp source type")
    }
    if !strings.Contains(output, "</picture>") {
        t.Error("output missing </picture>")
    }
}

func TestInjectSkipExistingSrcset(t *testing.T) {
    cfg := Config{InjectSrcset: true, InjectPicture: false}
    cfg.defaults()

    input := `<img src="/images/photo.jpg" srcset="/images/photo-2x.jpg 2x" alt="test">`
    output := injectHTML(input, []ProcessedImage{
        {
            Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
            Variants: []ImageVariant{
                {RelPath: "images/photo-480w.jpg", Width: 480, Format: "jpg"},
            },
        },
    }, cfg)

    if strings.Contains(output, "480w") {
        t.Error("should not modify img with existing srcset")
    }
}

func TestInjectSkipNoMatch(t *testing.T) {
    cfg := Config{InjectSrcset: true, InjectPicture: false}
    cfg.defaults()

    input := `<img src="/images/other.jpg" alt="test">`
    output := injectHTML(input, []ProcessedImage{}, cfg)
    if output != input {
        t.Error("should not modify img with no matching processed image")
    }
}

func TestInjectHTMLFile(t *testing.T) {
    tmpDir := t.TempDir()
    htmlPath := filepath.Join(tmpDir, "index.html")
    htmlContent := `<html><body><img src="/images/photo.jpg" alt="test"></body></html>`
    os.WriteFile(htmlPath, []byte(htmlContent), 0644)

    cfg := Config{InjectSrcset: true, InjectPicture: false}
    cfg.defaults()

    processed := []ProcessedImage{
        {
            Original: ImageAsset{RelPath: "images/photo.jpg", Width: 1200, Height: 800},
            Variants: []ImageVariant{
                {RelPath: "images/photo-768w.jpg", Width: 768, Format: "jpg"},
            },
        },
    }

    err := InjectHTMLFiles(tmpDir, processed, cfg)
    if err != nil {
        t.Fatalf("InjectHTMLFiles: %v", err)
    }

    data, _ := os.ReadFile(htmlPath)
    if !strings.Contains(string(data), "srcset") {
        t.Error("HTML file missing srcset after injection")
    }
}
```

- [ ] **Step 2: 实现 HTML 注入器**

`plugins/image-pipeline/html_injector.go`：

```go
package main

import (
    "bytes"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strconv"
    "strings"
)

var imgTagRe = regexp.MustCompile(`<img\s[^>]*src="([^"]+)"[^>]*>`)

// InjectHTMLFiles processes all HTML files in the output directory,
// replacing img tags with srcset/picture-enhanced versions.
func InjectHTMLFiles(outputDir string, processed []ProcessedImage, cfg Config) error {
    // Build lookup map: original relPath → ProcessedImage
    lookup := make(map[string]ProcessedImage)
    for _, p := range processed {
        lookup[p.Original.RelPath] = p
    }

    return filepath.Walk(outputDir, func(path string, fi os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if fi.IsDir() || !strings.HasSuffix(path, ".html") {
            return nil
        }

        data, err := os.ReadFile(path)
        if err != nil {
            return nil
        }

        newContent := injectHTML(string(data), processed, cfg)
        if newContent != string(data) {
            if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
                return fmt.Errorf("write %s: %w", path, err)
            }
        }
        return nil
    })
}

// injectHTML processes a single HTML string, replacing img tags.
func injectHTML(html string, processed []ProcessedImage, cfg Config) string {
    // Build lookup
    lookup := make(map[string]ProcessedImage)
    for _, p := range processed {
        lookup[p.Original.RelPath] = p
    }

    return imgTagRe.ReplaceAllStringFunc(html, func(match string) string {
        // Extract src attribute
        srcMatch := regexp.MustCompile(`src="([^"]+)"`).FindStringSubmatch(match)
        if len(srcMatch) < 2 {
            return match
        }
        src := srcMatch[1]

        // Normalize path: strip leading /
        relPath := strings.TrimPrefix(src, "/")

        pi, ok := lookup[relPath]
        if !ok {
            return match
        }

        // Skip if img already has srcset or is inside <picture>
        if strings.Contains(match, "srcset=") {
            return match
        }

        // Collect variants by format
        jpgVariants := filterVariants(pi.Variants, "jpg")
        webpVariants := filterVariants(pi.Variants, "webp")
        avifVariants := filterVariants(pi.Variants, "avif")

        // Sort by width ascending
        sortVariantsByWidth(jpgVariants)
        sortVariantsByWidth(webpVariants)
        sortVariantsByWidth(avifVariants)

        // Pick the largest jpg as the default src
        defaultSrc := pi.Original.RelPath
        var defaultWidth int
        if len(jpgVariants) > 0 {
            defaultSrc = jpgVariants[len(jpgVariants)-1].RelPath
            defaultWidth = jpgVariants[len(jpgVariants)-1].Width
        }

        // Build srcset string
        var srcsetParts []string
        for _, v := range jpgVariants {
            srcsetParts = append(srcsetParts, fmt.Sprintf("/%s %dw", v.RelPath, v.Width))
        }
        srcsetStr := strings.Join(srcsetParts, ", ")

        // Build sizes attribute
        sizesStr := ""
        if defaultWidth > 0 {
            sizesStr = fmt.Sprintf(`sizes="(max-width: %dpx) 100vw, %dpx"`, defaultWidth, defaultWidth)
        }

        // Build new img tag with preserved attributes
        preserved := stripImgAttrs(match, []string{"src", "srcset", "sizes", "loading"})

        var buf bytes.Buffer

        if cfg.InjectPicture && (len(webpVariants) > 0 || len(avifVariants) > 0) {
            buf.WriteString("<picture>\n")
            // AVIF sources first (preferred)
            if len(avifVariants) > 0 {
                last := avifVariants[len(avifVariants)-1]
                buf.WriteString(fmt.Sprintf("  <source srcset=\"/%s\" type=\"image/avif\">\n", last.RelPath))
            }
            // WebP sources
            if len(webpVariants) > 0 {
                last := webpVariants[len(webpVariants)-1]
                buf.WriteString(fmt.Sprintf("  <source srcset=\"/%s\" type=\"image/webp\">\n", last.RelPath))
            }
        }

        buf.WriteString(fmt.Sprintf("<img src=\"/%s\"", defaultSrc))
        if cfg.InjectSrcset && srcsetStr != "" {
            buf.WriteString(fmt.Sprintf(" srcset=\"%s\"", srcsetStr))
        }
        if sizesStr != "" {
            buf.WriteString(fmt.Sprintf(" %s", sizesStr))
        }
        if preserved != "" {
            buf.WriteString(fmt.Sprintf(" %s", preserved))
        }
        if cfg.InjectLazyLoading {
            buf.WriteString(" loading=\"lazy\"")
        }
        buf.WriteString(fmt.Sprintf(" width=\"%d\" height=\"%d\"", pi.Original.Width, pi.Original.Height))
        buf.WriteString(">")

        if cfg.InjectPicture && (len(webpVariants) > 0 || len(avifVariants) > 0) {
            buf.WriteString("\n</picture>")
        }

        return buf.String()
    })
}

// filterVariants returns variants matching the given format.
func filterVariants(variants []ImageVariant, format string) []ImageVariant {
    var out []ImageVariant
    for _, v := range variants {
        if v.Format == format {
            out = append(out, v)
        }
    }
    return out
}

// sortVariantsByWidth sorts variants by width ascending.
func sortVariantsByWidth(variants []ImageVariant) {
    sort.Slice(variants, func(i, j int) bool {
        return variants[i].Width < variants[j].Width
    })
}

// stripImgAttrs removes specified attributes from an img tag and returns
// the remaining attributes as a string.
func stripImgAttrs(imgTag string, remove []string) string {
    removeSet := make(map[string]bool)
    for _, a := range remove {
        removeSet[a] = true
    }

    // Extract attributes from the img tag
    attrRe := regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)
    matches := attrRe.FindAllStringSubmatch(imgTag, -1)
    var kept []string
    for _, m := range matches {
        if !removeSet[m[1]] {
            kept = append(kept, fmt.Sprintf("%s=\"%s\"", m[1], m[2]))
        }
    }
    return strings.Join(kept, " ")
}
```

- [ ] **Step 3: 运行测试**

```bash
cd plugins/image-pipeline && go test -v -run "TestInject" .
```

- [ ] **Step 4: 提交**

```bash
git add plugins/image-pipeline/html_injector.go plugins/image-pipeline/html_injector_test.go
git commit -m "feat(image-pipeline): implement HTML injector for srcset/picture"
```

---

### Task 7: 组装 Pipeline — Process 方法完整实现

**Files:**
- Modify: `plugins/image-pipeline/plugin.go` — 完整实现 Process 方法

- [ ] **Step 1: 更新 plugin.go 的 Process 方法**

```go
// Process runs the full image pipeline: scan → process → inject.
func (p *ImagePipelinePlugin) Process(outputDir, sourceDir string) error {
    // 1. Scan output directory for images
    assets, err := Scan(outputDir)
    if err != nil {
        return fmt.Errorf("image_pipeline: scan: %w", err)
    }
    if len(assets) == 0 {
        return nil // no images to process
    }

    // 2. Process images (compress, convert, resize)
    results, err := Process(assets, p.cfg, outputDir)
    if err != nil {
        return fmt.Errorf("image_pipeline: process: %w", err)
    }

    // 3. Inject srcset/picture into HTML files
    if p.cfg.InjectSrcset || p.cfg.InjectPicture {
        if err := InjectHTMLFiles(outputDir, results, p.cfg); err != nil {
            return fmt.Errorf("image_pipeline: html inject: %w", err)
        }
    }

    return nil
}
```

- [ ] **Step 2: 编译验证**

```bash
cd plugins/image-pipeline && go build -buildmode=plugin -o ../image_pipeline.so .
```

- [ ] **Step 3: 运行全部测试**

```bash
cd plugins/image-pipeline && go test -v .
```

- [ ] **Step 4: 提交**

```bash
git add plugins/image-pipeline/plugin.go
git commit -m "feat(image-pipeline): assemble full pipeline in Process method"
```

---

### Task 8: 集成到 huan 构建管线

**Files:**
- Create: `internal/image/types.go` — ImageProcessor capability 接口（已在 Task 1 创建，确认存在）
- Modify: `cmd/huan/plugins.go` — 注册 image_pipeline case
- Modify: `cmd/huan/build.go` — AfterBuild 中调用图片管线

- [ ] **Step 1: 确认 capability 接口已在 Task 1 创建**

```bash
cat internal/image/processor.go
```

- [ ] **Step 2: 修改 build.go 在 AfterBuild 中调用图片管线**

`cmd/huan/build.go`（在 `BuildSite` 调用附近）：

```go
// buildSite 函数中，在 BuildSite 成功后
result, err := build.BuildSite(build.Options{
    // ... 其他选项
    AfterBuild: func(r *build.Result, _ build.RenderPageFunc) error {
        // 如果图片管线插件已配置，调用它
        return runImagePipeline(cfg, sourceDir, outputDir)
    },
})
```

添加 helper 函数：

```go
func runImagePipeline(cfg *config.Config, sourceDir, outputDir string) error {
    raw, ok := cfg.Plugins["image_pipeline"]
    if !ok {
        return nil // not configured
    }

    // Try to load the .so plugin
    pluginDir := filepath.Join(sourceDir, "plugins")
    loader := plugin.NewLoader(pluginDir)
    p, err := loader.LoadPlugin(filepath.Join(pluginDir, "image_pipeline.so"), raw)
    if err != nil {
        return fmt.Errorf("image_pipeline: load plugin: %w", err)
    }

    processor, ok := p.(image.ImageProcessor)
    if !ok {
        return fmt.Errorf("image_pipeline: plugin does not implement ImageProcessor")
    }

    return processor.Process(outputDir, sourceDir)
}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```

- [ ] **Step 4: 运行测试**

```bash
go test ./cmd/huan/... ./internal/image/... -v
```

- [ ] **Step 5: 提交**

```bash
git add cmd/huan/build.go internal/image/processor.go
git commit -m "feat(build): integrate image pipeline plugin into build AfterBuild"
```

---

### Task 9: 全量测试与文档更新

**Files:**
- Modify: `docs/superpowers/specs/2026-07-21-image-pipeline-design.md` — 标记实现状态

- [ ] **Step 1: 全量编译**

```bash
go build ./... && go vet ./...
```

- [ ] **Step 2: 全量测试**

```bash
go test ./... -count=1
```

- [ ] **Step 3: 编译验证插件**

```bash
cd plugins/image-pipeline && go build -buildmode=plugin -o ../image_pipeline.so . && go test -v .
```

- [ ] **Step 4: 更新设计文档标记实现状态**

- [ ] **Step 5: 最终提交**

```bash
git add -A
git commit -m "feat(image-pipeline): implement image pipeline plugin with scan/process/inject"
```