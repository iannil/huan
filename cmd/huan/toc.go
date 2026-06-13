package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/iannil/huan/internal/content"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	partNumberRE = regexp.MustCompile(`\d+`)
)

type specialFile struct {
	typ   string
	title string
}

type tocEntry struct {
	slug  string
	title string
}

type tocVolume struct {
	Name    string
	Entries []tocEntry
}

func newTocCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "toc",
		Short: "Generate table-of-contents markdown for books/practices/products",
		Long: `Scan content/{books,practices,products}/ and write three markdown files
to developer/toc/:

  books-toc.md      — books organized by volume → book → part → chapter
  practices-toc.md  — practices organized by season → practice → part → chapter
  products-toc.md   — flat list of product titles

Structure mirrors data/books.yaml and data/practices.yaml.`,
		Args: cobra.NoArgs,
		RunE: runToc,
	}
}

func runToc(cmd *cobra.Command, args []string) error {
	outDir := filepath.Join(sourceDir, "developer", "toc")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create toc dir: %w", err)
	}

	for _, kind := range []string{"books", "practices", "products"} {
		md, err := generateToc(sourceDir, kind)
		if err != nil {
			return err
		}
		path := filepath.Join(outDir, kind+"-toc.md")
		if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s\n", path)
	}
	return nil
}

// generateToc produces the markdown TOC for the given kind. kind is one of
// "books", "practices", or "products".
func generateToc(sourceDir, kind string) (string, error) {
	switch kind {
	case "books":
		return generateCollectionToc(sourceDir, "books", "book")
	case "practices":
		return generateCollectionToc(sourceDir, "practices", "practice")
	case "products":
		return generateProductsToc(sourceDir)
	default:
		return "", fmt.Errorf("unknown kind: %s", kind)
	}
}

type collectionData struct {
	Collection []struct {
		Volume     string `yaml:"volume"`
		Season     string `yaml:"season"`
		Books      []struct {
			Slug  string `yaml:"slug"`
			Title string `yaml:"title"`
		} `yaml:"books"`
		Practices []struct {
			Slug  string `yaml:"slug"`
			Title string `yaml:"title"`
		} `yaml:"practices"`
	} `yaml:"collection"`
	PartTitles map[string]map[string]string `yaml:"part_titles"`
}

// generateCollectionToc reads data/<kind>.yaml and walks the matching content
// tree. `kind` is the directory name (books/practices); `labelSingular` is
// the heading word ("book"/"practice") but currently unused since the data
// keys are derived from kind.
func generateCollectionToc(sourceDir, kind, _ string) (string, error) {
	yamlPath := filepath.Join(sourceDir, "data", kind+".yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Sprintf("# %s目录\n\n*未找到%s数据*\n", kind, kind), nil
	}
	var c collectionData
	if err := yaml.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("parse %s: %w", yamlPath, err)
	}

	titleCn := "书籍目录"
	dirPrefix := "volume"
	listKey := "books"
	if kind == "practices" {
		titleCn = "实践目录"
		dirPrefix = "season"
		listKey = "practices"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", titleCn)

	contentRoot := filepath.Join(sourceDir, "content", kind)

	for i, vol := range c.Collection {
		volNum := i + 1
		volDir := filepath.Join(contentRoot, fmt.Sprintf("%s-%d", dirPrefix, volNum))

		volName := vol.Volume
		if volName == "" {
			volName = vol.Season
		}
		fmt.Fprintf(&sb, "## %s\n\n", volName)

		var entries []tocEntry
		switch listKey {
		case "books":
			for _, b := range vol.Books {
				entries = append(entries, tocEntry{b.Slug, b.Title})
			}
		case "practices":
			for _, p := range vol.Practices {
				entries = append(entries, tocEntry{p.Slug, p.Title})
			}
		}

		for _, entry := range entries {
			fmt.Fprintf(&sb, "### %s\n\n", entry.title)
			entryDir := filepath.Join(volDir, entry.slug)
			if _, err := os.Stat(entryDir); os.IsNotExist(err) {
				sb.WriteString("*内容待添加*\n\n")
				continue
			}
			writeBookToc(&sb, entryDir, entry.slug, c.PartTitles)
		}
	}

	// Node's join-based output ends each section with the section's own trailing
	// \n and relies on the next push for the blank-line separator. We bake the
	// separator into each WriteString, so at EOF we have one extra \n — trim it.
	out := sb.String()
	if strings.HasSuffix(out, "\n\n") {
		out = out[:len(out)-1]
	}
	return out, nil
}

