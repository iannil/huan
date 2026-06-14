// Package qwen3 implements the Translator capability via local Qwen3 models
// served by Ollama HTTP API. See docs/adr/0008-translator-capability-qwen3-plugin.md.
package qwen3

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the typed Qwen3 plugin configuration, parsed from
// cfg.Plugins["qwen3_translate"] (the map[string]any from yaml, already
// ${VAR}-interpolated by internal/config/interpolate.go).
type Config struct {
	// Endpoint is the Ollama HTTP base URL (e.g. "http://localhost:11434").
	// Required.
	Endpoint string `yaml:"endpoint" json:"endpoint"`

	// Model is the default model identifier (e.g. "qwen3-next:80b-a3b-instruct-q4_K_M").
	// Required.
	Model string `yaml:"model" json:"model"`

	// FallbackModel is used when the default model is unavailable (e.g.
	// user hasn't pulled it yet). Optional.
	FallbackModel string `yaml:"fallback_model" json:"fallback_model"`

	// TimeoutSeconds is the per-call LLM timeout. Defaults to 120.
	TimeoutSeconds int `yaml:"timeout_seconds" json:"timeout_seconds"`

	// Concurrency limits parallel LLM calls. Ollama single-instance is
	// effectively serial (one model loaded per inference), so default 1.
	// Set higher only if running multiple Ollama instances behind a LB.
	Concurrency int `yaml:"concurrency" json:"concurrency"`

	// SystemPromptFile is the path (relative to project root) to the
	// user-editable system prompt markdown file. Required.
	SystemPromptFile string `yaml:"system_prompt_file" json:"system_prompt_file"`

	// GlossaryFile is the path (relative to project root) to the manual
	// term dictionary YAML (zh-term: en-translation). Required.
	GlossaryFile string `yaml:"glossary_file" json:"glossary_file"`

	// ExamplesDir is the optional path to few-shot example files.
	// Empty in v1 (see ADR 0008 §5).
	ExamplesDir string `yaml:"examples_dir" json:"examples_dir"`

	// Quality configures post-translation quality check thresholds.
	Quality QualityConfig `yaml:"quality" json:"quality"`

	// SiteTranslations caches site-level metadata translations (e.g.
	// subTitle, description, keywords) per target language. Used by
	// i18n build pipeline (see ADR 0007 §6).
	SiteTranslations map[string]SiteTranslation `yaml:"site_translations" json:"site_translations"`
}

// QualityConfig holds post-translation quality check thresholds.
type QualityConfig struct {
	// LengthRatioMin/Max define the acceptable out_chars/src_chars range
	// (character expansion ratio). Outside this range triggers a soft
	// warning (and one retry). Defaults [0.5, 3.5] accommodate zh→en
	// expansion (observed up to ~3.0 on long prose).
	LengthRatioMin float64 `yaml:"length_ratio_min" json:"length_ratio_min"`
	LengthRatioMax float64 `yaml:"length_ratio_max" json:"length_ratio_max"`

	// TargetLanguageThreshold is the minimum fraction of output that must
	// be the target language (0.0-1.0). Default 0.8.
	TargetLanguageThreshold float64 `yaml:"target_language_threshold" json:"target_language_threshold"`

	// MarkdownStructureTolerance is the ±N count diff allowed for
	// headings/lists/links/images between source and output.
	MarkdownStructureTolerance int `yaml:"markdown_structure_tolerance" json:"markdown_structure_tolerance"`

	// EnforceGlossary enables post-validation of glossary compliance.
	EnforceGlossary bool `yaml:"enforce_glossary" json:"enforce_glossary"`

	// RetryOnViolation is the max retry count when soft checks fail.
	RetryOnViolation int `yaml:"retry_on_violation" json:"retry_on_violation"`
}

// SiteTranslation holds translated site-level metadata for one language.
type SiteTranslation struct {
	SubTitle      string   `yaml:"subTitle" json:"subTitle"`
	Description   string   `yaml:"description" json:"description"`
	Keywords      []string `yaml:"keywords" json:"keywords"`
	FooterSlogan  string   `yaml:"footerSlogan" json:"footerSlogan"`
}

// defaults applies sensible defaults for unset fields. Called by ParseConfig.
func (c *Config) defaults() {
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 120
	}
	if c.Concurrency == 0 {
		c.Concurrency = 1
	}
	if c.Quality.LengthRatioMin == 0 {
		c.Quality.LengthRatioMin = 0.5
	}
	if c.Quality.LengthRatioMax == 0 {
		c.Quality.LengthRatioMax = 3.5
	}
	if c.Quality.TargetLanguageThreshold == 0 {
		c.Quality.TargetLanguageThreshold = 0.8
	}
	if c.Quality.MarkdownStructureTolerance == 0 {
		c.Quality.MarkdownStructureTolerance = 2
	}
	if c.Quality.RetryOnViolation == 0 {
		c.Quality.RetryOnViolation = 1
	}
}

// validate returns an error if required fields are missing or have invalid
// values. Called by ParseConfig after defaults.
func (c Config) validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("qwen3: endpoint is required (e.g. http://localhost:11434)")
	}
	if c.Model == "" {
		return fmt.Errorf("qwen3: model is required (e.g. qwen3-next:80b-a3b-instruct-q4_K_M)")
	}
	if c.SystemPromptFile == "" {
		return fmt.Errorf("qwen3: system_prompt_file is required (e.g. i18n/translate-prompt-zh-en.md)")
	}
	if c.GlossaryFile == "" {
		return fmt.Errorf("qwen3: glossary_file is required (e.g. i18n/terms.yaml)")
	}
	return nil
}

// Timeout returns the configured per-call timeout as a time.Duration.
func (c Config) Timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// ParseConfig unmarshals the raw plugin config map into a typed Config and
// applies defaults + validation. The map should already be ${VAR}-interpolated
// by the config layer.
func ParseConfig(raw map[string]any) (Config, error) {
	// Round-trip through YAML re-encode for canonical decoding. This is the
	// same pattern internal/deploy/cloudflare uses — handles nested maps,
	// numeric coercion, and camelCase ↔ snake_case YAML tags uniformly.
	buf, err := yaml.Marshal(raw)
	if err != nil {
		return Config{}, fmt.Errorf("qwen3: re-encode raw config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(buf, &cfg); err != nil {
		return Config{}, fmt.Errorf("qwen3: decode config: %w", err)
	}
	cfg.defaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
