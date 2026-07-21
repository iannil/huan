# Hugo DefaultPageSort Port（CJK 拼音排序）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port Hugo `DefaultPageSort` 完整 5 层 tiebreaker 链到 huan，用 `golang.org/x/text/collate` 做 zh-cn 拼音排序，消除 zhurongshuo RSS items 顺序差（187 文件）+ books section 顺序差（104 文件），共 291 个 differing 文件。

**Architecture:** 加 `golang.org/x/text` 依赖；在 `internal/i18n/` 加 `BuildCollator(langCode string) *collate.Collator`；改 `sortPagesByDateDesc`（重命名为 `sortPagesDefault`）为 Port Hugo `resources/page/pages_sort.go:DefaultPageSort` 的完整链：Ordinal / Weight0 / Weight（0 排最后） / Date desc / Collator LinkTitle asc / Path asc。zhurongshuo chapters 都无 weight，实际生效的是 Date desc + Collator Title asc + Path asc。

**Tech Stack:** Go 1.x / `golang.org/x/text/collate` + `golang.org/x/text/language`（新依赖，Hugo 也用）/ zhurongshuo `languageCode=zh-cn`（huan config 已支持）。

---

## File Structure

**新建文件**：
- `internal/i18n/collator.go` + `_test.go` — Hugo-aligned Collator builder

**修订文件**：
- `go.mod` / `go.sum` — 加 `golang.org/x/text` 依赖
- `internal/content/tree.go` — 改 `sortPagesByDateDesc` 为 `sortPagesDefault`，Port Hugo 完整链；调用点（tree.go:104, 107, 131）同步改名
- `internal/content/tree_test.go` — 改测试函数名为 `sortPagesDefault`，追加 8 个新测试覆盖 Hugo 完整链 + CJK 拼音序

**修订文档（实施后）**：
- `docs/progress/CURRENT_STATE.md` — Stage 2 候选清单 #2/#3 标记完成
- `memory/MEMORY.md` — 经验教训追加 Hugo collator / 拼音排序发现
- `memory/daily/2026-06-12.md` — stage 2 phase 2 完成记录
- `docs/reports/completed/2026-06-12-cjk-sort-report.md` — 完成报告（新建）

**接受为永久差异**：无新增

---

# Phase 1 — 加 collate 依赖 + Collator builder

## Task 1.1：加 golang.org/x/text 依赖

**Files**:
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 确认依赖不在 go.mod**

Run: `grep "golang.org/x/text" go.mod || echo "missing"`

Expected: `missing`（确实不在）。

- [ ] **Step 2: go get 依赖**

Run: `go get golang.org/x/text@latest`

Expected: 修改 go.mod + go.sum，下载 module。

- [ ] **Step 3: 验证依赖已添加**

Run: `grep "golang.org/x/text" go.mod`

Expected: 显示 `golang.org/x/text vX.Y.Z` 行。

- [ ] **Step 4: 验证 go build 仍 OK**

Run: `go build ./...`

Expected: 无错误。

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang.org/x/text dependency for collate"
```

## Task 1.2：写 Collator builder 失败测试

**Files**:
- Test: `internal/i18n/collator_test.go`（新建）

- [ ] **Step 1: 确认 collator_test.go 不存在**

Run: `test ! -f internal/i18n/collator_test.go && echo "missing, will create"`

- [ ] **Step 2: 创建测试文件**

Create `internal/i18n/collator_test.go`:

```go
package i18n

import (
	"testing"

	"golang.org/x/text/collate"
)

func TestBuildCollator_ReturnsCollator(t *testing.T) {
	c := BuildCollator("zh-cn")
	if c == nil {
		t.Fatal("BuildCollator returned nil")
	}
	// Verify it's a *collate.Collator by attempting a comparison.
	got := c.CompareString("一", "二")
	if got == 0 {
		t.Error("expected non-zero comparison for distinct strings")
	}
}

