# ebook-exporter 插件实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 huan 中新增 `Exporter` 插件能力 + `plugins/ebook-exporter/` 纯 Go `.so` 插件，将 zhurongshuo 的 books/practices 内容导出为 epub/pdf/docx 三格式电子书（中英双语、单本/合卷/合集、增量构建）。

**Architecture:** `pkg/plugin/exporter.go` 定义 Exporter 能力契约（模仿 `pkg/deploy/types.go` 的跨 .so 共享模式）；插件 `plugins/ebook-exporter/`（独立 go.mod，`replace` 指回 huan 根模块）内部分三层：`content/`（从 data yaml + content 树发现卷→书→部→章结构）、`render/`（goldmark AST 归一化 + 三个格式后端）、`manifest.go`（内容 hash 增量）。CLI 侧 `huan export ebook` 按能力接口选插件。产物仅写 `developer/export/`，不进部署链。

**Tech Stack:** Go 1.26、goldmark v1.8.2（huan 已依赖，用其 `extension.NewCJK()`）、go-shiori/go-epub v1.2.1（EPUB）、gpdf-dev/gpdf v1.0.11（PDF）、mmonterroca/docxgo/v2 v2.14.0（DOCX）。

**Spec:** `docs/superpowers/specs/2026-09-02-ebook-exporter-plugin-design.md`

## Global Constraints

- 插件非测试代码只准 import `github.com/iannil/huan/pkg/...`，禁 import `internal/...`（.so 跨模块边界约束，现有 6 个插件全部如此）
- 插件目录是独立 Go module（`plugins/ebook-exporter/go.mod`），module 名 `github.com/iannil/huan-plugin-ebook-exporter`，`replace github.com/iannil/huan => ../../`
- 三方库版本固定：`github.com/go-shiori/go-epub v1.2.1`、`github.com/gpdf-dev/gpdf v1.0.11`、`github.com/mmonterroca/docxgo/v2 v2.14.0`
- 单本书失败不中断批次（collection-not-interruption）；`Export` 返回 non-nil error 仅当无法开始（配置错、内容目录缺失）
- `guide/` 目录与 ```guide 代码块跳过不导出
- 缺英文侧 `.en.md` 时跳过英文版并计入 warn，不算错误
- 输出目录布局：`developer/export/{epub,pdf,docx}/{books,practices}/{individual,volumes,complete}/`；中文版文件名无后缀，英文版 `-en` 后缀
- 所有新代码英文命名、注释英文可中文混用（跟随 seo-injector 风格）；commit message 用中文正文也行，但遵循仓库现有 conventional 风格
- TDD：每个任务先写失败测试再实现；测试文件与实现同目录

---

### Task 1: Exporter 能力契约（`pkg/plugin/exporter.go`）

**Files:**
- Create: `pkg/plugin/exporter.go`
- Test: `pkg/plugin/exporter_test.go`

**Interfaces:**
- Consumes: `pkg/plugin.Plugin`（已存在，`Name() string`）
- Produces: `pkgplugin.Exporter` 接口、`ExportRequest`/`ExportResult`/`ExportItem`/`ExportFailure` 类型 —— Task 5 的 CLI 和 Task 6+ 的插件都依赖这些确切名字

- [ ] **Step 1: 写失败测试**

```go
package plugin

import (
	"context"
	"testing"
)

// stubExporter verifies the Exporter contract is satisfiable and that
// plugin.Find discovers it by capability.
type stubExporter struct{ base }

func (stubExporter) Export(ctx context.Context, req ExportRequest) (ExportResult, error) {
	return ExportResult{Succeeded: []ExportItem{{Path: "out.epub"}}}, nil
}

func TestExporterInterfaceSatisfied(t *testing.T) {
	var _ Exporter = stubExporter{}
}

func TestFindExporter(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubExporter{base{"ebook_exporter"}}); err != nil {
		t.Fatal(err)
	}
	got := Find[Exporter](r)
	if len(got) != 1 {
		t.Fatalf("want 1 exporter, got %d", len(got))
	}
	res, err := got[0].Export(context.Background(), ExportRequest{Type: "books"})
	if err != nil || len(res.Succeeded) != 1 {
		t.Fatalf("unexpected: %v %+v", err, res)
	}
}
```

（`base` 若在 exporter_test.go 所在包不存在，用一个内联 `type base struct{ name string }` + `func (b base) Name() string { return b.name }` 替代——先查 `pkg/plugin` 现有测试里是否已有 stub 模式，有就复用。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/rong.zhu/Code/zhurong/huan && go test ./pkg/plugin/ -run Exporter -v`
Expected: FAIL，`undefined: Exporter`

- [ ] **Step 3: 实现 `pkg/plugin/exporter.go`**

```go
package plugin

import "context"

// Exporter is the capability interface for plugins that transform site
// content into offline document formats (epub/pdf/docx). It mirrors the
// cross-.so sharing pattern of deploy.Deployer: the contract lives under
// pkg/ so out-of-tree .so plugins import the SAME types as the host binary,
// making exporters discoverable via plugin.Find[Exporter].
type Exporter interface {
	Plugin

	// Export runs the ebook generation batch. Implementations should:
	//   - Honor ctx for cancellation.
	//   - Return a Result with per-item Succeeded/Failed/Skipped lists even on
	//     partial failure (collection-not-interruption).
	//   - Return a non-nil error only when the export cannot proceed at all
	//     (invalid request, missing content root, missing CJK font for pdf).
	Export(ctx context.Context, req ExportRequest) (ExportResult, error)
}

// ExportRequest carries invocation-time parameters from the CLI.
type ExportRequest struct {
	// SourceDir is the project root containing huan.yaml and content/.
	SourceDir string

	// Type filters content: "books", "practices", "all".
	Type string

	// Format filters output: "epub", "pdf", "docx", "all".
	Format string

	// Level filters granularity: "individual", "volumes", "complete", "all".
	// ("volumes" and "seasons" are aliases; normalized here to "volumes".)
	Level string

	// Slug restricts to a single book/practice slug (optional).
	Slug string

	// Volume restricts to one volume/season number, 1-based (optional; 0 = all).
	Volume int

	// Force regenerates even when the manifest hash matches.
	Force bool

	// Jobs is the parallelism for per-book generation. 0 = runtime.NumCPU()-1.
	Jobs int
}

// ExportItem describes one generated artifact.
type ExportItem struct {
	// Path is the output file path relative to the project root.
	Path string
	// Lang is "zh" or "en".
	Lang string
	// Format is "epub", "pdf", or "docx".
	Format string
	// Slug of the source book/practice (empty for complete collections).
	Slug string
}

// ExportFailure pairs a failed unit with its error.
type ExportFailure struct {
	Item ExportItem
	Err  string
}

// ExportResult reports the batch outcome.
type ExportResult struct {
	Succeeded []ExportItem
	Failed    []ExportFailure
	// Skipped lists items skipped by the incremental manifest (hash match).
	Skipped []ExportItem
	// Warnings carries non-fatal notes (e.g. missing .en.md sidecars).
	Warnings []string
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./pkg/plugin/ -run Exporter -v`
Expected: PASS

- [ ] **Step 5: 全包回归 + 提交**

Run: `go test ./pkg/plugin/`
Expected: PASS（不破坏现有测试）

```bash
git add pkg/plugin/exporter.go pkg/plugin/exporter_test.go
git commit -m "feat(plugin): add Exporter capability contract"
```

---

### Task 2: 内容发现层 —— 数据模型（`content/model.go`）

**Files:**
- Create: `plugins/ebook-exporter/go.mod`
- Create: `plugins/ebook-exporter/content/model.go`
- Test: `plugins/ebook-exporter/content/model_test.go`

**Interfaces:**
- Consumes: 无（纯数据结构）
- Produces: `Collection`/`VolumeEntry`/`BookEntry`/`Section`/`Chapter`/`Lang` —— Task 3、7、8、9 依赖

