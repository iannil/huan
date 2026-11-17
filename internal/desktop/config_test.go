package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if len(cfg.RecentSites) != 0 {
		t.Errorf("expected empty recent sites, got %v", cfg.RecentSites)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &DesktopConfig{}
	cfg.AddRecentSite("/tmp/site-a")
	cfg.AddRecentSite("/tmp/site-b")
	cfg.AddRecentSite("/tmp/site-a") // dup moves to front
	if cfg.RecentSites[0] != "/tmp/site-a" || len(cfg.RecentSites) != 2 {
		t.Fatalf("recent order wrong: %v", cfg.RecentSites)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.RecentSites[0] != "/tmp/site-a" || len(got.RecentSites) != 2 {
		t.Errorf("round trip mismatch: %v", got.RecentSites)
	}
}

func TestLoadOrDefault_CorruptFileBackedUpAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	cfg, err := LoadOrDefault(path)
	if err != nil {
		t.Fatalf("LoadOrDefault should recover, got: %v", err)
	}
	if len(cfg.RecentSites) != 0 {
		t.Errorf("expected default config, got %v", cfg.RecentSites)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("corrupt file not backed up: %v", err)
	}
}

func TestAddRecentSite_CapTenDedupToFront(t *testing.T) {
	cfg := &DesktopConfig{}
	for i := 0; i < 12; i++ {
		cfg.AddRecentSite(string(rune('a'+i))) // 12 distinct
	}
	if len(cfg.RecentSites) != 10 {
		t.Fatalf("cap failed: %d", len(cfg.RecentSites))
	}
	oldest := cfg.RecentSites[len(cfg.RecentSites)-1]
	cfg.AddRecentSite(oldest)
	if cfg.RecentSites[0] != oldest || len(cfg.RecentSites) != 10 {
		t.Errorf("re-add should move to front without growth: %v", cfg.RecentSites)
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.json")
	if err := Save(path, &DesktopConfig{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var cfg DesktopConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}
