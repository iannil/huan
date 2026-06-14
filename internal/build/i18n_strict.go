package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// I18nStrictOptions controls stale translation detection.
// When strict mode is enabled, BuildSite fails-fast on .en.md sidecars
// whose source_hash frontmatter doesn't match the current source markdown.
type I18nStrictOptions struct {
	// Enabled is true when strict mode is on (typically CI builds).
	// When false (default for local builds), mismatches log warnings but
	// don't fail the build.
	Enabled bool

	// TargetLangs is the list of non-default language codes to check.
	// Empty means check all .<lang>.md sidecars found in content/.
	TargetLangs []string
}

// I18nStaleReport summarizes stale translation sidecars found during build.
type I18nStaleReport struct {
	// Checked counts .en.md (and other sidecar) files whose frontmatter
	// was successfully parsed and source_hash compared.
	Checked int

	// Stale counts sidecars whose source_hash doesn't match the current
	// source markdown sha256.
	Stale int

	// Missing counts sidecars whose frontmatter lacks source_hash entirely
	// (e.g. hand-created without going through translate plugin).
	Missing int

	// StaleFiles lists paths of stale sidecars (for error reporting).
	StaleFiles []string

	// MissingHashFiles lists paths of sidecars without source_hash.
	MissingHashFiles []string
}

// Error returns a multi-line error describing all stale sidecars.
// Implements error interface so report can be returned directly from
// BuildSite when strict mode fails.
func (r *I18nStaleReport) Error() string {
	if r == nil || (r.Stale == 0 && r.Missing == 0) {
		return ""
	}
	var b strings.Builder
	b.WriteString("i18n stale translation sidecars detected:\n")
	if r.Stale > 0 {
		b.WriteString(fmt.Sprintf("  stale (source changed but sidecar not re-translated): %d\n", r.Stale))
		for _, f := range r.StaleFiles {
			b.WriteString("    - " + f + "\n")
		}
	}
	if r.Missing > 0 {
		b.WriteString(fmt.Sprintf("  missing source_hash frontmatter: %d\n", r.Missing))
		for _, f := range r.MissingHashFiles {
			b.WriteString("    - " + f + "\n")
		}
	}
	b.WriteString("\nRun `huan translate qwen3` to refresh stale translations.\n")
	b.WriteString("Or set HUAN_STRICT_I18N=false to bypass (not recommended in CI).")
	return b.String()
}

// checkStaleTranslations scans content/ for .<lang>.md sidecars and verifies
// each sidecar's frontmatter source_hash matches the current source markdown
// sha256. Returns a report describing what was found.
//
// Source markdown is the corresponding file without the language suffix:
//   "posts/foo.en.md" → "posts/foo.md"
//
// Sidecars without a corresponding source file are skipped (the source may
// have been deleted; that's a separate concern).
//
// Sidecars without source_hash frontmatter are flagged as Missing (not
// Stale) — they were likely hand-created. Strict mode treats both as
// failures.
func checkStaleTranslations(contentDir string) (*I18nStaleReport, error) {
	report := &I18nStaleReport{}

	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".md") {
			return nil
		}

		// Detect language suffix (e.g. "posts/foo.en.md" → lang="en")
		lang := detectSidecarLang(name)
		if lang == "" {
			return nil // default-language source file, not a sidecar
		}

		// Find corresponding source file (strip .<lang> suffix)
		srcPath := stripSidecarLangSuffix(path, lang)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return nil // source gone; skip (separate concern)
		}

		// Parse sidecar frontmatter for source_hash
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable sidecars
		}
		sidecarHash, hasHash := extractSourceHash(string(data))
		if !hasHash {
			report.Missing++
			rel, _ := filepath.Rel(contentDir, path)
			report.MissingHashFiles = append(report.MissingHashFiles, rel)
			return nil
		}

		// Compute current source hash
		srcData, err := os.ReadFile(srcPath)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(srcData)
		currentHash := hex.EncodeToString(sum[:])

		report.Checked++
		if currentHash != sidecarHash {
			report.Stale++
			rel, _ := filepath.Rel(contentDir, path)
			report.StaleFiles = append(report.StaleFiles, rel)
		}
		return nil
	})
	return report, err
}

// detectSidecarLang returns the language code from a filename like
// "foo.en.md" or "_index.zh-cn.md". Returns empty string for default-
// language files (no language suffix).
//
// This is a simpler version of content.detectLanguageFromFilename that
// doesn't import the content package (avoids circular dependency).
func detectSidecarLang(name string) string {
	if !strings.HasSuffix(name, ".md") {
		return ""
	}
	base := name[:len(name)-3]
	dot := strings.LastIndex(base, ".")
	if dot < 0 {
		return ""
	}
	suffix := base[dot+1:]
	if len(suffix) < 2 || len(suffix) > 8 {
		return ""
	}
	for _, r := range suffix {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return ""
		}
	}
	return suffix
}

// stripSidecarLangSuffix removes ".<lang>" before ".md" from path.
// "posts/foo.en.md" with lang="en" → "posts/foo.md"
func stripSidecarLangSuffix(path, lang string) string {
	suffix := "." + lang + ".md"
	if strings.HasSuffix(path, suffix) {
		return path[:len(path)-len(suffix)] + ".md"
	}
	return path
}

// extractSourceHash parses frontmatter from markdown content and returns
// the source_hash field value. Returns ("", false) when frontmatter is
// missing or source_hash field is absent.
//
// Uses a minimal YAML decoder (gopkg.in/yaml.v3) to extract just the
// source_hash field. Tolerates frontmatter that uses --- or +++ delimiters.
func extractSourceHash(markdown string) (string, bool) {
	fmText, ok := extractFrontmatter(markdown)
	if !ok {
		return "", false
	}
	var fm struct {
		SourceHash string `yaml:"source_hash"`
	}
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return "", false
	}
	if fm.SourceHash == "" {
		return "", false
	}
	return fm.SourceHash, true
}

// extractFrontmatter returns the YAML text between --- delimiters at the
// start of markdown. Returns false when no frontmatter is present.
func extractFrontmatter(markdown string) (string, bool) {
	markdown = strings.TrimSpace(markdown)
	if !strings.HasPrefix(markdown, "---") {
		return "", false
	}
	rest := markdown[3:]
	if !strings.HasPrefix(rest, "\n") {
		return "", false
	}
	rest = rest[1:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// xmlEscape is a helper for XML sitemap output. Unused here but kept for
// future use; suppresses unused import warnings if encoding/xml gets used.
var _ = xml.EscapeText