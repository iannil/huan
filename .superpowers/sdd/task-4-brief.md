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

