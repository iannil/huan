# Cloudflare & Qwen3 Translate 插件解耦计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 cloudflare 和 qwen3_translate 从 huan 编译期内置插件迁移为独立仓库的 `.so` 插件，彻底解耦。huan 主仓库不再 import 这两个插件的实现代码。

**Architecture:** 两个插件各自独立为 Go module，通过 `go build -buildmode=plugin` 编译为 `.so`。huan daemon 启动时通过 Loader 加载。插件代码引用 `internal/deploy` 和 `internal/translate` 的 capability 接口，这些接口留在 huan 主仓库。插件仓库通过 `go.mod` replace 指向本地 huan 路径来引用内部包。

**Tech Stack:** Go 1.26.2, go plugin, gopkg.in/yaml.v3

## Global Constraints

- huan 主仓库的 `go.mod` 不再 import `cloudflare` 或 `qwen3` 包
- `internal/deploy/types.go` 和 `internal/translate/types.go` 保留在 huan 主仓库（capability 接口）
- `internal/observability` 保留在 huan 主仓库
- 插件 `.so` 通过 `--plugin-dir` 加载
- 插件 `.so` 编译时通过 `go.mod replace` 引用 huan 内部包
- 所有现有测试通过

---

### Task 1: 清理 huan 主仓库中的编译期插件注册

**Files:**
- Modify: `cmd/huan/plugins.go` — 删除 cloudflare 和 qwen3_translate 的 case
- Modify: `cmd/huan/plugins_test.go` — 删除对应测试
- Modify: `cmd/huan/plugin_cmd.go` — 删除 `translate` capability 引用（已不存在）
- Delete: `cmd/huan/plugins.go` 中 `import` 的 cloudflare 和 qwen3 包

**注意：此任务只做删除，不创建任何新文件。插件代码 `internal/deploy/cloudflare/` 和 `internal/translate/qwen3/` 暂时保留（Task 2 迁移到独立仓库时会删除）**

- [ ] **Step 1: 删除 plugins.go 中的 cloudflare 和 qwen3_translate case**

```go
// cmd/huan/plugins.go — 修改 newPluginRegistry
func newPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
    r := plugin.NewRegistry()
    for name, raw := range cfg.Plugins {
        switch name {
        // 删除 cloudflare 和 qwen3_translate 的 case
        // 未知插件仍报错（fail-fast）
        default:
            return nil, fmt.Errorf("plugin %q: unknown (not compiled in)", name)
        }
    }
    return r, nil
}
```

同时删除 `import` 中的 cloudflare 和 qwen3 包。

- [ ] **Step 2: 更新 capabilityLabels — 删除 translate 引用**

```go
// cmd/huan/plugins.go — capabilityLabels
func capabilityLabels(p plugin.Plugin) []string {
    var labels []string
    if _, ok := p.(deploy.Deployer); ok {
        labels = append(labels, "deploy")
    }
    // 删除 translate.Translator 引用
    // 因为 qwen3 不再编译期内置，运行时通过 .so 加载，不会被 capabilityLabels 看到
    // 但 deploy.Deployer 保留（未来可能有其他编译期 deploy 插件）
    return labels
}
```

- [ ] **Step 3: 删除 `import "github.com/iannil/huan/internal/translate"` （如果只剩下 capabilityLabels 在用）**

删除后，`cmd/huan/plugins.go` 不再 import `internal/translate`。

- [ ] **Step 4: 更新 plugins_test.go — 删除 qwen3 相关测试**

删除 `TestNewPluginRegistry_ValidCloudflare` 和 `TestNewPluginRegistry_CloudflareMissingFieldsReturnsError` 等测试（因为它们依赖 `newPluginRegistry` 中 `cloudflare` case 的存在）。

- [ ] **Step 5: 更新 plugin_cmd.go — 删除 `translate` import**

删除 `import "github.com/iannil/huan/internal/translate"`（如果已不再使用）。

