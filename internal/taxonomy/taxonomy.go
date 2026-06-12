// Package taxonomy builds tag/category taxonomies from page frontmatter.
package taxonomy

import (
	"sort"

	"github.com/iannil/huan/internal/content"
)

// Taxonomy maps a term (e.g., tag name) to the pages tagged with it.
type Taxonomy map[string]WeightedPages

// WeightedPages is a slice of pages associated with a taxonomy term.
type WeightedPages []*content.Page

// TermEntry represents a single term with its pages, for term-list rendering.
type TermEntry struct {
	Name  string
	Pages WeightedPages
	Count int
}

// Build constructs a taxonomy map from pages, keyed by the given frontmatter field.
// Hugo's taxonomy keys are urlize-form (lowercase ASCII, CJK preserved).
// e.g., Build(pages, "tags") builds the tags taxonomy.
func Build(pages []*content.Page, field string) Taxonomy {
	tax := Taxonomy{}

	for _, p := range pages {
		if p.Draft {
			continue
		}

		var terms []string
		switch field {
		case "tags":
			terms = p.Tags
		default:
			// Only tags is currently supported
			continue
		}

		for _, term := range terms {
			if term == "" {
				continue
			}
			// Hugo stores taxonomy terms in urlized form (lowercase ASCII,
			// CJK preserved) so .Site.Taxonomies.tags keys match the URL path.
			key := hugoUrlize(term)
			tax[key] = append(tax[key], p)
		}
	}

	return tax
}

// BuildAll builds all taxonomies (currently just "tags") and returns them as a map.
func BuildAll(pages []*content.Page) map[string]Taxonomy {
	return map[string]Taxonomy{
		"tags": Build(pages, "tags"),
	}
}

// ByCount returns term entries sorted by page count (descending).
// Hugo's .Data.Terms.ByCount behavior.
func (t Taxonomy) ByCount() []TermEntry {
	entries := make([]TermEntry, 0, len(t))
	for term, pages := range t {
		entries = append(entries, TermEntry{
			Name:  term,
			Pages: pages,
			Count: len(pages),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// Alphabetical returns term entries sorted alphabetically.
func (t Taxonomy) Alphabetical() []TermEntry {
	entries := make([]TermEntry, 0, len(t))
	for term, pages := range t {
		entries = append(entries, TermEntry{
			Name:  term,
			Pages: pages,
			Count: len(pages),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// Count returns the number of unique terms.
func (t Taxonomy) Count() int {
	return len(t)
}

// hugoUrlize mirrors Hugo's urlize for taxonomy keys:
// lowercase ASCII, preserve CJK and other Unicode, replace whitespace with -.
func hugoUrlize(s string) string {
	var b []rune
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b = append(b, r+32)
		case r == ' ' || r == '\t' || r == '\n':
			b = append(b, '-')
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			b = append(b, r)
		default:
			b = append(b, r)
		}
	}
	return string(b)
}
