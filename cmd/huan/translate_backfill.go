package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/spf13/cobra"
)

// newTranslateBackfillCmd builds `huan translate backfill`.
//
// It targets exactly one situation: a source markdown (e.g. foo.md) and its
// translation sidecar (foo.en.md) both exist, but the sidecar's frontmatter
// lacks source_hash — typically a hand-authored translation, or one produced
// by an older huan that predates the source_hash field. Such sidecars are
// flagged as "missing source_hash" by the strict i18n build check and fail CI.
//
// Backfill computes the current source hash and writes the huan-managed sidecar
// metadata (translation_of / source_lang / target_lang / source_hash) into the
// sidecar's frontmatter WITHOUT touching its body or existing fields.
//
// It deliberately does NOT touch:
//   - sidecars that already have a source_hash (those are handled by the stale
//     check / `huan translate qwen3`);
//   - sources whose sidecar is absent (that is a "missing translation", not a
//     missing hash);
//   - frontmatter-only / empty-body sources (mirrors the translator's own skip).
func newTranslateBackfillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill source_hash + sidecar metadata for existing translations that lack it",
		Long: "Backfill stamps huan-managed metadata (translation_of / source_lang / " +
			"target_lang / source_hash) onto translation sidecars that already exist but " +
			"lack source_hash. The sidecar body and existing frontmatter fields are " +
			"preserved. Sources without a sidecar, sidecars that already have source_hash, " +
			"and empty-body sources are skipped. Use --dry-run to preview.",
		Args: cobra.NoArgs,
		RunE: runTranslateBackfill,
	}
	cmd.Flags().Bool("dry-run", false, "list sidecars that would be backfilled without writing")
	cmd.Flags().String("source-lang", "zh-cn", "source language code")
	cmd.Flags().String("target-lang", "en", "target language code")
	return cmd
}

func runTranslateBackfill(cmd *cobra.Command, args []string) error {
	if _, err := config.Load(sourceDir); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	sourceLang, _ := cmd.Flags().GetString("source-lang")
	targetLang, _ := cmd.Flags().GetString("target-lang")

	contentDir := filepath.Join(sourceDir, "content")
	srcFiles, err := discoverSourceMarkdown(contentDir, sourceLang)
	if err != nil {
		return fmt.Errorf("discover source files: %w", err)
	}

	var backfilled, skippedHasHash, skippedNoSidecar, skippedEmpty int
	for _, srcPath := range srcFiles {
		srcData, err := os.ReadFile(srcPath)
		if err != nil {
			continue
		}

		// Mirror the translator's skip rule: frontmatter-only / empty-body
		// sources never receive a sidecar hash.
		if _, body, perr := content.ParseFrontmatter(srcData); perr == nil &&
			strings.TrimSpace(body) == "" {
			skippedEmpty++
			continue
		}

		sidecar := sidecarPath(srcPath, targetLang)
		sidecarData, err := os.ReadFile(sidecar)
		if err != nil {
			skippedNoSidecar++ // sidecar absent: a missing translation, not our job
			continue
		}

		relSrc, err := filepath.Rel(contentDir, srcPath)
		if err != nil {
			relSrc = srcPath
		}

		newContent, changed, berr := backfillSidecarContent(
			srcData, string(sidecarData), relSrc, sourceLang, targetLang)
		if berr != nil {
			fmt.Printf("  WARN: cannot parse %s: %v\n", sidecar, berr)
			continue
		}
		if !changed {
			skippedHasHash++
			continue
		}

		rel, _ := filepath.Rel(contentDir, sidecar)
		if dryRun {
			fmt.Printf("  WOULD backfill: %s\n", rel)
		} else {
			if werr := os.WriteFile(sidecar, []byte(newContent), 0644); werr != nil {
				return fmt.Errorf("write %s: %w", sidecar, werr)
			}
			fmt.Printf("  backfilled: %s\n", rel)
		}
		backfilled++
	}

	fmt.Printf("Backfill (target: %s)\n", targetLang)
	fmt.Printf("  backfilled:           %d\n", backfilled)
	fmt.Printf("  skipped (has hash):   %d\n", skippedHasHash)
	fmt.Printf("  skipped (no sidecar): %d\n", skippedNoSidecar)
	fmt.Printf("  skipped (empty src):  %d\n", skippedEmpty)
	if dryRun {
		fmt.Printf("  (dry-run: no files written)\n")
	}
	return nil
}

// backfillSidecarContent returns the sidecar content with huan-managed metadata
// stamped into its frontmatter. The source_hash is sha256 of the full source
// file bytes (identical to writeTranslatedSidecar and the build-side stale
// check, so a backfilled sidecar reads as "current" until the source changes).
//
// changed is false (and content returned unchanged) when the sidecar already
// has a non-empty source_hash. Only fields that are absent/empty are added;
// existing frontmatter fields and the body are preserved verbatim.
func backfillSidecarContent(srcData []byte, sidecar, translationOf, sourceLang, targetLang string) (string, bool, error) {
	fm, _, err := content.ParseFrontmatter([]byte(sidecar))
	if err != nil {
		return "", false, err
	}
	if fmHasNonEmptyString(fm, "source_hash") {
		return sidecar, false, nil
	}

	hash := sha256Hex(srcData)

	// Add only the managed fields that are missing; source_hash is always
	// added here (we returned early above if it was present).
	var adds []string
	if !fmHasNonEmptyString(fm, "translation_of") {
		adds = append(adds, fmt.Sprintf("translation_of: %s", translationOf))
	}
	if !fmHasNonEmptyString(fm, "source_lang") {
		adds = append(adds, fmt.Sprintf("source_lang: %s", sourceLang))
	}
	if !fmHasNonEmptyString(fm, "target_lang") {
		adds = append(adds, fmt.Sprintf("target_lang: %s", targetLang))
	}
	adds = append(adds, fmt.Sprintf("source_hash: %s", hash))
	addText := strings.Join(adds, "\n")

	fmText, rest, ok := splitFrontmatter(sidecar)
	if !ok {
		// Sidecar has no frontmatter: wrap the whole content as the body
		// under a fresh frontmatter block.
		return "---\n" + addText + "\n---\n\n" + sidecar, true, nil
	}
	return "---\n" + fmText + "\n" + addText + "\n---" + rest, true, nil
}

// fmHasNonEmptyString reports whether key exists in fm as a non-blank string.
func fmHasNonEmptyString(fm map[string]interface{}, key string) bool {
	v, ok := fm[key]
	if !ok {
		return false
	}
	s, _ := v.(string)
	return strings.TrimSpace(s) != ""
}

// splitFrontmatter splits content that begins with a "---\n ... \n---" YAML
// frontmatter block into the frontmatter body (without surrounding delimiters
// or their adjacent newlines) and the remainder (everything after the closing
// "---", typically starting with "\n\n" then the markdown body).
//
// Reconstruction is exact: content == "---\n" + fm + "\n---" + rest.
// Returns ok=false when no leading frontmatter block is found.
func splitFrontmatter(s string) (fm, rest string, ok bool) {
	if !strings.HasPrefix(s, "---\n") {
		return "", "", false
	}
	after := s[len("---"):] // starts with "\n"
	idx := strings.Index(after, "\n---")
	if idx < 0 {
		return "", "", false
	}
	fm = after[1:idx]        // frontmatter body, no trailing newline
	rest = after[idx+len("\n---"):] // everything after the closing "---"
	return fm, rest, true
}
