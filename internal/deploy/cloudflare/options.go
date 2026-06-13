package cloudflare

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the typed Cloudflare plugin configuration, parsed from
// cfg.Plugins["cloudflare"] (the map[string]any from yaml). The map is already
// ${VAR}-interpolated by the config layer (see internal/config/interpolate.go).
type Config struct {
	AccountID string      `yaml:"accountId" json:"accountId"`
	APIToken  string      `yaml:"apiToken"  json:"apiToken"`
	Pages     PagesConfig `yaml:"pages"     json:"pages"`
	// R2 R2Config `yaml:"r2"`      // PR2
	// Worker WorkerConfig `yaml:"worker"`  // PR3
}

// PagesConfig captures Cloudflare Pages project settings.
type PagesConfig struct {
	// Project is the CF Pages project name (created in dashboard). Required.
	Project string `yaml:"project" json:"project"`

	// Branch is the default deployment branch (typically "main"). Required.
	// Override at deploy time via --branch=preview or similar.
	Branch string `yaml:"branch" json:"branch"`
}

// ParseConfig decodes raw (already-interpolated) yaml map into typed Config
// and validates required fields. Returns error on missing account_id, token,
// pages.project, or pages.branch.
func ParseConfig(raw map[string]any) (Config, error) {
	if raw == nil {
		return Config{}, fmt.Errorf("cloudflare plugin config is empty")
	}
	// Marshal back to yaml bytes and decode into typed struct. We use yaml
	// round-trip (not json) because huan.yaml is yaml; values that came from
	// ${VAR} interpolation are now plain strings either way.
	data, err := yaml.Marshal(raw)
	if err != nil {
		return Config{}, fmt.Errorf("re-marshal cloudflare config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode cloudflare config: %w", err)
	}

	// Required-field validation. Empty strings indicate the field was missing
	// or referenced an unset env var (caught earlier by strict interpolation).
	if cfg.AccountID == "" {
		return cfg, fmt.Errorf("accountId is required (set plugins.cloudflare.accountId, typically ${CLOUDFLARE_ACCOUNT_ID})")
	}
	if cfg.APIToken == "" {
		return cfg, fmt.Errorf("apiToken is required (set plugins.cloudflare.apiToken, typically ${CLOUDFLARE_API_TOKEN})")
	}
	if cfg.Pages.Project == "" {
		return cfg, fmt.Errorf("pages.project is required")
	}
	if cfg.Pages.Branch == "" {
		return cfg, fmt.Errorf("pages.branch is required")
	}
	return cfg, nil
}

// LoadConfigFromFile is a convenience for tests that want to parse a yaml file
// directly without going through the full huan config pipeline.
func LoadConfigFromFile(path string) (Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Config{}, fmt.Errorf("read %q: %w", path, err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse %q: %w", path, err)
	}
	return ParseConfig(raw)
}
