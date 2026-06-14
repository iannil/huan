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

		title, body, err := readSourceMarkdown(srcPath)
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
		if err := writeTranslatedSidecar(outPath, srcPath, sourceLang, targetLang, resp); err != nil {
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

// readSourceMarkdown extracts the title and body from a source markdown file.
//
// Uses internal/content.ParseFrontmatter for proper YAML frontmatter parsing
// (handles --- / +++ delimiters, escaped quotes, multi-line values). Title
// comes from frontmatter's `title:` field; falls back to filename basename
// when frontmatter lacks title (rare but tolerated).
//
// Body is the markdown content AFTER frontmatter (no YAML leak into LLM
// prompt). Trailing/leading whitespace is trimmed.
func readSourceMarkdown(path string) (title, body string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	fm, bodyText, err := content.ParseFrontmatter(data)
	if err != nil {
		return "", "", fmt.Errorf("parse frontmatter: %w", err)
	}

	// Extract title from frontmatter; fall back to filename if absent
	if t, ok := fm["title"]; ok {
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

	return title, bodyText, nil
}

// writeTranslatedSidecar writes the translated content as a .en.md sidecar
// with frontmatter metadata per ADR 0008 §5.
func writeTranslatedSidecar(outPath, srcPath, sourceLang, targetLang string, resp *translate.Response) error {
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

	// Build frontmatter + body. Include translated title so build pipeline
	// can render the en page with proper page title (otherwise title falls
	// back to filename-based placeholder).
	frontmatter := fmt.Sprintf(`---
title: %q
translation_of: %s
source_lang: %s
target_lang: %s
source_hash: %s
model: %s
translated_at: %s
translator: qwen3
quality_checks:
  xml_parse: %t
  language_detection: %t
  markdown_structure: %t
  format_purity: %t
  length_ratio: %.4f
  glossary_compliance: %t
  retry_count: %d
tokens_used: %d
---

`,
		resp.Title,
		relSrc,
		sourceLang,
		targetLang,
		sourceHash,
		resp.Model,
		utcNow(),
		resp.QualityChecks.XMLParse,
		resp.QualityChecks.LanguageDetection,
		resp.QualityChecks.MarkdownStructure,
		resp.QualityChecks.FormatPurity,
		resp.QualityChecks.LengthRatio,
		resp.QualityChecks.GlossaryCompliance,
		resp.QualityChecks.RetryCount,
		resp.TokensUsed,
	)

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(outPath, []byte(frontmatter+resp.Body+"\n"), 0644)
}

