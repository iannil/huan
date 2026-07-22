package build

import "strings"

// resolveSourceFromURL derives the source file path (relative to content/)
// from a page URL. Used by JIT rendering when the URL is not in the DAG
// (e.g., a newly-created draft not yet captured by a full build).
//
// URL conventions (match Hugo/huan content layout):
//   /                          → _index.md            (home)
//   /posts/                    → posts/_index.md      (section)
//   /posts/hello/              → posts/hello.md       (regular page)
//   /posts/2026/new-year/      → posts/2026/new-year.md
//   /posts/_index/             → posts/_index.md      (explicit)
//
// Returns "" only if the input cannot be parsed (effectively never for a
// normalized URL); callers should still verify the file exists on disk.
func resolveSourceFromURL(pageURL string) string {
	u := strings.Trim(pageURL, "/")
	if u == "" {
		return "_index.md" // home
	}
	parts := strings.Split(u, "/")
	last := parts[len(parts)-1]
	if last == "_index" {
		// /posts/_index/ → posts/_index.md
		return strings.Join(parts, "/") + ".md"
	}
	if len(parts) == 1 {
		// /posts/ → posts/_index.md (single segment = section index)
		return parts[0] + "/_index.md"
	}
	// /posts/hello/ → posts/hello.md
	return strings.Join(parts, "/") + ".md"
}
