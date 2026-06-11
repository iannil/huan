package template

import (
	"sort"
	"time"
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
func (p PageSlice) GroupByDate(layout string) []DateGroup {
	groups := map[string]PageSlice{}
	var keys []string
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
		result = append(result, DateGroup{Key: k, Pages: groups[k]})
	}
	return result
}

var _ = time.Time{}
