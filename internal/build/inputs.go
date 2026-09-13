package build

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
)

// buildInputs is a raw snapshot owned by one multilingual build. Language
// pipelines copy pages and data before exposing them to filters or hooks.
type buildInputs struct {
	pages     []*content.Page
	data      map[string]interface{}
	available map[string]map[string]bool
	stale     *I18nStaleReport
	staleErr  error
}

func loadBuildInputs(sourceDir string, cfg *config.Config, timings *Timings) (*buildInputs, error) {
	in := &buildInputs{}
	contentDir := filepath.Join(sourceDir, "content")
	if cfg.IsMultiLanguage() {
		_ = timings.Measure("multi", "shared input preparation/stale check", func() error {
			in.stale, in.staleErr = checkStaleTranslations(contentDir)
			return in.staleErr
		})
	}
	if err := timings.Measure("multi", "shared input preparation/content", func() error {
		var err error
		in.pages, err = content.LoadDir(contentDir)
		return err
	}); err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}
	if err := timings.Measure("multi", "shared input preparation/data", func() error {
		var err error
		in.data, err = content.LoadDataFiles(filepath.Join(sourceDir, "data"))
		return err
	}); err != nil {
		return nil, fmt.Errorf("load data: %w", err)
	}
	in.available = availableTranslationsFromPages(in.pages, cfg)
	return in, nil
}

func cloneInputPage(src *content.Page) *content.Page {
	out := *src
	out.Tags = slices.Clone(src.Tags)
	out.Keywords = slices.Clone(src.Keywords)
	// Build, Cascade and Sitemap contain only value fields.
	out.Parent = nil
	out.Pages, out.RegularPages, out.RegularPagesRecursive, out.Sections = nil, nil, nil, nil
	return &out
}

func cloneInputData(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, value := range x {
			out[k] = cloneInputData(value)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[interface{}]interface{}, len(x))
		for k, value := range x {
			out[k] = cloneInputData(value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, value := range x {
			out[i] = cloneInputData(value)
		}
		return out
	default:
		return v
	}
}

func availableTranslationsFromPages(pages []*content.Page, cfg *config.Config) map[string]map[string]bool {
	out := make(map[string]map[string]bool)
	for _, p := range pages {
		if out[p.RelPath] == nil {
			out[p.RelPath] = make(map[string]bool)
		}
		lang := p.Language
		if lang == "" {
			lang = cfg.DefaultLanguageCode()
		}
		out[p.RelPath][lang] = true
	}
	return out
}
