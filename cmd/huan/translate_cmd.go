package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/translate"
	"github.com/spf13/cobra"
)

// newTranslateCmd builds the `huan translate` subcommand tree.
//
// v1 implements:
//   huan translate qwen3                # translate all stale posts
//   huan translate qwen3 --file <path>  # single post
//   huan translate qwen3 --all --force  # force re-translate all
//   huan translate qwen3 --dry-run      # list stale files without calling LLM
//   huan translate status               # report translation state
//
// Future: huan translate terms --propose (glossary maintenance)
func newTranslateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "translate",
		Short: "Translate content between languages via a translator plugin",
		Long:  "Translate runs a translator plugin (e.g. qwen3_translate) to generate .en.md sidecars from source markdown. See `huan plugin list` for available translator plugins.",
	}
	cmd.AddCommand(newTranslateQwen3Cmd())
	cmd.AddCommand(newTranslateStatusCmd())
	cmd.AddCommand(newTranslateAuditCmd())
	cmd.AddCommand(newTranslateBackfillCmd())
	return cmd
}

func newTranslateQwen3Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "qwen3",
		Short: "Translate via local Qwen3 model (Ollama HTTP API)",
		Args:  cobra.NoArgs,
		RunE:  runTranslateQwen3,
	}
	cmd.Flags().String("file", "", "translate a single source file (relative to content/)")
	cmd.Flags().Bool("all", false, "scan all source files (default; mutually exclusive with --file)")
	cmd.Flags().Bool("force", false, "force re-translation even when source_hash matches")
	cmd.Flags().Bool("dry-run", false, "list files that would be translated without calling LLM")
	cmd.Flags().Int("limit", 0, "max files to translate (0 = no limit; use for batch testing before full run)")
	cmd.Flags().Int("progress-every", 10, "print progress summary every N successful files (0 = disabled). Summary shows completed/total, failures, elapsed time, throughput (files/min), and ETA — useful for long runs of 100+ files.")
	cmd.Flags().String("model", "", "override configured model for this invocation")
	cmd.Flags().String("source-lang", "zh-cn", "source language code")
	cmd.Flags().String("target-lang", "en", "target language code")
	return cmd
}

func newTranslateStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report translation state (cached / stale / missing) for all source files",
		Args:  cobra.NoArgs,
		RunE:  runTranslateStatus,
	}
	return cmd
}

