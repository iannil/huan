# ebook-exporter 导出文件名随语言本地化 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ebook-exporter 导出的文件名随语言本地化——中文版用中文书名、英文版用英文书名，并自动清理旧 slug 命名文件。

**Architecture:** 在插件包新增 `filename.go`（`sanitizeFilename` + `fileStem` 两个纯函数），`outPath` 改为从 unit 取语言对应的书名作为主干；`manifestKey` 保持 slug 语义不变，靠内容哈希前缀升级（v3→v4）强制全量重导；渲染成功后删除同单元的旧 slug 路径文件。

**Tech Stack:** Go（`plugins/ebook-exporter` 模块，package main），标准库 `strings`/`os`/`path/filepath`。

**Spec:** `docs/superpowers/specs/2026-09-09-ebook-export-filename-l10n-design.md`

## Global Constraints

- 所有测试命令在 `plugins/ebook-exporter` 目录下执行：`go test ./...`
- 需要系统 CJK 字体的集成测试已用 `t.Skipf` 保护，无字体环境跳过不算失败
- 代码用英文（含注释），交流与文档用中文
- manifest key 结构 `<kind>/<dirName>/<base>.<lang>.<format>` 不变（`base` 仍是 slug）
- 哈希前缀字符串精确为 `publication-v4-20260909:`（替换 `publication-v3-20260905:`）
- 不要动工作区里已修改的其他文件（`internal/daemon/watcher.go`、`internal/dev/*`、`internal/version/VERSION`），提交时只 add 本计划涉及的文件

---

### Task 1: sanitizeFilename 纯函数

**Files:**
- Create: `plugins/ebook-exporter/filename.go`
- Test: `plugins/ebook-exporter/filename_test.go`

**Interfaces:**
- Consumes: 无（纯标准库）
- Produces: `func sanitizeFilename(s string) string` — Task 2 的 `fileStem` 依赖

- [ ] **Step 1: 写失败测试**

创建 `plugins/ebook-exporter/filename_test.go`：

