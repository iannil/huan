### Task 1: Registry 增强 — 新增 Unregister 方法

**Files:**
- Modify: `internal/plugin/plugin.go` — 新增 `Unregister` 方法
- Modify: `internal/plugin/plugin_test.go` — 新增 `Unregister` 测试

**Interfaces:**
- Produces: `Registry.Unregister(name string) bool`

- [ ] **Step 1: Write the failing test**

在 `internal/plugin/plugin_test.go` 末尾添加：

```go
func TestUnregister_Success(t *testing.T) {
    r := NewRegistry()
    if err := r.Register(&stubPlugin{name: "alpha"}); err != nil {
        t.Fatalf("Register alpha: %v", err)
    }
    got := r.Unregister("alpha")
    if !got {
        t.Error("Unregister(alpha): want true, got false")
    }
    if _, ok := r.Get("alpha"); ok {
        t.Error("Get(alpha) after Unregister returned ok=true")
    }
}

func TestUnregister_NotFound(t *testing.T) {
    r := NewRegistry()
    got := r.Unregister("nonexistent")
    if got {
        t.Error("Unregister(nonexistent): want false, got true")
    }
}

func TestUnregister_MaintainsOrder(t *testing.T) {
    r := NewRegistry()
    _ = r.Register(&stubPlugin{name: "alpha"})
    _ = r.Register(&stubPlugin{name: "bravo"})
    _ = r.Register(&stubPlugin{name: "charlie"})
    r.Unregister("bravo")
    want := []string{"alpha", "charlie"}
    got := r.Names()
    if len(got) != len(want) {
        t.Fatalf("Names() len = %d, want %d", len(got), len(want))
    }
    for i, name := range want {
        if got[i] != name {
            t.Errorf("Names()[%d] = %q, want %q", i, got[i], name)
        }
    }
}
```

Run: `go test ./internal/plugin/ -run "TestUnregister_" -v`
Expected: COMPILATION ERROR or FAIL (no Unregister method)

- [ ] **Step 2: 实现 Unregister**

在 `internal/plugin/plugin.go` 的 `All()` 方法之后添加：

```go
// Unregister removes a plugin by name. Returns false if the name wasn't
// registered. After Unregister, the plugin is no longer returned by Get,
// All, Names, or Find[T].
func (r *Registry) Unregister(name string) bool {
    if _, exists := r.plugins[name]; !exists {
        return false
    }
    delete(r.plugins, name)
    for i, n := range r.order {
        if n == name {
            r.order = append(r.order[:i], r.order[i+1:]...)
            break
        }
    }
    return true
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/plugin/ -run "TestUnregister_" -v`
Expected: All 3 tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/plugin/plugin.go internal/plugin/plugin_test.go
git commit -m "feat(plugin): add Registry.Unregister for runtime plugin removal"
```

---

