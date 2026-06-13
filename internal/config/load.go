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
//
// Pipeline: read bytes → unmarshal to map → ${VAR} interpolate (strict) →
// re-marshal → unmarshal to Config struct. The two-stage unmarshal lets us
// run interpolation on the generic tree before type-checked decoding.
func Load(sourceDir string) (*Config, error) {
	path := filepath.Join(sourceDir, "huan.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Stage 1: unmarshal to generic map.
	var rawMap map[string]any
	if err := yaml.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("parse config (stage 1 raw): %w", err)
	}

	// Stage 2: interpolate ${VAR} (strict mode; unset → error).
	interpolated, err := Interpolate(rawMap)
	if err != nil {
		return nil, fmt.Errorf("interpolate config: %w", err)
	}

	// Stage 3: re-marshal and unmarshal to typed Config.
	interpolatedBytes, err := yaml.Marshal(interpolated)
	if err != nil {
		return nil, fmt.Errorf("re-marshal interpolated config: %w", err)
	}

	cfg := Defaults()
	if err := yaml.Unmarshal(interpolatedBytes, cfg); err != nil {
		return nil, fmt.Errorf("parse config (stage 3 typed): %w", err)
	}

	// Keep Services.RSS in sync with the top-level RSS config (Hugo compat)
	cfg.Services.RSS = cfg.RSS

	cfg.BaseURLTemplate = template.URL(cfg.BaseURL)

	return cfg, nil
}