func runTranslateQwen3(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	registry, err := newPluginRegistry(cfg)
	if err != nil {
		return fmt.Errorf("plugin registry: %w", err)
	}

	p, ok := registry.Get("qwen3_translate")
	if !ok {
		// Graceful skip: deploy.sh calls `huan translate qwen3` unconditionally;
		// if the plugin isn't configured (no qwen3_translate block in huan.yaml),
		// exit cleanly with a friendly message rather than erroring out. This
		// keeps the deploy chain resilient — operators who don't use i18n
		// translation shouldn't have to gate the call.
		fmt.Fprintln(os.Stderr, "translate: qwen3_translate plugin not configured; skipping (add plugins.qwen3_translate.* to huan.yaml to enable)")
		return nil
	}
	translator, ok := p.(translate.Translator)
	if !ok {
		return fmt.Errorf("qwen3_translate plugin does not implement Translator (internal error)")
	}

	fileFlag, _ := cmd.Flags().GetString("file")
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	limit, _ := cmd.Flags().GetInt("limit")
	progressEvery, _ := cmd.Flags().GetInt("progress-every")
	sourceLang, _ := cmd.Flags().GetString("source-lang")
	targetLang, _ := cmd.Flags().GetString("target-lang")

	// Discover source files
	contentDir := filepath.Join(sourceDir, "content")
	sourceFiles, err := discoverSourceMarkdown(contentDir, sourceLang)
	if err != nil {
		return fmt.Errorf("discover source files: %w", err)
	}

	if fileFlag != "" {
		// Filter to single file
		full := filepath.Join(contentDir, fileFlag)
		found := false
		for _, f := range sourceFiles {
			if f == full {
				sourceFiles = []string{f}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("file %s not found in content/", fileFlag)
		}
	}

	// Load glossary from configured glossary_file (relative to sourceDir)
	glossary := make(map[string]string)
	if rawQCfg, ok := cfg.Plugins["qwen3_translate"]; ok {
		if gf, ok := rawQCfg["glossary_file"].(string); ok && gf != "" {
			full := gf
			if !filepath.IsAbs(full) {
				full = filepath.Join(sourceDir, full)
			}
			loaded, err := loadGlossary(full)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: could not load glossary %s: %v\n", full, err)
			} else {
				glossary = loaded
			}
		}
	}

	// Filter stale
	stale := filterStaleFiles(sourceFiles, sourceLang, targetLang, force)

	fmt.Fprintf(os.Stderr, "translate: %d source files, %d stale, %d cached\n",
		len(sourceFiles), len(stale), len(sourceFiles)-len(stale))

	if dryRun {
		fmt.Println("Dry-run: would translate:")
		for _, f := range stale {
			rel, _ := filepath.Rel(contentDir, f)
			fmt.Printf("  %s\n", rel)
		}
		return nil
	}

	if len(stale) == 0 {
		fmt.Println("Nothing to translate (all cached).")
		return nil
	}

	// Apply --limit (batch testing mode). 0 = no limit (full run).
	if limit > 0 && limit < len(stale) {
		fmt.Fprintf(os.Stderr, "limit: translating first %d of %d stale files\n", limit, len(stale))
		stale = stale[:limit]
	}

	// Translate each file
	succeeded := 0
	failed := 0
	startTime := time.Now()
	for i, srcPath := range stale {
		rel, _ := filepath.Rel(contentDir, srcPath)
		fmt.Printf("[%d/%d] translating %s ... ", i+1, len(stale), rel)

		title, body, srcFM, err := readSourceMarkdown(srcPath)
		if err != nil {
			fmt.Printf("READ FAIL: %v\n", err)
			failed++
			continue
		}

		// Skip files with empty body (e.g., _index.md with only frontmatter).
		// These are listing/section pages with no translatable content; trying
		// to translate would error in the plugin with "request content is empty".
		// Count as skipped (not failed) so the run isn't blocked.
		if strings.TrimSpace(body) == "" {
			fmt.Printf("SKIP (empty body)\n")
			continue
		}

		resp, err := translator.Translate(context.Background(), translate.Request{
			SourceLang:  sourceLang,
			TargetLang:  targetLang,
			Title:       title,
			Content:     body,
			ContentType: "markdown",
			Glossary:    glossary,
		})
		if err != nil {
			fmt.Printf("TRANSLATE FAIL: %v\n", err)
			failed++
			continue
		}

		hardFails := resp.QualityChecks.HardCheckFailures()
		if len(hardFails) > 0 {
			fmt.Printf("QUALITY FAIL: %v\n", hardFails)
			failed++
			continue
		}

		// Write .en.md sidecar
		outPath := sidecarPath(srcPath, targetLang)
		if err := writeTranslatedSidecar(outPath, srcPath, sourceLang, targetLang, resp, srcFM, glossary); err != nil {
			fmt.Printf("WRITE FAIL: %v\n", err)
			failed++
			continue
		}
		fmt.Printf("OK (%d tokens, ratio %.2f)\n", resp.TokensUsed, resp.QualityChecks.LengthRatio)
		succeeded++

		// Periodic progress summary. Helps users monitoring long runs
		// (100+ files) know "still alive" + ETA. Printed every N successful
		// files when --progress-every > 0 (default 10).
		if progressEvery > 0 && succeeded%progressEvery == 0 {
			elapsed := time.Since(startTime)
			processed := succeeded + failed
			remaining := len(stale) - processed - skippedSoFar(stale, succeeded, failed)
			var throughputPerMin float64
			var eta time.Duration
			if elapsed.Minutes() > 0 {
				throughputPerMin = float64(processed) / elapsed.Minutes()
				if throughputPerMin > 0 {
					eta = time.Duration(float64(remaining)/throughputPerMin*float64(time.Minute))
				}
			}
			fmt.Fprintf(os.Stderr,
				"[progress] %d/%d done (%.1f%%) | %d failed | %s elapsed | %.1f files/min | ETA %s\n",
				succeeded, len(stale), float64(processed)*100.0/float64(len(stale)),
				failed, elapsed.Round(time.Second), throughputPerMin, eta.Round(time.Minute))
		}
	}

	// Final summary
	elapsed := time.Since(startTime).Round(time.Second)
	fmt.Fprintf(os.Stderr, "\ntranslate summary: succeeded=%d failed=%d elapsed=%s\n",
		succeeded, failed, elapsed)
	if failed > 0 {
		return fmt.Errorf("%d files failed translation", failed)
	}
	return nil
}

// skippedSoFar is a stub used in ETA calculation. Since we don't track
// skipped count separately (the for-loop continues without incrementing
// succeeded/failed for skipped files), this returns 0. For better ETA
// accuracy, future PR can track skipped count explicitly.
func skippedSoFar(stale []string, succeeded, failed int) int {
	// Best-effort: total processed = succeeded + failed + skipped, so
	// skipped = (succeeded + failed + skipped) - succeeded - failed.
	// We can't know this without tracking; return 0 (overestimates ETA).
	return 0
}

func runTranslateStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	_ = cfg

	contentDir := filepath.Join(sourceDir, "content")
	sourceFiles, err := discoverSourceMarkdown(contentDir, "zh-cn")
	if err != nil {
		return fmt.Errorf("discover source files: %w", err)
	}

	cached := 0
	stale := 0
	missing := 0
	for _, srcPath := range sourceFiles {
		state := classifyTranslationState(srcPath, "en")
		switch state {
		case "cached":
			cached++
		case "stale":
			stale++
		case "missing":
			missing++
		}
	}

	fmt.Printf("Translation status (target: en)\n")
	fmt.Printf("  total source files: %d\n", len(sourceFiles))
	fmt.Printf("  cached (source_hash match): %d\n", cached)
	fmt.Printf("  stale (source changed):     %d\n", stale)
	fmt.Printf("  missing (.en.md absent):    %d\n", missing)
	return nil
}

// discoverSourceMarkdown walks contentDir and returns markdown files matching
// the default language (excludes .en.md / .zh-cn.md sidecars).
//
// v1 stub: returns all .md files that don't have a language suffix.
// PR2 will refine this once i18n build pipeline lands.
func discoverSourceMarkdown(contentDir, defaultLang string) ([]string, error) {
	var files []string
	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if filepath.Ext(name) != ".md" {
			return nil
		}
		// Skip sidecars: foo.en.md / foo.zh-cn.md
		if isSidecar(name, defaultLang) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// isSidecar returns true if filename has a language suffix like .en.md or .zh-cn.md.
func isSidecar(filename, defaultLang string) bool {
	// Strip .md
	base := filename
	if len(base) > 3 && base[len(base)-3:] == ".md" {
		base = base[:len(base)-3]
	}
	// Check for .<lang> suffix
	if base == "" {
		return false
	}
	// Common language codes (2-letter or language-region)
	suffixes := []string{
		".en", ".zh-cn", ".zh-tw", ".ja", ".ko", ".fr", ".de", ".es",
		"." + defaultLang,
	}
	for _, sfx := range suffixes {
		if len(base) > len(sfx) && base[len(base)-len(sfx):] == sfx {
			return true
		}
	}
	return false
}

// filterStaleFiles returns the subset of sourceFiles whose translation is
// stale (source_hash mismatch or .en.md missing or forced).
//
// v1 stub: treats all files as stale when force=true, otherwise returns
// files with missing .en.md only (source_hash comparison comes with PR2).
func filterStaleFiles(files []string, sourceLang, targetLang string, force bool) []string {
	if force {
		return files
	}
	var stale []string
	for _, f := range files {
		sidecar := sidecarPath(f, targetLang)
		if _, err := os.Stat(sidecar); os.IsNotExist(err) {
			stale = append(stale, f)
		}
	}
	return stale
}

// classifyTranslationState returns "cached" / "stale" / "missing" for a source file.
//
// v1 stub: only distinguishes missing vs present; source_hash check lands in PR2.
func classifyTranslationState(srcPath, targetLang string) string {
	sidecar := sidecarPath(srcPath, targetLang)
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		return "missing"
	}
	return "cached"
}

// sidecarPath returns the path to the translated sidecar file.
//
// Example: content/posts/foo.md → content/posts/foo.en.md
func sidecarPath(srcPath, targetLang string) string {
	dir := filepath.Dir(srcPath)
	base := filepath.Base(srcPath)
	// Insert .<lang> before .md
	if len(base) > 3 && base[len(base)-3:] == ".md" {
		base = base[:len(base)-3] + "." + targetLang + ".md"
	}
	return filepath.Join(dir, base)
}

// readSourceMarkdown extracts the title, body, and full frontmatter map
// from a source markdown file.
//
// Uses internal/content.ParseFrontmatter for proper YAML frontmatter parsing
// (handles --- / +++ delimiters, escaped quotes, multi-line values). Title
// comes from frontmatter's `title:` field; falls back to filename basename
// when frontmatter lacks title (rare but tolerated).
//
// Body is the markdown content AFTER frontmatter (no YAML leak into LLM
// prompt). Trailing/leading whitespace is trimmed.
//
// frontmatter is the full parsed map (used by writeTranslatedSidecar to
// mirror date/hidden/draft/slug/tags/keywords/description into the en
// sidecar, with tags translated via the glossary).
func readSourceMarkdown(path string) (title, body string, fm map[string]interface{}, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", nil, err
	}
	fmMap, bodyText, err := content.ParseFrontmatter(data)
	if err != nil {
		return "", "", nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	// Extract title from frontmatter; fall back to filename if absent
	if t, ok := fmMap["title"]; ok {
		switch v := t.(type) {
		case string:
			title = v
		default:
			title = fmt.Sprintf("%v", v)
		}
	}
	if title == "" {
		base := filepath.Base(path)
		title = strings.TrimSuffix(base, ".md")
	}

	return title, bodyText, fmMap, nil
}

