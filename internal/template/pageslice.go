package template

import (
	"sort"
	"time"

	"github.com/iannil/huan/internal/i18n"
)

// PageSlice is a sortable, chainable slice of page contexts.
// Implemented as []interface{} so it is interchangeable with the result of
// Hugo's slice() function in Go templates (which returns []interface{}).
type PageSlice []interface{}

// asCtx safely extracts a *Context from a slice element.
func asCtx(v interface{}) *Context {
	if c, ok := v.(*Context); ok {
		return c
	}
	return nil
}

// AsCtx is the exported form of asCtx.
func AsCtx(v interface{}) *Context {
	return asCtx(v)
}

// ByDate sorts pages by Date ascending and returns the result.
func (p PageSlice) ByDate() PageSlice {
	out := make(PageSlice, len(p))
	copy(out, p)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := asCtx(out[i]), asCtx(out[j])
		if a == nil || b == nil {
			return false
		}
		return a.Date.Before(b.Date)
	})
	return out
}

// ByLastmod sorts pages by Lastmod ascending and returns the result.
func (p PageSlice) ByLastmod() PageSlice {
	out := make(PageSlice, len(p))
	copy(out, p)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := asCtx(out[i]), asCtx(out[j])
		if a == nil || b == nil {
			return false
		}
		return a.Lastmod.Before(b.Lastmod)
	})
	return out
}

// ByPublishDate sorts pages by PublishDate ascending.
func (p PageSlice) ByPublishDate() PageSlice {
	return p.ByDate()
}

// Reverse returns the slice in reverse order.
func (p PageSlice) Reverse() PageSlice {
	out := make(PageSlice, len(p))
	for i, v := range p {
		out[len(p)-1-i] = v
	}
	return out
}

// Len returns the number of pages.
func (p PageSlice) Len() int { return len(p) }

// SortDefault sorts the slice in-place using Hugo's DefaultPageSort:
//   - Weight (0 sorts last; otherwise ascending)
//   - Date desc
//   - Linkable Title asc (uses Title when LinkTitle empty)
//   - RelPermalink asc (byte-level)
//
// The collator is required for the Title tie-break layer. Callers should
// construct it once per build via i18n.BuildCollator(langCode) and reuse.
func (p PageSlice) SortDefault(coll CompareFunc) {
	if coll == nil {
		// Fallback: byte-level comparison on Title
		coll = func(a, b string) int {
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			default:
				return 0
			}
		}
	}
	sort.SliceStable(p, func(i, j int) bool {
		a, b := asCtx(p[i]), asCtx(p[j])
		if a == nil || b == nil {
			return false
		}
		// Layer 1: Weight (0 sorts last; otherwise ascending)
		if a.Weight == 0 && b.Weight != 0 {
			return false
		}
		if a.Weight != 0 && b.Weight == 0 {
			return true
		}
		if a.Weight != b.Weight {
			return a.Weight < b.Weight
		}
		// Layer 2: Date desc
		if !a.Date.Equal(b.Date) {
			return a.Date.After(b.Date)
		}
		// Layer 3: Collator on Title asc
		if c := coll(a.Title, b.Title); c != 0 {
			return c < 0
		}
		// Layer 4: RelPermalink asc (byte-level)
		return a.RelPermalink < b.RelPermalink
	})
}

// CompareFunc is a string comparison function used by SortDefault for the
// Title tie-break layer. Returns <0 if a<b, 0 if equal, >0 if a>b.
type CompareFunc func(a, b string) int

// First returns the first page, or a safe default Context if empty.
// Returning a non-nil value avoids template nil-pointer panics when callers
// chain methods like .First.Lastmod.
func (p PageSlice) First() *Context {
	if len(p) == 0 {
		return &Context{}
	}
	return asCtx(p[0])
}

// latest returns the page with the most recent Lastmod.
func (p PageSlice) latest() *Context {
	if len(p) == 0 {
		return nil
	}
	latest := asCtx(p[0])
	for _, v := range p[1:] {
		if c := asCtx(v); c != nil && c.Lastmod.After(latest.Lastmod) {
			latest = c
		}
	}
	return latest
}

// DateGroup is a Hugo-compatible group of pages sharing a date key.
type DateGroup struct {
	Key   string
	Pages PageSlice
}

// GroupByDate groups pages by year, returning groups in descending order.
// Within each group, pages are sorted by Date desc with Title desc (via the
// site collator) as tiebreaker — matching Hugo's observed behavior. Hugo's
// GroupByDate groups pages by the formatted date key (reverse chronological
// order across groups), and within each group applies a Date desc → Title
// desc → Path desc ordering (effectively the reverse of DefaultPageSort's
// tiebreakers, since GroupByDate sorts by Date desc as primary key).
//
// For zhurongshuo, two posts can share the exact same Date timestamp (e.g.
// post 0801 and 0802 both at 2021-08-08T12:27:45+08:00). Hugo's tiebreaker
// for these is the reverse of the site collator's Title comparison.
func (p PageSlice) GroupByDate(layout string) []DateGroup {
	groups := map[string]PageSlice{}
	var keys []string
	// Detect site language for collator. Use the first page with a non-nil
	// Site; fall back to empty (English) for synthetic test contexts.
	langCode := ""
	for _, v := range p {
		c := asCtx(v)
		if c != nil && c.Site != nil && c.Site.LanguageCode != "" {
			langCode = c.Site.LanguageCode
			break
		}
	}
	coll := i18n.BuildCollator(langCode)
	for _, v := range p {
		c := asCtx(v)
		if c == nil || c.Date.IsZero() {
			continue
		}
		key := c.Date.Format(layout)
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], c)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	result := make([]DateGroup, 0, len(keys))
	for _, k := range keys {
		pages := groups[k]
		// Sort within group: Date desc → Title desc (collator) → Path desc.
		// This matches Hugo's empirical behavior for tied dates.
		sort.SliceStable(pages, func(i, j int) bool {
			a, b := asCtx(pages[i]), asCtx(pages[j])
			if a == nil || b == nil {
				return false
			}
			// Date desc: newer sorts first
			if !a.Date.Equal(b.Date) {
				return a.Date.After(b.Date)
			}
			// Tiebreak: Title desc via collator (reverse of DefaultPageSort's asc)
			if c := coll.CompareString(a.Title, b.Title); c != 0 {
				return c > 0
			}
			// Final tiebreak: File.Path desc
			apath, bpath := "", ""
			if a.File != nil {
				apath = a.File.Path
			}
			if b.File != nil {
				bpath = b.File.Path
			}
			return apath > bpath
		})
		result = append(result, DateGroup{Key: k, Pages: pages})
	}
	return result
}

var _ = time.Time{}
