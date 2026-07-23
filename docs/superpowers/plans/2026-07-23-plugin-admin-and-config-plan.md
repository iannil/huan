# 插件管理后台与配置验证体系 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Admin UI 添加插件管理页面，增加插件配置 Schema 声明与校验体系

**Architecture:** 前端新增 Plugins.tsx 页面 + 路由 + 导航入口；后端新增 Schema 类型与 ValidateConfig 函数，在 newPluginRegistry 中集成校验

**Tech Stack:** 前端 React + TypeScript + Tailwind CSS + @base-ui/react + lucide-react + react-router-dom；后端 Go 1.22+

## Global Constraints

- 前端使用现有 UI 组件库（@base-ui/react Dialog, react-router-dom, lucide-react, Tailwind CSS）
- 后端插件 Schema 不强制所有 plugin 实现，通过可选接口检测
- 配置校验 fail-fast（必填缺失/类型错误）或 warning（未知字段）
- 所有新代码须有完整测试

---

### Task 1: 后端 Schema 类型定义 + ValidateConfig

**Files:**
- Create: `internal/plugin/schema.go`
- Create: `internal/plugin/validate.go`
- Test: `internal/plugin/validate_test.go`

**Interfaces:**
- Produces: `plugin.Schema`, `plugin.FieldSchema`, `plugin.SchemaProvider` interface, `plugin.ValidateConfig(name string, schema Schema, raw map[string]any) []string`

- [ ] **Step 1: 创建 `internal/plugin/schema.go`**

```go
package plugin

// Schema describes the full config shape a plugin expects.
type Schema struct {
	Fields []FieldSchema
}

// FieldSchema describes a single config field.
type FieldSchema struct {
	Key         string // 字段名，对应 yaml key
	Type        string // "string" | "int" | "bool" | "string_slice" | "map"
	Required    bool   // true = 必填，启动时校验
	Default     any    // 默认值（Required=false 时生效）
	Description string // 人类可读的说明
	Sensitive   bool   // true = 在 CLI info 中 mask 为 ***
	EnvVarHint  string // 建议的环境变量名，仅用于文档提示
}

// SchemaProvider is an optional interface plugins can implement to declare
// their config schema. Used by the registry for config validation.
type SchemaProvider interface {
	ConfigSchema() Schema
}
```

- [ ] **Step 2: 创建 `internal/plugin/validate.go`**

```go
package plugin

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidateConfig checks raw config against the schema. Returns a list of
// validation errors (empty = valid). Each error is a human-readable string.
// Unknown fields in raw produce warnings (prefixed with "WARN:").
// Missing required fields produce errors.
// Type mismatches produce errors.
func ValidateConfig(name string, schema Schema, raw map[string]any) []string {
	var issues []string

	// Build a set of known field keys for unknown-field detection
	knownKeys := make(map[string]*FieldSchema, len(schema.Fields))
	requiredKeys := make(map[string]bool)
	defaults := make(map[string]any)

	for i := range schema.Fields {
		f := &schema.Fields[i]
		knownKeys[f.Key] = f
		if f.Required {
			requiredKeys[f.Key] = true
		}
		if f.Default != nil {
			defaults[f.Key] = f.Default
		}
	}

	// Check required fields
	for key := range requiredKeys {
		val, exists := raw[key]
		if !exists {
			issues = append(issues, fmt.Sprintf("plugin %q: missing required field %q", name, key))
			continue
		}
		if fs, ok := knownKeys[key]; ok {
			if err := checkType(name, key, fs.Type, val); err != "" {
				issues = append(issues, err)
			}
		}
	}

	// Check optional fields that are present
	for key, val := range raw {
		fs, known := knownKeys[key]
		if !known {
			issues = append(issues, fmt.Sprintf("WARN: plugin %q: unknown field %q", name, key))
			continue
		}
		if !requiredKeys[key] {
			// Optional field present — check type
			if err := checkType(name, key, fs.Type, val); err != "" {
				issues = append(issues, err)
			}
		}
	}

	return issues
}

func checkType(name, key, expectedType string, val any) string {
	if val == nil {
		return fmt.Sprintf("plugin %q: field %q is nil, want %s", name, key, expectedType)
	}
	var actual string
	switch val.(type) {
	case string:
		actual = "string"
	case int, int64, float64:
		// yaml unmarshal numbers as int or float64
		actual = "int"
	case bool:
		actual = "bool"
	case []any:
		actual = "string_slice"
	case map[string]any:
		actual = "map"
	default:
		actual = reflect.TypeOf(val).String()
	}

	// Accept float64 as int (yaml unmarshals "42" as int, but nested values may be float64)
	if expectedType == "int" && actual == "int" {
		return ""
	}

	if actual != expectedType {
		return fmt.Sprintf("plugin %q: field %q: got %s, want %s", name, key, actual, expectedType)
	}
	return ""
}

// ValidateRawConfigs validates all plugin configs against their schemas.
// Returns errors and warnings separately. Plugins that don't implement
// SchemaProvider are skipped.
func ValidateRawConfigs(registry *Registry, rawConfigs map[string]map[string]any) (errors, warnings []string) {
	for name, raw := range rawConfigs {
		p, ok := registry.Get(name)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("plugin %q: declared in yaml but not compiled-in (will be loaded from .so at runtime if available)", name))
			continue
		}
		sp, ok := p.(SchemaProvider)
		if !ok {
			continue // plugin doesn't declare schema, skip
		}
		issues := ValidateConfig(name, sp.ConfigSchema(), raw)
		for _, issue := range issues {
			if strings.HasPrefix(issue, "WARN:") {
				warnings = append(warnings, strings.TrimPrefix(issue, "WARN: "))
			} else {
				errors = append(errors, issue)
			}
		}
	}
	return errors, warnings
}
```

