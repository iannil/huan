package equiv

import (
	"fmt"
	"os"
	"path/filepath"
)

// Mode selects which equivalence check to run.
type Mode string

const (
	ModeByte       Mode = "byte"       // raw cmp (radar, never fails)
	ModeNormalized Mode = "normalized" // HTML normalize then cmp (visual)
	ModeSEO        Mode = "seo"        // SEO field extract then cmp
	ModeAI         Mode = "ai"         // AI field extract then cmp
)

// Report is the output of a single mode comparison over a pair of dirs.
type Report struct {
	Mode       Mode
	Identical  int
	Differing  []string // relative paths
	MissingInA []string // files only in B
	ExtraInA   []string // files only in A
}

// Pass returns true if this mode's gate is satisfied.
// byte mode always passes (radar only); others require zero differing files
// AND zero missing/extra files.
func (r Report) Pass() bool {
	if r.Mode == ModeByte {
		return true
	}
	return len(r.Differing) == 0 && len(r.MissingInA) == 0 && len(r.ExtraInA) == 0
}

// CompareDirs runs the given mode across two parallel directory trees.
// Files considered: .html / .htm / .xml.
func CompareDirs(mode Mode, dirA, dirB string) (Report, error) {
	r := Report{Mode: mode}
	filesA, err := collectFiles(dirA)
	if err != nil {
		return r, err
	}
	filesB, err := collectFiles(dirB)
	if err != nil {
		return r, err
	}
	setA, setB := toSet(filesA), toSet(filesB)
	for f := range setA {
		if !setB[f] {
			r.ExtraInA = append(r.ExtraInA, f)
		}
	}
	for f := range setB {
		if !setA[f] {
			r.MissingInA = append(r.MissingInA, f)
		}
	}
	for f := range setA {
		if !setB[f] {
			continue
		}
		a, _ := os.ReadFile(filepath.Join(dirA, f))
		b, _ := os.ReadFile(filepath.Join(dirB, f))
		if compareContent(mode, string(a), string(b)) {
			r.Identical++
		} else {
			r.Differing = append(r.Differing, f)
		}
	}
	return r, nil
}

func compareContent(mode Mode, a, b string) bool {
	switch mode {
	case ModeByte:
		return a == b
	case ModeNormalized:
		return NormalizeHTML(a) == NormalizeHTML(b)
	case ModeSEO:
		return ExtractSEO(a).Equal(ExtractSEO(b))
	case ModeAI:
		return ExtractAI(a).Equal(ExtractAI(b))
	}
	return false
}

func collectFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		ext := filepath.Ext(rel)
		if ext == ".html" || ext == ".htm" || ext == ".xml" {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out, err
}

func toSet(s []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}

// FormatSummary returns a human-readable summary line.
func (r Report) FormatSummary() string {
	status := "PASS"
	if !r.Pass() {
		status = "FAIL"
	}
	return fmt.Sprintf("[%s] mode=%s identical=%d differing=%d missing=%d extra=%d",
		status, r.Mode, r.Identical, len(r.Differing), len(r.MissingInA), len(r.ExtraInA))
}
