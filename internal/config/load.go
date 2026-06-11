package config

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and parses huan.yaml from the given source directory.
// It starts from Defaults and overlays the parsed values.
func Load(sourceDir string) (*Config, error) {
	path := filepath.Join(sourceDir, "huan.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := Defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.BaseURLTemplate = template.URL(cfg.BaseURL)

	return cfg, nil
}
