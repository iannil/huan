package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// ContentItem is a single content entry in the API output.
type ContentItem struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Date        string   `json:"date"`
	Description string   `json:"description,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Plain       string   `json:"plain,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// GenerateContentAPI writes /api/{section}.json for each top-level section.
// Only pages that pass the draft/future/expired filter are included.
func GenerateContentAPI(outputDir string, site *content.Site, cfg *config.Config, includeDrafts, includeFuture, includeExpired bool, now time.Time) error {
	apiDir := filepath.Join(outputDir, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		return fmt.Errorf("mkdir api: %w", err)
	}

	// Group pages by section
	sections := map[string][]ContentItem{}
	for _, p := range site.Pages {
		if p.Kind != "page" {
			continue
		}
		if p.Draft && !includeDrafts {
			continue
		}
		if !includeFuture && !p.PublishDateParsed.IsZero() && p.PublishDateParsed.After(now) {
			continue
		}
		if !includeExpired && !p.ExpiryDateParsed.IsZero() && p.ExpiryDateParsed.Before(now) {
			continue
		}

		item := ContentItem{
			Title:       p.Title,
			URL:         cfg.BaseURL + strings.TrimPrefix(p.URL, "/"),
			Date:        p.Date,
			Description: p.Description,
			Summary:     string(p.Summary),
			Plain:       p.Plain,
			Tags:        p.Tags,
		}
		sections[p.Section] = append(sections[p.Section], item)
	}

	for section, items := range sections {
		sort.Slice(items, func(i, j int) bool {
			return items[i].Date > items[j].Date
		})
		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", section, err)
		}
		path := filepath.Join(apiDir, section+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return nil
}
