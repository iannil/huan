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
		Use:   "new <path>",
		Short: "Create new content from archetype",
		Args:  cobra.ExactArgs(1),
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

func renderArchetype(relPath string) (string, error) {
	archetypePath := filepath.Join(sourceDir, "archetypes", "default.md")
	tmpl, err := os.ReadFile(archetypePath)
	if err != nil {
		return defaultArchetype(relPath), nil
	}

	name := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	now := time.Now().Format(time.RFC3339)

	out := string(tmpl)
	out = strings.ReplaceAll(out, "{{ .Date }}", now)
	out = renderNameTemplate(out, name)
	return out, nil
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
