package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iannil/huan/internal/i18n/audit"
	"github.com/spf13/cobra"
)

func newTranslateAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit zh/en parity and per-page language correctness against a running `huan serve`",
		Long: "Audit crawls a running dev server (huan serve), enumerates the zh and en sitemaps, " +
			"and reports (1) parity: source pages missing an .en.md or orphan sidecars, and " +
			"(2) language correctness: English pages containing untranslated Chinese, or Chinese " +
			"pages that look like English. Writes a Markdown report. Read-only: never modifies content.",
		Args: cobra.NoArgs,
		RunE: runTranslateAudit,
	}
	cmd.Flags().String("base-url", "http://localhost:1313", "base URL of a running `huan serve`")
	cmd.Flags().String("allow", "i18n/audit-allow.txt", "glob allowlist file (relative to source dir) exempting refs from findings; ignored if absent")
	cmd.Flags().String("report", "", "report output path (default: docs/reports/i18n-audit-<date>.md under source dir)")
	cmd.Flags().Float64("cjk-threshold", 0.2, "max tolerated CJK fraction in an English page's prose before flagging")
	cmd.Flags().Int("concurrency", 8, "number of concurrent page fetches")
	cmd.Flags().Bool("fail", false, "exit non-zero when any (non-exempt) finding is reported")
	return cmd
}

func runTranslateAudit(cmd *cobra.Command, args []string) error {
	baseURL, _ := cmd.Flags().GetString("base-url")
	allowFlag, _ := cmd.Flags().GetString("allow")
	reportFlag, _ := cmd.Flags().GetString("report")
	cjkThreshold, _ := cmd.Flags().GetFloat64("cjk-threshold")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	failOnFindings, _ := cmd.Flags().GetBool("fail")

	baseURL = strings.TrimRight(baseURL, "/")
	contentDir := filepath.Join(sourceDir, "content")

	// 1. Parity from source files (the translation pipeline's own definition):
	//    a source .md with non-empty body should have a .en.md sidecar.
	sources, orphans, err := scanSourceParity(contentDir)
	if err != nil {
		return fmt.Errorf("scan source parity: %w", err)
	}
	var findings []audit.Finding
	findings = append(findings, audit.ComputeMissingEN(sources)...)
	findings = append(findings, audit.ComputeOrphanEN(orphans)...)

	// 2. Language correctness from rendered HTML served by `huan serve`.
	client := &http.Client{Timeout: 30 * time.Second}
	zhURLs, err := fetchSitemapLocs(client, baseURL+"/sitemap.xml")
	if err != nil {
		return fmt.Errorf("fetch zh sitemap (is `huan serve` running at %s?): %w", baseURL, err)
	}
	enURLs, err := fetchSitemapLocs(client, baseURL+"/en/sitemap.xml")
	if err != nil {
		return fmt.Errorf("fetch en sitemap: %w", err)
	}
	fmt.Fprintf(os.Stderr, "audit: %d zh pages, %d en pages from sitemaps\n", len(zhURLs), len(enURLs))

	langFindings := checkPagesLanguage(client, zhURLs, enURLs, cjkThreshold, concurrency)
	findings = append(findings, langFindings...)

	// 3. Apply allowlist: split findings into active vs exempted.
	patterns := loadAllowlist(filepath.Join(sourceDir, allowFlag))
	active, exempted := splitByAllowlist(findings, patterns)

	// 4. Write report.
	reportPath := reportFlag
	if reportPath == "" {
		reportPath = filepath.Join(sourceDir, "docs", "reports",
			fmt.Sprintf("i18n-audit-%s.md", time.Now().Format("2006-01-02")))
	}
	if err := writeAuditReport(reportPath, baseURL, len(zhURLs), len(enURLs), active, exempted); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	// 5. Summarize to stdout.
	printAuditSummary(active, exempted, reportPath)

	if failOnFindings && len(active) > 0 {
		return fmt.Errorf("audit found %d issue(s)", len(active))
	}
	return nil
}

