package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config is the typed image-pipeline plugin configuration.
type Config struct {
	// Quality is the compression quality (1-100). Default: 80.
	Quality int `yaml:"quality" json:"quality"`

	// Formats lists output formats to generate (e.g., ["webp", "avif"]).
	Formats []string `yaml:"formats" json:"formats"`

	// Sizes lists responsive image widths to generate (e.g., [320, 640, 1024]).
	Sizes []int `yaml:"sizes" json:"sizes"`

	// Enabled toggles the plugin. Default: true.
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// ParseConfig converts raw map[string]any to typed Config.
func ParseConfig(raw map[string]any) (Config, error) {
	if raw == nil {
		return Config{Quality: 80, Enabled: true}, nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return Config{}, fmt.Errorf("re-marshal image-pipeline config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode image-pipeline config: %w", err)
	}
	// Apply defaults.
	if cfg.Quality == 0 {
		cfg.Quality = 80
	}
	if !cfg.Enabled && raw["enabled"] == nil {
		cfg.Enabled = true
	}
	return cfg, nil
}