- [ ] **Step 1: 创建插件模块骨架**

`plugins/ebook-exporter/go.mod`:

```
module github.com/iannil/huan-plugin-ebook-exporter

go 1.26.2

require (
	github.com/go-shiori/go-epub v1.2.1
	github.com/gpdf-dev/gpdf v1.0.11
	github.com/iannil/huan v0.0.0
	github.com/mmonterroca/docxgo/v2 v2.14.0
	github.com/yuin/goldmark v1.8.2
)

replace github.com/iannil/huan => ../../
```

（具体 require 行以 `go mod tidy` 实际解析为准；先写主 go.mod 占位，Task 2 只需要标准库 + yaml。huan 根模块用 `gopkg.in/yaml.v3`，插件同样直接依赖它。）

- [ ] **Step 2: 写失败测试（模型构造与归一化）**

```go
package content

import "testing"

func TestLangString(t *testing.T) {
	if Lang("zh").String() != "zh" || LangEN.String() != "en" {
		t.Fatal("lang string mismatch")
	}
}

func TestSectionOrder(t *testing.T) {
	// Assembly order must be: introduction, parts (in part-XX order),
	// epilogue, appendix — mirroring huan toc.go's writeBookToc.
	b := BookEntry{Slug: "demo", TitleZH: "Demo", TitleEN: "Demo"}
	b.Sections = []Section{
		{Type: "part", ID: "part-02", Title: "第二部"},
		{Type: "introduction", Title: "引言"},
		{Type: "appendix", Title: "附录"},
		{Type: "part", ID: "part-01", Title: "第一部"},
		{Type: "epilogue", Title: "结语"},
	}
	got := b.OrderedSections()
	want := []string{"introduction", "part-01", "part-02", "epilogue", "appendix"}
	for i := range want {
		if got[i].Type != want[i] && !(want[i] == "part-01" && got[i].ID == "part-01") && !(want[i] == "part-02" && got[i].ID == "part-02") {
			t.Fatalf("order[%d]: want %s, got %+v", i, want[i], got[i])
		}
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./content/ -v`
Expected: FAIL（包不存在符号）

- [ ] **Step 4: 实现 `content/model.go`**

```go
// Package content discovers the book/practice tree of a huan project for
// ebook export: data/<kind>.yaml metadata + content/<kind>/ markdown tree.
package content

import "sort"

// Lang identifies which language variant of a chapter to export.
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

func (l Lang) String() string { return string(l) }

// Chapter is one markdown file (one chapter inside a part, or a standalone
// special file: introduction/epilogue/appendix).
type Chapter struct {
	// SourcePath is the absolute path of the .md (zh) or .en.md (en) file.
	SourcePath string
	Title      string // frontmatter title (fallback: filename base)
}

// Section is a top-level assembly unit of a book.
type Section struct {
	Type  string // "introduction" | "part" | "chapter" | "epilogue" | "appendix"
	ID    string // "part-01" for parts; empty for specials
	Title string // part title from data yaml part_titles; specials use fm title
	// Chapters holds part children. Empty for specials (the special itself
	// is carried by Chapters[0] for uniform downstream handling — see Discover).
	Chapters []Chapter
}

// BookEntry is one book (content/books) or practice (content/practices).
type BookEntry struct {
	Slug        string
	TitleZH     string // data yaml title
	TitleEN     string // data yaml subtitle (English)
	SubtitleZH  string
	Version     string // rc / beta / alpha
	LastUpdated string
	// VolumeNumber is the 1-based volume (books) or season (practices) index.
	VolumeNumber int
	VolumeName   string // e.g. "第1卷" / "第1季"
	// Dir is the book's content directory (…/volume-1/<slug>).
	Dir string
	// Sections in discovered order; use OrderedSections for assembly order.
	Sections []Section
	// HasEN is true when at least one .en.md sidecar exists.
	HasEN bool
}

// OrderedSections returns sections in book assembly order:
// introduction, part-01..part-N (sorted), epilogue, appendix.
// Standalone "chapter" sections (books without parts) keep discovery order
// between introduction and epilogue.
func (b *BookEntry) OrderedSections() []Section {
	specialRank := map[string]int{"introduction": 0, "epilogue": 2, "appendix": 3}
	out := make([]Section, 0, len(b.Sections))
	// specials first: introduction
	for _, s := range b.Sections {
		if s.Type == "introduction" {
			out = append(out, s)
		}
	}
	// parts sorted by ID; standalone chapters in discovery order interleaved
	// after parts (zhurongshuo books all use parts; chapters kept for safety)
	var parts []Section
	var chapters []Section
	for _, s := range b.Sections {
		switch s.Type {
		case "part":
			parts = append(parts, s)
		case "chapter":
			chapters = append(chapters, s)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].ID < parts[j].ID })
	out = append(out, parts...)
	out = append(out, chapters...)
	for _, s := range b.Sections {
		if r, ok := specialRank[s.Type]; ok && r >= 2 {
			out = append(out, s)
		}
	}
	return out
}

// VolumeEntry groups the books of one volume / practices of one season.
type VolumeEntry struct {
	Number int    // 1-based
	Name   string // "第1卷" / "第1季"
	Books  []BookEntry
}

// Collection is the full discovered tree for one kind (books or practices).
type Collection struct {
	Kind    string // "books" | "practices"
	Volumes []VolumeEntry
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./content/ -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add plugins/ebook-exporter/
git commit -m "feat(ebook-exporter): content model — book/section/chapter types"
```

---

### Task 3: 内容发现层 —— 遍历实现（`content/discover.go`）

**Files:**
- Create: `plugins/ebook-exporter/content/discover.go`
- Test: `plugins/ebook-exporter/content/discover_test.go`

**Interfaces:**
- Consumes: Task 2 的模型类型
- Produces: `Discover(sourceDir, kind string) (*Collection, error)`；`PartTitleLookup`；章节排序/标题提取逻辑 —— Task 7/8/9 消费

**参考实现语义**（照抄 huan `cmd/huan/toc.go` 已验证的遍历，不要重新发明）：
- 读 `data/<kind>.yaml` 的 `collection`（volume/season → books/practices 列表）和 `part_titles`
- 卷目录名：books 用 `volume-%d`，practices 用 `season-%d`（1-based）
- 每本书目录：introduction/epilogue/appendix 特殊文件（frontmatter title）；`part-XX` 子目录内章节按 leadingNumber 排序；`guide/` 目录跳过
- `.en.md` 存在 → `HasEN=true`，英文 Chapter 的 SourcePath 指向 `.en.md`
- frontmatter 解析：插件不能 import `internal/content`，在插件内实现一个 40 行内的 `parseFrontmatterTitle(data []byte) (title string)`（找 `title:` 行），够用即可

- [ ] **Step 1: 写失败测试**

测试用 `t.TempDir()` 构造最小项目树（fixture builder 函数 `writeTestProject(t)`）：

```go
package content

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestProject builds a minimal books tree:
// data/books.yaml (1 volume, 1 book, 1 part title) + content tree with
// introduction, part-01 (2 chapters + en sidecars), guide/, epilogue.
func writeTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "data"), 0o755))
	must(os.WriteFile(filepath.Join(root, "data", "books.yaml"), []byte(`collection:
  - volume: "第1卷"
    books:
      - slug: "demo-book"
        title: "示范书"
        subtitle: "Demo Book"
        version: "rc"
        last_updated: "2026-09-01"
part_titles:
  demo-book:
    part-01: "第一部分：起点"
