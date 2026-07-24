package sitemap

import (
	"encoding/xml"
	"strings"
)

// EnhanceOptions carries parameters for a single sitemap enhancement.
type EnhanceOptions struct {
	DefaultPriority   map[string]float64
	DefaultChangefreq map[string]string
}

// urlEntry represents a single <url> element in sitemap.xml.
type urlEntry struct {
	Loc        string  `xml:"loc"`
	Lastmod    string  `xml:"lastmod,omitempty"`
	Changefreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// urlSet is the root element of sitemap.xml.
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

// EnhanceSitemap reads sitemap XML, fills in missing priority/changefreq, and returns the enhanced XML.
// If the XML cannot be parsed, returns the original src unchanged.
func EnhanceSitemap(src string, opts *EnhanceOptions) string {
	var us urlSet
	if err := xml.Unmarshal([]byte(src), &us); err != nil {
		return src
	}
	if len(us.URLs) == 0 {
		return src
	}

	changed := false
	for i, u := range us.URLs {
		kind := GuessKindFromURL(u.Loc)

		// Fill priority if missing
		if u.Priority == 0 && opts != nil && opts.DefaultPriority != nil {
			if pri, ok := opts.DefaultPriority[kind]; ok {
				us.URLs[i].Priority = pri
				changed = true
			}
		}

		// Fill changefreq if missing
		if u.Changefreq == "" && opts != nil && opts.DefaultChangefreq != nil {
			if cf, ok := opts.DefaultChangefreq[kind]; ok {
				us.URLs[i].Changefreq = cf
				changed = true
			}
		}
	}

	if !changed {
		return src
	}

	// Re-marshal with nice formatting
	output, err := xml.MarshalIndent(us, "", "  ")
	if err != nil {
		return src
	}

	return xml.Header + string(output) + "\n"
}

// GuessKindFromURL infers the page kind from a sitemap <loc> URL.
// The URL may be absolute (https://example.com/posts/) or relative (/posts/).
func GuessKindFromURL(loc string) string {
	// Extract path from absolute URL if needed
	pathPart := loc
	if strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://") {
		// Find the path after the host
		// Use strings.IndexByte since we only need '/' and it's faster
		for i := 8; i < len(loc); i++ {
			if loc[i] == '/' {
				pathPart = loc[i:]
				break
			}
		}
	}

	// Normalize: remove trailing slash, remove index.html
	clean := strings.TrimSuffix(pathPart, "/")
	clean = strings.TrimSuffix(clean, "/index.html")

	// Root path → home
	if clean == "" || clean == "/" {
		return "home"
	}

	// Split into segments
	clean = strings.TrimPrefix(clean, "/")
	segments := strings.Split(clean, "/")

	// /tags/ → taxonomy, /tags/something/ → term
	if len(segments) >= 1 && segments[0] == "tags" {
		if len(segments) == 1 {
			return "taxonomy"
		}
		return "term"
	}

	// /categories/ → taxonomy, /categories/something/ → term
	if len(segments) >= 1 && segments[0] == "categories" {
		if len(segments) == 1 {
			return "taxonomy"
		}
		return "term"
	}

	// /page/N/ → page (paginated home)
	if len(segments) >= 1 && segments[0] == "page" {
		return "page"
	}

	// Single segment → section (e.g. /posts/, /about/)
	if len(segments) == 1 {
		return "section"
	}

	// Multiple segments → page (e.g. /posts/my-post/, /2024/01/post/)
	return "page"
}