package dag

// rules.go: dependency-edge rules used by BuildFromSite.
//
// Dependency semantics (corrected):
//
//	Aggregator pages DEPEND ON the articles they aggregate, because they
//	DISPLAY article content. When an article changes, every aggregator that
//	lists it must be rebuilt. Concretely:
//
//	  - article ("page" kind): DependsOn = [] (leaf)
//	  - section list page:     DependsOn = [all article URLs in this section]
//	  - home page:             DependsOn = [all section URLs] (home iterates sections)
//	  - term/tag page:         DependsOn = [all article URLs tagged with this tag]
//
// AffectedBy then walks DependedBy (who lists me) starting from the changed
// article, which yields the article + every aggregator that lists it. This is
// the inverse of the earlier (buggy) design where articles forward-depended on
// their aggregators; that design left the aggregators stale after an article
// edit because nothing depended on the article.
