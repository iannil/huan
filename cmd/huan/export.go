package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/i18n"
	"github.com/iannil/huan/pkg/plugin"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export content (default: posts CSV archive; 'ebook' for ebook formats)",
		Long: `Bare ` + "`huan export`" + ` walks content/posts/ for .md files, extracts
frontmatter dates and cleaned body text, and writes a date-sorted
(RFC 4180, UTF-8 BOM) CSV to developer/posts-YYYYMMDD.csv by default.
Use --output-dir and --filename-prefix to choose the archive location and name.
Prior CSV archives with the same literal prefix and an eight-digit date are removed.

Subcommands: csv (same as bare), ebook (offline document formats via an
exporter plugin).`,
		Args: cobra.NoArgs,
		RunE: runExport,
	}
	addExportCSVFlags(cmd)
	cmd.AddCommand(newExportCsvCmd())
	cmd.AddCommand(newExportEbookCmd())
	return cmd
}

func newExportCsvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "csv",
		Short: "Export content/posts/*.md to a CSV archive",
		Long: `Walk content/posts/ for .md files, extract frontmatter dates and
cleaned body text, and write a date-sorted (RFC 4180, UTF-8 BOM) CSV to
developer/posts-YYYYMMDD.csv by default.

Use --output-dir and --filename-prefix to choose the archive location and name.
Prior CSV archives with the same literal prefix and an eight-digit date are removed.`,
		Args: cobra.NoArgs,
		RunE: runExport,
	}
	addExportCSVFlags(cmd)
	return cmd
}

func addExportCSVFlags(cmd *cobra.Command) {
	// Local flags: ebook exports have their own output contract.
	cmd.Flags().String("output-dir", "developer", "CSV output directory (relative to source, or absolute)")
	cmd.Flags().String("filename-prefix", "posts-", "literal CSV filename prefix (non-empty, without path separators)")
}

func newExportEbookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ebook",
		Short: "Export books/practices to ebook formats (epub/pdf/docx) via an exporter plugin",
		Args:  cobra.NoArgs,
		RunE:  runExportEbook,
	}
	cmd.Flags().String("type", "all", "content type filter: books, practices, all")
	cmd.Flags().String("format", "all", "output format filter: epub, pdf, docx, all")
	cmd.Flags().String("level", "all", "granularity filter: individual, volumes (alias: seasons), complete, all")
	cmd.Flags().String("slug", "", "restrict to a single book/practice slug")
	cmd.Flags().Int("volume", 0, "restrict to one volume/season number, 1-based (0 = all)")
	cmd.Flags().Bool("force", false, "regenerate even when the manifest hash matches")
	cmd.Flags().Int("jobs", 0, "parallelism for per-book generation (0 = NumCPU-1)")
	cmd.Flags().String("plugin-dir", "", "directory containing .so plugin files (default: <source>/plugins)")
	return cmd
}

// normalizeExportLevel maps CLI level aliases to contract values
// ("seasons" and "volumes" are the same granularity).
func normalizeExportLevel(level string) string {
	if level == "seasons" {
		return "volumes"
	}
	return level
}

