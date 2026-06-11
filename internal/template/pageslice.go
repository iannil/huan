package template

import (
	"sort"
	"time"
)

// PageSlice is a sortable, chainable slice of page contexts.
// It mirrors Hugo's Pages type, supporting ByDate, ByLastmod, Reverse etc.
type PageSlice []*Context

// ByDate sorts pages by Date ascending and returns the result.
func (p PageSlice) ByDate() PageSlice {
	out := make(PageSlice, len(p))
	copy(out, p)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out
}

// ByLastmod sorts pages by Lastmod ascending and returns the result.
func (p PageSlice) ByLastmod() PageSlice {
	out := make(PageSlice, len(p))
	copy(out, p)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Lastmod.Before(out[j].Lastmod)
	})
	return out
}

// ByPublishDate sorts pages by PublishDate ascending.
func (p PageSlice) ByPublishDate() PageSlice {
	out := make(PageSlice, len(p))
	copy(out, p)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out
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

// First returns the first page (nil if empty).
func (p PageSlice) First() *Context {
	if len(p) == 0 {
		return nil
	}
	return p[0]
}

// latest returns the page with the most recent Lastmod (used by RSS template).
func (p PageSlice) latest() *Context {
	if len(p) == 0 {
		return nil
	}
	latest := p[0]
	for _, c := range p[1:] {
		if c.Lastmod.After(latest.Lastmod) {
			latest = c
		}
	}
	return latest
}

// GroupByDate groups pages by year, returning groups in descending order.
// Used by list templates that show "GroupByDate 2006".
type DateGroup struct {
	Key   string
	Pages PageSlice
}

// GroupByDate groups pages by the given time layout key (e.g., "2006").
func (p PageSlice) GroupByDate(layout string) []DateGroup {
	groups := map[string]PageSlice{}
	var keys []string
	for _, c := range p {
		if c.Date.IsZero() {
			continue
		}
		key := c.Date.Format(layout)
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], c)
	}
	// Sort keys descending (newest year first)
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	result := make([]DateGroup, 0, len(keys))
	for _, k := range keys {
		result = append(result, DateGroup{Key: k, Pages: groups[k]})
	}
	return result
}

// ensure TimeResult imports time
var _ = time.Time{}
