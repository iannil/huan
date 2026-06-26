package admin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// buildTree constructs a hierarchical folder tree from the flat content items.
// Only folder nodes are returned; file-level nodes are not included since the
// tree is used purely for navigation in the admin sidebar. Each folder node
// shows the recursive count of content items under it.
// The display name of each folder uses the directory's own name.
func (co *contentOps) buildTree(items []ContentItem) []*TreeNode {
	// First pass: build a map of every directory path → set of subdirectory names.
	// Track which directories contain _index.md so we can get a proper label.
	type dirInfo struct {
		hasIndex bool
		title    string
		count    int // recursive file count including all descendants
	}
	dirs := map[string]*dirInfo{}
	fileCount := map[string]int{} // directory path → number of files in that dir

	// Collect all unique directory paths from item RelPaths.
	for _, item := range items {
		dir := filepath.Dir(item.RelPath)
		if dir == "." {
			rootKey := "/" // sentinel for root-level files
			if _, ok := dirs[rootKey]; !ok {
				dirs[rootKey] = &dirInfo{}
			}
			fileCount[rootKey]++
			continue
		}
		if _, ok := dirs[dir]; !ok {
			dirs[dir] = &dirInfo{}
		}
		fileCount[dir]++

		// Walk up to ensure all parent dirs exist in the map.
		parent := filepath.Dir(dir)
		for parent != "." {
			if _, ok := dirs[parent]; !ok {
				dirs[parent] = &dirInfo{}
			}
			parent = filepath.Dir(parent)
		}
		// Ensure root sentinel exists.
		if _, ok := dirs["/"]; !ok {
			dirs["/"] = &dirInfo{}
		}
	}

	// Detect _index.md entries to annotate folder titles.
	for _, item := range items {
		base := filepath.Base(item.RelPath)
		dir := filepath.Dir(item.RelPath)
		if base == "_index.md" {
			key := dir
			if key == "." {
				key = "/"
			}
			if d, ok := dirs[key]; ok {
				d.hasIndex = true
				d.title = item.Title
			}
		}
	}

	// Compute recursive counts: for each dir, add up file counts of all subdirs.
	// Sort dirs by path length (descending) so children are accumulated before parents.
	type dirEntry struct {
		path  string
		entry *dirInfo
	}
	sorted := make([]dirEntry, 0, len(dirs))
	for path, info := range dirs {
		sorted = append(sorted, dirEntry{path, info})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].path) > len(sorted[j].path) // longer paths first (children before parents)
	})
	for _, de := range sorted {
		parent := filepath.Dir(de.path)
		if parent == "." {
			parent = "/"
		}
		if p, ok := dirs[parent]; ok {
			// Accumulate this dir's count into its parent.
			p.count += de.entry.count + fileCount[de.path]
		}
		// Also accumulate the direct file count into the dir's own recursive count.
		de.entry.count += fileCount[de.path]
	}

	// Build sorted root-level children.
	var rootNodes []*TreeNode
	for path, info := range dirs {
		if path == "/" {
			continue
		}
		base := filepath.Base(path)
		name := base
		if info.hasIndex && info.title != "" {
			name = info.title
		}
		node := &TreeNode{
			Name:     name,
			Path:     path,
			Type:     "folder",
			Count:    info.count,
			Children: []*TreeNode{},
		}
		rootNodes = append(rootNodes, node)
	}

	// Sort root nodes by name.
	sort.Slice(rootNodes, func(i, j int) bool {
		return rootNodes[i].Name < rootNodes[j].Name
	})

	// Now build the hierarchy by parent assignment.
	// Map path → node for lookup.
	nodeMap := make(map[string]*TreeNode, len(rootNodes))
	for _, node := range rootNodes {
		nodeMap[node.Path] = node
	}

	// Assign children to parents.
	var topLevel []*TreeNode
	for _, node := range rootNodes {
		parent := filepath.Dir(node.Path)
		if parent == "." {
			// This is a top-level section.
			topLevel = append(topLevel, node)
			continue
		}
		if p, ok := nodeMap[parent]; ok {
			p.Children = append(p.Children, node)
		} else {
			// Parent not in tree (unlikely but safe).
			topLevel = append(topLevel, node)
		}
	}

	// Sort children of each folder by name.
	var sortChildren func(nodes []*TreeNode)
	sortChildren = func(nodes []*TreeNode) {
		for _, n := range nodes {
			if len(n.Children) > 0 {
				sort.Slice(n.Children, func(i, j int) bool {
					return n.Children[i].Name < n.Children[j].Name
				})
				sortChildren(n.Children)
			}
		}
	}
	sort.Slice(topLevel, func(i, j int) bool {
		return topLevel[i].Name < topLevel[j].Name
	})
	sortChildren(topLevel)

	return topLevel
}

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