func runExportEbook(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Two-stage registry pattern (same as deploy): compiled-in plugins first,
	// then huan.yaml-declared dynamic plugins, then diagnose the gap.
	registry, err := newPluginRegistry(cfg, sourceDir, "")
	if err != nil {
		return fmt.Errorf("plugin registry: %w", err)
	}
	exporters := plugin.Find[plugin.Exporter](registry)
	if len(exporters) == 0 {
		pluginDir, _ := cmd.Flags().GetString("plugin-dir")
		if pluginDir == "" {
			pluginDir = pluginDirFromSource(sourceDir)
		}
		loadConfiguredPlugins(registry, pluginDir, sourceDir, cfg.Plugins)
		exporters = plugin.Find[plugin.Exporter](registry)
	}
	if len(exporters) == 0 {
		if hint := diagnoseCapabilityGap(registry, "plugin.Exporter"); hint != "" {
			return fmt.Errorf("no exporter plugin available: %s", hint)
		}
		return fmt.Errorf("no exporter plugin available (declare an exporter plugin under huan.yaml plugins)")
	}

	exporter := exporters[0]

	typ, _ := cmd.Flags().GetString("type")
	format, _ := cmd.Flags().GetString("format")
	level := normalizeExportLevel(mustStringFlag(cmd, "level"))
	slug, _ := cmd.Flags().GetString("slug")
	volume, _ := cmd.Flags().GetInt("volume")
	force, _ := cmd.Flags().GetBool("force")
	jobs, _ := cmd.Flags().GetInt("jobs")

	req := plugin.ExportRequest{
		SourceDir: sourceDir,
		Type:      typ,
		Format:    format,
		Level:     level,
		Slug:      slug,
		Volume:    volume,
		Force:     force,
		Jobs:      jobs,
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := exporter.Export(ctx, req)
	if err != nil {
		return err
	}

	fmt.Printf("export ebook: %d ok, %d failed, %d skipped, %d warnings\n",
		len(res.Succeeded), len(res.Failed), len(res.Skipped), len(res.Warnings))
	for _, f := range res.Failed {
		fmt.Printf("  failed: %s: %s\n", f.Item.Path, f.Err)
	}
	for _, w := range res.Warnings {
		fmt.Printf("  warning: %s\n", w)
	}

	if len(res.Failed) > 0 {
		return fmt.Errorf("%d item(s) failed", len(res.Failed))
	}
	return nil
}

// mustStringFlag fetches a string flag that is known to exist; it exists to
// keep flag reads terse without discarding errors silently.
func mustStringFlag(cmd *cobra.Command, name string) string {
	v, err := cmd.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return v
}

func runExport(cmd *cobra.Command, args []string) error {
	prefix := mustStringFlag(cmd, "filename-prefix")
	if strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, "/\\\x00") {
		return fmt.Errorf("filename-prefix must be non-empty and contain no path separators or NUL")
	}
	postsDir := filepath.Join(sourceDir, "content", "posts")
	outputDir := mustStringFlag(cmd, "output-dir")
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(sourceDir, outputDir)
	}

	if _, err := os.Stat(postsDir); os.IsNotExist(err) {
		return fmt.Errorf("posts directory does not exist: %s", postsDir)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	rows, err := collectPostRows(postsDir)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("export: no eligible posts found")
		return nil
	}

	// Match bash `sort -r` under zh_CN.UTF-8: reverse locale-aware order on
	// the full joined line (date + body), so posts sharing a date tiebreak by
	// body in pinyin order.
	coll := i18n.BuildCollator("zh-cn")
	sort.SliceStable(rows, func(i, j int) bool {
		return coll.CompareString(
			strings.Join(rows[i][:], ","),
			strings.Join(rows[j][:], ","),
		) > 0
	})

	csvName := prefix + time.Now().Format("20060102") + ".csv"
	csvPath := filepath.Join(outputDir, csvName)
	if err := writeCSV(csvPath, rows); err != nil {
		return err
	}

	removed := cleanupOldExports(outputDir, prefix, csvName)
	fmt.Printf("export: %d posts → %s (removed %d old CSVs)\n", len(rows), csvPath, removed)
	return nil
}

type postRow struct {
	date string
	body string
}

var (
	moreBlockRE  = regexp.MustCompile(`<!--more-->`)
	listStarRE   = regexp.MustCompile(`\*`)
	quoteRE      = regexp.MustCompile(`> `)
	whitespaceRE = regexp.MustCompile(`\s+`)
)

// collectPostRows walks postsDir for .md files and returns (date, last-paragraph)
// pairs after the same cleanup export.sh applies: drop newlines, strip
// markdown list/quote markers, drop <!--more-->, collapse whitespace.
func collectPostRows(postsDir string) ([][2]string, error) {
	var rows []postRow
	walkErr := filepath.Walk(postsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fm, body, err := content.ParseFrontmatter(data)
		if err != nil {
			return nil
		}
		date, _ := fm["date"].(string)
		if date == "" {
			if t, ok := fm["date"].(time.Time); ok {
				date = t.Format(time.RFC3339)
			}
		}
		if date == "" {
			return nil
		}
		para := lastParagraph(body)
		para = cleanParagraph(para)
		if para == "" {
			return nil
		}
		rows = append(rows, postRow{date: date, body: para})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	out := make([][2]string, len(rows))
	for i, r := range rows {
		out[i] = [2]string{r.date, r.body}
	}
	return out, nil
}

// lastParagraph returns the entire body with newlines collapsed, matching
// the bash export.sh behavior of `tr -d '\n\r'` on the body block. Despite
// the name, it is the full body, not just the final paragraph.
func lastParagraph(body string) string {
	return strings.TrimSpace(body)
}

func cleanParagraph(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = quoteRE.ReplaceAllString(s, "")
	s = listStarRE.ReplaceAllString(s, "")
	s = moreBlockRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// writeCSV writes rows as RFC 4180 CSV with UTF-8 BOM prefix.
func writeCSV(path string, rows [][2]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	w := csv.NewWriter(f)
	for _, r := range rows {
		if err := w.Write([]string{r[0], r[1]}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// cleanupOldExports deletes only regular prefix+YYYYMMDD.csv archives except current.
// Quote the prefix so user-supplied metacharacters cannot widen the cleanup scope.
func cleanupOldExports(dir, prefix, currentName string) int {
	if strings.TrimSpace(prefix) == "" {
		return 0
	}
	pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `[0-9]{8}\.csv$`)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if entry.Name() == currentName || !pattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
			removed++
		}
	}
	return removed
}
