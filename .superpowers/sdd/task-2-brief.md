### Task 2: 集成配置校验到 newPluginRegistry

**Files:**
- Modify: `cmd/huan/plugins.go`
- Modify: `cmd/huan/main.go`
- Test: `cmd/huan/plugins_test.go`

**Interfaces:**
- Consumes: `plugin.ValidateConfig`, `plugin.ValidateRawConfigs`, `plugin.SchemaProvider`
- Produces: 在 `newPluginRegistry` 中集成校验逻辑

- [ ] **Step 1: 修改 `cmd/huan/plugins.go`，在 `newPluginRegistry` 末尾集成校验**

在 `newPluginRegistry` 函数末尾添加配置校验逻辑。注意 `newPluginRegistry` 当前只返回 `(*plugin.Registry, error)`，需要改为在返回前校验配置。

```go
func newPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
	r := plugin.NewRegistry()
	for name := range cfg.Plugins {
		switch name {
		// ### Compiled-in plugins ###
		// Add `case "name":` here for plugins compiled into the binary.
		// Example:
		//   case "cloudflare":
		//       cfCfg, err := cloudflare.ParseConfig(raw)
		//       if err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
		//       if err := r.Register(cloudflare.New(cfCfg)); err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }

		// ### .so plugins (handled at runtime by LifecycleManager) ###
		default:
			// .so plugin — will be loaded from the plugins/ directory at
			// runtime. Silently skip at compile time.
		}
	}

	// Validate configs against schemas for compiled-in plugins
	// (SO plugins are validated at load time by the LifecycleManager)
	if errs, warns := plugin.ValidateRawConfigs(r, cfg.Plugins); len(errs) > 0 || len(warns) > 0 {
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "huan: plugin config warning: %s\n", w)
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("plugin config errors:\n  - %s", strings.Join(errs, "\n  - "))
		}
	}

	return r, nil
}
```

需要添加 import：`"strings"` 和 `"os"`。

- [ ] **Step 2: 更新 `cmd/huan/plugins_test.go`，添加配置校验的测试**

```go
func TestNewPluginRegistry_SchemaValidation(t *testing.T) {
	// Register a test schema plugin via the plugins map.
	// Since newPluginRegistry only handles compiled-in plugins (switch cases),
	// unknown plugins go to the default case and produce warnings.
	// This test verifies the warning path.
	cfg := &config.Config{
		Plugins: map[string]map[string]any{
			"unknown_plugin": {"some_field": "value"},
			"another_unknown": {},
		},
	}
	r, err := newPluginRegistry(cfg)
	if err != nil {
		t.Fatalf("unknown plugins should not cause error, got: %v", err)
	}
	if len(r.All()) != 0 {
		t.Errorf("got %d plugins, want 0", len(r.All()))
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./cmd/huan/ -run TestNewPluginRegistry -v`
Expected: ALL PASS

- [ ] **Step 4: 提交**

```bash
git add cmd/huan/plugins.go cmd/huan/plugins_test.go
git commit -m "feat(plugin): integrate config validation into newPluginRegistry

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