- [ ] **Step 6: 编译验证**

```bash
go build ./... && go vet ./...
```

预期：BUILD SUCCESS（但 `go test ./...` 中 plugins_test.go 的测试会失败，因为删除了 cloudflare case）

- [ ] **Step 7: 更新测试文件**

修改 `plugins_test.go`，删除依赖 cloudflare 和 qwen3 的测试用例。

- [ ] **Step 8: 运行测试**

```bash
go test ./cmd/huan/... -v
```

预期：ALL PASS

- [ ] **Step 9: 提交**

```bash
git add cmd/huan/plugins.go cmd/huan/plugins_test.go cmd/huan/plugin_cmd.go
git commit -m "refactor(plugin): remove cloudflare and qwen3_translate from compiled-in plugins"
```

---

### Task 2: 创建 cloudflare 独立插件仓库

**Files:**
- Create: `../huan-plugin-cloudflare/` — 独立目录
- Create: `../huan-plugin-cloudflare/go.mod`
- Create: `../huan-plugin-cloudflare/plugin.go` — 主插件文件（InitPlugin 导出）
- Create: `../huan-plugin-cloudflare/options.go` — 配置解析
- Copy: 从 `internal/deploy/cloudflare/` 复制所需文件
- Delete: 从 huan 主仓库删除 `internal/deploy/cloudflare/` 目录

**关键设计：** 插件仓库通过 `go.mod` 的 `replace` 指令引用 huan 主仓库的内部包：

```
// huan-plugin-cloudflare/go.mod
module github.com/iannil/huan-plugin-cloudflare

go 1.26.2

require github.com/iannil/huan v0.6.0

replace github.com/iannil/huan => ../huan
```

- [ ] **Step 1: 创建插件仓库目录和 go.mod**

```bash
mkdir -p /Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare
```

`/Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare/go.mod`：

```
module github.com/iannil/huan-plugin-cloudflare

go 1.26.2

require github.com/iannil/huan v0.0.0

replace github.com/iannil/huan => /Users/rong.zhu/Code/zhurong/huan
```

- [ ] **Step 2: 复制 cloudflare 插件代码**

从 `internal/deploy/cloudflare/` 复制所有 `.go` 文件到 `huan-plugin-cloudflare/`。

注意：`client.go`、`concurrency.go`、`git.go`、`hash.go`、`manifest.go`、`options.go`、`pages.go`、`plugin.go`、`r2.go`、`retry.go`、`worker.go` 都需要复制。

- [ ] **Step 3: 创建 InitPlugin 入口**

`/Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare/plugin_main.go`：

```go
package main

import (
    "github.com/iannil/huan/internal/deploy/cloudflare"
    "github.com/iannil/huan/internal/plugin"
)

// InitPlugin 是 .so 插件加载器查找的导出符号
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := cloudflare.ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    return cloudflare.New(parsedCfg), nil
}
```

- [ ] **Step 4: 编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare
go build -buildmode=plugin -o cloudflare.so .
```

预期：BUILD SUCCESS

- [ ] **Step 5: 删除 huan 主仓库的 cloudflare 代码**

```bash
rm -rf /Users/rong.zhu/Code/zhurong/huan/internal/deploy/cloudflare/
```

- [ ] **Step 6: 删除 deploy 包中的 cloudflare 引用**

检查 `internal/deploy/` 中是否还有 cloudflare 引用（如 `types.go` 中的 import），清理。

- [ ] **Step 7: 编译验证 huan 主仓库**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go build ./...
```

预期：BUILD SUCCESS（deploy 包本身不依赖 cloudflare 子包）

- [ ] **Step 8: 运行测试**

```bash
go test ./internal/deploy/... -v
```

预期：ALL PASS

- [ ] **Step 9: 提交**

```bash
# 在 huan 主仓库
git add -A
git commit -m "refactor(deploy): extract cloudflare plugin to external repository"
```

