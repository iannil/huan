package content

import (
	"testing"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/i18n"
)

// helper: build a page with title + date + weight for sort tests.
func newSortPage(title string, weight int, date time.Time, relPath string) *Page {
	return &Page{
		Title:      title,
		Weight:     weight,
		DateParsed: date,
		RelPath:    relPath,
	}
}

// helper: extract titles from sorted pages.
func pageTitles(pages []*Page) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.Title
	}
	return out
}

func TestSortPagesDefault_CJKPinyinOrder(t *testing.T) {
	// zh-cn pinyin order for 第一章..第七章:
	// 二(èr) < 六(liù) < 七(qī) < 三(sān) < 四(sì) < 五(wǔ) < 一(yī)
	d := time.Date(2025, 10, 14, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newSortPage("第一章 ...", 0, d, "/chapter-01.md"),
		newSortPage("第二章 ...", 0, d, "/chapter-02.md"),
		newSortPage("第三章 ...", 0, d, "/chapter-03.md"),
		newSortPage("第四章 ...", 0, d, "/chapter-04.md"),
		newSortPage("第五章 ...", 0, d, "/chapter-05.md"),
		newSortPage("第六章 ...", 0, d, "/chapter-06.md"),
		newSortPage("第七章 ...", 0, d, "/chapter-07.md"),
	}
	sortPagesDefault(pages, i18n.BuildCollator("zh-cn"))

	got := pageTitles(pages)
	want := []string{
		"第二章 ...", // èr
		"第六章 ...", // liù
		"第七章 ...", // qī
		"第三章 ...", // sān
		"第四章 ...", // sì
		"第五章 ...", // wǔ
		"第一章 ...", // yī
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d want %d (got: %v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("CJK pinyin pos %d: got %v, want %v", i, got, want)
			break
		}
	}
}

func TestSortPagesDefault_DateDescTakesPrecedence(t *testing.T) {
	d1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newSortPage("Same", 0, d2, "/old.md"),
		newSortPage("Same", 0, d1, "/new.md"),
	}
	sortPagesDefault(pages, i18n.BuildCollator("zh-cn"))
	if pages[0].RelPath != "/new.md" {
		t.Errorf("newer must come first; got %v", pages[0].RelPath)
	}
}

func TestSortPagesDefault_WeightNonZeroBeforeZero(t *testing.T) {
	// Hugo: weight 0 sorts last; weight N sorts before weight 0; among
	// non-zero, smaller weight first.
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newSortPage("A", 0, d, "/a.md"),
		newSortPage("B", 5, d, "/b.md"),
		newSortPage("C", 10, d, "/c.md"),
	}
	sortPagesDefault(pages, i18n.BuildCollator("zh-cn"))
	got := pageTitles(pages)
	want := []string{"B", "C", "A"} // weight 5, weight 10, weight 0 last
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("weight pos %d: got %q want %q (full: %v)", i, got[i], w, got)
		}
	}
}

func TestSortPagesDefault_BothWeightZeroFallsBackToTitleThenPath(t *testing.T) {
	// All weight 0, same date → Collator Title asc → Path asc
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newSortPage("Banana", 0, d, "/b.md"),
		newSortPage("Apple", 0, d, "/c.md"),
		newSortPage("Apple", 0, d, "/a.md"), // same title as above, different path
	}
	sortPagesDefault(pages, i18n.BuildCollator("en"))
	got := pages[0].Title + ":" + pages[0].RelPath + ", " + pages[1].Title + ":" + pages[1].RelPath + ", " + pages[2].Title + ":" + pages[2].RelPath
	want := "Apple:/a.md, Apple:/c.md, Banana:/b.md"
	if got != want {
		t.Errorf("fallback chain: got %s, want %s", got, want)
	}
}

func TestSortPagesDefault_PathTiebreakerIsByteLevel(t *testing.T) {
	// Same weight, date, title → path wins (byte level, NOT collator).
	d := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newSortPage("Same", 0, d, "/zeta.md"),
		newSortPage("Same", 0, d, "/alpha.md"),
		newSortPage("Same", 0, d, "/middle.md"),
	}
	sortPagesDefault(pages, i18n.BuildCollator("zh-cn"))
	got := []string{pages[0].RelPath, pages[1].RelPath, pages[2].RelPath}
	want := []string{"/alpha.md", "/middle.md", "/zeta.md"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("path tiebreaker pos %d: got %q want %q", i, got[i], w)
		}
	}
}