// scanSourceParity walks contentDir and returns the source-file parity state
// plus orphan .en.md paths (sidecars whose base source is missing). Reuses the
// translate command's discovery/sidecar helpers and empty-body convention.
func scanSourceParity(contentDir string) (sources []audit.SourceEntry, orphans []string, err error) {
	srcFiles, err := discoverSourceMarkdown(contentDir, "zh-cn")
	if err != nil {
		return nil, nil, err
	}
	for _, srcPath := range srcFiles {
		rel, _ := filepath.Rel(contentDir, srcPath)
		_, body, _, rerr := readSourceMarkdown(srcPath)
		hasBody := rerr == nil && strings.TrimSpace(body) != ""
		_, statErr := os.Stat(sidecarPath(srcPath, "en"))
		sources = append(sources, audit.SourceEntry{
			RelPath: filepath.ToSlash(rel),
			HasBody: hasBody,
			HasEN:   statErr == nil,
		})
	}

	// Orphan detection: walk *.en.md and check the base source exists.
	_ = filepath.Walk(contentDir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() {
			return werr
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".en.md") {
			return nil
		}
		base := strings.TrimSuffix(p, ".en.md") + ".md"
		if _, e := os.Stat(base); os.IsNotExist(e) {
			rel, _ := filepath.Rel(contentDir, p)
			orphans = append(orphans, filepath.ToSlash(rel))
		}
		return nil
	})
	return sources, orphans, nil
}

// sitemapXML mirrors the <urlset><url><loc> structure.
type sitemapXML struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

func fetchSitemapLocs(client *http.Client, url string) ([]string, error) {
	body, status, err := httpGet(client, url)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, status)
	}
	var sm sitemapXML
	if err := xml.Unmarshal(body, &sm); err != nil {
		return nil, fmt.Errorf("parse sitemap %s: %w", url, err)
	}
	out := make([]string, 0, len(sm.URLs))
	for _, u := range sm.URLs {
		loc := strings.TrimSpace(u.Loc)
		if loc != "" {
			out = append(out, loc)
		}
	}
	return out, nil
}

// checkPagesLanguage fetches each page concurrently and runs the language
// check appropriate to its sitemap origin (en sitemap → English check).
func checkPagesLanguage(client *http.Client, zhURLs, enURLs []string, cjkThreshold float64, concurrency int) []audit.Finding {
	type job struct {
		url     string
		english bool
	}
	jobs := make([]job, 0, len(zhURLs)+len(enURLs))
	for _, u := range enURLs {
		jobs = append(jobs, job{url: u, english: true})
	}
	for _, u := range zhURLs {
		jobs = append(jobs, job{url: u, english: false})
	}

	if concurrency < 1 {
		concurrency = 1
	}
	var (
		mu       sync.Mutex
		findings []audit.Finding
		wg       sync.WaitGroup
		sem      = make(chan struct{}, concurrency)
	)
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			body, status, err := httpGet(client, j.url)
			ref := urlToRef(j.url)
			if err != nil || status != http.StatusOK {
				mu.Lock()
				findings = append(findings, audit.Finding{
					Ref: ref, Kind: "FetchError",
					Evidence: fmt.Sprintf("status=%d err=%v", status, err),
				})
				mu.Unlock()
				return
			}
			page, perr := audit.Parse(strings.NewReader(string(body)))
			if perr != nil {
				return
			}
			var f *audit.Finding
			if j.english {
				f = audit.CheckEnglish(ref, page.Title+" "+page.Prose, cjkThreshold)
			} else {
				f = audit.CheckChinese(ref, page.Title+" "+page.Prose)
			}
			if f != nil {
				mu.Lock()
				findings = append(findings, *f)
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
	sort.Slice(findings, func(i, j int) bool { return findings[i].Ref < findings[j].Ref })
	return findings
}

func httpGet(client *http.Client, url string) ([]byte, int, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

// urlToRef reduces an absolute URL to its path for display.
func urlToRef(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[slash:]
		}
		return "/"
	}
	return u
}