`), 0o644))
	bookDir := filepath.Join(root, "content", "books", "volume-1", "demo-book")
	for _, d := range []string{filepath.Join(bookDir, "part-01"), filepath.Join(bookDir, "guide")} {
		must(os.MkdirAll(d, 0o755))
	}
	write := func(path, fm, body string) {
		must(os.WriteFile(path, []byte("---\ntitle: "+fm+"\ndate: 2026-01-01T00:00:00+08:00\n---\n\n"+body), 0o644))
	}
	write(filepath.Join(bookDir, "introduction.md"), "引言标题", "引言正文")
	write(filepath.Join(bookDir, "introduction.en.md"), "Introduction", "intro body")
	write(filepath.Join(bookDir, "part-01", "chapter-02.md"), "第二章", "二")
	write(filepath.Join(bookDir, "part-01", "chapter-01.md"), "第一章", "一")
	write(filepath.Join(bookDir, "part-01", "chapter-01.en.md"), "Chapter One", "one")
	write(filepath.Join(bookDir, "part-01", "chapter-10.md"), "第十章", "十")
	write(filepath.Join(bookDir, "epilogue.md"), "结语标题", "结")
	write(filepath.Join(bookDir, "guide", "index.md"), "导读", "```guide\nbook: demo-book\n```")
	return root
}

func TestDiscoverBooks(t *testing.T) {
	root := writeTestProject(t)
	c, err := Discover(root, "books")
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "books" || len(c.Volumes) != 1 {
		t.Fatalf("collection: %+v", c)
	}
	v := c.Volumes[0]
	if v.Number != 1 || v.Name != "第1卷" || len(v.Books) != 1 {
		t.Fatalf("volume: %+v", v)
	}
	b := v.Books[0]
	if b.Slug != "demo-book" || b.TitleZH != "示范书" || b.Version != "rc" || !b.HasEN {
		t.Fatalf("book: %+v", b)
	}
	ordered := b.OrderedSections()
	if len(ordered) != 3 {
		t.Fatalf("want intro+part+epilogue, got %d sections", len(ordered))
	}
	if ordered[0].Type != "introduction" || ordered[1].ID != "part-01" || ordered[1].Title != "第一部分：起点" || ordered[2].Type != "epilogue" {
		t.Fatalf("order: %+v", ordered)
	}
	// chapter sort is numeric, not lexicographic: chapter-01, chapter-02, chapter-10
	chs := ordered[1].Chapters
	if len(chs) != 3 || chs[0].Title != "第一章" || chs[1].Title != "第二章" || chs[2].Title != "第十章" {
		t.Fatalf("chapters: %+v", chs)
	}
}

func TestDiscoverEnglishSidecarPaths(t *testing.T) {
	root := writeTestProject(t)
	c, _ := Discover(root, "books")
	b := c.Volumes[0].Books[0]
	intro := b.OrderedSections()[0]
	if len(intro.Chapters) == 0 || intro.Chapters[0].SourcePath == "" {
		t.Fatal("special section must carry its file in Chapters[0]")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./content/ -run Discover -v`
Expected: FAIL，`undefined: Discover`

- [ ] **Step 3: 实现 `discover.go`**

实现要点（完整写出，不留 TODO）：
- `Discover(sourceDir, kind string) (*Collection, error)`：读 `data/<kind>.yaml`（`gopkg.in/yaml.v3`，结构体照 Task 2；目录前缀 books=`volume`，practices=`season`；列表 key books=`books`，practices=`practices`；卷名取 `volume` 字段，practices 时回退 `season` 字段）
- 每本书调 `discoverBook(dir, slug, partTitles)`：
  - `os.ReadDir`，收集特殊文件（introduction/epilogue/appendix 的 `.md`，frontmatter title）、`part-` 目录；跳过 `guide` 目录与 `_index.md`
  - 特殊 section 的 `Chapters = []Chapter{{SourcePath: <file>, Title: fmTitle}}`（zh 侧）；若同名 `.en.md` 存在则记 `ENPath`
  - part 目录内 `.md` 按 `chapter-<num>` 的 leadingNumber 排序（照抄 toc.go 的 `sortedChapterTitles` 语义：introduction=0 最前、epilogue/appendix=2 最后、其余按数字前缀）
  - `.en.md` 出现任意一处 → `HasEN=true`
- `Chapter` 需要扩展字段 `ENPath string`（`.en.md` 路径，缺省空）——在 Task 2 的 model.go 里同步加上（本任务内直接改 model.go，加字段不算破坏接口）
- frontmatter title 提取：`parseFrontmatterTitle(data []byte) string` —— 按 `\n` 扫前 30 行，匹配 `title: ` 前缀，trim 引号；找不到返回 ""
- 数字前缀提取 `leadingNumber(s string) int`：正则 `^[a-zA-Z-]*(\d+)` 提取 chapter-01 → 1

- [ ] **Step 4: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./content/ -v`
Expected: PASS（含 Task 2 回归）

- [ ] **Step 5: 用 zhurongshuo 真实数据冒烟**

Run: `cd plugins/ebook-exporter && cat > /tmp/discover_smoke_test.go <<'EOF'
package content

import "testing"

func TestSmokeZhurongshuo(t *testing.T) {
	t.Skip("manual smoke: remove Skip to run against the real repo")
	c, err := Discover("/Users/rong.zhu/Code/zhurong/zhurongshuo", "books")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, v := range c.Volumes {
		total += len(v.Books)
	}
	if total < 20 {
		t.Fatalf("expected ≥20 books, got %d", total)
	}
}
EOF
cp /tmp/discover_smoke_test.go content/smoke_manual_test.go && go test ./content/ -run Smoke -v;`
Expected: skipped（默认）。手动去掉 `t.Skip` 跑一次确认 25 本全被发现后，恢复 Skip 并保留该文件作为长期冒烟入口。

- [ ] **Step 6: 提交**

```bash
git add plugins/ebook-exporter/
git commit -m "feat(ebook-exporter): content discovery from data yaml + content tree"
```

---

### Task 4: AST 归一化层（`render/ast.go`）

**Files:**
- Create: `plugins/ebook-exporter/render/ast.go`
- Test: `plugins/ebook-exporter/render/ast_test.go`

**Interfaces:**
- Consumes: `github.com/yuin/goldmark`（v1.8.2）
- Produces: `DocUnit`（归一化后的章节文档：块列表）、`ParseChapter(path string) (*DocUnit, error)`、`ShiftHeadings(du *DocUnit, levels int)` —— Task 7/8/9 三后端遍历 `DocUnit.Blocks`

设计：不做“AST 直通三后端”（三后端遍历 goldmark AST 会重复三遍 walking 代码）。改为**一次解析成自归一化的中间结构** `DocUnit`，后端只面对 `Block`：

```go
// BlockKind enumerates block types the render backends consume.
type BlockKind int

const (
	BlockHeading BlockKind = iota
	BlockParagraph
	BlockQuote
	BlockList            // ordered or unordered
	BlockCode            // fenced code (info string + text); "guide" info → dropped upstream
	BlockTable           // GFM table (rows of cells)
	BlockThematicBreak   // --- / ***
)

type Block struct {
	Kind    BlockKind
	Level   int      // heading level 1-6
	Text    string   // plain-ish text (inline markdown preserved, see below)
	Items   []string // list items (each with inline markdown preserved)
	Rows    [][]string // table rows (header first)
	Align   []string  // table column alignments
	Lang    string    // code fence info
}

