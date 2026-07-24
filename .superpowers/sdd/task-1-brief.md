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