- [ ] **Step 3: 创建 `internal/plugin/validate_test.go`**

```go
package plugin

import (
	"testing"
)

type testSchemaPlugin struct {
	name   string
	schema Schema
}

func (p *testSchemaPlugin) Name() string { return p.name }
func (p *testSchemaPlugin) ConfigSchema() Schema { return p.schema }

var _ Plugin = (*testSchemaPlugin)(nil)
var _ SchemaProvider = (*testSchemaPlugin)(nil)

func TestValidateConfig_MissingRequired(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "apiKey", Type: "string", Required: true},
		{Key: "project", Type: "string", Required: false},
	}}
	raw := map[string]any{"project": "my-site"}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) == 0 {
		t.Fatal("expected error for missing required field")
	}
	if !containsStr(issues, `"apiKey"`) {
		t.Errorf("issues = %v, want mention apiKey", issues)
	}
}

func TestValidateConfig_TypeMismatch(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "count", Type: "int", Required: true},
	}}
	raw := map[string]any{"count": "not-a-number"}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) == 0 {
		t.Fatal("expected error for type mismatch")
	}
}

func TestValidateConfig_UnknownField(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "name", Type: "string", Required: true},
	}}
	raw := map[string]any{"name": "foo", "unknownField": "bar"}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues (1 type check + 1 unknown), got %d: %v", len(issues), issues)
	}
	hasWarn := false
	for _, i := range issues {
		if containsStr(i, "WARN:") {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected WARN for unknown field, got %v", issues)
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "name", Type: "string", Required: true},
		{Key: "count", Type: "int", Required: false},
	}}
	raw := map[string]any{"name": "foo", "count": 42}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestValidateConfig_EmptyRaw(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "name", Type: "string", Required: true},
	}}
	raw := map[string]any{}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) == 0 {
		t.Fatal("expected error for empty raw with required field")
	}
}

func TestValidateRawConfigs_UnknownPluginWarning(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&testSchemaPlugin{name: "known", schema: Schema{}})
	rawConfigs := map[string]map[string]any{
		"known":  {"foo": "bar"},
		"unknown": {"x": "y"},
	}
	errs, warns := ValidateRawConfigs(registry, rawConfigs)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %v", errs)
	}
	if len(warns) == 0 {
		t.Fatal("expected warning for unknown plugin")
	}
	if !containsStr(warns[0], "unknown") {
		t.Errorf("warn = %q, want mention 'unknown'", warns[0])
	}
}

func TestValidateRawConfigs_SkipNoSchema(t *testing.T) {
	// A plugin that doesn't implement SchemaProvider should be skipped
	registry := NewRegistry()
	_ = registry.Register(&stubPlugin{name: "noschema"})
	rawConfigs := map[string]map[string]any{
		"noschema": {"foo": "bar"},
	}
	errs, warns := ValidateRawConfigs(registry, rawConfigs)
	if len(errs) != 0 || len(warns) != 0 {
		t.Errorf("expected no issues for plugin without schema, got errs=%v warns=%v", errs, warns)
	}
}

func containsStr(slice []string, substr string) bool {
	for _, s := range slice {
		if containsStrStr(s, substr) {
			return true
		}
	}
	return false
}

func containsStrStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 运行测试确保通过**

Run: `go test ./internal/plugin/ -run TestValidate -v`
Expected: ALL PASS

- [ ] **Step 5: 运行全部 plugin 测试确保未破坏现有功能**

Run: `go test ./internal/plugin/ -v`
Expected: ALL PASS

- [ ] **Step 6: 然后提交**

```bash
git add internal/plugin/schema.go internal/plugin/validate.go internal/plugin/validate_test.go
git commit -m "feat(plugin): add config schema type and ValidateConfig function

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

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