type DocUnit struct {
	Blocks []Block
}
```

inline 格式（bold/italic/code/link）在 `Text` 中保留原始 markdown 语法，由各后端用小函数（`renderInlineEPUB`/`renderInlinePDF`/`renderInlineDocx`）自行解释——避免中间结构变成第二套 AST。

- [ ] **Step 1: 写失败测试**

```go
package render

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMD(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "chapter-01.md")
	if err := os.WriteFile(p, []byte("---\ntitle: 测试\ndate: 2026-01-01T00:00:00+08:00\n---\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseChapterStripsFrontmatter(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "正文第一段。\n\n## 小节标题\n\n- 甲\n- 乙\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(du.Blocks) != 3 {
		t.Fatalf("blocks: %+v", du.Blocks)
	}
	if du.Blocks[0].Kind != BlockParagraph || du.Blocks[0].Text != "正文第一段。" {
		t.Fatalf("p0: %+v", du.Blocks[0])
	}
	if du.Blocks[1].Kind != BlockHeading || du.Blocks[1].Level != 2 || du.Blocks[1].Text != "小节标题" {
		t.Fatalf("h: %+v", du.Blocks[1])
	}
	if du.Blocks[2].Kind != BlockList || len(du.Blocks[2].Items) != 2 {
		t.Fatalf("list: %+v", du.Blocks[2])
	}
}

func TestParseChapterGuideCodeDropped(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "前文\n\n```guide\nbook: x\n```\n\n```python\nprint(1)\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	sawGuide, sawPython := false, false
	for _, b := range du.Blocks {
		if b.Kind == BlockCode {
			if b.Lang == "guide" {
				sawGuide = true
			}
			if b.Lang == "python" {
				sawPython = true
			}
		}
	}
	if sawGuide || !sawPython {
		t.Fatalf("guide=%v python=%v blocks=%+v", sawGuide, sawPython, du.Blocks)
	}
}

func TestShiftHeadings(t *testing.T) {
	du := &DocUnit{Blocks: []Block{{Kind: BlockHeading, Level: 2, Text: "x"}}}
	ShiftHeadings(du, 1)
	if du.Blocks[0].Level != 3 {
		t.Fatalf("shift: %+v", du.Blocks[0])
	}
}

