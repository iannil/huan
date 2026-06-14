package cloudflare

import (
	"testing"
)

func TestAllWorkers_Empty(t *testing.T) {
	c := Config{}
	if got := c.AllWorkers(); len(got) != 0 {
		t.Errorf("empty config should return 0 workers, got %d", len(got))
	}
}

func TestAllWorkers_SingularOnly(t *testing.T) {
	c := Config{
		Worker: WorkerConfig{Name: "image-resizer", Script: "workers/image-resizer.js"},
	}
	got := c.AllWorkers()
	if len(got) != 1 {
		t.Fatalf("singular Worker should yield 1, got %d", len(got))
	}
	if got[0].Name != "image-resizer" {
		t.Errorf("name = %q, want image-resizer", got[0].Name)
	}
}

func TestAllWorkers_PluralOnly(t *testing.T) {
	c := Config{
		Workers: []WorkerConfig{
			{Name: "image-resizer", Script: "workers/image-resizer.js"},
			{Name: "i18n-router", Script: "workers/i18n-router.js"},
		},
	}
	got := c.AllWorkers()
	if len(got) != 2 {
		t.Fatalf("plural Workers should yield 2, got %d", len(got))
	}
}

func TestAllWorkers_PluralAndSingularMerged(t *testing.T) {
	// Plural takes precedence; singular appended only if Name doesn't collide.
	c := Config{
		Workers: []WorkerConfig{
			{Name: "image-resizer", Script: "workers/image-resizer.js"},
		},
		Worker: WorkerConfig{Name: "i18n-router", Script: "workers/i18n-router.js"},
	}
	got := c.AllWorkers()
	if len(got) != 2 {
		t.Fatalf("plural+singular distinct names should yield 2, got %d: %+v", len(got), got)
	}
	names := map[string]bool{}
	for _, w := range got {
		names[w.Name] = true
	}
	if !names["image-resizer"] || !names["i18n-router"] {
		t.Errorf("missing expected names, got %v", names)
	}
}

func TestAllWorkers_DedupOnName(t *testing.T) {
	// Same Name in plural and singular → singular skipped (plural wins).
	c := Config{
		Workers: []WorkerConfig{
			{Name: "image-resizer", Script: "workers/image-resizer.js"},
		},
		Worker: WorkerConfig{Name: "image-resizer", Script: "workers/OLD.js"},
	}
	got := c.AllWorkers()
	if len(got) != 1 {
		t.Fatalf("dedup should yield 1, got %d", len(got))
	}
	if got[0].Script != "workers/image-resizer.js" {
		t.Errorf("plural should win on Name collision; got script %q", got[0].Script)
	}
}

func TestAllWorkers_SkipsEmptyEntries(t *testing.T) {
	c := Config{
		Workers: []WorkerConfig{
			{Name: "real", Script: "real.js"},
			{}, // empty
			{Name: "also-real", Script: "also.js"},
		},
	}
	got := c.AllWorkers()
	if len(got) != 2 {
		t.Errorf("empty entries should be skipped, got %d: %+v", len(got), got)
	}
}

func TestHasWorkerConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"singular only", Config{Worker: WorkerConfig{Name: "x"}}, true},
		{"plural only", Config{Workers: []WorkerConfig{{Name: "x"}}}, true},
		{"both", Config{
			Worker:  WorkerConfig{Name: "x"},
			Workers: []WorkerConfig{{Name: "y"}},
		}, true},
		{"singular empty", Config{Worker: WorkerConfig{}}, false},
		{"plural with empty entries", Config{Workers: []WorkerConfig{{}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.HasWorkerConfigured()
			if got != tc.want {
				t.Errorf("HasWorkerConfigured = %v, want %v", got, tc.want)
			}
		})
	}
}
