package config

import (
	"bufio"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses huan.yaml from the given source directory.
// It starts from Defaults and overlays the parsed values.
//
// Pipeline: read .env → inject env vars → read bytes → unmarshal to map →
// ${VAR} interpolate (strict) → re-marshal → unmarshal to Config struct.
// The two-stage unmarshal lets us run interpolation on the generic tree
// before type-checked decoding.
//
// .env loading: if <sourceDir>/.env exists, its KEY=VALUE lines are loaded
// into os.Environ BEFORE huan.yaml interpolation. Existing env vars are NOT
// overridden (CI-injected vars take precedence over local .env). This lets
// projects keep CF API tokens / API keys in a gitignored .env file locally
// while CI injects them via the workflow's env: block.
func Load(sourceDir string) (*Config, error) {
	// Stage 0: load .env if present (does NOT override existing env vars).
	if err := loadDotEnv(sourceDir); err != nil {
		return nil, fmt.Errorf("load .env: %w", err)
	}

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

// loadDotEnv reads <sourceDir>/.env (if present) and sets each KEY=VALUE
// pair into os.Environ via os.Setenv. DOES NOT override vars that are
// already set in the environment — CI-injected credentials take precedence
// over local .env entries.
//
// File format (standard dotenv):
//   - Lines starting with # or empty lines are ignored
//   - Lines must match KEY=VALUE format (KEY must be [A-Z_][A-Z0-9_]*)
//   - Surrounding quotes on VALUE are stripped ("value" → value, 'value' → value)
//   - Inline comments after VALUE are NOT supported (would be ambiguous with =)
//   - No shell expansion (no $VAR substitution within values)
//
// Returns nil (no-op) when .env file doesn't exist — most huan projects
// don't use .env and shouldn't be forced to.
func loadDotEnv(sourceDir string) error {
	envPath := filepath.Join(sourceDir, ".env")
	f, err := os.Open(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no .env file — fine
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		// Skip empty + comment lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Must have = separator
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return fmt.Errorf(".env line %d: missing '=' in %q", lineNum, line)
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		// Validate key format (matches ${VAR} interpolation regex)
		if len(key) < 1 || !isValidEnvKey(key) {
			return fmt.Errorf(".env line %d: invalid key %q (must match [A-Z_][A-Z0-9_]*)", lineNum, key)
		}
		// Strip surrounding quotes if present
		value = stripEnvValueQuotes(value)
		// DO NOT override existing env var (CI-injected takes precedence)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, value)
	}
	return scanner.Err()
}

// isValidEnvKey returns true if key matches [A-Z_][A-Z0-9_]* (same regex
// as ${VAR} interpolation in interpolate.go).
func isValidEnvKey(key string) bool {
	if len(key) < 1 {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if !((r >= 'A' && r <= 'Z') || r == '_') {
				return false
			}
			continue
		}
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

// stripEnvValueQuotes strips surrounding single or double quotes from a
// dotenv value. Returns input unchanged if quotes don't match.
//
//   "value" → value
//   'value' → value
//   "value  → "value (no closing quote, leave alone)
//   value   → value (no quotes)
func stripEnvValueQuotes(v string) string {
	if len(v) < 2 {
		return v
	}
	first, last := v[0], v[len(v)-1]
	if first == '"' && last == '"' {
		return v[1 : len(v)-1]
	}
	if first == '\'' && last == '\'' {
		return v[1 : len(v)-1]
	}
	return v
}
