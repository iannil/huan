package main

import (
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
func loadGlossary(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read glossary: %w", err)
	}
	// Strip comment lines (YAML allows # comments but only at start of line
	// when key starts unquoted — be defensive and strip them).
	stripped := stripYAMLComments(string(data))
	var out map[string]string
	if err := yaml.Unmarshal([]byte(stripped), &out); err != nil {
		return nil, fmt.Errorf("parse glossary YAML: %w", err)
	}
	if out == nil {
		out = make(map[string]string)
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