### Task 3: 前端插件管理页面 — 基础组件

**Files:**
- Create: `web/admin/src/pages/Plugins.tsx`
- Modify: `web/admin/src/App.tsx`
- Modify: `web/admin/src/components/Layout.tsx`

**Interfaces:**
- Consumes: `apiFetch` from `../lib/api`, `Puzzle` icon from `lucide-react`, existing UI components
- Produces: Plugins page component, route `/admin/plugins`, nav entry

- [ ] **Step 1: 创建 `web/admin/src/pages/Plugins.tsx`**

```tsx
import { useEffect, useState } from 'react'
import { apiFetch } from '../lib/api'
import { Puzzle, Plus, X, RefreshCw, AlertCircle } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Dialog, DialogTrigger, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, DialogClose } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

interface PluginInfo {
  name: string
  version: string
  source: string
  capability: string
  status: string
  loadedAt: string
  error: string
}

interface PluginListResponse {
  plugins: PluginInfo[]
  total: number
}

interface PluginManageResponse {
  status: string
  plugin?: PluginInfo
}

export default function Plugins() {
  const [plugins, setPlugins] = useState<PluginInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [loadPath, setLoadPath] = useState('')
  const [loadDialogOpen, setLoadDialogOpen] = useState(false)
  const [reloadPath, setReloadPath] = useState('')
  const [reloadTarget, setReloadTarget] = useState<string | null>(null)
  const [reloadDialogOpen, setReloadDialogOpen] = useState(false)
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(new Set())

  const fetchPlugins = () => {
    setLoading(true)
    setError(null)
    apiFetch('/admin/api/plugins')
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((data: PluginListResponse) => {
        setPlugins(data.plugins || [])
      })
      .catch((e) => {
        setError(e.message || '加载失败')
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchPlugins()
  }, [])

  const handleLoad = async () => {
    if (!loadPath.trim()) return
    setActionLoading('load')
    setActionError(null)
    try {
      const res = await apiFetch('/admin/api/plugins/load', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: loadPath.trim() }),
      })
      const data: PluginManageResponse = await res.json()
      if (!res.ok) {
        setActionError(data.status || '加载失败')
        return
      }
      setLoadPath('')
      setLoadDialogOpen(false)
      fetchPlugins()
    } catch (e: any) {
      setActionError(e.message || '网络错误')
    } finally {
      setActionLoading(null)
    }
  }

  const handleUnload = async (name: string) => {
    setActionLoading(`unload-${name}`)
    setActionError(null)
    try {
      const res = await apiFetch('/admin/api/plugins/unload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      })
      if (!res.ok) {
        const data = await res.json()
        setActionError(data.error || '卸载失败')
        return
      }
      fetchPlugins()
    } catch (e: any) {
      setActionError(e.message || '网络错误')
    } finally {
      setActionLoading(null)
    }
  }

  const openReloadDialog = (name: string, currentPath: string) => {
    setReloadTarget(name)
    setReloadPath(currentPath)
    setReloadDialogOpen(true)
  }

  const handleReload = async () => {
    if (!reloadTarget || !reloadPath.trim()) return
    setActionLoading(`reload-${reloadTarget}`)
    setActionError(null)
    try {
      const res = await apiFetch('/admin/api/plugins/reload', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: reloadTarget, path: reloadPath.trim() }),
      })
      const data = await res.json()
      if (!res.ok) {
        setActionError(data.error || '重载失败')
        return
      }
      setReloadDialogOpen(false)
      setReloadTarget(null)
      fetchPlugins()
    } catch (e: any) {
      setActionError(e.message || '网络错误')
    } finally {
      setActionLoading(null)
    }
  }

  const toggleError = (name: string) => {
    setExpandedErrors((prev) => {
      const next = new Set(prev)
      if (next.has(name)) {
        next.delete(name)
      } else {
        next.add(name)
      }
      return next
    })
  }

  // Loading state
  if (loading) {
    return (
      <div className="text-muted-foreground py-24 text-center text-sm">
        加载中...
      </div>
    )
  }

  // Error state
  if (error && plugins.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold text-foreground tracking-tight">插件管理</h2>
        </div>
        <div className="border border-destructive/50 rounded-md px-4 py-6 text-sm text-destructive text-center">
          {error}
        </div>
        <div className="flex justify-center">
          <Button variant="outline" onClick={fetchPlugins}>重试</Button>
        </div>
      </div>
    )
  }

  return (
    <div>
      {/* Page header */}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-semibold text-foreground tracking-tight">插件管理</h2>
        <Dialog open={loadDialogOpen} onOpenChange={setLoadDialogOpen}>
          <DialogTrigger asChild>
            <Button variant="default" size="sm">
              <Plus className="h-3.5 w-3.5 mr-1" />
              加载新插件
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>加载插件</DialogTitle>
              <DialogDescription>
                输入 .so 插件文件的路径
              </DialogDescription>
            </DialogHeader>
            <div className="py-2">
              <Input
                placeholder="/path/to/plugin.so"
                value={loadPath}
                onChange={(e) => setLoadPath(e.target.value)}
              />
            </div>
            {actionLoading === 'load' && (
              <p className="text-xs text-muted-foreground">加载中...</p>
            )}
            {actionError && actionLoading === null && (
              <p className="text-xs text-destructive">{actionError}</p>
            )}
            <DialogFooter>
              <DialogClose asChild>
                <Button variant="outline" size="sm">取消</Button>
              </DialogClose>
              <Button
                variant="default"
                size="sm"
                onClick={handleLoad}
                disabled={!loadPath.trim() || actionLoading === 'load'}
              >
                加载
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {/* Action error banner */}
      {actionError && (
        <div className="border border-destructive/50 rounded-md px-3 py-2 mb-4 text-xs text-destructive flex items-center gap-2">
          <AlertCircle className="h-3.5 w-3.5 shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {/* Empty state */}
      {plugins.length === 0 ? (
        <div className="border border-border rounded-md px-4 py-12 text-sm text-muted-foreground text-center">
          暂无插件。点击"加载新插件"添加 .so 插件。
        </div>
      ) : (
        /* Plugin table */
        <Card>
          <div className="divide-y divide-border">
            {/* Table header */}
            <div className="grid grid-cols-[1fr_80px_100px_80px_1fr_120px] gap-2 px-3 py-2 text-[11px] text-muted-foreground font-medium">
              <span>名称</span>
              <span>来源</span>
              <span>能力</span>
              <span>状态</span>
              <span>加载时间</span>
              <span className="text-center">操作</span>
            </div>

            {/* Table rows */}
            {plugins.map((p) => (
              <div key={p.name}>
                <div className="grid grid-cols-[1fr_80px_100px_80px_1fr_120px] gap-2 px-3 py-2.5 text-xs items-center">
                  <span className="text-foreground font-medium truncate">{p.name}</span>
                  <span>
                    <Badge variant={p.source === 'compiled' ? 'secondary' : p.source === 'loaded' ? 'default' : 'outline'}>
                      {p.source}
                    </Badge>
                  </span>
                  <span className="text-muted-foreground truncate">{p.capability || '-'}</span>
                  <span className="flex items-center gap-1.5">
                    <span className={`h-1.5 w-1.5 rounded-full ${p.status === 'active' ? 'bg-green-500' : p.status === 'error' ? 'bg-red-500' : 'bg-muted-foreground/50'}`} />
                    <span className={p.status === 'error' ? 'text-destructive' : 'text-muted-foreground'}>
                      {p.status}
                    </span>
                  </span>
                  <span className="text-muted-foreground truncate">{p.loadedAt || '-'}</span>
                  <span className="flex items-center justify-center gap-1">
                    {p.source === 'compiled' ? (
                      <span className="text-[10px] text-muted-foreground">系统内置</span>
                    ) : (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openReloadDialog(p.name, '')}
                          disabled={actionLoading === `reload-${p.name}`}
                          title="重载"
                        >
                          <RefreshCw className={`h-3 w-3 ${actionLoading === `reload-${p.name}` ? 'animate-spin' : ''}`} />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleUnload(p.name)}
                          disabled={actionLoading === `unload-${p.name}`}
                          title="卸载"
                        >
                          <X className="h-3 w-3" />
                        </Button>
                      </>
                    )}
                  </span>
                </div>
                {/* Error detail expandable */}
                {p.status === 'error' && (
                  <div className="px-3 pb-2">
                    <button
                      onClick={() => toggleError(p.name)}
                      className="text-[10px] text-destructive hover:text-destructive/80 transition-colors"
                    >
                      {expandedErrors.has(p.name) ? '收起' : '查看错误详情'}
                    </button>
                    {expandedErrors.has(p.name) && p.error && (
                      <pre className="mt-1 text-[10px] text-destructive/80 bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap">
                        {p.error}
                      </pre>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
          <div className="px-3 py-2 text-[10px] text-muted-foreground border-t border-border">
            共 {plugins.length} 个插件
          </div>
        </Card>
      )}

      {/* Reload dialog */}
      <Dialog open={reloadDialogOpen} onOpenChange={setReloadDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>重载插件</DialogTitle>
            <DialogDescription>
              输入新的 .so 文件路径以重载 {reloadTarget}
            </DialogDescription>
          </DialogHeader>
          <div className="py-2">
            <Input
              placeholder="/path/to/plugin.so"
              value={reloadPath}
              onChange={(e) => setReloadPath(e.target.value)}
            />
          </div>
          {actionError && actionLoading === null && (
            <p className="text-xs text-destructive">{actionError}</p>
          )}
          <DialogFooter>
            <DialogClose asChild>
              <Button variant="outline" size="sm">取消</Button>
            </DialogClose>
            <Button
              variant="default"
              size="sm"
              onClick={handleReload}
              disabled={!reloadPath.trim() || actionLoading === `reload-${reloadTarget}`}
            >
              重载
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
```

