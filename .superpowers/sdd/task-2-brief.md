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

