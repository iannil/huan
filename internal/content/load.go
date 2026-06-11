package content

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/novel_ttl/huan/internal/config"
	"gopkg.in/yaml.v3"
)

const frontmatterDelim = "---"

// ParseFrontmatter splits a markdown file into frontmatter and body.
func ParseFrontmatter(data []byte) (frontmatter map[string]interface{}, body string, err error) {
	data = bytes.TrimSpace(data)

	if !bytes.HasPrefix(data, []byte(frontmatterDelim)) {
		return nil, string(data), nil
	}

	// Find closing delimiter
	end := bytes.Index(data[len(frontmatterDelim):], []byte(frontmatterDelim))
	if end == -1 {
		return nil, "", fmt.Errorf("unclosed frontmatter")
	}

	fmData := data[len(frontmatterDelim) : len(frontmatterDelim)+end]
	bodyData := data[len(frontmatterDelim)+end+len(frontmatterDelim):]

	frontmatter = make(map[string]interface{})
	if err := yaml.Unmarshal(fmData, &frontmatter); err != nil {
		return nil, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	return frontmatter, strings.TrimSpace(string(bodyData)), nil
}

// loadPageFromFrontmatter creates a Page from parsed frontmatter and file info.
func loadPageFromFrontmatter(fm map[string]interface{}, body, relPath string) (*Page, error) {
	p := &Page{
		RawContent: body,
		RelPath:    relPath,
	}

	// String fields
	p.Title = strField(fm, "title")
	p.Description = strField(fm, "description")
	p.Author = strField(fm, "author")
	p.Image = strField(fm, "image")
	p.FeaturedImage = strField(fm, "featured_image")
	p.Slug = strField(fm, "slug")
	p.Type = strField(fm, "type")
	p.Access = strField(fm, "access")
	p.EncryptGroup = strField(fm, "encryptGroup")
	p.EncryptMode = strField(fm, "encryptMode")

	// Bool fields
	p.Draft = boolField(fm, "draft")
	p.Hidden = boolField(fm, "hidden")

	// Int fields
	p.EncryptRatio = intField(fm, "encryptRatio")
	p.Weight = intField(fm, "weight")

	// String slices
	p.Tags = strSliceField(fm, "tags")
	p.Keywords = strSliceField(fm, "keywords")

	// Date parsing
	p.Date = strField(fm, "date")
	p.DateParsed = parseDate(p.Date)
	if lm := strField(fm, "lastmod"); lm != "" {
		p.Lastmod = lm
		p.LastmodParsed = parseDate(lm)
	} else {
		p.Lastmod = p.Date
		p.LastmodParsed = p.DateParsed
	}

	// Build config
	if bc, ok := fm["build"].(map[string]interface{}); ok {
		p.Build = config.BuildConfig{
			List:             strField(bc, "list"),
			Render:           strField(bc, "render"),
			PublishResources: boolField(bc, "publishResources"),
		}
	}

	// Cascade config
	if cc, ok := fm["cascade"].(map[string]interface{}); ok {
		p.Cascade = config.CascadeConfig{}
		if cbc, ok := cc["build"].(map[string]interface{}); ok {
			p.Cascade.Build = config.BuildConfig{
				List:             strField(cbc, "list"),
				Render:           strField(cbc, "render"),
				PublishResources: boolField(cbc, "publishResources"),
			}
		}
		if sc, ok := cc["sitemap"].(map[string]interface{}); ok {
			p.Cascade.Sitemap.Disable = boolField(sc, "disable")
		}
	}

	// Sitemap config
	if sc, ok := fm["sitemap"].(map[string]interface{}); ok {
		p.Sitemap.Disable = boolField(sc, "disable")
	}

	// Default access to public
	if p.Access == "" {
		p.Access = "public"
	}

	return p, nil
}

// LoadDir recursively loads all .md files from the content directory.
func LoadDir(contentDir string) ([]*Page, error) {
	var pages []*Page

	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		relPath, err := filepath.Rel(contentDir, path)
		if err != nil {
			return fmt.Errorf("relpath %s: %w", path, err)
		}
		// Normalize to forward slashes
		relPath = filepath.ToSlash(relPath)

		fm, body, err := ParseFrontmatter(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		page, err := loadPageFromFrontmatter(fm, body, relPath)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		page.FilePath = path

		pages = append(pages, page)
		return nil
	})

	return pages, err
}

// Helper functions for extracting typed values from map[string]interface{}

func strField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case time.Time:
		// YAML may parse dates into time.Time; format as RFC3339 for re-parsing
		return val.Format("2006-01-02T15:04:05Z07:00")
	default:
		return fmt.Sprintf("%v", val)
	}
}

func boolField(m map[string]interface{}, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true"
	default:
		return false
	}
}

func intField(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func strSliceField(m map[string]interface{}, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return val
	default:
		return nil
	}
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