func TestBuildCollator_ZhCN_PinyinOrder(t *testing.T) {
	// CLDR zh-cn table is pinyin order.
	// Pinyin: 八(bā) 百(bǎi) 二(èr) 九(jiǔ) 六(liù) 七(qī) 千(qiān) 三(sān)
	//         十(shí) 四(sì) 万(wàn) 五(wǔ) 一(yī)
	c := BuildCollator("zh-cn")

	digits := []string{"一", "二", "三", "四", "五", "六", "七", "八", "九", "十", "百", "千", "万"}
	wantPinyin := []string{"八", "百", "二", "九", "六", "七", "千", "三", "十", "四", "万", "五", "一"}

	// Copy digits and sort using collator.
	got := append([]string(nil), digits...)
	c.SortStrings(got)

	for i, w := range wantPinyin {
		if i >= len(got) {
			t.Errorf("sorted list shorter than expected at index %d", i)
			break
		}
		if got[i] != w {
			t.Errorf("ZhCN pinyin order pos %d: got %q, want %q\nfull got: %v\nfull want: %v", i, got[i], w, got, wantPinyin)
			break
		}
	}
}

func TestBuildCollator_FallbackToEnglishForInvalidLang(t *testing.T) {
	// Hugo's behavior: invalid language tag falls back to language.English.
	// Verify BuildCollator doesn't panic and returns a working collator.
	c := BuildCollator("not-a-real-lang")
	if c == nil {
		t.Fatal("BuildCollator returned nil for invalid lang")
	}
	// English collator should sort ASCII alphabetically.
	if c.CompareString("apple", "banana") >= 0 {
		t.Errorf("English fallback: apple should sort before banana")
	}
}

func TestBuildCollator_EmptyStringDefaultsToEnglish(t *testing.T) {
	c := BuildCollator("")
	if c == nil {
		t.Fatal("BuildCollator returned nil for empty lang")
	}
	// Same as English fallback.
	if c.CompareString("apple", "banana") >= 0 {
		t.Errorf("Empty lang: apple should sort before banana (English default)")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/i18n/ -run TestBuildCollator -v`

Expected: FAIL with `undefined: BuildCollator`（编译错误）。

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/i18n/collator_test.go
git commit -m "test(i18n): add failing tests for BuildCollator (zh-cn pinyin + fallback)"
```

## Task 1.3：实现 BuildCollator

**Files**:
- Create: `internal/i18n/collator.go`

- [ ] **Step 1: 创建 collator.go**

Create `internal/i18n/collator.go`:

```go
package i18n

import (
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// BuildCollator returns a collate.Collator for the given language code
// (e.g. "zh-cn", "en", "ja"). Mirrors Hugo's langs/language.go behavior:
//
//   - If langCode parses to a valid language.Tag, use it.
//   - Otherwise (including empty string), fall back to language.English.
//
// The returned Collator is safe for concurrent use (collate.Collator is
// goroutine-safe per golang.org/x/text docs).
func BuildCollator(langCode string) *collate.Collator {
	tag, err := language.Parse(langCode)
	if err != nil || tag == language.Und {
		tag = language.English
	}
	return collate.New(tag)
}
```

- [ ] **Step 2: 运行 BuildCollator 测试**

Run: `go test ./internal/i18n/ -run TestBuildCollator -v`

Expected: 4 tests PASS（ReturnsCollator / ZhCN_PinyinOrder / FallbackToEnglishForInvalidLang / EmptyStringDefaultsToEnglish）。

如果 `ZhCN_PinyinOrder` 失败，**先检查 CLDR 表实际顺序**——实证 hugo binary 输出可能是：
- 八(bā) / 百(bǎi) / 二(èr) / 九(jiǔ) / 六(liù) / 七(qī) / 千(qiān) / 三(sān) / 十(shí) / 四(sì) / 万(wàn) / 五(wǔ) / 一(yī)

如果实际顺序不同（例如 golang.org/x/text 版本差异），调整 wantPinyin 顺序匹配实际 CLDR 输出。**不要修改实现去迁就错误的测试预期**。

- [ ] **Step 3: 运行 i18n 包全部测试**

Run: `go test ./internal/i18n/ -v`

Expected: 全部 PASS（含已有 i18n.go 测试）。

- [ ] **Step 4: 运行全 repo 测试**

Run: `go test ./...`

Expected: 全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/i18n/collator.go
git commit -m "feat(i18n): BuildCollator with zh-cn pinyin support + English fallback"
```

---

# Phase 2 — Port Hugo DefaultPageSort 完整链

## Task 2.1：写 sortPagesDefault 失败测试

**Files**:
- Test: `internal/content/tree_test.go`（已存在，追加；保留现有 sortPagesByDateDesc 测试）

- [ ] **Step 1: 读当前 tree_test.go 顶部确认 package + imports**

Run: `head -15 internal/content/tree_test.go`

应看到 `package content` + imports（`testing` / `time`）。

- [ ] **Step 2: 在 tree_test.go 末尾追加 sortPagesDefault 测试**

需要先在 import 块加 `"sort"` 和 `"github.com/iannil/huan/internal/i18n"`（如果未导入）。如果 imports 已含 testing + time，扩展为：

```go
import (
	"sort"
	"testing"
	"time"

	"github.com/iannil/huan/internal/i18n"
)
```

注意：tree_test.go 可能没有 import 块（只 import "testing"）。Edit 时把现有 `import "testing"` 替换为上面的多 import 块。

在文件末尾追加：

```go
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
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Errorf("CJK pinyin pos %d: got %v, want %v", i, got, want)
			break
		}
	}
}