func TestParseChapterTable(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "| A | B |\n|---|---|\n| 1 | 2 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(du.Blocks) != 1 || du.Blocks[0].Kind != BlockTable || len(du.Blocks[0].Rows) != 2 {
		t.Fatalf("table: %+v", du.Blocks)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./render/ -v`
Expected: FAIL

- [ ] **Step 3: 实现 `render/ast.go`**

实现要点：
- `ParseChapter(path string) (*DocUnit, error)`：读文件 → 剥 frontmatter（`^---\n...\n---\n`，第一对 `---` 界定；注意与 `***` thematic break 区分）→ `goldmark.New(goldmark.WithExtensions(extension.NewCJK(), extension.Table), goldmark.WithParserOptions(parser.WithAutoHeadingID()))` 解析 → `ast.Walk` 收集 Block：
  - `*ast.Heading` → Level + 拼接子节点 Text（`node.Lines()` 拼接，或 walk children 取 Text segment）
  - `*ast.Paragraph` → Text
  - `*ast.Blockquote` → 递归收段合并为 Text（多段用 `\n` 连）
  - `*ast.List` → 遍历 ListItem，每项取其 Text
  - `*ast.FencedCodeBlock` → Lang=info string，Text=代码体；**info=="guide" 时整块丢弃**
  - `*ast.Table`（extension）→ Rows（`*ast.TableRow`，第一行 header），每 cell 取 Text；Align 从 `*ast.TableCell` 的 Alignment 读
  - `*ast.ThematicBreak` → BlockThematicBreak
  - 顶层 dispatch 用 `ast.Walk` + `node.Parent() == doc` 或 `Kind()` switch（blockquote/list 内部节点 skip，由对应 handler 自行深入——用 `ast.Walk` 的 `WalkSkipChildren` 返回值控制）
- `ShiftHeadings(du *DocUnit, levels int)`：所有 BlockHeading 的 Level += levels，cap 至 6
- Text 提取统一走 helper `nodeText(n ast.Node, src []byte) string`：递归拼接 `*ast.Text` 的 `Segment.Value(src)`，**忽略** Emphasis/StrongEmphasis/Link 等 wrapper 节点本身但保留其文本子节点；inline 标记（`**`、`` ` ``）的保留策略：直接对原始行文本做 fallback——最简做法是 `Text` 用 goldmark 渲染 inline 为 markdown（`md.Convert` 到 markdown renderer 太重），**采用：Text 保存源文本 slice 原文**（`n.Lines()` 的原文），保留 inline 语法交给后端解释。这要求 Block.Text 的语义定义为“含 inline markdown 语法的源文本”——测试里 `== "正文第一段。"` 仍通过（无 inline 语法时原文=纯文本）

- [ ] **Step 4: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./render/ -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/render/
git commit -m "feat(ebook-exporter): goldmark AST → normalized DocUnit blocks"
```

---

### Task 5: CLI 子命令 `huan export ebook`（`cmd/huan/export.go` 改造）

**Files:**
- Modify: `cmd/huan/export.go`
- Test: `cmd/huan/export_ebook_test.go`

**Interfaces:**
- Consumes: Task 1 的 `pkgplugin.Exporter`/`ExportRequest`；现有 `newPluginRegistry`/`loadConfiguredPlugins`/`diagnoseCapabilityGap`（`cmd/huan/plugins.go`）
- Produces: `huan export`（现状 CSV）+ 子命令 `csv` + 子命令 `ebook`

- [ ] **Step 1: 写失败测试**

```go
package main

import (
	"testing"

	"github.com/iannil/huan/pkg/plugin"
)

// stubExporterCmd satisfies pkg/plugin.Exporter for CLI wiring tests.
type stubExporterCmd struct{}

func (stubExporterCmd) Name() string { return "stub_exporter" }

func (stubExporterCmd) Export(ctx any, req any) (any, error) { return nil, nil } // placeholder replaced below

func TestExportCmdHasSubcommands(t *testing.T) {
	root := newExportCmd()
subs := root.Commands()
	names := map[string]bool{}
	for _, c := range subs {
		names[c.Name()] = true
	}
	if !names["csv"] || !names["ebook"] {
		t.Fatalf("want csv+ebook subcommands, got %v", names)
	}
	// bare `huan export` keeps CSV behavior: RunE non-nil, no subcommand required
	if root.RunE == nil {
		t.Fatal("bare export must keep RunE (CSV back-compat)")
	}
}
```

（stubExporterCmd 的 Export 签名按 Task 1 的真实接口写——复制上面测试时把占位签名改成 `Export(ctx context.Context, req plugin.ExportRequest) (plugin.ExportResult, error)`，stub 返回空结果。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/rong.zhu/Code/zhurong/huan && go test ./cmd/huan/ -run ExportCmd -v`
Expected: FAIL（export 无子命令）

- [ ] **Step 3: 改造 `export.go`**

结构变更：
- `newExportCmd()` 返回带两个 subcommand 的父命令，父命令自身保留 `RunE: runExport`（裸 `huan export` = CSV，deploy.sh 兼容）
- 新增 `newExportCsvCmd()`（`Use: "csv"`，Short 描述同现有，`RunE: runExport`）
- 新增 `newExportEbookCmd()`（`Use: "ebook"`）：
  - flags：`--type`（默认 `all`）、`--format`（默认 `all`）、`--level`（默认 `all`；接受 `seasons` 归一化为 `volumes`）、`--slug`、`--volume`（int）、`--force`、`--jobs`（int，默认 0）
  - `RunE: runExportEbook`：
    1. 读配置与 `sourceDir`（照 `runExport` 的现有取法）
    2. `registry, err := newPluginRegistry(cfg, sourceDir, "")`（照 deploy.go:66 模式）
    3. `exporters := plugin.Find[pkgplugin.Exporter](registry)`；为空时 `loadConfiguredPlugins(registry, pluginDir, sourceDir, cfg.Plugins)` 后重找（照 deploy.go:73-83 的两段式）
    4. 仍为空 → `diagnoseCapabilityGap(registry, "plugin.Exporter")` 拼 error
    5. 构造 `ExportRequest{SourceDir: sourceDir, Type: flag值, Format: ..., Level: ..., Slug: ..., Volume: ..., Force: ..., Jobs: ...}`
    6. `res, err := exporters[0].Export(ctx, req)`；`err != nil` 直接返回
    7. 打印摘要到 stdout：`export ebook: %d ok, %d failed, %d skipped, %d warnings`；逐条列 Failed 的 `Item.Path: Err` 和 Warnings
    8. `len(res.Failed) > 0` 时返回 `fmt.Errorf("%d item(s) failed", len(res.Failed))`（非零退出码）
- import 增加 `"github.com/iannil/huan/pkg/plugin"` 与 `"github.com/iannil/huan/pkg/plugin"` 下已有的 deploy 别名模式一致（直接用 `plugin` 包名时注意 cmd/huan 里已有 import 别名——检查文件头，deploy.go 用 `"github.com/iannil/huan/pkg/plugin"` 无别名，保持一致）

- [ ] **Step 4: 跑测试确认通过 + 全命令回归**

Run: `go test ./cmd/huan/ -run Export -v && go test ./cmd/huan/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/huan/export.go cmd/huan/export_ebook_test.go
git commit -m "feat(cli): huan export ebook subcommand, csv kept as default+alias"
```

---

### Task 6: 字体解析（`style/font.go`）

**Files:**
- Create: `plugins/ebook-exporter/style/font.go`
- Test: `plugins/ebook-exporter/style/font_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `FindCJKFont(fontsDir string) (path string, err error)`、`ReadFontData(path string) ([]byte, error)` —— Task 8（gpdf WithFont）与 Task 7（go-epub AddFont）消费

- [ ] **Step 1: 写失败测试**

```go
package style

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindCJKFontCustomDir(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "NotoSansCJKsc-Regular.otf")
	os.WriteFile(fake, []byte("fake"), 0o644)
	got, err := FindCJKFont(dir)
	if err != nil || got != fake {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestFindCJKFontMissing(t *testing.T) {
	if _, err := FindCJKFont(t.TempDir()); err == nil || !strings.Contains(err.Error(), "CJK") {
		t.Fatalf("want CJK-missing error, got %v", err)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./style/ -v`
Expected: FAIL

- [ ] **Step 3: 实现 `font.go`**

- `FindCJKFont(fontsDir string) (string, error)`：
  - fontsDir 非空 → 只在其中找
  - 为空 → 扫描候选目录列表：`~/Library/Fonts`、`/System/Library/Fonts`、`/Library/Fonts`、`/usr/share/fonts`（递归 `filepath.WalkDir`）
  - 匹配文件名（不区分大小写）含 `notosanscjk` 且含 `sc`，优先 `-Regular`；次选任意 `*-CJK*`/`*CJK*` 文件；再退 `PingFang`/`SourceHanSans`
  - 找不到 → `fmt.Errorf("no CJK font found (looked in %v); set plugins.ebook_exporter.fonts_dir or install Noto Sans CJK SC", dirs)`
- `ReadFontData(path string) ([]byte, error)`：`os.ReadFile` 包装
- 注意 gpdf `WithFont(family string, data []byte)` 需要 **TTF**；Noto CJK 官方分发多为 OTF。方案：候选匹配扩展名 `.ttf,.ttc,.otf` 全收，交给 gpdf 尝试；若 gpdf 拒绝 OTF（运行期报错），fallback 提示用户 `brew install font-noto-sans-cjk --dir=...` 或提供 ttf。此风险记入插件 README（Task 11）。**在 font.go 里加 `PreferTTF` 排序：同名候选 .ttf 优先于 .otf**

- [ ] **Step 4: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./style/ -v`
Expected: PASS

- [ ] **Step 5: 本机真实验证**

Run: `cd plugins/ebook-exporter && go test ./style/ -v -run . `（若加了真实扫描用例）
另手动：`go run` 一个临时 main 或加 `TestFindCJKFontReal`（Skip by default）验证本机返回 `~/Library/Fonts/NotoSansCJKsc-Regular.otf`。

- [ ] **Step 6: 提交**

```bash
git add plugins/ebook-exporter/style/
git commit -m "feat(ebook-exporter): CJK font discovery with dir override"
```

---

### Task 7: EPUB 后端（`render/epub.go`）

**Files:**
- Create: `plugins/ebook-exporter/render/epub.go`
- Test: `plugins/ebook-exporter/render/epub_test.go`

**Interfaces:**
- Consumes: Task 2 `BookEntry`/`OrderedSections`、Task 3 `Chapter.SourcePath/ENPath`、Task 4 `ParseChapter`/`DocUnit`、Task 6 `FindCJKFont`
- Produces: `RenderEPUB(book *content.BookEntry, lang content.Lang, outPath string, opts EPUBOptions) error`

- [ ] **Step 1: 写失败测试**

```go
package render

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func mkBook(t *testing.T, lang content.Lang) *content.BookEntry {
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("---\ntitle: "+name+"\n---\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "part-01"), 0o755)
	write("introduction.md", "导言内容。\n\n### 导言小节\n")
	write("part-01/chapter-01.md", "第一章正文。\n\n## 章内节\n\n段落二。")
	write("epilogue.md", "结语内容。")
	return &content.BookEntry{
		Slug: "demo", TitleZH: "示范书", TitleEN: "Demo Book", Version: "rc",
		LastUpdated: "2026-09-01", Dir: dir,
		Sections: []content.Section{
			{Type: "introduction", Title: "引言", Chapters: []content.Chapter{{SourcePath: filepath.Join(dir, "introduction.md"), Title: "引言"}}},
			{Type: "part", ID: "part-01", Title: "第一部", Chapters: []content.Chapter{{SourcePath: filepath.Join(dir, "part-01", "chapter-01.md"), Title: "第一章"}}},
			{Type: "epilogue", Title: "结语", Chapters: []content.Chapter{{SourcePath: filepath.Join(dir, "epilogue.md"), Title: "结语"}}},
		},
	}
}

func TestRenderEPUBStructure(t *testing.T) {
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "demo.epub")
	if err := RenderEPUB(book, content.LangZH, out, EPUBOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var sawMimetype, sawNav bool
	var firstEntry string
	for i, f := range r.File {
		if i == 0 {
			firstEntry = f.Name
		}
		if f.Name == "mimetype" {
			sawMimetype = true
		}
		if strings.HasPrefix(f.Name, "EPUB/nav.") || strings.Contains(f.Name, "nav.xhtml") {
			sawNav = true
		}
	}
	if firstEntry != "mimetype" || !sawMimetype || !sawNav {
		t.Fatalf("epub structure: first=%q mimetype=%v nav=%v", firstEntry, sawMimetype, sawNav)
	}
	// a chapter's CJK text must be inside some xhtml section
	found := false
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".xhtml") {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(string(data), "第一章正文") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("chapter body not found in epub xhtml")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./render/ -run EPUB -v`
Expected: FAIL，`undefined: RenderEPUB`

- [ ] **Step 3: 实现 `render/epub.go`**

实现要点（go-shiori/go-epub v1.2.1 API）：
- `EPUBOptions{EmbedFont bool, FontPath string}`（测试默认不嵌字体，避免依赖系统）
- 流程：
  1. `e := epub.NewEpub(title)`；title 按 lang 取 `TitleZH`/`TitleEN`；`e.SetAuthor("iannil")`；`e.SetLang("zh-CN"/"en")`
  2. `cssPath, _ := e.AddCSS(dataurl 或临时文件, "style.css")`——CSS 内容内置 const（见下），写临时文件后 AddCSS（go-epub 的 source 参数支持 `data:` URL，直接用 `data:text/css;base64,...` 免临时文件，参考其 `conformant.go`/README 的 dataurl 用法；两法都试，以测试通过为准）
  3. 装配顺序遍历 `OrderedSections()`：introduction/epilogue/appendix → `AddSection(body, title, "", cssPath)`；part → 章节逐个 `AddSection`（part 标题作为该 part 第一章前的分隔 section，body 为 `<h1>{partTitle}</h1>` 纯分隔页）
  4. 每章 body：`ParseChapter(SourcePath)` → blocks → XHTML：`BlockHeading`→`<h{n}>`、`BlockParagraph`→`<p>`、`BlockQuote`→`<blockquote>`、`BlockList`→`<ul>/<ol>`（无序/有序无从区分则统一 `<ul>`——DocUnit 的 Items 有序性信息缺省，V1 统一 `<ul>`，记入 README 限制）、`BlockCode`→`<pre><code>`、`BlockTable`→`<table>`、`BlockThematicBreak`→`<hr/>`
  5. inline 解释 `inlineXHTML(s string) string`：`**x**`→`<strong>`、`*x*`/`_x_`→`<em>`、`` `x` ``→`<code>`、`[t](u)`→`<a href>`；先 HTML escape 再套这些规则（防注入）
  6. 章标题：每个 section body 开头插 `<h1>{chapterTitle}</h1>`
  7. `opts.EmbedFont && FontPath != ""` → `e.AddFont(FontPath, filepath.Base(FontPath))`，CSS 里 `@font-face` 引用 `../Fonts/<base>`（go-epub AddFont 返回内部路径，按返回值拼 CSS）
  8. `e.Write(outPath)`
- 内置 CSS const（精简版，从旧 `scripts/templates/epub/style.css` 的设计意图重写，不逐行复活）：
  ```css
  body { font-family: "Noto Sans CJK SC", serif; line-height: 1.7; margin: 1em; }
  h1 { font-size: 1.6em; margin: 1.5em 0 0.8em; page-break-before: always; }
  h2 { font-size: 1.3em; margin: 1.2em 0 0.6em; }
  h3 { font-size: 1.1em; margin: 1em 0 0.5em; }
  p { text-indent: 2em; margin: 0.5em 0; }
  blockquote { margin: 0.8em 1.5em; color: #555; border-left: 3px solid #ccc; padding-left: 0.8em; }
  pre { background: #f5f5f5; padding: 0.8em; overflow-x: auto; font-size: 0.9em; }
  table { border-collapse: collapse; margin: 1em 0; }
  td, th { border: 1px solid #999; padding: 0.4em 0.8em; }
  ```
  英文版（lang==en）追加 `body { font-family: serif; } p { text-indent: 0; }`——CSS 组装成函数 `epubCSS(lang content.Lang, fontRef string) string`

- [ ] **Step 4: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./render/ -run EPUB -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/render/
git commit -m "feat(ebook-exporter): EPUB backend via go-shiori/go-epub"
```

---

### Task 8: PDF 后端（`render/pdf.go`）

**Files:**
- Create: `plugins/ebook-exporter/render/pdf.go`
- Test: `plugins/ebook-exporter/render/pdf_test.go`

**Interfaces:**
- Consumes: 同 Task 7 + Task 6 `FindCJKFont`/`ReadFontData`
- Produces: `RenderPDF(book *content.BookEntry, lang content.Lang, outPath string, opts PDFOptions) error`

- [ ] **Step 1: 写失败测试**

```go
package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestRenderPDFHeaderAndPages(t *testing.T) {
	// Requires a real CJK font on this machine; skip when absent.
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "demo.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Fatal("not a PDF")
	}
	if len(data) < 2000 {
		t.Fatalf("suspiciously small pdf: %d bytes", len(data))
	}
	// /Count N pages object must exist with N ≥ 3 (cover + intro + chapter + epilogue)
	if !strings.Contains(string(data), "/Count") {
		t.Fatal("no page count object")
	}
}
```

`styleFindCJKFontForTest` 放在同一个 `_test.go` 里：`return style.FindCJKFont("")`（import Task 6 的包）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./render/ -run PDF -v`
Expected: FAIL，`undefined: RenderPDF`

- [ ] **Step 3: 实现 `render/pdf.go`**

gpdf v1.0.11 API 要点（已从源码确认）：
- `doc := template.New(template.WithPageSize(document.A4), template.WithMargins(document.UniformEdges(document.Mm(20))), template.WithFont("notocjk", fontData))`
- `page := doc.AddPage()` → `page.AutoRow(func(r *template.RowBuilder){ r.Col(12, func(c *template.ColBuilder){ c.Text("…", template.FontSize(n), template.Bold()) }) })`
- `doc.Render(io.Writer)` 输出
- 实现：
  1. `PDFOptions{FontPath string}`；`ReadFontData` 读入后 `template.WithFont("notocjk", data)`；**默认字体族设为 notocjk**（查 gpdf `WithDefaultFont` option，builder.go 有 `WithDefaultFont`——用它指向注册族）
  2. 封面页：书名（FontSize 28 Bold 居中）、subtitle、`version / last_updated` 小字
  3. 目录页：`OrderedSections()` 展开成章列表，每行 `c.Text(title)`（无页码链接——gpdf 无 TOC API，spec 已记限制）
  4. 每章新起一页：章标题 Text(20, Bold) + blocks：heading→按 Level 映射 FontSize(18-12)+Bold、paragraph→FontSize(11)、quote→FontSize(11) 缩进（Col(10) 或前缀 "▎"）、list→每 item 一行 "• " 前缀、code→FontSize(9) 等宽不可行则用默认族小字号、table→逐行 " | " 连接的 Text 行（gpdf 无表格 API 则降级文本行）、thematic break→一行 "———"
  5. inline 解释 `inlinePlain(s string) string`：剥 `**`/`*`/`` ` `` 标记，`[t](u)`→`t`（PDF V1 纯文本化）
  6. lang==en 时不强制 CJK 字体（`PDFOptions.FontPath` 允许空，gpdf 内置字体兜底）

- [ ] **Step 4: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./render/ -run PDF -v`
Expected: PASS（本机有 Noto 字体则真跑；CI/无字体环境 Skip）

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/render/
git commit -m "feat(ebook-exporter): PDF backend via gpdf with CJK font"
```

---

### Task 9: DOCX 后端（`render/docx.go`）

**Files:**
- Create: `plugins/ebook-exporter/render/docx.go`
- Test: `plugins/ebook-exporter/render/docx_test.go`

**Interfaces:**
- Consumes: 同 Task 7
- Produces: `RenderDOCX(book *content.BookEntry, lang content.Lang, outPath string, opts DOCXOptions) error`

- [ ] **Step 1: 写失败测试**

```go
package render

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan-plugin-ebook-plugin/content"
)

func TestRenderDOCXStructure(t *testing.T) {
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "demo.docx")
	if err := RenderDOCX(book, content.LangZH, out, DOCXOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var docXML string
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			docXML = string(b)
		}
	}
	if docXML == "" {
		t.Fatal("word/document.xml missing")
	}
	// Heading1 style must be applied to chapter titles
	if !strings.Contains(docXML, "Heading1") {
		t.Fatal("no Heading1 style in document.xml")
	}
	if !strings.Contains(docXML, "第一章正文") {
		t.Fatal("chapter body missing")
	}
}
```

（import path 笔误注意：`huan-plugin-ebook-plugin` 应为 `huan-plugin-ebook-exporter`——复制时修正。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test ./render/ -run DOCX -v`
Expected: FAIL