```go
package main

import "testing"

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain ascii", "Demo Book", "Demo Book"},
		{"chinese with fullwidth colon", "实在建构：从无限可能到有限确定", "实在建构：从无限可能到有限确定"},
		{"halfwidth colon to fullwidth", "Reality Construction: From A to B", "Reality Construction：From A to B"},
		{"windows illegal chars dropped", `a\b/c*d?e"f<g>h|i`, "abcdefghi"},
		{"control chars dropped", "a\x00b\x1fc", "abc"},
		{"whitespace collapsed", "a \t\n b", "a b"},
		{"leading trailing spaces and dots", "  ..name.. ", "name"},
		{"empty stays empty", "///", ""},
		{"only spaces", "   ", ""},
	}
	for _, c := range cases {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("%s: sanitizeFilename(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test -run TestSanitizeFilename ./...`
Expected: FAIL，报 `undefined: sanitizeFilename`

- [ ] **Step 3: 最小实现**

创建 `plugins/ebook-exporter/filename.go`：

```go
package main

import "strings"

// sanitizeFilename makes s safe as a filename stem on all platforms:
// half-width colons become full-width (Chinese-style), Windows-illegal
// characters and control runes are dropped, whitespace runs collapse to a
// single space, and leading/trailing spaces and dots are trimmed. Whitespace
// immediately following a full-width colon is swallowed, since CJK typography
// never spaces after it. An empty return means the caller must fall back
// (see fileStem).
func sanitizeFilename(s string) string {
	var b strings.Builder
	suppressSpace := false
	for _, r := range s {
		switch {
		case r == ':' || r == '：':
			b.WriteRune('：')
			suppressSpace = true
		case suppressSpace && unicode.IsSpace(r):
			// dropped: whitespace right after a full-width colon
		case r < 0x20 || strings.ContainsRune(`\/*?"<>|`, r):
			// dropped
		default:
			suppressSpace = false
			b.WriteRune(r)
		}
	}
	return strings.Trim(strings.Join(strings.Fields(b.String()), " "), " .")
}
```

> **执行修订（2026-09-09，Task 1）**：原片段无法通过计划自带的测试表
> （转换后的全角冒号后不应有空格）。按 TDD 以测试为准，实现对半角与
> 既有全角冒号统一吞掉后续空格，import 需增加 `unicode`。

- [ ] **Step 4: 运行确认通过**

Run: `cd plugins/ebook-exporter && go test -run TestSanitizeFilename ./...`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/filename.go plugins/ebook-exporter/filename_test.go
git commit -m "feat(ebook): add sanitizeFilename for localized export filenames"
```

---

### Task 2: fileStem 按语言取书名

**Files:**
- Modify: `plugins/ebook-exporter/filename.go`
- Test: `plugins/ebook-exporter/filename_test.go`

**Interfaces:**
- Consumes: `sanitizeFilename(s string) string`（Task 1）；`unit`（`plugin.go:124`，字段 `baseName string`、`agg *content.BookEntry`）；`content.LangZH` / `content.LangEN`（`content/model.go:11`）；`BookEntry.TitleZH` / `TitleEN`（`content/model.go:40-41`）
- Produces: `func fileStem(u *unit, lang content.Lang) string` — Task 3 的 `outPath` 依赖

- [ ] **Step 1: 写失败测试**

在 `plugins/ebook-exporter/filename_test.go` 末尾追加：

```go
func TestFileStem(t *testing.T) {
	bilingual := &unit{
		baseName: "reality-construction",
		agg: &content.BookEntry{
			TitleZH: "实在建构：从无限可能到有限确定",
			TitleEN: "Reality Construction: From Infinite Possibility to Finite Certainty",
		},
	}
	if got := fileStem(bilingual, content.LangZH); got != "实在建构：从无限可能到有限确定" {
		t.Errorf("zh stem = %q", got)
	}
	if got := fileStem(bilingual, content.LangEN); got != "Reality Construction：From Infinite Possibility to Finite Certainty" {
		t.Errorf("en stem = %q", got)
	}

	// EN title missing: falls back to the ZH side, "-en" keeps files distinct.
	zhOnly := &unit{
		baseName: "demo-book",
		agg:      &content.BookEntry{TitleZH: "示范书", TitleEN: "示范书"},
	}
	if got := fileStem(zhOnly, content.LangZH); got != "示范书" {
		t.Errorf("zh-only stem = %q", got)
	}
	if got := fileStem(zhOnly, content.LangEN); got != "示范书-en" {
		t.Errorf("zh-only en stem = %q", got)
	}

	// Aggregate unit: synthesized titles from expandUnits flow through as-is.
	vol := &unit{
		baseName: "volume-1",
		agg:      &content.BookEntry{TitleZH: "第1卷合集", TitleEN: "Collected Books: Volume 1"},
	}
	if got := fileStem(vol, content.LangZH); got != "第1卷合集" {
		t.Errorf("volume zh stem = %q", got)
	}
	if got := fileStem(vol, content.LangEN); got != "Collected Books：Volume 1" {
		t.Errorf("volume en stem = %q", got)
	}

	// Both titles sanitize to empty: fall back to the slug (export never fails).
	blank := &unit{baseName: "demo-book", agg: &content.BookEntry{}}
	if got := fileStem(blank, content.LangZH); got != "demo-book" {
		t.Errorf("blank zh stem = %q", got)
	}
	if got := fileStem(blank, content.LangEN); got != "demo-book-en" {
		t.Errorf("blank en stem = %q", got)
	}
}
```

并在文件头部 import 中加入 `"github.com/iannil/huan-plugin-ebook-exporter/content"`，即：

```go
import (
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)
```

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test -run TestFileStem ./...`
Expected: FAIL，报 `undefined: fileStem`

- [ ] **Step 3: 最小实现**

在 `plugins/ebook-exporter/filename.go` 末尾追加（import 增加 `"github.com/iannil/huan-plugin-ebook-exporter/content"`）：

```go
// fileStem returns the output filename stem for one unit in one language:
// the ZH title for zh, the EN title for en (aggregates carry synthesized
// titles from expandUnits). A missing EN title falls back to the ZH side —
// the plugin-wide missing-EN policy — and then a "-en" suffix is appended so
// the two languages' files stay distinct. An empty title falls back to the
// slug baseName so export never fails on naming.
func fileStem(u *unit, lang content.Lang) string {
	title := u.agg.TitleZH
	if lang == content.LangEN {
		title = u.agg.TitleEN
	}
	stem := sanitizeFilename(title)
	if stem == "" {
		stem = u.baseName
	}
	if lang == content.LangEN && stem == fileStem(u, content.LangZH) {
		stem += "-en"
	}
	return stem
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd plugins/ebook-exporter && go test -run 'TestSanitizeFilename|TestFileStem' ./...`
Expected: PASS（两个测试都过）

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/filename.go plugins/ebook-exporter/filename_test.go
git commit -m "feat(ebook): add fileStem returning per-language title stems"
```

---

### Task 3: outPath 改用书名主干 + legacyPath 保留旧规则

**Files:**
- Modify: `plugins/ebook-exporter/plugin.go:196-203`（`outPath`）、`plugin.go:468`、`plugin.go:511`（调用点）
- Test: `plugins/ebook-exporter/filename_test.go`

**Interfaces:**
- Consumes: `fileStem(u *unit, lang content.Lang) string`（Task 2）
- Produces: `func outPath(outRoot, kind, dirName string, u *unit, lang content.Lang, format string) string`；`func legacyPath(outRoot, kind, dirName, base string, lang content.Lang, format string) string` — Task 4 的清理逻辑依赖 `legacyPath`

- [ ] **Step 1: 写失败测试**

在 `plugins/ebook-exporter/filename_test.go` 末尾追加：

```go
func TestOutPathUsesTitleStem(t *testing.T) {
	u := &unit{
		kind:     "books",
		dirName:  "individual",
		baseName: "demo-book",
		agg:      &content.BookEntry{TitleZH: "示范书", TitleEN: "Demo Book: A Story"},
	}
	wantZH := filepath.Join("out", "epub", "books", "individual", "示范书.epub")
	if got := outPath("out", u.kind, u.dirName, u, content.LangZH, "epub"); got != wantZH {
		t.Errorf("zh outPath = %q, want %q", got, wantZH)
	}
	wantEN := filepath.Join("out", "epub", "books", "individual", "Demo Book：A Story.epub")
	if got := outPath("out", u.kind, u.dirName, u, content.LangEN, "epub"); got != wantEN {
		t.Errorf("en outPath = %q, want %q", got, wantEN)
	}
}

func TestLegacyPathIsSlugBased(t *testing.T) {
	got := legacyPath("out", "books", "individual", "demo-book", content.LangZH, "epub")
	if want := filepath.Join("out", "epub", "books", "individual", "demo-book.epub"); got != want {
		t.Errorf("zh legacyPath = %q, want %q", got, want)
	}
	got = legacyPath("out", "books", "individual", "demo-book", content.LangEN, "epub")
	if want := filepath.Join("out", "epub", "books", "individual", "demo-book-en.epub"); got != want {
		t.Errorf("en legacyPath = %q, want %q", got, want)
	}
}
```

import 中加入 `"path/filepath"`。

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test -run 'TestOutPath|TestLegacyPath' ./...`
Expected: FAIL（`outPath` 参数不匹配编译错误 / `legacyPath` undefined）

- [ ] **Step 3: 实现**

`plugins/ebook-exporter/plugin.go` 中，把现有 `outPath`（196-203 行）整体替换为：

```go
// outPath builds <outRoot>/<format>/<kind>/<dirName>/<stem>.<ext>, where the
// stem is the language-appropriate book title (see fileStem).
func outPath(outRoot, kind, dirName string, u *unit, lang content.Lang, format string) string {
	return filepath.Join(outRoot, format, kind, dirName, fileStem(u, lang)+"."+format)
}

// legacyPath is the pre-v4 slug-based output path for one unit+language,
// kept solely so successful re-exports can clean up stale artifacts after
// the filename localization (spec: 2026-09-09-ebook-export-filename-l10n).
func legacyPath(outRoot, kind, dirName, base string, lang content.Lang, format string) string {
	name := base
	if lang != content.LangZH {
		name += "-" + string(lang)
	}
	return filepath.Join(outRoot, format, kind, dirName, name+"."+format)
}
```

更新两个调用点（保持实参顺序与函数新签名一致）：

- 468 行附近（增量跳过记录 Skipped 时）：
  `Path: outPath(outRoot, u.kind, u.dirName, u, lang, f),`
- 511 行附近（Phase 2 渲染时）：
  `out := outPath(outRoot, j.u.kind, j.u.dirName, j.u, lang, f)`

- [ ] **Step 4: 运行全量测试**

Run: `cd plugins/ebook-exporter && go test ./...`
Expected: PASS（旧集成测试不直接断言文件名，应不受影响；CJK 字体缺失时相关测试 SKIP）

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/plugin.go plugins/ebook-exporter/filename_test.go
git commit -m "feat(ebook): name export files by per-language book title"
```

---

### Task 4: 哈希前缀升级 + 旧文件自动清理

**Files:**
- Modify: `plugins/ebook-exporter/plugin.go:438`（哈希前缀）、Phase 2 渲染成功处（~520 行）、`jobResult` 结构（~484 行）、Phase 3 收集处（~540 行）
- Test: `plugins/ebook-exporter/export_flow_test.go`

**Interfaces:**
- Consumes: `legacyPath(outRoot, kind, dirName, base string, lang content.Lang, format string) string`（Task 3）
- Produces: 无新增导出符号；行为变更（清理 + 全量重导一次）

- [ ] **Step 1: 写失败测试**

在 `plugins/ebook-exporter/export_flow_test.go` 末尾追加：

```go
// TestExportRenamesSlugArtifacts verifies the filename localization rollout:
// a stale slug-named artifact left by a pre-v4 export is removed once the
// unit re-exports under its localized title, and the localized file exists.
func TestExportRenamesSlugArtifacts(t *testing.T) {
	if _, ferr := style.FindCJKFont(""); ferr != nil {
		t.Skipf("no CJK font: %v", ferr)
	}
	root := writeBookProject(t)
	p, _ := InitPlugin(nil)
	ex := p.(plugin.Exporter)

	// Simulate a pre-v4 export: slug-named epub sitting in the output dir.
	legacy := filepath.Join(root, "developer/export/epub/books/individual/demo-book.epub")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "epub"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(res, res.Succeeded, "epub"); len(got) != 1 {
		t.Fatalf("epub succeeded = %+v failed=%+v", res.Succeeded, res.Failed)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy slug artifact still exists (err=%v)", err)
	}
	localized := filepath.Join(root, "developer/export/epub/books/individual/示范书.epub")
	if _, err := os.Stat(localized); err != nil {
		t.Fatalf("localized artifact missing: %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test -run TestExportRenamesSlugArtifacts ./...`
Expected: FAIL —— 旧文件仍存在（`legacy slug artifact still exists`）

- [ ] **Step 3: 实现**

`plugins/ebook-exporter/plugin.go` 三处修改：

(a) 438 行哈希前缀升级（强制已缓存单元重导一次，完成改名与清理）：

```go
	hash := func(u *unit) string { return "publication-v4-20260909:" + assetHash + ":" + ComputeHash(u.mdPaths) }
```

(b) `jobResult` 结构（~484 行）增加 warnings 字段：

```go
	type jobResult struct {
		items []plugin.ExportItem
		fails []plugin.ExportFailure
		// hash is the unit's content hash, recorded per format that
		// rendered successfully in every language of this job.
		hash string
		// formatOK[f] is true when format f rendered successfully for all
		// langs of this job.
		formatOK map[string]bool
		// warns collects non-fatal issues (e.g. failed stale-artifact
		// removal) surfaced as result Warnings in phase 3.
		warns []string
	}
```

(c) Phase 2 渲染成功分支（`jr.items = append(...)` 之后、`}` 之前）追加旧文件清理：

```go
					jr.items = append(jr.items, plugin.ExportItem{
						Path: out, Lang: string(lang), Format: f, Slug: j.u.agg.Slug,
					})
					// Clean up the pre-v4 slug-named artifact, if any, now
					// that the localized file rendered successfully.
					if legacy := legacyPath(outRoot, j.u.kind, j.u.dirName, j.u.baseName, lang, f); legacy != out {
						if rmErr := os.Remove(legacy); rmErr != nil && !os.IsNotExist(rmErr) {
							jr.warns = append(jr.warns, fmt.Sprintf("remove stale %s: %v", legacy, rmErr))
						}
					}
```

(d) Phase 3 收集处（`res.Failed = append(...)` 之后）：

```go
		res.Succeeded = append(res.Succeeded, jr.items...)
		res.Failed = append(res.Failed, jr.fails...)
		res.Warnings = append(res.Warnings, jr.warns...)
```

- [ ] **Step 4: 运行全量测试**

Run: `cd plugins/ebook-exporter && go test ./...`
Expected: PASS（CJK 字体缺失时集成测试 SKIP）

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/plugin.go plugins/ebook-exporter/export_flow_test.go
git commit -m "feat(ebook): clean up stale slug artifacts after filename localization"
```

---

### Task 5: 文档更新与收尾

**Files:**
- Modify: `plugins/ebook-exporter/README.md:46`
- Create: `docs/progress/2026-09-09-ebook-export-filename-l10n.md`
- Modify: `memory/daily/2026-09-09.md`

**Interfaces:**
- Consumes: 无代码依赖
- Produces: 无

- [ ] **Step 1: 更新 README 产物说明**

`plugins/ebook-exporter/README.md` 46 行，把：

```
产物落 `developer/export/{epub,pdf,docx}/books|practices|posts/{individual|volumes|complete}/`，英文版带 `-en` 后缀。重跑时未变化的书自动跳过（增量 manifest）。
```

改为：

```
产物落 `developer/export/{epub,pdf,docx}/books|practices|posts/{individual|volumes|complete}/`，文件名随语言本地化：中文版用中文书名（`title`），英文版用英文书名（`subtitle`，半角冒号转全角）；英文书名缺失回退中文书名并加 `-en` 后缀。重跑时未变化的书自动跳过（增量 manifest）。旧 slug 命名文件在重导出成功后自动清理。
```

- [ ] **Step 2: 写进展文档**

创建 `docs/progress/2026-09-09-ebook-export-filename-l10n.md`：

```markdown
# ebook-exporter 导出文件名随语言本地化 — 进展

- 日期：2026-09-09
- 状态：已完成开发，待验收
- 规格：`docs/superpowers/specs/2026-09-09-ebook-export-filename-l10n-design.md`
- 计划：`docs/superpowers/plans/2026-09-09-ebook-export-filename-l10n.md`

## 修改内容

- 新增 `plugins/ebook-exporter/filename.go`：`sanitizeFilename`（非法字符清洗，半角冒号转全角）与 `fileStem`（按语言取书名主干，EN 回退时加 `-en`）。
- `outPath` 改用 `fileStem`，输出文件名变为语言对应的书名；`legacyPath` 保留旧 slug 规则。
- 内容哈希前缀升级 `publication-v3-20260905` → `publication-v4-20260909`，全量重导一次完成改名。
- 渲染成功后自动删除旧 slug 命名文件；删除失败仅记 Warning。

## 验证

- `cd plugins/ebook-exporter && go test ./...` 全部通过（需 CJK 字体的集成测试在无字体环境 SKIP）。
- 集成测试 `TestExportRenamesSlugArtifacts` 覆盖：旧文件清理 + 新名落盘。

## 待办

- 在 zhurongshuo 仓库实际执行 `huan export ebook --type all --format all --level all` 验收产物文件名。
```

- [ ] **Step 3: 更新每日记忆**

在 `memory/daily/2026-09-09.md` 末尾追加一条：实现计划已执行完毕，列出关键提交哈希（用 `git log --oneline -4` 取实际值）。

- [ ] **Step 4: 全量回归 + 提交**

Run: `cd plugins/ebook-exporter && go test ./... && cd ../.. && go build ./...`
Expected: 全部 PASS，构建成功

```bash
git add plugins/ebook-exporter/README.md docs/progress/2026-09-09-ebook-export-filename-l10n.md memory/daily/2026-09-09.md
git commit -m "docs(ebook): record filename localization rollout"
```

---

## Self-Review 结论

- **Spec 覆盖**：纯标题命名（Task 2/3）、冒号转全角与清洗（Task 1）、`-en` 冲突保留（Task 2）、自动清理（Task 4）、哈希前缀升级（Task 4）、README/进展文档（Task 5）——全部需求均有对应任务。
- **占位符**：无 TBD/TODO；所有代码步骤均含完整代码。
- **类型一致性**：`fileStem(u *unit, lang content.Lang) string`、`outPath(outRoot, kind, dirName string, u *unit, lang content.Lang, format string) string`、`legacyPath(outRoot, kind, dirName, base string, lang content.Lang, format string) string` 在 Task 2/3/4 间签名一致。