func TestSortPagesDefault_RealWorldZhurongshuoChapters(t *testing.T) {
	// Mirror real zhurongshuo book: chapters 一..七, all weight 0, all same date.
	d := time.Date(2025, 10, 14, 0, 0, 0, 0, time.UTC)
	pages := []*Page{
		newSortPage("第一章 权力的本体论：作为可能性收敛的能力", 0, d, "/chapter-01.md"),
		newSortPage("第二章 评估异化：从共识建构到单向认证", 0, d, "/chapter-02.md"),
		newSortPage("第三章 算法的全景敞视：信用评分与社会治理", 0, d, "/chapter-03.md"),
		newSortPage("第四章 平台资本主义与强制自愿", 0, d, "/chapter-04.md"),
		newSortPage("第五章 公共领域的瓦解：语境污染与语义极化", 0, d, "/chapter-05.md"),
		newSortPage("第六章 微观抵抗的策略：边缘体系的建立", 0, d, "/chapter-06.md"),
		newSortPage("第七章 制度层面的重构：迈向评估正义", 0, d, "/chapter-07.md"),
	}
	sortPagesDefault(pages, i18n.BuildCollator("zh-cn"))
	got := pageTitles(pages)
	// Hugo observed order: 二/六/七/三/四/五/一
	want := []string{
		"第二章 评估异化：从共识建构到单向认证",
		"第六章 微观抵抗的策略：边缘体系的建立",
		"第七章 制度层面的重构：迈向评估正义",
		"第三章 算法的全景敞视：信用评分与社会治理",
		"第四章 平台资本主义与强制自愿",
		"第五章 公共领域的瓦解：语境污染与语义极化",
		"第一章 权力的本体论：作为可能性收敛的能力",
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d want %d (got: %v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("zhurongshuo chapters pos %d:\n  got:  %q\n  want: %q\nfull got: %v", i, got[i], w, got)
			break
		}
	}
}

func TestBuildTree_FiltersBuildListNever(t *testing.T) {
	// Pages with Build.List == "never" should NOT appear in site.RegularPages.
	// Use distinct dates so sort order is deterministic regardless of collator.
	now := time.Now()
	pages := []*Page{
		{Title: "First", RelPath: "posts/a.md", Kind: "page",
			DateParsed: now.Add(2 * time.Hour), Section: "posts"},
		{Title: "Hidden", RelPath: "posts/b.md", Kind: "page",
			DateParsed: now.Add(1 * time.Hour), Section: "posts",
			Build: config.BuildConfig{List: "never"}},
		{Title: "Second", RelPath: "posts/c.md", Kind: "page",
			DateParsed: now, Section: "posts"},
	}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}
	titles := pageTitles(site.RegularPages)
	want := []string{"First", "Second"}
	if len(titles) != len(want) {
		t.Fatalf("expected %d regular pages, got %d: %v", len(want), len(titles), titles)
	}
	for i, w := range want {
		if i >= len(titles) || titles[i] != w {
			t.Errorf("pos %d: got %v, want %v", i, titles, want)
			break
		}
	}
	// Sanity: "Hidden" must not appear anywhere in site.RegularPages.
	for _, title := range titles {
		if title == "Hidden" {
			t.Errorf("Hidden page leaked into site.RegularPages: %v", titles)
		}
	}
}

func TestBuildTree_InheritsCascadeListNever(t *testing.T) {
	// Child page in section whose _index.md has cascade.build.list=never
	// should be filtered from site.RegularPages.
	now := time.Now()
	section := &Page{
		Title:      "Hidden Section",
		RelPath:    "hidden/_index.md",
		Kind:       "section",
		Section:    "hidden",
		DateParsed: now,
		Cascade: config.CascadeConfig{
			Build: config.BuildConfig{List: "never"},
		},
	}
	child := &Page{
		Title:      "Hidden Child",
		RelPath:    "hidden/page.md",
		Kind:       "page",
		Section:    "hidden",
		DateParsed: now,
		// Child doesn't set Build.List — should inherit "never" from section cascade.
	}
	visible := &Page{
		Title:      "Visible",
		RelPath:    "posts/a.md",
		Kind:       "page",
		Section:    "posts",
		DateParsed: now,
	}
	pages := []*Page{section, child, visible}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}
	titles := pageTitles(site.RegularPages)
	// Should only contain "Visible"; "Hidden Child" excluded via inherited cascade.
	if len(titles) != 1 || titles[0] != "Visible" {
		t.Errorf("expected only [Visible], got %v", titles)
	}
}
