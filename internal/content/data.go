package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadDataFiles loads all data files from the data/ directory.
// Supports .yaml, .yml, .json. Returns a nested map matching the directory structure.
// e.g., data/books.yaml → map["books"], data/encrypted/content.json → map["encrypted"]["content"]
func LoadDataFiles(dataDir string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("read data dir: %w", err)
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dataDir, entry.Name())

		if entry.IsDir() {
			sub, err := LoadDataFiles(fullPath)
			if err != nil {
				return nil, err
			}
			result[entry.Name()] = sub
			continue
		}

		ext := filepath.Ext(entry.Name())
		name := strings.TrimSuffix(entry.Name(), ext)

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fullPath, err)
		}

		switch ext {
		case ".yaml", ".yml":
			var val interface{}
			if err := yaml.Unmarshal(data, &val); err != nil {
				return nil, fmt.Errorf("parse %s: %w", fullPath, err)
			}
			result[name] = convertYAMLToMap(val)
		case ".json":
			var val interface{}
			if err := json.Unmarshal(data, &val); err != nil {
				return nil, fmt.Errorf("parse %s: %w", fullPath, err)
			}
			result[name] = val
		}
	}

	return result, nil
}

// convertYAMLToMap ensures yaml.Unmarshal nested structures are map[string]interface{}.
func convertYAMLToMap(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for k, v := range val {
			m[fmt.Sprintf("%v", k)] = convertYAMLToMap(v)
		}
		return m
	case []interface{}:
		for i, item := range val {
			val[i] = convertYAMLToMap(item)
		}
		return val
	default:
		return v
	}
}
