package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <kind>/<path>",
		Short: "Create new content from archetype",
		Long: `Create new content from archetype.

The path is relative to content/. The first path segment is the archetype
kind: huan looks for archetypes/<kind>.md, falling back to archetypes/default.md,
then a built-in default. Examples:

  huan new posts/my-post.md         → archetypes/posts.md (or default.md)
  huan new book-chapter/v1/ch01.md  → archetypes/book-chapter.md (or default.md)
  huan new my-post.md               → archetypes/default.md
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(args[0])
		},
	}
}

func runNew(relPath string) error {
	contentDir := filepath.Join(sourceDir, "content")
	targetPath := filepath.Join(contentDir, relPath)

	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("file already exists: %s", targetPath)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	content, err := renderArchetype(relPath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Created %s\n", targetPath)
	return nil
}

// renderArchetype selects the archetype template for relPath and renders it.
// Selection order: archetypes/<kind>.md → archetypes/default.md → built-in.
// kind is the top-level path segment (the content section).
func renderArchetype(relPath string) (string, error) {
	kind := archetypeKind(relPath)
	if kind != "" && kind != "default" {
		if tmpl, ok := readArchetype(kind); ok {
			return renderArchetypeTemplate(tmpl, relPath), nil
		}
	}
	if tmpl, ok := readArchetype("default"); ok {
		return renderArchetypeTemplate(tmpl, relPath), nil
	}
	return defaultArchetype(relPath), nil
}

// readArchetype returns the template content for the given kind and true if
// archetypes/<kind>.md exists.
func readArchetype(kind string) (string, bool) {
	tmpl, err := os.ReadFile(filepath.Join(sourceDir, "archetypes", kind+".md"))
	if err != nil {
		return "", false
	}
	return string(tmpl), true
}

// archetypeKind extracts the archetype kind from a content-relative path:
// the first path segment. Returns "" when relPath has no directory part.
//   "posts/my-post.md"          → "posts"
//   "books/v1/ch01.md"          → "books"
//   "my-post.md"                → ""
func archetypeKind(relPath string) string {
	relPath = filepath.ToSlash(relPath)
	idx := strings.IndexByte(relPath, '/')
	if idx <= 0 {
		return ""
	}
	return relPath[:idx]
}

func renderArchetypeTemplate(tmpl, relPath string) string {
	name := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	now := time.Now().Format(time.RFC3339)
	out := strings.ReplaceAll(tmpl, "{{ .Date }}", now)
	out = renderNameTemplate(out, name)
	return out
}

func defaultArchetype(relPath string) string {
	name := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	now := time.Now().Format(time.RFC3339)
	title := strings.ReplaceAll(name, "-", " ")
	title = strings.Title(title)
	return fmt.Sprintf("---\ntitle: %q\ndate: %s\ndraft: true\n---\n", title, now)
}

var replaceRE = regexp.MustCompile(`{{\s*replace\s+(\S+)\s+"([^"]*)"\s+"([^"]*)"\s*\|\s*title\s*}}`)

func renderNameTemplate(tmpl, name string) string {
	out := replaceRE.ReplaceAllStringFunc(tmpl, func(match string) string {
		m := replaceRE.FindStringSubmatch(match)
		if len(m) != 4 {
			return match
		}
		val := m[1]
		if val == ".Name" {
			val = name
		}
		val = strings.ReplaceAll(val, m[2], m[3])
		return strings.Title(val)
	})
	out = strings.ReplaceAll(out, "{{ .Name }}", name)
	return out
}