// writeTranslatedSidecar writes the translated content as a .en.md sidecar
// with frontmatter metadata per ADR 0008 §5 + frontmatter parity (date /
// hidden / draft / slug / tags / keywords / description mirrored from
// source; tags translated via glossary with fail-fast on missing terms).
func writeTranslatedSidecar(outPath, srcPath, sourceLang, targetLang string, resp *translate.Response, srcFM map[string]interface{}, glossary map[string]string) error {
	// Compute source hash
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read source for hash: %w", err)
	}
	sourceHash := sha256Hex(srcData)

	// Relative translation_of path
	relSrc, err := filepath.Rel(filepath.Join(sourceDir, "content"), srcPath)
	if err != nil {
		relSrc = srcPath
	}

	// Translate tags via glossary (fail-fast on missing).
	translatedTags, err := translateTagList(srcFM, glossary)
	if err != nil {
		return fmt.Errorf("translate tags: %w", err)
	}
	// Keywords share the same dictionary as tags (in zhurongshuo they're
	// usually identical; if not, still translates via same glossary).
	translatedKeywords, err := translateKeywordList(srcFM, glossary)
	if err != nil {
		return fmt.Errorf("translate keywords: %w", err)
	}

	// Build frontmatter. Mirror source fields first (date / hidden / draft /
	// slug / description), then huan-managed metadata. Tags use translated
	// list; if source had no tags field, omit it (true mirror).
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: %q\n", resp.Title))

	// Mirror scalar fields from source frontmatter (skip if absent in source)
	if v, ok := srcFM["date"]; ok {
		// yaml.v3 parses ISO timestamps into time.Time; preserve ISO 8601
		// format (RFC3339) instead of Go's default Time.String() which
		// produces "2006-01-02 15:04:05 -0700 MST".
		if t, isTime := v.(time.Time); isTime {
			b.WriteString(fmt.Sprintf("date: %s\n", t.Format(time.RFC3339)))
		} else {
			b.WriteString(fmt.Sprintf("date: %v\n", v))
		}
	}
	if v, ok := srcFM["hidden"]; ok {
		b.WriteString(fmt.Sprintf("hidden: %v\n", v))
	}
	if v, ok := srcFM["draft"]; ok {
		b.WriteString(fmt.Sprintf("draft: %v\n", v))
	}
	// Translated tags (if source had tags)
	if translatedTags != nil {
		b.WriteString("tags: " + formatYAMLStringList(translatedTags) + "\n")
	}
	// Translated keywords (if source had keywords)
	if translatedKeywords != nil {
		b.WriteString("keywords: " + formatYAMLStringList(translatedKeywords) + "\n")
	}
	// description: only write if source has non-CJK description (no
	// Chinese in .en.md policy). Source Chinese descriptions are dropped.
	if desc, ok := descriptionForSidecar(srcFM); ok {
		b.WriteString(fmt.Sprintf("description: %q\n", desc))
	}
	if v, ok := srcFM["slug"]; ok {
		if s, isStr := v.(string); isStr {
			b.WriteString(fmt.Sprintf("slug: %q\n", s))
		} else {
			b.WriteString(fmt.Sprintf("slug: %v\n", v))
		}
	}

	// huan-managed metadata
	b.WriteString(fmt.Sprintf("translation_of: %s\n", relSrc))
	b.WriteString(fmt.Sprintf("source_lang: %s\n", sourceLang))
	b.WriteString(fmt.Sprintf("target_lang: %s\n", targetLang))
	b.WriteString(fmt.Sprintf("source_hash: %s\n", sourceHash))
	b.WriteString(fmt.Sprintf("model: %s\n", resp.Model))
	b.WriteString(fmt.Sprintf("translated_at: %s\n", utcNow()))
	b.WriteString("translator: qwen3\n")
	b.WriteString("quality_checks:\n")
	b.WriteString(fmt.Sprintf("  xml_parse: %t\n", resp.QualityChecks.XMLParse))
	b.WriteString(fmt.Sprintf("  language_detection: %t\n", resp.QualityChecks.LanguageDetection))
	b.WriteString(fmt.Sprintf("  markdown_structure: %t\n", resp.QualityChecks.MarkdownStructure))
	b.WriteString(fmt.Sprintf("  format_purity: %t\n", resp.QualityChecks.FormatPurity))
	b.WriteString(fmt.Sprintf("  length_ratio: %.4f\n", resp.QualityChecks.LengthRatio))
	b.WriteString(fmt.Sprintf("  glossary_compliance: %t\n", resp.QualityChecks.GlossaryCompliance))
	b.WriteString(fmt.Sprintf("  retry_count: %d\n", resp.QualityChecks.RetryCount))
	b.WriteString(fmt.Sprintf("tokens_used: %d\n", resp.TokensUsed))
	b.WriteString("---\n\n")

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(outPath, []byte(b.String()+resp.Body+"\n"), 0644)
}

