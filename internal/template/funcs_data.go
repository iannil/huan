package template

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// parseYAML decodes YAML data without imposing a content schema.
func parseYAML(source string) (any, error) {
	var data any
	if err := yaml.Unmarshal([]byte(source), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// errorf aborts template execution with a caller-provided diagnostic.
func errorf(format string, args ...any) (string, error) {
	return "", fmt.Errorf(format, args...)
}
