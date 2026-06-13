package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/iannil/huan/internal/content"
	"github.com/iannil/huan/internal/i18n"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export content/posts/*.md to a CSV archive in developer/",
		Long: `Walk content/posts/ for .md files, extract the frontmatter date and
the last body paragraph, and write the result as a date-sorted
(RFC 4180, UTF-8 BOM) CSV to developer/祝融说_副本YYYYMMDD.csv.

Old CSVs in developer/ matching the same prefix are removed so only the
latest is kept.`,
		Args: cobra.NoArgs,
		RunE: runExport,
	}
}

func runExport(cmd *cobra.Command, args []string) error {
	postsDir := filepath.Join(sourceDir, "content", "posts")
	developerDir := filepath.Join(sourceDir, "developer")

	if _, err := os.Stat(postsDir); os.IsNotExist(err) {
		return fmt.Errorf("posts directory does not exist: %s", postsDir)
	}
	if err := os.MkdirAll(developerDir, 0o755); err != nil {
		return fmt.Errorf("create developer dir: %w", err)
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

	csvName := fmt.Sprintf("祝融说_副本%s.csv", time.Now().Format("20060102"))
	csvPath := filepath.Join(developerDir, csvName)
	if err := writeCSV(csvPath, rows); err != nil {
		return err
	}

	removed := cleanupOldExports(developerDir, csvName)
	fmt.Printf("export: %d posts → %s (removed %d old CSVs)\n", len(rows), csvPath, removed)
	return nil
}

type postRow struct {
	date string
	body string
}

var (
	moreBlockRE = regexp.MustCompile(`<!--more-->`)
	listStarRE  = regexp.MustCompile(`\*`)
	quoteRE     = regexp.MustCompile(`> `)
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

// cleanupOldExports deletes prior 祝融说_副本*.csv files in dir except current.
// Returns the number of files removed.
func cleanupOldExports(dir, currentName string) int {
	pattern := "祝融说_副本*.csv"
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return 0
	}
	removed := 0
	for _, m := range matches {
		if filepath.Base(m) == currentName {
			continue
		}
		if err := os.Remove(m); err == nil {
			removed++
		}
	}
	return removed
}
