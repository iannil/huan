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
	AccountID string       `yaml:"accountId" json:"accountId"`
	APIToken  string       `yaml:"apiToken"  json:"apiToken"`
	Pages     PagesConfig  `yaml:"pages"     json:"pages"`
	R2        R2Config     `yaml:"r2"        json:"r2"`
	Worker    WorkerConfig `yaml:"worker"    json:"worker"`
}

// R2Config captures Cloudflare R2 (S3-compatible) settings.
type R2Config struct {
	// AccountID is used to construct the S3 endpoint URL
	// (<accountID>.r2.cloudflarestorage.com). Required unless Endpoint is set.
	AccountID string `yaml:"accountId" json:"accountId"`

	// AccessKeyID and SecretAccessKey are S3-style credentials for R2.
	// Generate in CF dashboard under R2 > Manage R2 API Tokens.
	AccessKeyID     string `yaml:"accessKeyId" json:"accessKeyId"`
	SecretAccessKey string `yaml:"secretAccessKey" json:"secretAccessKey"`

	// Bucket is the R2 bucket name (pre-created in CF dashboard per ADR 0002 §10).
	Bucket string `yaml:"bucket" json:"bucket"`

	// Endpoint overrides the default R2 URL pattern (for testing).
	Endpoint string `yaml:"endpoint" json:"endpoint"`

	// Sync is the list of local-to-remote path mappings. Each entry uploads
	// files from local `From` directory to remote `To` key prefix.
	// Example: {from: "static/images", to: "images"} uploads static/images/a.jpg
	// to bucket key "images/a.jpg".
	Sync []SyncMapping `yaml:"sync" json:"sync"`
}

// SyncMapping declares one local-to-remote path mapping for R2 sync.
type SyncMapping struct {
	// From is the local directory (or single file) to upload.
	From string `yaml:"from" json:"from"`

	// To is the remote key prefix (without trailing slash).
	// For directory mappings, files become <To>/<relative-path>.
	// For single-file mappings, To becomes the key directly.
	To string `yaml:"to" json:"to"`
}

// validate checks that all required R2 fields are present.
func (c R2Config) validate() error {
	// AccountID is required unless Endpoint overrides.
	if c.AccountID == "" && c.Endpoint == "" {
		return fmt.Errorf("r2.accountId is required (or set r2.endpoint for testing)")
	}
	if c.AccessKeyID == "" {
		return fmt.Errorf("r2.accessKeyId is required (typically ${CLOUDFLARE_R2_ACCESS_KEY_ID})")
	}
	if c.SecretAccessKey == "" {
		return fmt.Errorf("r2.secretAccessKey is required (typically ${CLOUDFLARE_R2_SECRET_ACCESS_KEY})")
	}
	if c.Bucket == "" {
		return fmt.Errorf("r2.bucket is required")
	}
	return nil
}

// HasR2Configured returns true if the R2 block looks intentional (any field set).
// Used by Plugin.Deploy to decide whether to error or skip when target="r2".
func (c Config) HasR2Configured() bool {
	r := c.R2
	return r.AccountID != "" || r.AccessKeyID != "" || r.Bucket != "" || len(r.Sync) > 0
}

// WorkerConfig captures Cloudflare Workers settings for the modules API
// (PUT /accounts/{id}/workers/scripts/{name}).
type WorkerConfig struct {
	// Name is the Worker script name (must match across deploys; renames are
	// not supported by the CF API). Required.
	Name string `yaml:"name" json:"name"`

	// Script is the local path (relative to huan.yaml dir) of the single-file
	// ES module .js source. Required.
	Script string `yaml:"script" json:"script"`

	// CompatibilityDate defaults to "2024-01-01" if empty.
	CompatibilityDate string `yaml:"compatibilityDate" json:"compatibilityDate"`

	// Bindings declares resources the Worker can access (R2 buckets, KV
	// namespaces, env vars, etc.). huan serializes them into the upload
	// metadata JSON. See WorkerBinding for supported types.
	Bindings []WorkerBinding `yaml:"bindings" json:"bindings"`

	// Routes declares route patterns the Worker handles. Each entry must
	// include Pattern and Zone (zone name like "zhurongshuo.com").
	Routes []WorkerRoute `yaml:"routes" json:"routes"`
}

// WorkerBinding declares one resource binding for a Worker.
//
// Type values supported by CF Workers modules API:
//   - "r2_bucket"     — R2 bucket binding (requires Bucket)
//   - "kv_namespace"  — KV namespace (requires NamespaceID)
//   - "vars"          — plain-text env var (requires Value)
//   - "secret_text"   — secret env var (requires Value; this is the
//                       non-Wrangler-managed variant)
//   - "d1"            — D1 database (requires ID)
//
// huan does NOT validate binding types — it serializes what you declare and
// lets CF reject unknown types. This keeps the surface minimal as CF adds
// new binding kinds.
type WorkerBinding struct {
	Type        string `yaml:"type"        json:"type"`
	Name        string `yaml:"name"        json:"name"`         // env var name in Worker (e.g. "R2_BUCKET")
	Bucket      string `yaml:"bucket"      json:"bucket,omitempty"`
	NamespaceID string `yaml:"namespaceId" json:"namespace_id,omitempty"`
	ID          string `yaml:"id"          json:"id,omitempty"`
	Value       string `yaml:"value"       json:"value,omitempty"`
}

// WorkerRoute declares one route pattern + zone for the Worker.
type WorkerRoute struct {
	Pattern string `yaml:"pattern" json:"pattern"` // e.g. "r2.zhurongshuo.com/*"
	Zone    string `yaml:"zone"    json:"zone,omitempty"`    // zone name (e.g. "zhurongshuo.com")
}

// validate checks that all required Worker fields are present. CompatibilityDate
// defaults to "2024-01-01" at deploy time, not here (so plugin info reflects
// user intent).
func (c WorkerConfig) validate() error {
	if c.Name == "" {
		return fmt.Errorf("worker.name is required")
	}
	if c.Script == "" {
		return fmt.Errorf("worker.script is required (path to single-file ES module .js)")
	}
	return nil
}

// HasWorkerConfigured returns true if the Worker block looks intentional.
func (c Config) HasWorkerConfigured() bool {
	w := c.Worker
	return w.Name != "" || w.Script != ""
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
