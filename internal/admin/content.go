package admin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/content"
	"gopkg.in/yaml.v3"
)

// contentOps wraps file-system operations on Markdown content files.
type contentOps struct {
	contentDir string
}

func newContentOps(contentDir string) *contentOps {
	return &contentOps{contentDir: contentDir}
}

// listAll returns all content files grouped by section.
func (co *contentOps) listAll() (map[string][]ContentItem, error) {
	pages, err := content.LoadDir(co.contentDir)
	if err != nil {
		return nil, fmt.Errorf("load content: %w", err)
	}

	sections := make(map[string][]ContentItem)
	for _, p := range pages {
		if p.RelPath == "" {
			continue
		}
		item := ContentItem{
			Title:       p.Title,
			RelPath:     p.RelPath,
			Section:     coalesce(p.Section, detectSection(p.RelPath)),
			Kind:        coalesce(p.Kind, detectKind(p.RelPath)),
			Draft:       p.Draft,
			Hidden:      p.Hidden,
			Date:        p.Date,
			Tags:        p.Tags,
			Description: p.Description,
			Slug:        p.Slug,
			Language:    coalesce(p.Language, detectLanguage(p.RelPath)),
			URL:         p.URL,
		}
		sec := item.Section
		if sec == "" {
			sec = "_root"
		}
		sections[sec] = append(sections[sec], item)
	}
	return sections, nil
}

// readOne reads a single content file and returns full detail.
func (co *contentOps) readOne(relPath string) (*ContentDetail, error) {
	fullPath := filepath.Join(co.contentDir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	fm, body, err := content.ParseFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	// Build a Page from frontmatter to extract fields
	p := &content.Page{}
	if err := marshalMapToStruct(fm, p); err != nil {
		return nil, fmt.Errorf("build page: %w", err)
	}
	p.RelPath = relPath

	// Compute section from path
	p.Section = detectSection(relPath)

	// Compute kind
	p.Kind = detectKind(relPath)

	detail := &ContentDetail{
		ContentItem: ContentItem{
			Title:       p.Title,
			RelPath:     p.RelPath,
			Section:     p.Section,
			Kind:        p.Kind,
			Draft:       p.Draft,
			Hidden:      p.Hidden,
			Date:        p.Date,
			Tags:        p.Tags,
			Description: p.Description,
			Slug:        p.Slug,
			Language:    detectLanguage(relPath),
		},
		RawContent:  body,
		Frontmatter: fm,
	}
	return detail, nil
}

// create writes a new Markdown content file.
func (co *contentOps) create(req CreateContentRequest) (*ContentDetail, error) {
	filename := req.Filename
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}

	dir := co.contentDir
	if req.Section != "" && req.Section != "_root" {
		dir = filepath.Join(dir, req.Section)
	}

	fullPath := filepath.Join(dir, filename)

	if _, err := os.Stat(fullPath); err == nil {
		return nil, fmt.Errorf("file already exists: %s", fullPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	fm := map[string]interface{}{
		"title": req.Title,
		"date":  now,
		"draft": req.Draft,
	}

	fmYAML, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	out := "---\n" + string(fmYAML) + "---\n"
	if err := os.WriteFile(fullPath, []byte(out), 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	relPath, _ := filepath.Rel(co.contentDir, fullPath)
	return co.readOne(relPath)
}

// update overwrites frontmatter and body of an existing content file.
func (co *contentOps) update(relPath string, req UpdateContentRequest) (*ContentDetail, error) {
	fullPath := filepath.Join(co.contentDir, relPath)
	if _, err := os.Stat(fullPath); err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmBytes, err := yaml.Marshal(req.Frontmatter)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	sb.Write(fmBytes)
	sb.WriteString("---\n")

	body := req.RawContent
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	sb.WriteString(body)

	if err := os.WriteFile(fullPath, []byte(sb.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return co.readOne(relPath)
}

// remove deletes a content file.
func (co *contentOps) remove(relPath string) error {
	fullPath := filepath.Join(co.contentDir, relPath)
	return os.Remove(fullPath)
}

// --- helpers ---

// coalesce returns first non-empty string, or "" if both empty.
func coalesce(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func detectSection(relPath string) string {
	parts := strings.SplitN(relPath, string(filepath.Separator), 2)
	if len(parts) < 2 {
		return "_root"
	}
	return parts[0]
}

func detectKind(relPath string) string {
	base := filepath.Base(relPath)
	if base == "_index.md" {
		return "section"
	}
	if base == "index.md" {
		return "page" // leaf bundle
	}
	return "page"
}

func detectLanguage(relPath string) string {
	name := filepath.Base(relPath)
	ext := filepath.Ext(name)
	withoutExt := strings.TrimSuffix(name, ext)
	parts := strings.Split(withoutExt, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

// marshalMapToStruct unmarshals a map into a struct via YAML round-trip.
func marshalMapToStruct(m map[string]interface{}, target interface{}) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, target)
}