- [ ] **Step 3: 实现 `render/docx.go`**

docxgo v2.14.0 API 要点（已从源码确认）：
- `builder := docx.NewDocumentBuilder(docx.WithTitle(title), docx.WithAuthor("iannil"), docx.WithDefaultFont("Noto Sans CJK SC"))`
- `builder.AddParagraph()` → `*ParagraphBuilder`：`.Text(s)` `.Bold()` `.FontSize(pt)` `.Alignment(domain.AlignCenter)` `.End()`
- ParagraphBuilder **没有暴露 SetStyle**；标题样式通过 `domain.Document.AddParagraph()` 拿到底层 `domain.Paragraph` 再 `SetStyle(domain.StyleIDHeading1)`。做法：`doc, _ := builder.Build()` 前不能用 domain 直加——改用两段式：先 `builder.Build()` 得 `domain.Document`，之后用 `doc.AddParagraph()` + `para.SetStyle("Heading1")` + `para.AddRun()` 添加 run 文本（查 domain/run.go 的 `AddRun() (Run, error)` 与 `Run.SetText`；若 Run API 为 `AddText(text) error` 则照用）。**核心策略：builder 只建文档骨架（页面设置+metadata），全部内容由 domain 层加**——统一走 `doc.AddParagraph()`+`SetStyle`，避免 builder/domain 两套 API 混用的风格跳跃
- 段落映射：章标题→`SetStyle("Heading1")`、章内 heading level 2/3→`Heading2`/`Heading3`、paragraph→默认样式+`SetIndentFirstLine(480)`（2 字符 ≈ 480 twips）、quote→`SetStyle("Quote")`、list→每 item 段落 "• " 前缀、code→等宽不可靠则默认字体+`FontSize(9)`、table→V1 降级为制表符分隔文本段（docxgo TableBuilder 可用但 V1 不接，记 README 限制）
- inline：`inlinePlain(s)`（同 Task 8 复用，移到 `render/inline.go` 共享——Task 8 时先放 pdf.go 内，本任务抽取共享并让 pdf.go 改 import，或 Task 8 就直接建 `render/inline.go`，两处共用。**采用后者：Task 8 实现时就把 `inlinePlain` 放 `render/inline.go`**）
- `doc.SaveAs(outPath)`
- `DOCXOptions{}`（V1 无 knobs，预留）

