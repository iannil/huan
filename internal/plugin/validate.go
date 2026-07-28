package plugin

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/iannil/huan/internal/config"
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

	for i := range schema.Fields {
		f := &schema.Fields[i]
		knownKeys[f.Key] = f
		if f.Required {
			requiredKeys[f.Key] = true
		}
	}

	// Check required fields: must exist and be non-zero
	for key := range requiredKeys {
		val, exists := raw[key]
		if !exists {
			issues = append(issues, fmt.Sprintf("plugin %q: missing required field %q", name, key))
			continue
		}
		// Check for zero/empty values
		if val == nil || val == "" || val == 0 || val == false {
			issues = append(issues, fmt.Sprintf("plugin %q: required field %q is empty", name, key))
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
	switch v := val.(type) {
	case string:
		actual = "string"
	case int, int64:
		actual = "int"
	case float64:
		// YAML unmarshals numbers as int or float64. Accept only whole numbers as int.
		if v == float64(int64(v)) {
			actual = "int"
		} else {
			actual = "float64"
		}
	case bool:
		actual = "bool"
	case []any:
		actual = "string_slice"
	case map[string]any:
		actual = "map"
	default:
		actual = reflect.TypeOf(val).String()
	}

	if actual != expectedType {
		return fmt.Sprintf("plugin %q: field %q: got %s, want %s", name, key, actual, expectedType)
	}
	return ""
}

// ValidateRawConfigs validates all plugin configs against their schemas.
// Returns errors and warnings separately. Plugins that don't implement
// SchemaProvider are skipped.
//
// soExists reports whether a plugin's .so file is resolvable on disk (e.g.
// via Loader.Resolve). A plugin declared in yaml but absent from `registry`
// is not necessarily a problem: build-stage registries deliberately exclude
// dynamic plugins (deploy/translate/…), which the daemon or a command loads
// from their .so at runtime. So a missing-from-registry plugin only warrants
// a warning when its .so is genuinely absent. When soExists is nil, every
// missing-from-registry plugin is treated as having no .so (warn).
func ValidateRawConfigs(registry *Registry, rawConfigs map[string]config.PluginConfig, soExists func(name string) bool) (errors, warnings []string) {
	for name, pc := range rawConfigs {
		raw := stripReservedFields(pc.Config)
		p, ok := registry.Get(name)
		if !ok {
			// Its .so is resolvable — it will be loaded at runtime by the
			// daemon/command. Stay silent (expected, not a failure).
			if soExists != nil && soExists(name) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("plugin %q: declared in yaml but no .so found in $HUAN_HOME or plugins/", name))
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

// stripReservedFields removes PluginConfig-level fields (like "category")
// that are not part of the plugin's own config schema.
func stripReservedFields(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "category" {
			continue
		}
		out[k] = v
	}
	return out
}
