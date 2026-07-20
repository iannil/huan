package dag

import "github.com/iannil/huan/internal/content"

// PageDependencies returns the list of page paths that a given page depends on.
// The dependency graph is built by traversing these edges.
func PageDependencies(pg *content.Page) []string {
	deps := make([]string, 0, 8)

	switch pg.Kind {
	case "page":
		// Article page depends on its section list page, home page, and tag pages.
		if pg.Section != "" {
			deps = append(deps, "/"+pg.Section+"/")
		}
		deps = append(deps, "/")
		for _, tag := range pg.Tags {
			deps = append(deps, "/tags/"+tag+"/")
		}
	case "section":
		// Section list page contains its child pages. Dependencies are
		// reverse: the section page is depended on by its children.
		// Section itself depends on home page.
		deps = append(deps, "/")
	case "home":
		// Home page depends on all published pages (reverse edges).
		// No forward dependencies.
	case "term":
		// Term page depends on home page.
		deps = append(deps, "/")
	}

	return deps
}

// IsReverseDependency reports whether a dependency is inherently "reverse"
// (i.e., A depends on the existence of B, but changes to A don't affect B).
// Such edges are not traversed in the forward direction during incremental builds.
func IsReverseDependency(from, to string) bool {
	// Section pages are "reverse" dependencies of their children:
	// changing a section page doesn't affect article pages.
	return false
}