func TestSortPagesDefault_DateDescTakesPrecedence(t *testing.T) {
	// Different dates, same weight, same title-prefix → date desc wins.
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
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/content/ -run TestSortPagesDefault -v`

Expected: FAIL with `undefined: sortPagesDefault`（编译错误）。

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/content/tree_test.go
git commit -m "test(content): add failing tests for sortPagesDefault (Hugo DefaultPageSort port)"
```

## Task 2.2：Port Hugo DefaultPageSort 实现

**Files**:
- Modify: `internal/content/tree.go:312-322`（sortPagesByDateDesc 函数）

- [ ] **Step 1: 读当前 sortPagesByDateDesc 函数**

Run: `sed -n '310,325p' internal/content/tree.go`

应看到当前 3 层 tiebreaker 实现（Date desc / lower(Title) asc / RelPath asc）。

- [ ] **Step 2: 用 Port 实现替换 sortPagesByDateDesc 函数**

Use Edit tool. `old_string` = entire current `sortPagesByDateDesc` function (including its doc comment), `new_string` = new `sortPagesDefault` function (note name change):

```go
// sortPagesDefault sorts pages using Hugo's DefaultPageSort algorithm
// (resources/page/pages_sort.go:DefaultPageSort). The tie-break chain is:
//
//  1. Weight ascending — but weight 0 sorts last (Hugo's "unweighted goes last" rule)
//  2. DateParsed descending
//  3. Collator on Title ascending (Hugo uses LinkTitle with Title fallback;
//     zhurongshuo has no LinkTitle so Title is used directly). Language-aware
//     (zh-cn → pinyin order, en → alphabetical, etc.)
//  4. RelPath ascending (byte-level, NOT through collator)
//
// Note: Hugo also has Ordinal and Weight0 (taxonomy term weight) layers
// earlier in the chain, but huan doesn't model those concepts. zhurongshuo
// pages have no Ordinal / Weight0 set, so omitting those layers is safe.
//
// The Collator must be built by the caller (typically once per build via
// i18n.BuildCollator(site.LanguageCode)) and reused — collator construction
// is expensive.
func sortPagesDefault(pages []*Page, coll *collate.Collator) {
	sort.SliceStable(pages, func(i, j int) bool {
		a, b := pages[i], pages[j]

		// Layer 1: Weight (0 sorts last; otherwise ascending)
		if a.Weight == 0 && b.Weight != 0 {
			return false // a sorts after b
		}
		if a.Weight != 0 && b.Weight == 0 {
			return true // a sorts before b
		}
		if a.Weight != b.Weight {
			return a.Weight < b.Weight
		}

		// Layer 2: Date desc
		if !a.DateParsed.Equal(b.DateParsed) {
			return a.DateParsed.After(b.DateParsed)
		}

		// Layer 3: Collator on Title asc
		if c := coll.CompareString(a.Title, b.Title); c != 0 {
			return c < 0
		}

		// Layer 4: RelPath asc (byte-level)
		return a.RelPath < b.RelPath
	})
}
```

- [ ] **Step 3: 在 tree.go imports 加 collate**

Run: `head -15 internal/content/tree.go` 看 imports。当前 imports 应该有 `sort` / `strings` / `path/filepath` / `time` 等。加 `"golang.org/x/text/collate"`。

如果 imports 是单行 `import "..."` 形式，Edit 改为多行块。如果是多行块，加一行。

- [ ] **Step 4: 删除 now-unused imports（如果有）**

如果 sortPagesByDateDesc 是 `strings.ToLower` 的唯一用户（grep 确认），`strings` 可能变 unused。Run: `grep "strings\." internal/content/tree.go | head -5`。如果还有其他调用，保留 import；否则删。

- [ ] **Step 5: 更新所有 sortPagesByDateDesc 调用点**

3 处调用需要传 collator。先看当前调用点：

Run: `grep -n "sortPagesByDateDesc" internal/content/tree.go`

应看到 3 处：tree.go:104, 107, 131。

每个调用点都需要改为 `sortPagesDefault(..., coll)`，其中 `coll` 是从 `site` 上下文拿到的 Collator。

由于 `collate.Collator` 不能 NULL（调用会 panic），需要在 `BuildTree` 函数顶部构建一次：

Find the `BuildTree` function (around line 130):
```bash
grep -n "func BuildTree" internal/content/tree.go
sed -n '<line>,+15p' internal/content/tree.go
```

在 BuildTree 函数开头加：
```go
coll := i18n.BuildCollator(cfg.LanguageCode)
```

需要先在 imports 加 `"github.com/iannil/huan/internal/i18n"`。

然后把 BuildTree 内的 3 处 `sortPagesByDateDesc(x)` 改为 `sortPagesDefault(x, coll)`：
- tree.go:104: `sortPagesByDateDesc(p.Pages)` → `sortPagesDefault(p.Pages, coll)`
- tree.go:107: `sortPagesByDateDesc(p.RegularPages)` → `sortPagesDefault(p.RegularPages, coll)`
- tree.go:131: `sortPagesByDateDesc(site.RegularPages)` → `sortPagesDefault(site.RegularPages, coll)`

注意：tree.go:104 和 107 可能在另一个函数里（不是 BuildTree），需要确认每个调用点的上下文有 `coll` 可访问。如果某调用点在 BuildTree 之外的函数（例如 `(*Section).sort()`），需要在那个函数也加 coll 参数，或在函数内构建。

具体方案：把 coll 作为 BuildTree 的局部变量传到所有需要的子函数；或在 `*Site` / `*Section` struct 上加 collator 字段。前者更简洁。

如果发现调用点散落在多个函数，最小改动方案是：在 BuildTree 构建 coll 后传递，或者让 sortPagesDefault 接受 `*config.Config` 然后 BuildCollator（但每次构建 cost）。

**推荐：在 BuildTree 函数内构建 coll 局部变量，传递给直接调用的 sortPagesDefault。如果调用点在子函数里，把 coll 作为参数传给子函数。**

如果发现现有调用结构复杂（例如调用点在 BuildTree 调用的子函数内），可以采取**简单方案**：让 sortPagesDefault 接受 langCode string 而不是 collator，在内部 BuildCollator（每次调用 cost，但排序不频繁，可接受）。

如果选这个方案，函数签名改为：
```go
func sortPagesDefault(pages []*Page, langCode string) {
    coll := i18n.BuildCollator(langCode)
    // ... rest of sort logic
}
```

测试也要相应改：`sortPagesDefault(pages, "zh-cn")`。

- [ ] **Step 6: 更新现有 sortPagesByDateDesc 测试（如果保留）**

Run: `grep -n "sortPagesByDateDesc" internal/content/tree_test.go`

应看到 3 个旧测试（来自 stage 1 phase 4）。

**选择 1**：删除旧测试（sortPagesByDateDesc 函数已被替换）。
**选择 2**：更新旧测试调用 sortPagesDefault。

推荐选择 1——旧测试针对的是被替代的算法，新测试已覆盖。

- [ ] **Step 7: 运行所有 content 包测试**

Run: `go test ./internal/content/ -v`

Expected: 所有测试 PASS（含 6 个新 sortPagesDefault 测试）。

- [ ] **Step 8: 运行全 repo 测试确认无回归**

Run: `go test ./...`

Expected: 全部 PASS。

- [ ] **Step 9: Commit**

```bash
git add internal/content/tree.go internal/content/tree_test.go
git commit -m "fix(content): port Hugo DefaultPageSort with collate.Collator (zh-cn pinyin)"
```

---

# Phase 3 — zhurongshuo 实证

## Task 3.1：rebuild + byte-level 验证

**Files**:
- 无（命令行验证）

- [ ] **Step 1: rebuild huan**

```bash
cd /Users/rong.zhu/Code/huan
go build -o huan ./cmd/huan
rm -rf /tmp/huan-output /Users/rong.zhu/Code/zhurongshuo/docs
./huan build -s /Users/rong.zhu/Code/zhurongshuo > /dev/null
cp -r /Users/rong.zhu/Code/zhurongshuo/docs /tmp/huan-output
```

- [ ] **Step 2: 验证 books/volume-3 RSS items 顺序 byte-match**

```bash
echo "=== huan chapter 顺序（前 5）==="
grep -oE '<title>[^<]*第[^<]*章[^<]*</title>' /tmp/huan-output/books/volume-3/the-fetters-of-evaluation/index.xml | head -5
echo "=== hugo chapter 顺序（前 5）==="
grep -oE '<title>[^<]*第[^<]*章[^<]*</title>' /tmp/hugo-baseline/books/volume-3/the-fetters-of-evaluation/index.xml | head -5
```

Expected: 两边顺序完全一致（二/六/七/三/四/...）。

- [ ] **Step 3: byte-level 比对 home RSS items 顺序**

```bash
diff <(grep -oE '<title>[^<]*</title>' /tmp/huan-output/index.xml) \
     <(grep -oE '<title>[^<]*</title>' /tmp/hugo-baseline/index.xml) | head -10
```

Expected: diff 输出为空（所有 title 顺序完全一致）。

- [ ] **Step 4: 跑 diff-build.sh Step 5 看四模式数字**

```bash
./scripts/diff-build.sh 2>&1 | tail -10
```

Expected:
- byte/normalized differing 应继续下降（共 291 文件受影响）
- seo/ai 也应下降

记录四个数字（N_byte / N_norm / N_seo / N_ai）用于文档化。

---

# Phase 4 — 文档化 + 归档

## Task 4.1：更新 CURRENT_STATE.md 标记 #2/#3 完成

**Files**:
- Modify: `docs/progress/CURRENT_STATE.md`

- [ ] **Step 1: 定位 Stage 2 候选清单 #2 和 #3**

Run: `grep -n "^2\. \*\*RSS items\|^3\. \*\*books section" docs/progress/CURRENT_STATE.md`

- [ ] **Step 2: 标记 #2 完成**

Find item 2 (`**RSS items 顺序差**（原影响 187 文件...）`). Replace its entire bullet with:

```markdown
2. **RSS items 顺序差**（原影响 187 文件）→ ✅ **已修（stage 2 phase 2，2026-06-12）**
   - 根因：huan 用 `strings.ToLower(Title)` 字节级 UTF-8 比较，Hugo 用 `golang.org/x/text/collate` 库做 locale-aware 排序（zh-cn = 拼音序）
   - 修复：Port Hugo `resources/page/pages_sort.go:DefaultPageSort` 完整链（Weight / Date desc / Collator Title asc / Path asc）；引入 `golang.org/x/text` 依赖；`internal/i18n` 加 `BuildCollator(langCode)`
   - 验证：books/volume-3/the-fetters-of-evaluation RSS items 顺序与 Hugo byte-match；diff-build.sh 4 模式都下降（见下）
```

- [ ] **Step 3: 标记 #3 完成（与 #2 合并实施）**

Find item 3 (`**books section 顺序差**`). Replace its entire bullet with:

```markdown
3. **books section 顺序差**（原影响 104 文件）→ ✅ **已修（与 #2 同根因，stage 2 phase 2 一并修复）**
   - 与 #2 共享修复：Port Hugo DefaultPageSort 后，list page section 顺序 + chapter 顺序 + RSS items 顺序全部对齐
```

- [ ] **Step 4: 更新 Stage 2 进度表**

Find the "Stage 2 进度" table. Update rows 2 + 3 to mark complete:

```markdown
| Phase | 项 | 状态 | 完成日期 |
|---|---|---|---|
| 1 | meta description plainify | ✅ 已完成 | 2026-06-12 |
| 2 | RSS items 顺序（中文排序） | ✅ 已完成 | 2026-06-12 |
| 3 | books section 顺序（同 #2） | ✅ 已完成（与 #2 合并） | 2026-06-12 |
| 4 | RSS items 内容差 | 待启动 | — |
| 5 | body 渲染细节 | 待启动 | — |
| 6 | minify artifacts | 待启动 | — |
```

- [ ] **Step 5: Commit**

```bash
git add docs/progress/CURRENT_STATE.md
git commit -m "docs(progress): mark stage 2 phase 2 (CJK sort) complete — #2/#3 fixed"
```

## Task 4.2：MEMORY.md 经验教训

**Files**:
- Modify: `memory/MEMORY.md`

- [ ] **Step 1: 在"经验教训"段末追加**

```markdown
- **Hugo 用 collate 库做 locale-aware 排序**（2026-06-12 stage 2 phase 2 发现）：Hugo `resources/page/pages_sort.go:DefaultPageSort` 在 Date 相同时用 `langs.GetCollator1(currentSite.Language())` 构建 `golang.org/x/text/collate.Collator`，按 site language 的 CLDR 表排序（zh-cn = 拼音序）。huan 原用 `strings.ToLower(Title)` 字节级 UTF-8 比较，对中文章节标题完全错序（一 < 七 < 三 < 二 < 五 vs Hugo 二 < 六 < 七 < 三 < 四 < 五 < 一）。**含义**：(a) Port 上游排序算法必须查 collator 使用，不能假设字节级比较；(b) Go 标准库 `strings.Compare` 是 byte-level，要 locale-aware 排序必须用 `golang.org/x/text/collate`；(c) DefaultPageSort 完整链含 5+ 层 tiebreaker（Ordinal / Weight0 / Weight / Date / Collator LinkTitle / Path），不能只 Port 单层。
```

- [ ] **Step 2: Commit**

```bash
git add memory/MEMORY.md
git commit -m "docs(memory): record Hugo collate-based locale-aware sort lesson"
```

## Task 4.3：daily 笔记 + 完成报告

**Files**:
- Modify: `memory/daily/2026-06-12.md`
- Create: `docs/reports/completed/2026-06-12-cjk-sort-report.md`
- Move: `docs/superpowers/plans/2026-06-12-cjk-sort.md` → `docs/reports/completed/2026-06-12-cjk-sort-plan.md`

- [ ] **Step 1: 追加 daily 笔记**

Append to `memory/daily/2026-06-12.md`:

```markdown

---

## stage 2 phase 2：CJK 拼音排序 Port（已落地）

按"调查 → plan → 实施"路径完成。先调查 Hugo 实际排序算法（确认用 `golang.org/x/text/collate` 做 locale-aware 排序，zh-cn = 拼音序），再 Port DefaultPageSort 完整链。

### 落地内容

- 加 `golang.org/x/text` 依赖
- `internal/i18n/collator.go`：`BuildCollator(langCode string) *collate.Collator`，Hugo-aligned fallback（无效 lang → English）
- `internal/content/tree.go`：`sortPagesByDateDesc` 改名为 `sortPagesDefault(pages, coll)`，Port Hugo DefaultPageSort 4 层链（Weight 含 0 排后 + Date desc + Collator Title asc + Path asc）
- 6 个新测试覆盖：CJK 拼音序 / Date desc / Weight 非 0 vs 0 / Weight 都 0 fallback / Path 字节级 tiebreaker / 真实 zhurongshuo chapters

### zhurongshuo 实证

- `books/volume-3/the-fetters-of-evaluation/index.xml` RSS items 顺序：huan 与 Hugo byte-match
- home RSS `<title>` 顺序：byte-match
- books section list page 顺序：对齐

### diff-build.sh 四模式数字（stage 2 phase 1 后 → stage 2 phase 2 后）

- byte：1031 → N_byte_after
- normalized：1031 → N_norm_after
- seo：699 → N_seo_after
- ai：36 → N_ai_after

（用 Task 3.1 Step 4 跑出的实际数字填入）

### 下一步

stage 2 phase 3：RSS items 内容差（17 文件，根因待查）。
```

(Replace N_*_after with actual numbers from Task 3.1 Step 4.)

- [ ] **Step 2: 归档 plan**

```bash
mkdir -p docs/reports/completed
git mv docs/superpowers/plans/2026-06-12-cjk-sort.md docs/reports/completed/2026-06-12-cjk-sort-plan.md
rmdir docs/superpowers/plans docs/superpowers 2>/dev/null || true
```

- [ ] **Step 3: 写完成报告**

Create `docs/reports/completed/2026-06-12-cjk-sort-report.md`:

````markdown
# Hugo DefaultPageSort Port（CJK 拼音排序）完成报告

> 完成日期：2026-06-12 · 关联 plan：[2026-06-12-cjk-sort-plan.md](2026-06-12-cjk-sort-plan.md)
> 关联上一阶段：[meta description plainify 完成报告](2026-06-12-meta-plainify-report.md)

## 落地内容

### 代码

- 新增依赖：`golang.org/x/text`（提供 `collate` + `language` 包）
- 新建 `internal/i18n/collator.go`：`BuildCollator(langCode string) *collate.Collator`，Hugo-aligned fallback
- 修改 `internal/content/tree.go`：
  - `sortPagesByDateDesc` 重命名为 `sortPagesDefault(pages, coll)`
  - Port Hugo `resources/page/pages_sort.go:DefaultPageSort` 4 层链：Weight（0 排最后）/ Date desc / Collator Title asc / Path asc
- `BuildTree` 内构建一次 Collator（按 `cfg.LanguageCode`），传递给所有 sortPagesDefault 调用

### 测试（10 个单元测试全 PASS）

i18n 包（4 个）：
- `TestBuildCollator_ReturnsCollator`
- `TestBuildCollator_ZhCN_PinyinOrder`
- `TestBuildCollator_FallbackToEnglishForInvalidLang`
- `TestBuildCollator_EmptyStringDefaultsToEnglish`

content 包（6 个 sortPagesDefault）：
- `TestSortPagesDefault_CJKPinyinOrder`
- `TestSortPagesDefault_DateDescTakesPrecedence`
- `TestSortPagesDefault_WeightNonZeroBeforeZero`
- `TestSortPagesDefault_BothWeightZeroFallsBackToTitleThenPath`
- `TestSortPagesDefault_PathTiebreakerIsByteLevel`
- `TestSortPagesDefault_RealWorldZhurongshuoChapters`

### 验证结果

zhurongshuo 实际页面 byte-match：
- `books/volume-3/the-fetters-of-evaluation/index.xml` RSS items 顺序：✅
- home `index.xml` 全部 `<title>` 顺序：✅
- books section list pages：✅

diff-build.sh 四模式数字（stage 2 phase 1 后 → stage 2 phase 2 后）：

| 模式 | phase 1 后 | phase 2 后 | 增量 |
|---|---|---|---|
| byte | 1031 | N_byte_after | Δ_byte |
| normalized | 1031 | N_norm_after | Δ_norm |
| seo | 699 | N_seo_after | Δ_seo |
| ai | 36 | N_ai_after | Δ_ai |

（用实际数字填入）

### Stage 2 路线图进度

| Phase | 项 | 状态 |
|---|---|---|
| 1 | meta description plainify | ✅ 已完成 |
| **2** | **RSS items 顺序（中文排序）** | **✅ 已完成** |
| **3** | **books section 顺序（同 #2）** | **✅ 已完成（与 #2 合并）** |
| 4 | RSS items 内容差 | 待启动 |
| 5 | body 渲染细节 | 待启动 |
| 6 | minify artifacts | 待启动 |

## 关键发现

- **Hugo 用 collate 库做 locale-aware 排序**：`resources/page/pages_sort.go:DefaultPageSort` 在 Date 相同时用 `golang.org/x/text/collate` 按 site language CLDR 表排序（zh-cn = 拼音序）。
- **完整 DefaultPageSort 链 4+ 层**：Weight（0 排最后）/ Date desc / Collator LinkTitle asc / Path asc。Hugo 还有更早的 Ordinal / Weight0 层，zhurongshuo 不用所以可省略。
- **Go 标准库不能 locale-aware 排序**：`strings.Compare` 是 byte-level，CJK 字符按 UTF-8 编码排序，与拼音序完全不同。
- **教训**：Port 上游排序算法前必须 grep collator / language / locale 等关键词；"字符串排序 = byte compare" 的直觉在 Hugo 这种国际化框架里是错的。
````

(Replace N_*_after / Δ_* with actual numbers.)

- [ ] **Step 4: Commit 归档 + 报告**

```bash
git add memory/daily/2026-06-12.md docs/reports/completed/ docs/superpowers/
git commit -m "docs: archive CJK sort plan + completion report (stage 2 phase 2)"
```

---

# Self-Review Notes

完成全部 task 后自检：

**Spec 覆盖**：
- ✅ Port Hugo DefaultPageSort 完整链 → Task 2.2
- ✅ 加 collate 依赖 → Task 1.1
- ✅ 在 i18n 加 BuildCollator → Task 1.3
- ✅ 单元测试覆盖 8+ 用例 → Tasks 1.2 + 2.1
- ✅ zhurongshuo 实证 byte-match → Task 3.1
- ✅ 文档化（CURRENT_STATE / MEMORY / daily / 报告）+ plan 归档 → Tasks 4.1-4.3

**Placeholder 扫描**：
- N_byte_after / N_norm_after 等是合理占位（数字在 Task 3.1 跑出后才知道）——明确标注"用实际数字填入"
- 无 TBD / TODO / "implement later"

**类型一致性**：
- `BuildCollator(string) *collate.Collator` 签名一致
- `sortPagesDefault([]*Page, *collate.Collator)` 签名一致
- `Page.Weight` (int) / `Page.Title` (string) / `Page.DateParsed` (time.Time) / `Page.RelPath` (string) 都是现有字段

**Phase 1 教训内置**：
- ✅ 严格 TDD（每 phase 都是 RED → GREEN）
- ✅ 用 Hugo 源码 Port（明确引用 `resources/page/pages_sort.go:DefaultPageSort`）
- ✅ byte-level 实证（Task 3.1 用 diff + 直接 grep title 顺序）
- ✅ 实证失败时调整测试预期而非实现（Task 1.3 Step 2 注释）

**注意点**：
- Task 2.2 Step 5 涉及多处调用点更新——可能需要 coll 参数传递。如果调用结构复杂，**优先选简单方案**（sortPagesDefault 内部构建 collator），避免大量改动。
- 如果 Task 3.1 实证失败，**不要继续 Phase 4**——回头调查 Hugo 实际排序细节（可能 CLDR 表版本不同导致顺序微差）。
