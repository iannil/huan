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

// TestBuildTree_NestedSectionAutoCreatedRecursive reproduces the zhurongshuo
// posts/ layout: no _index.md anywhere, posts organized as
// posts/YEAR/MONTH/DAY.md. Hugo auto-creates a section page for the top-level
// directory and treats every descendant page as part of that section.
//
// Without the auto-create-before-parent-assignment fix, posts' pages would be
// parentless orphans and posts.RegularPagesRecursive would be empty, leaving
// the posts RSS feed with zero items.
func TestBuildTree_NestedSectionAutoCreatedRecursive(t *testing.T) {
	now := time.Now()
	pages := []*Page{
		{Title: "Post A", RelPath: "posts/2026/05/2601.md", Kind: "page", Section: "posts", DateParsed: now},
		{Title: "Post B", RelPath: "posts/2026/05/2701.md", Kind: "page", Section: "posts", DateParsed: now.Add(-time.Hour)},
		{Title: "Post C", RelPath: "posts/2020/11/0101.md", Kind: "page", Section: "posts", DateParsed: now.Add(-24 * time.Hour)},
	}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}

	sec, ok := site.Sections["posts"]
	if !ok {
		t.Fatal("posts section not auto-created (missing from site.Sections)")
	}
	// All three pages nest under the auto-created posts section.
	if got := len(sec.RegularPagesRecursive); got != 3 {
		t.Errorf("posts.RegularPagesRecursive len: got %d, want 3", got)
	}
	// Direct RegularPages: Hugo attaches nested pages directly to the
	// first-level section when no nested _index.md exists.
	if got := len(sec.RegularPages); got != 3 {
		t.Errorf("posts.RegularPages len: got %d, want 3", got)
	}
	// Every page's parent should be the posts section.
	for _, p := range pages {
		if p.Parent == nil {
			t.Errorf("page %q has no parent (should be posts section)", p.RelPath)
		} else if p.Parent != sec {
			t.Errorf("page %q parent: got %q, want posts section", p.RelPath, p.Parent.RelPath)
		}
	}
}

// TestBuildTree_AutoCreatedSectionTitleIsTitleCase verifies that an
// auto-created section page (no _index.md present) gets a title-cased
// version of the directory name, matching Hugo's behavior. Hugo emits
// "Posts on <site>" for the posts/ section RSS channel title; without
// title-casing huan would emit "posts on <site>".
func TestBuildTree_AutoCreatedSectionTitleIsTitleCase(t *testing.T) {
	now := time.Now()
	pages := []*Page{
		{Title: "Post A", RelPath: "posts/2026/05/2601.md", Kind: "page", Section: "posts", DateParsed: now},
	}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}

	sec, ok := site.Sections["posts"]
	if !ok {
		t.Fatal("posts section not auto-created")
	}
	if got, want := sec.Title, "Posts"; got != want {
		t.Errorf("auto-created section title: got %q, want %q", got, want)
	}
}

// TestBuildTree_AutoCreatedSectionTitleMultiWord verifies that hyphenated
// or multi-word directory names also get title-cased (hyphen → space, then
// title-case each word), matching Hugo's MakeTitle behavior.
func TestBuildTree_AutoCreatedSectionTitleMultiWord(t *testing.T) {
	now := time.Now()
	pages := []*Page{
		{Title: "Item", RelPath: "my-notes/2026/05/2601.md", Kind: "page", Section: "my-notes", DateParsed: now},
	}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}

	sec, ok := site.Sections["my-notes"]
	if !ok {
		t.Fatal("my-notes section not auto-created")
	}
	if got, want := sec.Title, "My Notes"; got != want {
		t.Errorf("auto-created multi-word section title: got %q, want %q", got, want)
	}
}

// TestBuildTree_SectionRegularPagesFollowsNearestIndexMd verifies Hugo's
// "nearest section ancestor" rule: a page belongs to the closest ancestor
// directory that has _index.md (or auto-created top-level section).
//
// zhurongshuo evidence:
//   - practices/ has _index.md, practices/season-1/ also has _index.md.
//     A chapter at practices/season-1/research/part-01/chapter-01.md belongs
//     to season-1, NOT practices. So practices.RegularPages should NOT include
//     the chapter.
//   - posts/ has NO _index.md (auto-created); no year/month/day _index.md.
//     So every posts/**/*.md has nearest-section-ancestor = posts, and
//     posts.RegularPages = all posts (recursive).
//
// Before this fix huan attached every page to its top-level section
// (page.Section field), producing 20 phantom items in practices.RegularPages
// and books.RegularPages.
func TestBuildTree_SectionRegularPagesFollowsNearestIndexMd(t *testing.T) {
	now := time.Now()
	practices := &Page{
		Title: "Practices", RelPath: "practices/_index.md", Kind: "section",
		Section: "practices", DateParsed: now,
	}
	season1 := &Page{
		Title: "Season 1", RelPath: "practices/season-1/_index.md", Kind: "section",
		Section: "practices", DateParsed: now,
	}
	chapter := &Page{
		Title: "Chapter 1", RelPath: "practices/season-1/research/part-01/chapter-01.md",
		Kind: "page", Section: "practices", DateParsed: now,
	}
	// Top-level practices post: no intermediate _index.md between it and
	// practices/_index.md, so its nearest section ancestor is practices itself.
	topPage := &Page{
		Title: "Top", RelPath: "practices/top.md", Kind: "page",
		Section: "practices", DateParsed: now,
	}
	pages := []*Page{practices, season1, chapter, topPage}
	cfg := &config.Config{LanguageCode: "en"}
	site, err := BuildTree(pages, cfg, "/test")
	if err != nil {
		t.Fatal(err)
	}

	// Find the practices section in site.Pages (the original pointer, not a copy).
	var practicesSec *Page
	for _, p := range site.Pages {
		if p.RelPath == "practices/_index.md" {
			practicesSec = p
			break
		}
	}
	if practicesSec == nil {
		t.Fatal("practices section not found in site.Pages")
	}

	// practices.RegularPages must be just [Top]; the chapter belongs to season-1.
	titles := pageTitles(practicesSec.RegularPages)
	want := []string{"Top"}
	if len(titles) != len(want) || titles[0] != want[0] {
		t.Errorf("practices.RegularPages: got %v, want %v (chapter belongs to season-1, not practices)", titles, want)
	}

	// practices.RegularPagesRecursive should still include all descendants
	// (chapter + top).
	if got, want := len(practicesSec.RegularPagesRecursive), 2; got != want {
		t.Errorf("practices.RegularPagesRecursive: got %d, want %d (chapter + top)", got, want)
	}

	// chapter.Parent must point to season-1 (nearest _index.md ancestor).
	if chapter.Parent == nil {
		t.Fatal("chapter.Parent is nil")
	}
	if chapter.Parent.RelPath != "practices/season-1/_index.md" {
		t.Errorf("chapter.Parent: got %q, want practices/season-1/_index.md", chapter.Parent.RelPath)
	}

	// season-1.RegularPages must include the chapter.
	season1Titles := pageTitles(season1.RegularPages)
	found := false
	for _, t := range season1Titles {
		if t == "Chapter 1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("season-1.RegularPages: got %v, want it to contain Chapter 1", season1Titles)
	}
}
