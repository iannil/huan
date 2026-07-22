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

// Filter controls a Query invocation.
type Filter struct {
	Section string // section filter
	Tag     string // tag filter
	Query   string // full-text (Title/Summary/Description, case-insensitive)
	Page    int    // 1-based page
	Limit   int    // page size, default 10, capped at 50
	Sort    string // "date" (default, desc); other values fall back to date desc
}

// Result is a paginated query response.
type Result struct {
	Data  []Item `json:"data"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

// Query returns a paginated, filtered, sorted slice of items.
func (ci *ContentIndex) Query(f Filter) Result {
	if f.Page < 1 {
		f.Page = 1
	}
	// Cap page to a sane upper bound. Without this, a huge page (e.g.
	// math.MaxInt64) overflows (page-1)*limit below, yielding a negative start
	// that bypasses the start>total guard and panics on matched[start:end].
	// Public, unauthenticated endpoint — this is a DoS surface.
	if f.Page > 100000 {
		f.Page = 100000
	}
	if f.Limit < 1 {
		f.Limit = 10
	}
	if f.Limit > 50 {
		f.Limit = 50
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	// Filter
	var matched []Item
	q := strings.ToLower(f.Query)
	for _, it := range ci.items {
		if f.Section != "" && it.Section != f.Section {
			continue
		}
		if f.Tag != "" && !containsString(it.Tags, f.Tag) {
			continue
		}
		if q != "" && !containsLower(it.Title, q) && !containsLower(it.Summary, q) && !containsLower(it.Description, q) {
			continue
		}
		matched = append(matched, it)
	}

	// Sort by date desc (stable)
	sortItemsByDateDesc(matched)

	total := len(matched)
	// Guard against integer overflow on huge page values (public endpoint).
	// Even with the 100000 page cap, defend against any negative start/end
	// that could slip through future changes to the cap or limit math.
	start := (f.Page - 1) * f.Limit
	if start < 0 || start > total {
		start = total
	}
	end := start + f.Limit
	if end < 0 || end > total {
		end = total
	}
	var data []Item
	if start < end {
		data = matched[start:end]
	}
	if data == nil {
		data = []Item{}
	}

	return Result{Data: data, Total: total, Page: f.Page, Limit: f.Limit}
}

// Tags returns a map of tag → page count.
func (ci *ContentIndex) Tags() map[string]int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	out := map[string]int{}
	for _, it := range ci.items {
		for _, tag := range it.Tags {
			out[tag]++
		}
	}
	return out
}

// Sections returns a map of section → page count.
func (ci *ContentIndex) Sections() map[string]int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	out := map[string]int{}
	for _, it := range ci.items {
		out[it.Section]++
	}
	return out
}

// --- helpers ---

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func containsLower(s, lower string) bool {
	return strings.Contains(strings.ToLower(s), lower)
}

func sortItemsByDateDesc(items []Item) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j-1].Date < items[j].Date; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}
