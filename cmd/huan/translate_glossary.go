package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// loadGlossary reads a YAML term dictionary (zh-term: en-translation)
// and returns it as a map.
//
// Format example (i18n/terms.yaml):
//
//	专注: focus
//	觉察: awareness
//	法: Dharma
//	道: the Way
//
// Comments (lines starting with #) and blank lines are ignored.
//
// Values are parsed as STRINGS, even when they look like booleans / numbers
// (e.g., `虚假: false` → "false" not boolean False). The dictionary is a
// zh→en mapping of category labels; English values like "false", "true",
// "null" are legitimate translations and must not be coerced.
func loadGlossary(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read glossary: %w", err)
	}
	// Strip comment lines (YAML allows # comments but only at start of line
	// when key starts unquoted — be defensive and strip them).
	stripped := stripYAMLComments(string(data))

	// Decode into map[string]interface{} first to avoid YAML scalar coercion
	// (false → bool, 42 → int), then stringify values. Decoding directly into
	// map[string]string works for most cases but silently drops entries whose
	// values YAML parses as non-string scalars.
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(stripped), &raw); err != nil {
		return nil, fmt.Errorf("parse glossary YAML: %w", err)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch s := v.(type) {
		case string:
			out[k] = s
		case nil:
			// skip — empty value
		default:
			// Use yaml.Marshal to round-trip non-string scalars (bool/int/float)
			// back to their YAML representation. This makes `false` → "false",
			// `42` → "42", etc.
			var buf bytes.Buffer
			enc := yaml.NewEncoder(&buf)
			enc.SetIndent(2)
			if err := enc.Encode(v); err == nil {
				enc.Close()
				out[k] = strings.TrimSpace(buf.String())
			}
		}
	}
	return out, nil
}

// stripYAMLComments removes lines that start with # (after optional whitespace).
// Inline comments (after content) are NOT stripped because they may appear
// inside quoted strings legitimately.
func stripYAMLComments(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