---

### Task 3: 创建 qwen3_translate 独立插件仓库

**Files:**
- Create: `../huan-plugin-qwen3/` — 独立目录
- Create: `../huan-plugin-qwen3/go.mod`
- Create: `../huan-plugin-qwen3/plugin_main.go` — InitPlugin 导出
- Copy: 从 `internal/translate/qwen3/` 复制所需文件
- Delete: 从 huan 主仓库删除 `internal/translate/qwen3/` 目录

- [ ] **Step 1: 创建插件仓库目录和 go.mod**

```bash
mkdir -p /Users/rong.zhu/Code/zhurong/huan-plugin-qwen3
```

`/Users/rong.zhu/Code/zhurong/huan-plugin-qwen3/go.mod`：

```
module github.com/iannil/huan-plugin-qwen3

go 1.26.2

require github.com/iannil/huan v0.0.0

replace github.com/iannil/huan => /Users/rong.zhu/Code/zhurong/huan
```

- [ ] **Step 2: 复制 qwen3 插件代码**

从 `internal/translate/qwen3/` 复制所有 `.go` 文件到 `huan-plugin-qwen3/`。

- [ ] **Step 3: 创建 InitPlugin 入口**

`/Users/rong.zhu/Code/zhurong/huan-plugin-qwen3/plugin_main.go`：

```go
package main

import (
    "github.com/iannil/huan/internal/plugin"
    "github.com/iannil/huan/internal/translate/qwen3"
)

// InitPlugin 是 .so 插件加载器查找的导出符号
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := qwen3.ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    // qwen3.New 需要 projectRoot 参数，从 cfg 中获取
    // 或者通过其他方式传递
    projectRoot := ""
    if v, ok := cfg["_project_root"].(string); ok {
        projectRoot = v
    }
    return qwen3.New(parsedCfg, projectRoot)
}
```

注意：qwen3.New 需要 `projectRoot` 参数，需要让 Loader 支持传递额外配置。

- [ ] **Step 4: 编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan-plugin-qwen3
go build -buildmode=plugin -o qwen3.so .
```

预期：BUILD SUCCESS

- [ ] **Step 5: 删除 huan 主仓库的 qwen3 代码**

```bash
rm -rf /Users/rong.zhu/Code/zhurong/huan/internal/translate/qwen3/
```

- [ ] **Step 6: 编译验证 huan 主仓库**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go build ./...
```

预期：BUILD SUCCESS

- [ ] **Step 7: 运行测试**

```bash
go test ./internal/translate/... -v
```

预期：ALL PASS（translate 包本身不依赖 qwen3 子包）

- [ ] **Step 8: 提交**

```bash
git add -A
git commit -m "refactor(translate): extract qwen3 plugin to external repository"
```

---

### Task 4: Loader 增强 — 支持传递配置和额外参数

**Files:**
- Modify: `internal/plugin/loader.go` — LoadPlugin 接受 config map 参数
- Modify: `internal/plugin/lifecycle.go` — 同步更新调用
- Modify: `internal/plugin/loader_test.go` — 同步测试

**背景：** 当前 Loader 的 LoadPlugin 只传空 map 给 InitPlugin。现在需要传递 huan.yaml 中的 `plugins.cloudflare.*` 配置，以及 qwen3 需要的 `_project_root` 等额外参数。

- [ ] **Step 1: 修改 LoadPlugin 签名**

```go
// LoadPlugin 加载 .so 并传递配置
func (l *Loader) LoadPlugin(path string, pluginCfg map[string]any) (Plugin, error)
```

- [ ] **Step 2: 修改 LifecycleManager 的 Load 方法**

```go
// Load 接受 config 参数
func (m *LifecycleManager) Load(soPath string, pluginCfg map[string]any) (Plugin, error)
```

- [ ] **Step 3: 修改 daemon.go 中 LifecycleManager 的 Start 调用**

