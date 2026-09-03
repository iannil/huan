// Package content discovers the book/practice tree of a huan project for
// ebook export: data/<kind>.yaml metadata + content/<kind>/ markdown tree.
package content

import "sort"

// Lang identifies which language variant of a chapter to export.
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

func (l Lang) String() string { return string(l) }

// Chapter is one markdown file (one chapter inside a part, or a standalone
// special file: introduction/epilogue/appendix).
type Chapter struct {
	// SourcePath is the absolute path of the .md (zh) or .en.md (en) file.
	SourcePath string
	// ENPath is the absolute path of the .en.md sidecar, or "" when absent.
	ENPath string
	Title  string // frontmatter title (fallback: filename base)
}

// Section is a top-level assembly unit of a book.
type Section struct {
	Type  string // "introduction" | "part" | "chapter" | "epilogue" | "appendix"
	ID    string // "part-01" for parts; empty for specials
	Title string // part title from data yaml part_titles; specials use fm title
	// Chapters holds part children. Empty for specials (the special itself
	// is carried by Chapters[0] for uniform downstream handling — see Discover).
	Chapters []Chapter
}

// BookEntry is one book (content/books) or practice (content/practices).
type BookEntry struct {
	Slug        string
	TitleZH     string // data yaml title
	TitleEN     string // data yaml subtitle (English)
	SubtitleZH  string
	Version     string // rc / beta / alpha
	LastUpdated string
	// VolumeNumber is the 1-based volume (books) or season (practices) index.
	VolumeNumber int
	VolumeName   string // e.g. "第1卷" / "第1季"
	// Dir is the book's content directory (…/volume-1/<slug>).
	Dir string
	// Sections in discovered order; use OrderedSections for assembly order.
	Sections []Section
	// HasEN is true when at least one .en.md sidecar exists.
	HasEN bool
}

// OrderedSections returns sections in book assembly order:
// introduction, part-01..part-N (sorted), epilogue, appendix.
// Standalone "chapter" sections (books without parts) keep discovery order
// between introduction and epilogue.
func (b *BookEntry) OrderedSections() []Section {
	out := make([]Section, 0, len(b.Sections))
	// specials first: introduction, then prologue (novel openings such as
	// practices/persona-and-performance ship their main content this way)
	for _, typ := range []string{"introduction", "prologue"} {
		for _, s := range b.Sections {
			if s.Type == typ {
				out = append(out, s)
			}
		}
	}
	// parts sorted by ID; standalone chapters in discovery order interleaved
	// after parts (zhurongshuo books all use parts; chapters kept for safety)
	var parts []Section
	var chapters []Section
	for _, s := range b.Sections {
		switch s.Type {
		case "part":
			parts = append(parts, s)
		case "chapter":
			chapters = append(chapters, s)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].ID < parts[j].ID })
	out = append(out, parts...)
	out = append(out, chapters...)
	// trailing specials in fixed order: epilogue, then appendix
	for _, s := range b.Sections {
		if s.Type == "epilogue" {
			out = append(out, s)
		}
	}
	for _, s := range b.Sections {
		if s.Type == "appendix" {
			out = append(out, s)
		}
	}
	return out
}

// VolumeEntry groups the books of one volume / practices of one season.
type VolumeEntry struct {
	Number int    // 1-based
	Name   string // "第1卷" / "第1季"
	Books  []BookEntry
}

// Collection is the full discovered tree for one kind (books or practices).
type Collection struct {
	Kind    string // "books" | "practices"
	Volumes []VolumeEntry
}
