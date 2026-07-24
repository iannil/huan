package plugin

import (
	"strings"
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
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (unknown field warning), got %d: %v", len(issues), issues)
	}
	if !strings.Contains(issues[0], "WARN:") {
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
	if !strings.Contains(warns[0], "unknown") {
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

func TestValidateConfig_BoolType(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "enabled", Type: "bool", Required: true},
	}}
	raw := map[string]any{"enabled": true}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) != 0 {
		t.Errorf("expected no issues for bool, got %v", issues)
	}
	// Wrong type
	raw2 := map[string]any{"enabled": "yes"}
	issues2 := ValidateConfig("test", schema, raw2)
	if len(issues2) == 0 {
		t.Error("expected error for bool type mismatch")
	}
}

func TestValidateConfig_StringSliceType(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "tags", Type: "string_slice", Required: true},
	}}
	raw := map[string]any{"tags": []any{"a", "b"}}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) != 0 {
		t.Errorf("expected no issues for string_slice, got %v", issues)
	}
	// Wrong type
	raw2 := map[string]any{"tags": "not-a-slice"}
	issues2 := ValidateConfig("test", schema, raw2)
	if len(issues2) == 0 {
		t.Error("expected error for string_slice type mismatch")
	}
}

func TestValidateConfig_MapType(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "metadata", Type: "map", Required: true},
	}}
	raw := map[string]any{"metadata": map[string]any{"key": "val"}}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) != 0 {
		t.Errorf("expected no issues for map, got %v", issues)
	}
	// Wrong type
	raw2 := map[string]any{"metadata": "not-a-map"}
	issues2 := ValidateConfig("test", schema, raw2)
	if len(issues2) == 0 {
		t.Error("expected error for map type mismatch")
	}
}

func TestValidateConfig_Float64AsInt(t *testing.T) {
	// YAML unmarshals "42" as float64 in some cases. Accept as int.
	schema := Schema{Fields: []FieldSchema{
		{Key: "count", Type: "int", Required: true},
	}}
	raw := map[string]any{"count": float64(42)}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) != 0 {
		t.Errorf("expected no issues for float64 whole number as int, got %v", issues)
	}
	// Non-whole float64 should be rejected
	raw2 := map[string]any{"count": float64(3.14)}
	issues2 := ValidateConfig("test", schema, raw2)
	if len(issues2) == 0 {
		t.Error("expected error for float64 non-whole number as int")
	}
}

func TestValidateConfig_RequiredEmptyString(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "apiKey", Type: "string", Required: true},
	}}
	// Empty string for required field should be rejected
	raw := map[string]any{"apiKey": ""}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) == 0 {
		t.Fatal("expected error for empty required string field")
	}
}

func TestValidateConfig_RequiredZeroInt(t *testing.T) {
	schema := Schema{Fields: []FieldSchema{
		{Key: "count", Type: "int", Required: true},
	}}
	// Zero for required int field should be rejected
	raw := map[string]any{"count": 0}
	issues := ValidateConfig("test", schema, raw)
	if len(issues) == 0 {
		t.Fatal("expected error for zero required int field")
	}
}

func containsStr(slice []string, substr string) bool {
	for _, s := range slice {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