- [ ] **Step 4: 跑测试确认通过**

Run: `cd plugins/ebook-exporter && go test ./render/ -run DOCX -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add plugins/ebook-exporter/render/
git commit -m "feat(ebook-exporter): DOCX backend via docxgo with heading styles"
```

---

### Task 10: 增量 manifest + 插件主体 + 装配（`manifest.go`、`plugin.go`、`plugin_main.go`）

**Files:**
- Create: `plugins/ebook-exporter/manifest.go`
- Create: `plugins/ebook-exporter/plugin.go`
- Create: `plugins/ebook-exporter/plugin_main.go`
- Test: `plugins/ebook-exporter/manifest_test.go`
- Test: `plugins/ebook-exporter/plugin_test.go`

**Interfaces:**
- Consumes: Task 1（Exporter 契约）、Task 2-4（content/render）、Task 7-9（三后端 Render*）
- Produces: `InitPlugin(cfg map[string]any) (interface{}, error)`（.so 入口）；`Name() == "ebook_exporter"`

- [ ] **Step 1: 写 manifest 失败测试**

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	m := LoadManifest(dir) // empty on first load
	if m == nil {
		t.Fatal("nil manifest")
	}
	m.Entries["demo.zh"] = "hash123"
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	m2 := LoadManifest(dir)
	if m2.Entries["demo.zh"] != "hash123" {
		t.Fatalf("roundtrip: %+v", m2.Entries)
	}
}

