package audit

import "sort"

// SourceEntry describes one default-language source markdown file and the
// state of its English counterpart. Populated by the CLI from the filesystem
// (reusing discoverSourceMarkdown / sidecarPath / readSourceMarkdown).
type SourceEntry struct {
	RelPath string // language-neutral source path, e.g. "products/foo.md"
	HasBody bool   // false for empty-body listing pages (_index.md) — never translated
	HasEN   bool   // .en.md sidecar exists
}

// ComputeMissingEN returns findings for source pages that have translatable
// body content but no .en.md sidecar. Empty-body listing pages are skipped:
// the translation pipeline itself SKIPs them (translate_cmd.go), so they are
// expected to have no English counterpart and are not violations.
//
// Results are sorted by RelPath for stable report output.
func ComputeMissingEN(sources []SourceEntry) []Finding {
	var out []Finding
	for _, s := range sources {
		if !s.HasBody || s.HasEN {
			continue
		}
		out = append(out, Finding{
			Ref:      s.RelPath,
			Kind:     KindMissingEN,
			Evidence: "source has body content but no .en.md sidecar",
		})
	}
	sortFindings(out)
	return out
}

// ComputeOrphanEN returns findings for English sidecar paths that have no
// corresponding default-language source. orphanRelPaths are .en.md paths
// (e.g. "products/foo.en.md") whose base source was not found.
func ComputeOrphanEN(orphanRelPaths []string) []Finding {
	out := make([]Finding, 0, len(orphanRelPaths))
	for _, p := range orphanRelPaths {
		out = append(out, Finding{
			Ref:      p,
			Kind:     KindOrphanEN,
			Evidence: ".en.md sidecar has no matching source .md",
		})
	}
	sortFindings(out)
	return out
}

func sortFindings(f []Finding) {
	sort.Slice(f, func(i, j int) bool { return f[i].Ref < f[j].Ref })
}