// writeBookToc writes the chapter structure for one book/practice directory.
func writeBookToc(sb *strings.Builder, entryDir, slug string, partTitles map[string]map[string]string) {
	items, err := os.ReadDir(entryDir)
	if err != nil {
		return
	}

	var specials []specialFile
	var partDirs []string

	for _, item := range items {
		name := item.Name()
		if item.IsDir() {
			if strings.HasPrefix(name, "part-") {
				partDirs = append(partDirs, name)
			}
			continue
		}
		if !strings.HasSuffix(name, ".md") || name == "_index.md" {
			continue
		}
		base := strings.TrimSuffix(name, ".md")
		switch base {
		case "introduction":
			title := extractTitle(filepath.Join(entryDir, name), base)
			specials = append([]specialFile{{"introduction", title}}, specials...)
		case "epilogue", "appendix":
			title := extractTitle(filepath.Join(entryDir, name), base)
			specials = append(specials, specialFile{base, title})
		}
	}

	sort.Strings(partDirs)

	for _, sp := range specials {
		if sp.typ == "introduction" {
			fmt.Fprintf(sb, "- %s\n", sp.title)
		}
	}
	if hasIntro(specials) {
		sb.WriteString("\n")
	}

	for _, partDir := range partDirs {
		partPath := filepath.Join(entryDir, partDir)
		partTitle := partDir
		if titles, ok := partTitles[slug]; ok {
			if t, ok := titles[partDir]; ok && t != "" {
				partTitle = t
			}
		}
		chapters := sortedChapterTitles(partPath)
		if len(chapters) == 0 {
			continue
		}
		fmt.Fprintf(sb, "#### %s\n\n", partTitle)
		for _, ch := range chapters {
			fmt.Fprintf(sb, "- %s\n", ch)
		}
		sb.WriteString("\n")
	}

	for _, sp := range specials {
		if sp.typ == "epilogue" || sp.typ == "appendix" {
			fmt.Fprintf(sb, "- %s\n", sp.title)
		}
	}
	sb.WriteString("\n")
}

// sortedChapterTitles lists chapter titles from partDir, applying the
// sort order generate-toc.js used: introduction first, then numeric prefix,
// epilogue/appendix last.
func sortedChapterTitles(partDir string) []string {
	entries, err := os.ReadDir(partDir)
	if err != nil {
		return nil
	}

	specialOrder := map[string]int{
		"introduction": 0,
		"epilogue":     2,
		"appendix":     2,
	}

	type chapter struct {
		name  string
		title string
	}
	var chapters []chapter
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "_index.md" {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".md")
		chapters = append(chapters, chapter{
			name:  base,
			title: extractTitle(filepath.Join(partDir, e.Name()), base),
		})
	}

	sort.SliceStable(chapters, func(i, j int) bool {
		a, b := chapters[i].name, chapters[j].name
		ai, aok := specialOrder[a]
		bj, bok := specialOrder[b]
		switch {
		case aok && bok:
			if ai != bj {
				return ai < bj
			}
		case aok:
			return ai == 0
		case bok:
			return bj != 0
		}
		na := leadingNumber(a)
		nb := leadingNumber(b)
		if na != nb {
			return na < nb
		}
		return a < b
	})

	out := make([]string, len(chapters))
	for i, c := range chapters {
		out[i] = c.title
	}
	return out
}

func leadingNumber(s string) int {
	m := partNumberRE.FindString(s)
	if m == "" {
		return 0
	}
	n, _ := strconv.Atoi(m)
	return n
}

func extractTitle(mdPath, fallback string) string {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return fallback
	}
	fm, _, err := content.ParseFrontmatter(data)
	if err != nil {
		return fallback
	}
	if t, ok := fm["title"].(string); ok && t != "" {
		return t
	}
	return fallback
}

// generateProductsToc produces a flat markdown list of product titles from
// content/products/*.md (excluding _index.md).
func generateProductsToc(sourceDir string) (string, error) {
	productsDir := filepath.Join(sourceDir, "content", "products")
	entries, err := os.ReadDir(productsDir)
	if err != nil {
		return "# 产品目录\n\n*未找到产品数据*\n", nil
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "_index.md" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString("# 产品目录\n\n")
	for _, n := range names {
		base := strings.TrimSuffix(n, ".md")
		title := extractTitle(filepath.Join(productsDir, n), base)
		fmt.Fprintf(&sb, "- %s\n", title)
	}
	return sb.String(), nil
}

func hasIntro(specials []specialFile) bool {
	for _, s := range specials {
		if s.typ == "introduction" {
			return true
		}
	}
	return false
}