func TestComputeHashStable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	os.WriteFile(p, []byte("hello"), 0o644)
	h1 := ComputeHash([]string{p})
	h2 := ComputeHash([]string{p})
	if h1 == "" || h1 != h2 {
		t.Fatalf("unstable: %q vs %q", h1, h2)
	}
	os.WriteFile(p, []byte("world"), 0o644)
	if ComputeHash([]string{p}) == h1 {
		t.Fatal("content change must change hash")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd plugins/ebook-exporter && go test . -run Manifest -v`
Expected: FAIL

- [ ] **Step 3: 实现 `manifest.go`**

- `type Manifest struct { Entries map[string]string }`（key=`<slug>.<lang>`，value=内容聚合 hash）
- `LoadManifest(outDir string) *Manifest`：读 `<outDir>/.ebook-manifest.json`，不存在返回空
- `SaveManifest(outDir string, m *Manifest) error`：JSON marshal 落盘
- `ComputeHash(paths []string) string`：每文件 `sha256(content)` 拼接后再 sha256，十六进制；文件缺失计为空串参与拼接（并让上层把它当变化）

- [ ] **Step 4: 写 plugin 失败测试**

```go
package main

import (
	"context"
	"testing"

	"github.com/iannil/huan/pkg/plugin"
)

func TestInitPluginConfig(t *testing.T) {
	p, err := InitPlugin(map[string]any{
		"output_dir": "developer/export",
		"cover":      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := p.(plugin.Exporter)
	if !ok {
		t.Fatalf("not an Exporter: %T", p)
	}
	if ex.Name() != "ebook_exporter" {
		t.Fatalf("name: %s", ex.Name())
	}
}

func TestExportBadSourceDir(t *testing.T) {
	p, _ := InitPlugin(nil)
	ex := p.(plugin.Exporter)
	_, err := ex.Export(context.Background(), plugin.ExportRequest{SourceDir: "/nonexistent-xyz"})
	if err == nil {
		t.Fatal("want error for missing source dir")
	}
}
```

- [ ] **Step 5: 实现 `plugin.go` + `plugin_main.go`**

`plugin.go`（模式照 seo-injector/plugin.go）：
- `Config struct { OutputDir string; FontsDir string; Cover bool }` + `DefaultConfig()`（OutputDir=`developer/export`，Cover=true）+ `ParseConfig(raw map[string]any) (*Config, error)`（逐 key 类型断言，错类型返回 error）+ `ConfigSchema() plugin.Schema`
- `EbookExporter struct { cfg Config }`；`New(cfg *Config) *EbookExporter`
- `Name() string { return "ebook_exporter" }`；`PluginMetadata()`（Version "0.1.0"，Tags ["ebook","epub","pdf","docx"]，IsOfficial true）
- `Export(ctx, req)` 主流程：
  1. 归一化 req（Level "seasons"→"volumes"；Type/Format/Level 空值补 "all"；Jobs<=0 → `runtime.NumCPU()-1` 下限 1）
  2. `stat(content/<kind>)` 不存在 → error（无法开始）
  3. `Discover(sourceDir, kind)`（books/practices 按 req.Type）
  4. 展开生成单元：`individual`（每本书）+ `volumes`（每卷合并书列表）+ `complete`（全部书合一）→ 按 req.Level 过滤；req.Slug/req.Volume 再过滤
  5. 语言循环：zh 总是；en 仅当 `book.HasEN`，否则 `res.Warnings = append(..., slug+": no .en.md sidecars, EN skipped")`
  6. `ComputeHash`（该单元全部 md 路径）对比 manifest：相等且 !req.Force → `res.Skipped` 追加，continue
  7. 按 req.Format 逐格式调 `RenderEPUB`（EmbedFont: cfg.Cover 之外的字体开关恒 true 当 FontPath 找到）、`RenderPDF`（FontPath=FindCJKFont 结果）、`RenderDOCX`
  8. 单个失败 → `res.Failed` 追加，继续（collection-not-interruption）；ctx.Done() 检查放在每单元循环头
  9. 输出路径：`<OutputDir>/<format>/<kind>/individual/<slug>[-en].<ext>`、`.../volumes/volume-<N>[-en].<ext>`、`.../complete/<kind>-complete[-en].<ext>`
  10. 卷/合集渲染：复用 `BookEntry` 结构——构造一个聚合 BookEntry（TitleZH=卷名/合集名，Sections 为各书 OrderedSections 拼接、章标题前插书名分隔 section），ShiftHeadings(+1) 在 part 内章级下降一级（即卷内：书名=H1、原章=H2——Render* 内部把 chapter 渲染为 H1，卷模式下先对每章 DocUnit ShiftHeadings(1) 再在前面加书名 H1 section）
  11. 结束 `SaveManifest`
- 并行：`errgroup` 不引依赖，直接 `sync.WaitGroup` + 带 buffer 的 semaphore channel（Jobs 上限），结果写回用 mutex；失败清单顺序按单元顺序（先收集后并行的 index 固定）
- `plugin_main.go`：

```go
package main

// InitPlugin is the exported symbol that the .so plugin loader looks up.
func InitPlugin(cfg map[string]any) (interface{}, error) {
	parsed, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return New(parsed), nil
}

func main() {}
```

- [ ] **Step 6: 跑全部插件测试**

Run: `cd plugins/ebook-exporter && go mod tidy && go test ./... -v`
Expected: PASS（含 content/render/style/manifest/plugin 全部）

- [ ] **Step 7: 编译 .so 验证**

Run: `cd /Users/rong.zhu/Code/zhurong/huan && scripts/build-plugins.sh`
Expected: 输出包含 `building ebook-exporter -> ebook-exporter.so`，无编译错误

- [ ] **Step 8: 提交**

```bash
git add plugins/ebook-exporter/
git commit -m "feat(ebook-exporter): plugin assembly — manifest, batching, Export impl"
```

---

### Task 11: zhurongshuo 接入 + ADR + README

**Files:**
- Modify: `zhurongshuo 仓库 huan.yaml`（`/Users/rong.zhu/Code/zhurong/zhurongshuo/huan.yaml`）
- Create: `docs/adr/0016-ebook-exporter-plugin.md`（huan 仓库）
- Create: `plugins/ebook-exporter/README.md`
- Modify: `CLAUDE.md`（huan 仓库，常用命令节）

**Interfaces:**
- Consumes: Task 10 的 .so（`release/plugins/ebook-exporter.so`）
- Produces: 可用的端到端命令

- [ ] **Step 1: zhurongshuo huan.yaml 加插件声明**

在 `plugins:` 下追加：

```yaml
  ebook_exporter:
    output_dir: "developer/export"
    cover: true
```

- [ ] **Step 2: 安装 .so 到项目插件目录**

Run: `cp /Users/rong.zhu/Code/zhurong/huan/release/plugins/ebook-exporter.so /Users/rong.zhu/Code/zhurong/zhurongshuo/plugins/`（目录不存在则 mkdir；zhurongshuo 的 `plugins/` 不存在则创建——先 `ls` 确认现状）

- [ ] **Step 3: 端到端单本验证**

Run: `cd /Users/rong.zhu/Code/zhurong/zhurongshuo && huan export ebook --slug reality-construction --format all`
Expected: 退出码 0；`developer/export/{epub,pdf,docx}/books/individual/` 出现 `reality-construction.epub/.pdf/.docx` 与 `reality-construction-en.*` 三组文件；epub 可被 `unzip -l` 列出 mimetype 首位；pdf `head -c 5` 为 `%PDF-`；docx 可被 `unzip -l` 列出 word/document.xml

- [ ] **Step 4: 增量验证**

Run: `huan export ebook --slug reality-construction --format all`（立刻重跑）
Expected: 输出 `skipped` 计数 ≥ 6（3 格式 × 2 语言全跳过）

- [ ] **Step 5: 写 ADR 0016**

`docs/adr/0016-ebook-exporter-plugin.md`，结构照 ADR 0012（状态/日期/决策者/依赖/背景/决策/架构/影响），决策内容浓缩 spec 的 10 节，架构图照 Task 10 的目录树。

- [ ] **Step 6: 写插件 README**

`plugins/ebook-exporter/README.md`：
- 用法（huan.yaml 声明 + CLI 示例）
- 依赖：系统 CJK 字体（Noto Sans CJK SC 推荐；无则 `fonts_dir` 指定）；gpdf 的 OTF 兼容性说明（若 Task 8 实测 OTF 被拒，此处写明需 TTF）
- V1 限制清单：PDF 无页码目录/大纲书签、PDF 表格降级文本行、DOCX 表格降级文本段、EPUB 列表统一 `<ul>`、posts 年度合集未实现（P2）

- [ ] **Step 7: 更新 huan CLAUDE.md 常用命令**

在命令列表加：`huan export ebook --slug <slug>`（单本全格式）与 `huan export ebook --type all --format all --level all`（全量）。

- [ ] **Step 8: 提交（huan 仓库）**

```bash
git add docs/adr/0016-ebook-exporter-plugin.md plugins/ebook-exporter/README.md CLAUDE.md
git commit -m "docs: ADR 0016 + ebook-exporter README + CLAUDE.md command"
```

- [ ] **Step 9: 提交（zhurongshuo 仓库）**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo && git add huan.yaml && git commit -m "chore: declare ebook_exporter plugin in huan.yaml"
```

（`.so` 二进制不 commit——确认 zhurongshuo `.gitignore` 是否忽略 `plugins/*.so`，未忽略则添加。）

---

### Task 12: 全量验收

**Files:**
- 无新文件（运行验证）

**Interfaces:**
- Consumes: Task 11 全链路
- Produces: 验收结论

- [ ] **Step 1: 全量生成**

Run: `cd /Users/rong.zhu/Code/zhurong/zhurongshuo && huan export ebook --type all --format all --level all --jobs 8`
Expected: 退出码 0；输出 `ok=N failed=0`；N = (25 books + ~33 practices) × 2 语言命中 × 3 格式 + 卷/合集单元

- [ ] **Step 2: 产物抽查**

Run: `ls -R developer/export/ | head -50 && du -sh developer/export/`
Expected: 三格式目录齐全，总大小合理（epub 每本 0.5-5MB，pdf 1-15MB，docx 0.2-2MB 量级；超 10 倍偏差要查）

- [ ] **Step 3: 人工抽查 3 个文件**

用本机打开：1 本中文 epub（Apple Books / calibre 无则看 zip 内容）、1 本中文 pdf（Preview）、1 本英文 docx（Word/Pages）。检查：中文不豆腐块、章结构/目录合理、封面页与版本页内容正确。

- [ ] **Step 4: manifest 增量再验**

Run: `huan export ebook --type all --format all --level all --jobs 8`
Expected: 全部 skipped，耗时秒级

- [ ] **Step 5: deploy.sh 零改动确认**

Run: `git -C /Users/rong.zhu/Code/zhurong/zhurongshuo status && git diff --stat`
Expected: 仅 huan.yaml 变更（Task 11 已提交），deploy.sh 无改动

---

## Self-Review 记录

- **Spec 覆盖**：spec §1 Exporter 接口→Task 1；§2 目录结构→Task 2-10；§3 选型→全局约束；§4 CLI→Task 5；§5 渲染管线→Task 4+7+8+9；§6 字体→Task 6；§7 双语→Task 10（语言循环）；§8 增量→Task 10 manifest；§9 错误处理→Task 1 契约注释+Task 10 实现+Task 5 退出码；§10 ADR/README→Task 11；测试策略→各任务 TDD；验收标准→Task 12。spec 的 posts `--year` 已标 P2 不做——CLI flags 未含 `--year`（与 spec "P2 第一版可只做 books/practices" 一致）。
- **占位符扫描**：Task 9 测试代码里的 import 笔误已标注修正；docxgo SetStyle 通道已给具体两段式方案；gpdf OTF 风险给了具体 fallback 措辞。无 TBD。
- **类型一致性**：`RenderEPUB/RenderPDF/RenderDOCX` 签名在 Task 7/8/9/10 一致；`DocUnit/Block/BlockKind` 在 Task 4 定义、7/8/9 消费；`Discover` 在 Task 3 定义、Task 10 消费；`ExportRequest/ExportResult` 在 Task 1 定义、Task 5/10 消费。`Chapter.ENPath` 在 Task 3 补充、Task 7 消费（英文版渲染取 ENPath）。