在 `Start()` 中，`ScanAndLoad` 后需要从 cfg.Plugins 中查找对应的配置，并传入。

- [ ] **Step 4: 更新测试**

```bash
go test ./internal/plugin/... -v
```

- [ ] **Step 5: 提交**

```bash
git add internal/plugin/loader.go internal/plugin/lifecycle.go
git commit -m "feat(plugin): support passing config to .so plugin InitPlugin"
```

---

### Task 5: 更新 deploy 和 translate CLI 命令以使用插件 Registry

**Files:**
- Modify: `cmd/huan/deploy.go` — 从 Registry 获取 deployer
- Modify: `cmd/huan/translate_cmd.go` — 从 Registry 获取 translator

**背景：** 当前 `deploy.go` 和 `translate_cmd.go` 硬编码了 `registry.Get("cloudflare")` 和 `registry.Get("qwen3_translate")`。插件解耦后，这些命令仍然可以通过 Registry 找到插件（如果已加载）。

- [ ] **Step 1: 检查 deploy.go — 确认 registry.Get("cloudflare") 会从运行时 Registry 查找**

`deploy.go` 的 `RunE` 中：

```go
p, ok := registry.Get("cloudflare")
```

当前 `registry` 来自 `newPluginRegistry(cfg)` — 但 cloudflare 已不在编译期插件中。需要改为从 daemon 的运行时 Registry 查找，或者在 CLI 命令中直接尝试加载 .so。

- [ ] **Step 2: 修改 deploy.go 支持从 .so 加载**

当 `registry.Get("cloudflare")` 找不到时，尝试从 `--plugin-dir` 或默认路径加载 `.so`：

```go
// 如果编译期未找到，尝试从 .so 加载
if !ok {
    loader := plugin.NewLoader(pluginDir)
    p, err = loader.LoadPlugin(pluginCandidatePath, cfg.Plugins["cloudflare"])
    if err != nil {
        return fmt.Errorf("cloudflare plugin not found (not compiled in and no .so loaded)")
    }
}
```

- [ ] **Step 3: 同样修改 translate_cmd.go**

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```

- [ ] **Step 5: 运行测试**

```bash
go test ./cmd/huan/... -v
```

- [ ] **Step 6: 提交**

```bash
git add cmd/huan/deploy.go cmd/huan/translate_cmd.go
git commit -m "feat(cli): support loading deploy/translate plugins from .so at runtime"
```

---

### Task 6: 更新 build 管线中引用 qwen3 配置的地方

**文件:**
- Modify: `internal/build/inject_translations.go` — 引用 `cfg.Plugins["qwen3_translate"]` 的部分

**背景：** `inject_translations.go` 中直接读取 `cfg.Plugins["qwen3_translate"]` 来获取 `site_translations` 配置。插件解耦后，这个配置仍然在 `huan.yaml` 的 `plugins.qwen3_translate` 中。不需要改代码，配置层面不受影响。

- [ ] **Step 1: 确认 inject_translations.go 不依赖 qwen3 包**

检查 `inject_translations.go` 的 import，确认它只引用 `config` 包，不引用 `translate/qwen3`。

- [ ] **Step 2: 不需要修改**

- [ ] **Step 3: 提交 → 无变更**

---

### Task 7: 全量测试与文档更新

**Files:**
- Modify: `docs/superpowers/specs/2026-07-20-daemon-hotplug-plugin-design.md` — 更新状态
- Create: 两个插件仓库的 README

- [ ] **Step 1: 全量编译**

```bash
go build ./... && go vet ./...
```

- [ ] **Step 2: 全量测试**

```bash
go test ./... -count=1
```

- [ ] **Step 3: 更新设计文档**

- [ ] **Step 4: 最终提交**

```bash
git add -A
git commit -m "feat: decouple cloudflare and qwen3 as external .so plugins"
```