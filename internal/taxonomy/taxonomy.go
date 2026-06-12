// Package taxonomy builds tag/category taxonomies from page frontmatter.
package taxonomy

import (
	"sort"

	"github.com/iannil/huan/internal/content"
)

// Taxonomy maps a term (e.g., tag name) to the pages tagged with it.
// Keys are in urlized form (lowercase ASCII, CJK preserved) so they match
// the URL path. To recover the original (display) casing for term titles,
// use OriginalCase().
type Taxonomy map[string]WeightedPages

// WeightedPages is a slice of pages associated with a taxonomy term.
type WeightedPages []*content.Page

// TermEntry represents a single term with its pages, for term-list rendering.
type TermEntry struct {
	Name  string
	Pages WeightedPages
	Count int
}

// originalCaseMap maps a urlized taxonomy key to its original-cased form
// (the first casing seen in frontmatter). Used by term-page renderers that
// need the display name (e.g., <title>FANFAN on ...</title>), while the
// urlized key is still used for filesystem paths and URL paths.
type originalCaseMap map[string]string

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

// BuildWithOriginalCase is like Build but also returns a map from urlized key
// to the original-cased form (first occurrence wins). Hugo's term-page .Title
// uses the original casing while the urlized key is used for URLs/paths.
func BuildWithOriginalCase(pages []*content.Page, field string) (Taxonomy, map[string]string) {
	tax := Taxonomy{}
	original := map[string]string{}

	for _, p := range pages {
		if p.Draft {
			continue
		}

		var terms []string
		switch field {
		case "tags":
			terms = p.Tags
		default:
			continue
		}

		for _, term := range terms {
			if term == "" {
				continue
			}
			key := hugoUrlize(term)
			tax[key] = append(tax[key], p)
			// Preserve first-seen original casing. Subsequent declarations
			// with different casing do not override the first.
			if _, exists := original[key]; !exists {
				original[key] = term
			}
		}
	}

	return tax, original
}

// BuildAll builds all taxonomies (currently just "tags") and returns them as a map.
func BuildAll(pages []*content.Page) map[string]Taxonomy {
	return map[string]Taxonomy{
		"tags": Build(pages, "tags"),
	}
}

// BuildAllWithOriginalCase is like BuildAll but also returns a per-plural map
// of urlized-key → original-cased name. Used to recover display casing for
// term-page titles (e.g. <title>FANFAN on ...</title>) while keeping the
// urlized key for filesystem paths and URL paths.
func BuildAllWithOriginalCase(pages []*content.Page) (map[string]Taxonomy, map[string]map[string]string) {
	tagsTax, tagsOriginal := BuildWithOriginalCase(pages, "tags")
	return map[string]Taxonomy{"tags": tagsTax},
		map[string]map[string]string{"tags": tagsOriginal}
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
