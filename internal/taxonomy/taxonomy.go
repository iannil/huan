// Package taxonomy builds tag/category taxonomies from page frontmatter.
package taxonomy

import (
	"sort"

	"github.com/novel_ttl/huan/internal/content"
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
// e.g., Build(pages, "tags") builds the tags taxonomy.
func Build(pages []*content.Page, field string) Taxonomy {
	tax := Taxonomy{}

	for _, p := range pages {
		if p.Draft {
			continue
		}
		if p.Build.List == "never" && p.Kind != "taxonomy" && p.Kind != "term" {
			// Pages excluded from listings shouldn't appear in taxonomy
			// But protected pages should still be tagged
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
			tax[term] = append(tax[term], p)
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