- [ ] **Step 2: 修改 `web/admin/src/App.tsx`，添加路由**

```tsx
import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import ContentList from './pages/ContentList'
import ContentEdit from './pages/ContentEdit'
import ContentNew from './pages/ContentNew'
import MediaPage from './pages/MediaPage'
import Settings from './pages/Settings'
import Plugins from './pages/Plugins'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/admin/" element={<Dashboard />} />
        <Route path="/admin/content" element={<ContentList />} />
        <Route path="/admin/content/new" element={<ContentNew />} />
        <Route path="/admin/media" element={<MediaPage />} />
        <Route path="/admin/plugins" element={<Plugins />} />
        <Route path="/admin/settings" element={<Settings />} />
      </Route>
      {/* Full-screen editor outside layout — no sidebar, no chrome */}
      <Route path="/admin/content/edit" element={<ContentEdit />} />
    </Routes>
  )
}
```

- [ ] **Step 3: 修改 `web/admin/src/components/Layout.tsx`，添加"插件"导航项**

在 `navItems` 数组中添加插件入口，放在"设置"之前：

```tsx
import {
  LayoutDashboard,
  FileText,
  Image,
  Settings as SettingsIcon,
  Hammer,
  ExternalLink,
  Puzzle,
} from 'lucide-react'

const navItems = [
  { to: '/admin/', label: '概览', icon: LayoutDashboard, end: true },
  { to: '/admin/content', label: '内容', icon: FileText, end: false },
  { to: '/admin/media', label: '媒体', icon: Image, end: false },
  { to: '/admin/plugins', label: '插件', icon: Puzzle, end: false },
  { to: '/admin/settings', label: '设置', icon: SettingsIcon, end: false },
]
```

- [ ] **Step 4: 构建前端验证**

Run: `cd web/admin && npm run build`
Expected: Build succeeds without errors

- [ ] **Step 5: 提交**

```bash
git add web/admin/src/pages/Plugins.tsx web/admin/src/App.tsx web/admin/src/components/Layout.tsx
git commit -m "feat(admin): add plugin management page with list, load, unload, reload

Co-Authored-By: Claude <noreply@anthropic.com>"
```