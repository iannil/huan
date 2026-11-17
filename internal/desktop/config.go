package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// appName is the directory name used under the user config dir.
const appName = "huan-desktop"

// maxRecentSites caps the recent-sites list.
const maxRecentSites = 10

// DesktopConfig is the persisted desktop app configuration.
type DesktopConfig struct {
	RecentSites []string `json:"recentSites"`
}

// DefaultConfigDir returns the per-user config directory for the desktop app.
func DefaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, appName), nil
}

// ConfigPath returns the config file path.
func ConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file; a missing file yields the default config.
func Load(path string) (*DesktopConfig, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &DesktopConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg DesktopConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// LoadOrDefault reads the config, recovering from a corrupt file by backing
// it up and returning defaults. Startup must never be blocked by config.
func LoadOrDefault(path string) (*DesktopConfig, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	_ = os.Rename(path, path+".bak")
	return &DesktopConfig{}, nil
}

// Save atomically writes the config, creating parent directories.
func Save(path string, cfg *DesktopConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return os.Rename(tmp, path)
}

// AddRecentSite records a site directory as most-recent, deduplicated,
// capped at maxRecentSites.
func (c *DesktopConfig) AddRecentSite(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	out := make([]string, 0, maxRecentSites)
	out = append(out, abs)
	for _, s := range c.RecentSites {
		if s != abs && len(out) < maxRecentSites {
			out = append(out, s)
		}
	}
	c.RecentSites = out
}
