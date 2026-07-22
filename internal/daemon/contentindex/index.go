// Package contentindex provides an in-memory query index over the
// pre-built /api/{section}.json files. The daemon loads it at startup and
// after each build, then serves read-only content queries via /api/v1/*.
package contentindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Item is a single content entry returned by the query API. The Plain
// (full-text body) field from the source JSON is intentionally dropped —
// the API returns metadata + summary only; full content is served via the
// pre-built page or JIT rendering.
type Item struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`     // relative, e.g. /posts/hello/
	Section     string   `json:"section"` // derived from source filename
	Date        string   `json:"date"`
	Description string   `json:"description,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// rawItem matches the source JSON (output.ContentItem) shape. Used only for
// decoding; URL is absolute, Section is filled from the filename. Plain is
// intentionally absent so the full-text body is dropped on decode.
type rawItem struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
}

// ContentIndex is the daemon's in-memory content query index.
// Thread-safe; callers reload via LoadFromDir after a build.
type ContentIndex struct {
	mu      sync.RWMutex
	items   []Item
	baseURL string
}

// NewContentIndex creates an empty index. baseURL is used to convert the
// absolute URLs in the source JSON back to relative paths.
func NewContentIndex(baseURL string) *ContentIndex {
	return &ContentIndex{baseURL: baseURL}
}

// LoadFromDir loads (or reloads) all section JSON files from
// <outputDir>/api/*.json. Malformed files are skipped with a warning.
// Missing api/ directory is not an error (yields an empty index).
func (ci *ContentIndex) LoadFromDir(outputDir string) error {
	apiDir := filepath.Join(outputDir, "api")
	entries, err := os.ReadDir(apiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no api dir → empty index
		}
		return fmt.Errorf("contentindex: read %s: %w", apiDir, err)
	}

	var items []Item
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		section := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.Join(apiDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "huan: contentindex: read %s: %v\n", entry.Name(), err)
			continue
		}
		var raw []rawItem
		if err := json.Unmarshal(data, &raw); err != nil {
			fmt.Fprintf(os.Stderr, "huan: contentindex: parse %s: %v\n", entry.Name(), err)
			continue
		}
		for _, r := range raw {
			items = append(items, Item{
				Title:       r.Title,
				URL:         ci.toRelative(r.URL),
				Section:     section,
				Date:        r.Date,
				Description: r.Description,
				Summary:     r.Summary,
				Tags:        r.Tags,
			})
		}
	}

	ci.mu.Lock()
	ci.items = items
	ci.mu.Unlock()
	return nil
}

// Len returns the number of indexed items.
func (ci *ContentIndex) Len() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return len(ci.items)
}

// GetByURL returns the item with the given relative URL.
func (ci *ContentIndex) GetByURL(url string) (Item, bool) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	for _, it := range ci.items {
		if it.URL == url {
			return it, true
		}
	}
	return Item{}, false
}

// toRelative strips the configured baseURL prefix from an absolute URL,
// ensuring the result starts with "/".
func (ci *ContentIndex) toRelative(absURL string) string {
	rel := strings.TrimPrefix(absURL, ci.baseURL)
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	return rel
}
