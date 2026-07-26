package main

import (
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	raw := map[string]any{
		"accountId": "test-account",
		"apiToken":  "test-token",
		"pages": map[string]any{
			"project": "my-site",
			"branch":  "main",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.AccountID != "test-account" {
		t.Errorf("AccountID = %q", cfg.AccountID)
	}
	if cfg.Pages.Project != "my-site" {
		t.Errorf("Pages.Project = %q", cfg.Pages.Project)
	}
}

func TestParseConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
	}{
		{"nil config", nil},
		{"missing accountId", map[string]any{"apiToken": "t", "pages": map[string]any{"project": "p", "branch": "b"}}},
		{"missing apiToken", map[string]any{"accountId": "a", "pages": map[string]any{"project": "p", "branch": "b"}}},
		{"missing pages.project", map[string]any{"accountId": "a", "apiToken": "t", "pages": map[string]any{"branch": "b"}}},
		{"missing pages.branch", map[string]any{"accountId": "a", "apiToken": "t", "pages": map[string]any{"project": "p"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig(tt.raw)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestR2Config_Validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  R2Config
	}{
		{"missing accountId and endpoint", R2Config{AccessKeyID: "k", SecretAccessKey: "s", Bucket: "b"}},
		{"missing accessKeyId", R2Config{AccountID: "a", SecretAccessKey: "s", Bucket: "b"}},
		{"missing secretAccessKey", R2Config{AccountID: "a", AccessKeyID: "k", Bucket: "b"}},
		{"missing bucket", R2Config{AccountID: "a", AccessKeyID: "k", SecretAccessKey: "s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestHasR2Configured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"has accountId", Config{R2: R2Config{AccountID: "a"}}, true},
		{"has bucket", Config{R2: R2Config{Bucket: "b"}}, true},
		{"has sync", Config{R2: R2Config{Sync: []SyncMapping{{From: "f", To: "t"}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.HasR2Configured()
			if got != tt.want {
				t.Errorf("HasR2Configured = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkerConfig_Validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  WorkerConfig
	}{
		{"missing name", WorkerConfig{Script: "worker.js"}},
		{"missing script", WorkerConfig{Name: "my-worker"}},
		{"invalid date format", WorkerConfig{Name: "w", Script: "w.js", CompatibilityDate: "2024/01/01"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestHasWorkerConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"singular worker", Config{Worker: WorkerConfig{Name: "w"}}, true},
		{"plural workers", Config{Workers: []WorkerConfig{{Name: "w"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.HasWorkerConfigured()
			if got != tt.want {
				t.Errorf("HasWorkerConfigured = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllWorkers(t *testing.T) {
	cfg := Config{
		Workers: []WorkerConfig{
			{Name: "alpha", Script: "a.js"},
			{Name: "bravo", Script: "b.js"},
		},
		Worker: WorkerConfig{Name: "charlie", Script: "c.js"},
	}
	workers := cfg.AllWorkers()
	if len(workers) != 3 {
		t.Fatalf("AllWorkers = %d, want 3", len(workers))
	}
}

func TestParseConfig_R2AccountIDFallback(t *testing.T) {
	raw := map[string]any{
		"accountId": "top-level-account",
		"apiToken":  "token",
		"pages":     map[string]any{"project": "p", "branch": "b"},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.R2.AccountID != "top-level-account" {
		t.Errorf("R2.AccountID = %q, want top-level-account (fallback)", cfg.R2.AccountID)
	}
}