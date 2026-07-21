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

