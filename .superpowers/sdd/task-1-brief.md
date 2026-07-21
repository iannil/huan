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

