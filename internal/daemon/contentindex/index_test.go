package contentindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const baseURL = "https://example.com/"

func TestLoadFromDir_LoadsAllSections(t *testing.T) {
	dir := t.TempDir()
	writeAPITestFile(t, dir, "posts.json", []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01", "tags": []string{"go"}},
	})
	writeAPITestFile(t, dir, "books.json", []map[string]any{
		{"title": "B1", "url": baseURL + "books/b1/", "date": "2026-01-02"},
	})

	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if ci.Len() != 2 {
		t.Errorf("Len = %d, want 2", ci.Len())
	}
}

func TestLoadFromDir_RelativeURL(t *testing.T) {
	dir := t.TempDir()
	writeAPITestFile(t, dir, "posts.json", []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01"},
	})

	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	item, ok := ci.GetByURL("/posts/p1/")
	if !ok {
		t.Fatal("GetByURL not found")
	}
	if item.URL != "/posts/p1/" {
		t.Errorf("URL = %q, want /posts/p1/ (relative)", item.URL)
	}
	if item.Section != "posts" {
		t.Errorf("Section = %q, want posts", item.Section)
	}
}

func TestLoadFromDir_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	// Good file
	writeAPITestFile(t, dir, "posts.json", []map[string]any{
		{"title": "P1", "url": baseURL + "posts/p1/", "date": "2026-01-01"},
	})
	// Bad file
	if err := os.WriteFile(filepath.Join(dir, "api", "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir should skip bad files, got: %v", err)
	}
	if ci.Len() != 1 {
		t.Errorf("Len = %d, want 1 (bad file skipped)", ci.Len())
	}
}

func TestLoadFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir empty dir: %v", err)
	}
	if ci.Len() != 0 {
		t.Errorf("Len = %d, want 0", ci.Len())
	}
}

func TestLoadFromDir_NoAPIDir(t *testing.T) {
	// outputDir without api/ subdir should not error.
	dir := t.TempDir()
	ci := NewContentIndex(baseURL)
	if err := ci.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir no api dir: %v", err)
	}
}

// writeAPITestFile writes a section JSON array to <dir>/api/<name>.
func writeAPITestFile(t *testing.T, dir, name string, items []map[string]any) {
	t.Helper()
	apiDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := mustJSON(items)
	if err := os.WriteFile(filepath.Join(apiDir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
