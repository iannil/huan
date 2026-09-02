package content

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var leadingNumberRE = regexp.MustCompile(`^[a-zA-Z-]*(\d+)`)

// chapterData / collectionData mirror data/<kind>.yaml structure (same shape
// huan's cmd/huan/toc.go parses).
type entryData struct {
	Slug        string `yaml:"slug"`
	Title       string `yaml:"title"`
	Subtitle    string `yaml:"subtitle"`
	Version     string `yaml:"version"`
	LastUpdated string `yaml:"last_updated"`
}

type volumeData struct {
	Volume    string      `yaml:"volume"`
	Season    string      `yaml:"season"`
	Books     []entryData `yaml:"books"`
	Practices []entryData `yaml:"practices"`
}

type collectionData struct {
	Collection []volumeData                 `yaml:"collection"`
	PartTitles map[string]map[string]string `yaml:"part_titles"`
}

// Discover reads data/<kind>.yaml and walks content/<kind>/ to build the full
// collection tree. kind is "books" or "practices".
func Discover(sourceDir, kind string) (*Collection, error) {
	if kind != "books" && kind != "practices" {
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
	yamlPath := filepath.Join(sourceDir, "data", kind+".yaml")
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", yamlPath, err)
	}
	var data collectionData
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", yamlPath, err)
	}

	dirPrefix := "volume"
	listKey := "books"
	if kind == "practices" {
		dirPrefix = "season"
		listKey = "practices"
	}
	contentRoot := filepath.Join(sourceDir, "content", kind)

	c := &Collection{Kind: kind}
	for i, vol := range data.Collection {
		volNum := i + 1
		volDir := filepath.Join(contentRoot, fmt.Sprintf("%s-%d", dirPrefix, volNum))
		volName := vol.Volume
		if volName == "" {
			volName = vol.Season
		}
		ve := VolumeEntry{Number: volNum, Name: volName}
		entries := vol.Books
		if listKey == "practices" {
			entries = vol.Practices
		}
		for _, e := range entries {
			entryDir := filepath.Join(volDir, e.Slug)
			if _, err := os.Stat(entryDir); os.IsNotExist(err) {
				continue
			}
			b, err := discoverBook(entryDir, e.Slug, data.PartTitles[e.Slug])
			if err != nil {
				return nil, err
			}
			b.TitleZH = e.Title
			b.TitleEN = e.Subtitle
			b.SubtitleZH = e.Subtitle
			b.Version = e.Version
			b.LastUpdated = e.LastUpdated
			b.VolumeNumber = volNum
			b.VolumeName = volName
			b.Dir = entryDir
			ve.Books = append(ve.Books, *b)
		}
		c.Volumes = append(c.Volumes, ve)
	}
	return c, nil
}

// discoverBook walks one book directory, collecting special files
// (introduction/epilogue/appendix) and part-XX directories with sorted
// chapters — mirroring huan's cmd/huan/toc.go writeBookToc traversal.
func discoverBook(dir, slug string, partTitles map[string]string) (*BookEntry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	b := &BookEntry{Slug: slug, Dir: dir}
	// readBookFile reads a zh file and, if a .en.md sidecar exists, its EN twin.
	readBookFile := func(path string) Chapter {
		ch := Chapter{SourcePath: path}
		ch.Title = parseFrontmatterTitleFile(path)
		en := strings.TrimSuffix(path, ".md") + ".en.md"
		if _, err := os.Stat(en); err == nil {
			ch.ENPath = en
			b.HasEN = true
		}
		return ch
	}

	for _, item := range items {
		name := item.Name()
		if item.IsDir() {
			if name == "guide" || !strings.HasPrefix(name, "part-") {
				continue
			}
			partPath := filepath.Join(dir, name)
			chs, err := discoverPartChapters(partPath)
			if err != nil {
				return nil, err
			}
			if len(chs) == 0 {
				continue
			}
			for i := range chs {
				if chs[i].ENPath != "" {
					b.HasEN = true
				}
			}
			title := name
			if t := partTitles[name]; t != "" {
				title = t
			}
			b.Sections = append(b.Sections, Section{Type: "part", ID: name, Title: title, Chapters: chs})
			continue
		}
		if item.IsDir() || !strings.HasSuffix(name, ".md") || name == "_index.md" || strings.HasSuffix(name, ".en.md") {
			continue
		}
		base := strings.TrimSuffix(name, ".md")
		var typ string
		switch base {
		case "introduction", "epilogue", "appendix":
			typ = base
		default:
			continue
		}
		ch := readBookFile(filepath.Join(dir, name))
		if ch.Title == "" {
			ch.Title = base
		}
		b.Sections = append(b.Sections, Section{Type: typ, Chapters: []Chapter{ch}})
	}
	return b, nil
}

// discoverPartChapters lists and sorts the .md chapters of one part directory,
// mirroring toc.go's sortedChapterTitles order: introduction first, numeric
// prefix ascending, epilogue/appendix last.
func discoverPartChapters(partPath string) ([]Chapter, error) {
	entries, err := os.ReadDir(partPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", partPath, err)
	}
	type named struct {
		ch Chapter
		nm string
	}
	var chs []named
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "_index.md" || strings.HasSuffix(name, ".en.md") {
			continue
		}
		path := filepath.Join(partPath, name)
		ch := Chapter{SourcePath: path, Title: parseFrontmatterTitleFile(path)}
		if ch.Title == "" {
			ch.Title = strings.TrimSuffix(name, ".md")
		}
		en := strings.TrimSuffix(path, ".md") + ".en.md"
		if _, err := os.Stat(en); err == nil {
			ch.ENPath = en
		}
		chs = append(chs, named{ch, strings.TrimSuffix(name, ".md")})
	}
	specialOrder := map[string]int{"introduction": 0, "epilogue": 2, "appendix": 2}
	sort.SliceStable(chs, func(i, j int) bool {
		a, b := chs[i].nm, chs[j].nm
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
		na, nb := leadingNumber(a), leadingNumber(b)
		if na != nb {
			return na < nb
		}
		return a < b
	})
	out := make([]Chapter, len(chs))
	for i, c := range chs {
		out[i] = c.ch
	}
	return out, nil
}

// parseFrontmatterTitleFile reads the file and extracts its frontmatter title,
// falling back to the filename base — like toc.go extractTitle.
func parseFrontmatterTitleFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return parseFrontmatterTitle(data)
}

// parseFrontmatterTitle scans the first 30 lines for a "title: " line and
// trims surrounding quotes. Returns "" when absent.
func parseFrontmatterTitle(data []byte) string {
	lines := strings.SplitN(string(data), "\n", 31)
	for i, line := range lines {
		if i >= 30 {
			break
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "title:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(trimmed, "title:"))
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}

// leadingNumber extracts the numeric prefix: chapter-01 → 1.
func leadingNumber(s string) int {
	m := leadingNumberRE.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