// translateTagList translates source frontmatter tags via glossary.
// Returns nil if source has no tags field. Returns error (fail-fast) if
// any tag is missing from glossary — per ADR 0008 §8, partial tag
// translation would fragment taxonomy.
func translateTagList(srcFM map[string]interface{}, glossary map[string]string) ([]string, error) {
	v, ok := srcFM["tags"]
	if !ok {
		return nil, nil
	}
	rawTags, err := toStringList(v)
	if err != nil {
		return nil, fmt.Errorf("tags field: %w", err)
	}
	return translateTerms(rawTags, glossary, "tag")
}

// translateKeywordList translates source frontmatter keywords via glossary.
// Drops any term that has no translation — keeps .en.md sidecar Chinese-free.
// Source keywords are usually 1-3 word tags (covered by glossary) or long
// descriptive phrases (book chapter titles — not in glossary). The latter
// are intentionally dropped rather than mirrored because the en sidecar
// must not contain Chinese (per user policy).
//
// Returns nil if source has no keywords field (so writeTranslatedSidecar
// omits the field entirely). Returns empty slice if source has empty list.
func translateKeywordList(srcFM map[string]interface{}, glossary map[string]string) ([]string, error) {
	v, ok := srcFM["keywords"]
	if !ok {
		return nil, nil
	}
	rawKw, err := toStringList(v)
	if err != nil {
		return nil, fmt.Errorf("keywords field: %w", err)
	}
	out := make([]string, 0, len(rawKw))
	for _, s := range rawKw {
		if en, ok := glossary[s]; ok && en != "" {
			out = append(out, en)
		}
		// Drop untranslated (don't append source term)
	}
	return out, nil
}

// descriptionForSidecar returns the description value to write to .en.md, or
// ("", false) to omit the field. Per user policy, .en.md must not contain
// Chinese. Source descriptions are usually empty (zhurongshuo convention)
// or contain long Chinese prose (book introductions). The latter is dropped
// rather than mirrored — translating them would require an LLM call per
// file (out of scope for tag-mirror feature).
func descriptionForSidecar(srcFM map[string]interface{}) (string, bool) {
	v, ok := srcFM["description"]
	if !ok {
		return "", false
	}
	s, isStr := v.(string)
	if !isStr {
		return "", false
	}
	// Drop if contains any CJK rune (no Chinese in .en.md policy)
	if hasCJK(s) {
		return "", false
	}
	return s, true
}

// hasCJK returns true if s contains any CJK Unified Ideograph rune.
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// translateTerms maps each source term through glossary, fail-fast on miss.
// Used for tags (taxonomy drivers — partial translation would fragment URL
// space). For lenient handling (keywords), see translateKeywordList.
func translateTerms(src []string, glossary map[string]string, kind string) ([]string, error) {
	if len(src) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(src))
	var missing []string
	for _, s := range src {
		if en, ok := glossary[s]; ok && en != "" {
			out = append(out, en)
		} else {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s not in glossary (add to i18n/terms.yaml): %v", kind, missing)
	}
	return out, nil
}

// toStringList coerces a parsed YAML value to []string. Accepts []interface{}
// (standard yaml.v3 output) or []string (already typed).
func toStringList(v interface{}) ([]string, error) {
	switch s := v.(type) {
	case []string:
		return s, nil
	case []interface{}:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			} else {
				out = append(out, fmt.Sprintf("%v", item))
			}
		}
		return out, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("expected list, got %T", v)
	}
}

// formatYAMLStringList formats []string as YAML inline list: ["a", "b", "c"].
// Empty list → "[]".
func formatYAMLStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		// YAML double-quoted string with escape
		b.WriteString("\"")
		b.WriteString(strings.ReplaceAll(s, "\"", "\\\""))
		b.WriteString("\"")
	}
	b.WriteString("]")
	return b.String()
}

