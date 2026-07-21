package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config is the typed image pipeline plugin configuration.
type Config struct {
	Formats           []string `yaml:"formats"`
	Quality           int      `yaml:"quality"`
	Sizes             []int    `yaml:"sizes"`
	InjectSrcset      bool     `yaml:"inject_srcset"`
	InjectPicture     bool     `yaml:"inject_picture"`
	InjectLazyLoading bool     `yaml:"inject_lazy_loading"`
	MaxDimension      int      `yaml:"max_dimension"`
	SkipLarger        bool     `yaml:"skip_larger"`
}

// defaults sets sensible defaults for unset fields.
// raw is the original config map used to detect explicit false values.
func (c *Config) defaults(raw map[string]any) {
	if c.Formats == nil {
		c.Formats = []string{"webp"}
	}
	if c.Quality == 0 {
		c.Quality = 80
	}
	if c.Sizes == nil {
		c.Sizes = nil
	}
	if _, ok := raw["inject_srcset"]; !ok {
		c.InjectSrcset = true
	}
	if _, ok := raw["inject_picture"]; !ok {
		c.InjectPicture = true
	}
	if _, ok := raw["inject_lazy_loading"]; !ok {
		c.InjectLazyLoading = true
	}
	if _, ok := raw["skip_larger"]; !ok {
		c.SkipLarger = true
	}
}

// validate returns an error if config is invalid.
func (c Config) validate() error {
	validFormats := map[string]bool{"webp": true, "avif": true}
	for _, f := range c.Formats {
		if !validFormats[f] {
			return fmt.Errorf("image_pipeline: unsupported format %q (supported: webp, avif)", f)
		}
	}
	if c.Quality < 1 || c.Quality > 100 {
		return fmt.Errorf("image_pipeline: quality must be 1-100, got %d", c.Quality)
	}
	for _, s := range c.Sizes {
		if s < 16 {
			return fmt.Errorf("image_pipeline: size %d too small (min 16px)", s)
		}
	}
	if c.MaxDimension < 0 {
		return fmt.Errorf("image_pipeline: max_dimension must be >= 0, got %d", c.MaxDimension)
	}
	return nil
}

// ParseConfig decodes the raw config map into Config with defaults + validation.
func ParseConfig(raw map[string]any) (Config, error) {
	if raw == nil {
		raw = map[string]any{}
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return Config{}, fmt.Errorf("image_pipeline: re-encode config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("image_pipeline: decode config: %w", err)
	}
	cfg.defaults(raw)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}