func loadAllowlist(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// splitByAllowlist partitions findings into active and exempted. A finding is
// exempted when its Ref matches any glob pattern (filepath.Match), tried both
// as-is and with a leading "/en" stripped so one pattern covers both languages.
func splitByAllowlist(findings []audit.Finding, patterns []string) (active, exempted []audit.Finding) {
	for _, f := range findings {
		if matchesAny(f.Ref, patterns) {
			exempted = append(exempted, f)
		} else {
			active = append(active, f)
		}
	}
	return active, exempted
}

func matchesAny(ref string, patterns []string) bool {
	candidates := []string{
		ref,
		strings.TrimPrefix(ref, "/"),
		strings.TrimPrefix(strings.TrimPrefix(ref, "/en"), "/"),
	}
	for _, pat := range patterns {
		for _, c := range candidates {
			if ok, _ := path.Match(pat, c); ok {
				return true
			}
		}
	}
	return false
}

// writeAuditReport renders the Markdown report grouped by top-level section.
func writeAuditReport(reportPath, baseURL string, zhCount, enCount int, active, exempted []audit.Finding) error {
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, f := range active {
		counts[f.Kind]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# i18n 页面审计报告\n\n")
	fmt.Fprintf(&b, "- 生成时间：%s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "- 服务地址：%s\n", baseURL)
	fmt.Fprintf(&b, "- 页面规模：zh %d 页，en %d 页\n\n", zhCount, enCount)

	fmt.Fprintf(&b, "## 汇总\n\n")
	fmt.Fprintf(&b, "| 类型 | 数量 | 含义 |\n|---|---|---|\n")
	fmt.Fprintf(&b, "| MissingEN | %d | 中文源页有正文但缺 .en.md |\n", counts[audit.KindMissingEN])
	fmt.Fprintf(&b, "| OrphanEN | %d | .en.md 无对应中文源 |\n", counts[audit.KindOrphanEN])
	fmt.Fprintf(&b, "| EnglishHasChinese | %d | 英文页正文残留中文 |\n", counts[audit.KindEnglishHasChinese])
	fmt.Fprintf(&b, "| ChineseLooksEnglish | %d | 中文页正文疑似英文 |\n", counts[audit.KindChineseLooksEnglish])
	if counts["FetchError"] > 0 {
		fmt.Fprintf(&b, "| FetchError | %d | 抓取失败（404/超时等）|\n", counts["FetchError"])
	}
	fmt.Fprintf(&b, "| 已豁免 | %d | 命中 allowlist，不计违规 |\n\n", len(exempted))

	writeFindingsSection(&b, "## 违规明细", active)
	if len(exempted) > 0 {
		writeFindingsSection(&b, "## 已豁免（allowlist 命中，供复核）", exempted)
	}

	if len(active) == 0 {
		fmt.Fprintf(&b, "\n**未发现违规。**\n")
	}
	return os.WriteFile(reportPath, []byte(b.String()), 0644)
}

// writeFindingsSection groups findings by top-level section (first path
// segment of the Ref) and emits a table per group.
func writeFindingsSection(b *strings.Builder, heading string, findings []audit.Finding) {
	fmt.Fprintf(b, "\n%s\n\n", heading)
	if len(findings) == 0 {
		fmt.Fprintf(b, "（无）\n")
		return
	}
	groups := map[string][]audit.Finding{}
	var order []string
	for _, f := range findings {
		g := topSegment(f.Ref)
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], f)
	}
	sort.Strings(order)
	for _, g := range order {
		fmt.Fprintf(b, "### %s（%d）\n\n", g, len(groups[g]))
		fmt.Fprintf(b, "| Ref | 类型 | 证据 |\n|---|---|---|\n")
		for _, f := range groups[g] {
			fmt.Fprintf(b, "| %s | %s | %s |\n",
				mdEscape(f.Ref), f.Kind, mdEscape(f.Evidence))
		}
		fmt.Fprintf(b, "\n")
	}
}

// topSegment returns the first path segment of a ref for grouping, e.g.
// "/en/products/foo/" → "products", "books/c.md" → "books".
func topSegment(ref string) string {
	r := strings.TrimPrefix(ref, "/")
	r = strings.TrimPrefix(r, "en/")
	if i := strings.IndexByte(r, '/'); i >= 0 {
		return r[:i]
	}
	if r == "" {
		return "(root)"
	}
	return r
}

func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func printAuditSummary(active, exempted []audit.Finding, reportPath string) {
	counts := map[string]int{}
	for _, f := range active {
		counts[f.Kind]++
	}
	fmt.Printf("i18n audit complete:\n")
	fmt.Printf("  MissingEN:           %d\n", counts[audit.KindMissingEN])
	fmt.Printf("  OrphanEN:            %d\n", counts[audit.KindOrphanEN])
	fmt.Printf("  EnglishHasChinese:   %d\n", counts[audit.KindEnglishHasChinese])
	fmt.Printf("  ChineseLooksEnglish: %d\n", counts[audit.KindChineseLooksEnglish])
	if counts["FetchError"] > 0 {
		fmt.Printf("  FetchError:          %d\n", counts["FetchError"])
	}
	fmt.Printf("  exempted (allowlist): %d\n", len(exempted))
	fmt.Printf("  report: %s\n", reportPath)
}
