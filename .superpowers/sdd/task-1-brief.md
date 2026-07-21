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

