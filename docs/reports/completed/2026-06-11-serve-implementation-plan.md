# huan serve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `huan serve` — HTTP file server + fsnotify file watching + LiveReload WebSocket, behaviorally similar to `hugo serve`.

**Architecture:** Extract existing `runBuild` logic into a reusable `internal/build.BuildSite` function. New `internal/serve` package owns HTTP server, debounced fsnotify watcher, and LiveReload hub. CLI handler in `cmd/huan/serve.go` wires them together. Livereload.js vendored via `//go:embed`.

**Tech Stack:** Go 1.26, cobra, fsnotify v1.7, coder/websocket v1.8, goldmark (existing), tdewolff/minify (existing). Spec: `docs/superpowers/specs/2026-06-11-huan-serve-design.md`.

**Working directory assumptions:** All commands run from `/Users/rong.zhu/Code/huan` unless noted. Content project lives at `/Users/rong.zhu/Code/zhurongshuo`.

**Refactor discipline (critical):** Phase A is a pure move — no behavior changes, no bug fixes, no optimizations. The safety net is `./scripts/diff-build.sh` byte-for-byte identical output vs Hugo. If diff fails after any Phase A task, that task introduced a regression — fix before moving on.

---

## Phase A — Refactor: Extract `internal/build` package

Goal: pull the 370-line build core out of `cmd/huan/main.go` into a reusable package. After this phase `huan build` still works identically.

### Task A1: Create `internal/build/summary.go`

**Files:**
- Create: `internal/build/summary.go`
- Modify: `cmd/huan/main.go` (remove the 3 functions, add import)

- [ ] **Step 1: Create the new file**

Create `internal/build/summary.go` with this exact content (functions moved verbatim from `main.go`):

```go
package build

import "strings"

// TruncateHTMLByWords truncates HTML content to approximately N "words"
// (CJK chars count as 1 word each, ASCII words split by whitespace).
// When N words are reached, it immediately cuts and closes any open tags.
// This matches Hugo's summary behavior (truncates mid-paragraph at word boundary).
func TruncateHTMLByWords(htmlStr string, n int) string {
	if n <= 0 {
		return htmlStr
	}
	count := 0
	inTag := false
	inWord := false
	var openTags []string

	for i := 0; i < len(htmlStr); i++ {
		c := htmlStr[i]
		if inTag {
			if c == '>' {
				inTag = false
			}
			continue
		}
		if c == '<' {
			inTag = true
			inWord = false
			tagEnd := strings.IndexByte(htmlStr[i:], '>')
			if tagEnd > 0 {
				tagContent := htmlStr[i+1 : i+tagEnd]
				if len(tagContent) > 0 && tagContent[0] == '/' {
					if len(openTags) > 0 {
						openTags = openTags[:len(openTags)-1]
					}
				} else if tagContent != "br" && tagContent != "hr" &&
					!strings.HasPrefix(tagContent, "br/") &&
					!strings.HasPrefix(tagContent, "img") &&
					!strings.HasPrefix(tagContent, "hr/") {
					name := tagContent
					if idx := strings.IndexAny(name, " /"); idx > 0 {
						name = name[:idx]
					}
					openTags = append(openTags, name)
				}
			}
			continue
		}
		if c >= 0x80 {
			if c&0xC0 != 0x80 {
				count++
				inWord = false
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
		if count >= n {
			result := htmlStr[:i+1]
			for j := len(openTags) - 1; j >= 0; j-- {
				result += "</" + openTags[j] + ">"
			}
			return result
		}
	}
	return htmlStr
}

// StripHTMLTagsForSummary strips HTML tags for plain text summary.
func StripHTMLTagsForSummary(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// CountWordsInPlain counts words in plain text using Hugo's algorithm:
// each CJK character counts as 1 word; ASCII words (split by whitespace) count as 1.
func CountWordsInPlain(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			count++
			inWord = false
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}
```

- [ ] **Step 2: Delete the old functions from main.go**

In `cmd/huan/main.go`, delete these three functions verbatim (currently at lines 540-648 per `grep -n "^func "`):
- `truncateHTMLByWords`
- `stripHTMLTagsForSummary`
- `countWordsInPlain`

- [ ] **Step 3: Update references in main.go**

The old code at main.go:107-110 and 119-121 calls these. Update call sites:

Replace `stripHTMLTagsForSummary(html)` → `build.StripHTMLTagsForSummary(html)`
Replace `countWordsInPlain(plain)` → `build.CountWordsInPlain(plain)`
Replace `truncateHTMLByWords(content, 120)` → `build.TruncateHTMLByWords(content, 120)`

Add import: `"github.com/novel_ttl/huan/internal/build"` to main.go's import block.

- [ ] **Step 4: Build and verify**

Run: `go build -o huan ./cmd/huan`
Expected: compiles cleanly, no errors.

- [ ] **Step 5: Byte-for-byte regression check**

Run: `./scripts/diff-build.sh`
Expected: prints "No differences found" (or whatever the script's success message is). Zero differing files.

- [ ] **Step 6: Commit**

```bash
git add internal/build/summary.go cmd/huan/main.go
git commit -m "refactor: move HTML summary helpers to internal/build"
```

---

### Task A2: Create `internal/build/context.go`

**Files:**
- Create: `internal/build/context.go`
- Modify: `cmd/huan/main.go` (remove ~9 functions, update references)

- [ ] **Step 1: Create the new file**

Create `internal/build/context.go`. Move these functions verbatim from `main.go`, renaming to exported (capital first letter):

| Old name (main.go) | New name (context.go) | Line range |
|---|---|---|
| `resolveRSSOutput` | `ResolveRSSOutput` | 428-431 |
| `buildSitemapContext` | `BuildSitemapContext` | 435-444 |
| `findHomeContext` | `FindHomeContext` | 447-454 |
| `filterMainSections` | `FilterMainSections` | 457-473 |
| `cloneContextForPagination` | `CloneContextForPagination` | 477-535 |
| `detectThemeName` | `DetectThemeName` | 650-662 |
| `urlEscape` | `URLEscape` | 670-691 |
| `buildTaxonomyContext` | `BuildTaxonomyContext` | 694-749 |
| `buildEmptyTaxonomyContext` | `BuildEmptyTaxonomyContext` | 754-771 |
| `buildTermContext` | `BuildTermContext` | 774-792 |
| `resolveTemplateName` | `ResolveTemplateName` | 795-840 |

The file header:

```go
package build

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/novel_ttl/huan/internal/content"
	tmpl "github.com/novel_ttl/huan/internal/template"
)
```

Move each function body verbatim. The function bodies reference `tmpl` and `content` packages — already in the imports above.

**Important:** the function bodies call each other (e.g., `buildTaxonomyContext` doesn't call others, but `cloneContextForPagination` references nothing external). Verify after the move by running `go vet ./...`. If there are intra-package calls (e.g., `urlEscape` called from `BuildTermContext`), update to the new exported name.

- [ ] **Step 2: Delete the old functions from main.go**

Remove all 11 functions listed above from `cmd/huan/main.go`. Keep their callers in `runBuild` intact — we'll update those next.

- [ ] **Step 3: Update call sites in main.go**

In `runBuild` (still in main.go until Task A3), every call to a moved function needs the `build.` prefix. Specifically:

| Old call | New call |
|---|---|
| `resolveRSSOutput(p)` | `build.ResolveRSSOutput(p)` |
| `buildSitemapContext(...)` | `build.BuildSitemapContext(...)` |
| `findHomeContext(lookup, site)` | `build.FindHomeContext(lookup, site)` |
| `filterMainSections(...)` | `build.FilterMainSections(...)` |
| `cloneContextForPagination(...)` | `build.CloneContextForPagination(...)` |
| `detectThemeName(sourceDir)` | `build.DetectThemeName(sourceDir)` |
| `resolveTemplateName(tmpls, p)` | `build.ResolveTemplateName(tmpls, p)` |

(`urlEscape` is called inside `BuildTermContext` and `BuildTaxonomyContext` — both already moved into context.go, so no main.go call sites remain.)

Add `net/url` removal if no other main.go code uses it (was only used by `urlEscape`).

- [ ] **Step 4: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles cleanly.

- [ ] **Step 5: Byte-for-byte regression check**

Run: `./scripts/diff-build.sh`
Expected: zero differences.

- [ ] **Step 6: Commit**

```bash
git add internal/build/context.go cmd/huan/main.go
git commit -m "refactor: move template/context helpers to internal/build"
```

---

### Task A3: Create `internal/build/build.go` with `BuildSite`

**Files:**
- Create: `internal/build/build.go`
- Modify: `cmd/huan/main.go` — `runBuild` shrinks to ~10 lines

- [ ] **Step 1: Create the new file with types**

Create `internal/build/build.go`:

```go
package build

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/novel_ttl/huan/internal/config"
	"github.com/novel_ttl/huan/internal/content"
	"github.com/novel_ttl/huan/internal/encrypt"
	"github.com/novel_ttl/huan/internal/i18n"
	"github.com/novel_ttl/huan/internal/markdown"
	"github.com/novel_ttl/huan/internal/output"
	"github.com/novel_ttl/huan/internal/shortcode"
	"github.com/novel_ttl/huan/internal/taxonomy"
	tmpl "github.com/novel_ttl/huan/internal/template"
)

// Options controls a single BuildSite invocation.
type Options struct {
	SourceDir        string
	OutputDir        string // absolute path
	IncludeDrafts    bool
	InjectLiveReload bool   // serve-only; when true, LiveReloadURL must be set
	LiveReloadURL    string // e.g. "ws://localhost:1313/livereload"; empty disables injection
	Logf             func(format string, args ...any)
}

// Result reports what happened during the build.
type Result struct {
	PagesRendered int
	FilesWritten  int
	BytesWritten  int64
	Errors        int
	Duration      time.Duration
}

func (o *Options) logf() func(string, ...any) {
	if o.Logf == nil {
		return func(format string, args ...any) { fmt.Printf(format, args...) }
	}
	return o.Logf
}
```

- [ ] **Step 2: Move `runBuild` body into `BuildSite`**

Cut the entire body of `runBuild` in `cmd/huan/main.go` (from line 57 `cfg, err := config.Load(sourceDir)` through line 424 `return nil`), and paste it as the body of a new function in `internal/build/build.go`:

```go
// BuildSite renders the full site from sourceDir into outputDir.
// Behavior matches the existing huan build command for byte-level Hugo parity.
func BuildSite(opts Options) (*Result, error) {
	start := time.Now()
	r := &Result{}
	logf := opts.logf()

	cfg, err := config.Load(opts.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	logf("Building site: %s\n", cfg.Title)
	logf("  Source:      %s\n", opts.SourceDir)
	logf("  Output:      %s\n", opts.OutputDir)
	logf("  BaseURL:     %s\n", cfg.BaseURL)

	// ... (entire body of the original runBuild goes here, with these substitutions) ...
	//   sourceDir       -> opts.SourceDir
	//   filepath.Join(sourceDir, cfg.PublishDir) -> opts.OutputDir
	//   build.*         -> direct calls (same package now)
	//   TruncateHTMLByWords / CountWordsInPlain / StripHTMLTagsForSummary -> direct
	//
	// Drafts handling: replace `if p.Draft { continue }` (around line 237) with:
	//   if p.Draft && !opts.IncludeDrafts { continue }
	//
	// (Task E1 will add LiveReload injection later — do NOT add any injection code here.)
	//
	// At the very end before returning, populate r:
	r.Duration = time.Since(start)
	return r, nil
}
```

**Concrete edits required during the move:**

1. Every reference to `sourceDir` becomes `opts.SourceDir`. There are ~12 such references in the body (config.Load, content.LoadDir, content.LoadDataFiles, filepath.Join for content/data/i18n/themes/static, detectThemeName, tmpl.LoadAllTemplates).

2. The writer instantiation (around line 211-216):
   ```go
   var writer *output.Writer
   if cfg.Minify {
       writer = output.NewWriterWithMinify(opts.OutputDir)
   } else {
       writer = output.NewWriter(opts.OutputDir)
   }
   ```
   No more `filepath.Join(sourceDir, cfg.PublishDir)` — `opts.OutputDir` is already absolute.

3. Drafts skip (line 237): `if p.Draft { continue }` → `if p.Draft && !opts.IncludeDrafts { continue }`

4. The final stats logging (lines 416-423) — populate Result fields. Replace:
   ```go
   files, bytes := writer.Stats()
   fmt.Printf("  Rendered:     %d pages\n", renderedCount)
   // ...
   ```
   with:
   ```go
   files, bytes := writer.Stats()
   r.FilesWritten = files
   r.BytesWritten = bytes
   r.PagesRendered = renderedCount
   r.Errors = errors
   logf("  Rendered:     %d pages\n", renderedCount)
   logf("  Output:       %d files, %.1f KB\n", files, float64(bytes)/1024)
   if errors > 0 {
       logf("  Errors:       %d\n", errors)
   }
   logf("Build complete.\n")
   ```

5. Drop the `fmt.Println("Build complete.")` standalone line — it's now in logf.

- [ ] **Step 3: Replace `runBuild` in main.go**

Replace the entire `runBuild` function in `cmd/huan/main.go` with:

```go
func runBuild(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	outputDir := filepath.Join(sourceDir, cfg.PublishDir)

	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")
	_, err = build.BuildSite(build.Options{
		SourceDir:     sourceDir,
		OutputDir:     outputDir,
		IncludeDrafts: includeDrafts,
	})
	return err
}
```

- [ ] **Step 4: Add `-D/--buildDrafts` flag to build command**

In `cmd/huan/main.go` where `buildCmd` is defined (around line 35-39), add the flag:

```go
buildCmd := &cobra.Command{
	Use:   "build",
	Short: "Build the site",
	RunE:  runBuild,
}
buildCmd.Flags().BoolP("buildDrafts", "D", false, "include draft content")
```

- [ ] **Step 5: Remove now-unused imports from main.go**

`main.go` no longer directly uses many imports — they live in `internal/build/build.go` now. Remove unused imports. Likely to remove from main.go's import block:
- `html/template`
- `os`
- `strings`
- `time` (if not used elsewhere)
- All the `internal/*` imports except `config` and `build`

Keep:
- `fmt`
- `path/filepath`
- `github.com/novel_ttl/huan/internal/build`
- `github.com/novel_ttl/huan/internal/config`
- `github.com/spf13/cobra`

After editing, run `goimports -w cmd/huan/main.go` if available, or manually fix imports by reading the compile errors.

- [ ] **Step 6: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles cleanly.

- [ ] **Step 7: Byte-for-byte regression check**

Run: `./scripts/diff-build.sh`
Expected: zero differences.

If diff fails: re-read main.go at the lines where the body was cut. Common bug sources:
- Forgot to convert some `sourceDir` reference
- Drafts handling broke existing behavior (shouldn't, since `IncludeDrafts=false` matches old `if p.Draft { continue }`)

- [ ] **Step 8: Commit**

```bash
git add internal/build/build.go cmd/huan/main.go
git commit -m "refactor: extract BuildSite into internal/build"
```

---

### Task A4: Verify Phase A complete

- [ ] **Step 1: Full test suite**

Run: `go test ./...`
Expected: PASS (no new tests, existing tests in pagination/shortcode/taxonomy/encrypt still green).

- [ ] **Step 2: Final diff sanity check**

Run: `./scripts/diff-build.sh`
Expected: zero differences vs Hugo baseline.

- [ ] **Step 3: Manual smoke test of build**

Run: `./huan build -s /Users/rong.zhu/Code/zhurongshuo`
Expected: builds without errors; outputs to `docs/` directory; terminal output looks similar to before refactor.

- [ ] **Step 4: main.go size check**

Run: `wc -l cmd/huan/main.go`
Expected: under 100 lines (was 858). The bulk is now in `internal/build/`.

No commit — this task is verification only.

---

## Phase B — Add dependencies

### Task B1: Add fsnotify and coder/websocket

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add deps**

Run:
```bash
go get github.com/fsnotify/fsnotify@v1.7.0
go get github.com/coder/websocket@v1.8.12
```

(Use whatever the latest 1.8.x is at run time; v1.8.12 is the version known at plan-writing time.)

- [ ] **Step 2: Verify go.mod**

Run: `grep -E "fsnotify|coder/websocket" go.mod`
Expected: both packages appear under `require`.

- [ ] **Step 3: Tidy**

Run: `go mod tidy`
Expected: no errors.

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: compiles. (The new deps aren't imported yet — that's fine.)

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add fsnotify and coder/websocket for serve"
```

---

## Phase C — Serve scaffolding (HTTP server)

Goal: get a minimal `huan serve` that builds once into a temp dir and serves over HTTP. No watch, no LiveReload yet.

### Task C1: Create `internal/serve/server.go` skeleton

**Files:**
- Create: `internal/serve/server.go`
- Test: `internal/serve/server_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/serve/server_test.go`:

```go
package serve

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeStaticFixture writes a couple of files into dir to serve.
func writeStaticFixture(t *testing.T, dir string) {
	t.Helper()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		mkdir := filepath.Dir(path)
		if err := osMkdirAll(mkdir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", mkdir, err)
		}
		if err := osWriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write("index.html", "<html><body>hello</body></html>")
	write("posts/foo.html", "<html><body>post foo</body></html>")
}

// osMkdirAll / osWriteFile are package-level vars so tests can intercept if needed.
// Define them in server.go (see Step 3).
```

Hmm — that's overengineered. Drop the indirection. Use `os.MkdirAll` and `os.WriteFile` directly:

```go
package serve

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeStaticFixture(t *testing.T, dir string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mustWrite("index.html", "<html><body>hello</body></html>")
	mustWrite("posts/foo.html", "<html><body>post foo</body></html>")
}

func TestServerServesStaticFiles(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	writeStaticFixture(t, tmp)

	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0", // :0 = OS picks free port
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrCh := make(chan string, 1)
	srv.addrCh = addrCh // for test: server publishes actual listen addr

	go srv.Run(ctx) //nolint:errcheck

	select {
	case addr := <-addrCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start within 2s")
	}

	addr := <-addrCh
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello") {
		t.Errorf("body = %q, want contains 'hello'", string(body))
	}

	resp2, err := http.Get("http://" + addr + "/posts/foo.html")
	if err != nil {
		t.Fatalf("get /posts/foo.html: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(body2), "post foo") {
		t.Errorf("body = %q, want contains 'post foo'", string(body2))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/serve/ -run TestServerServesStaticFiles -v`
Expected: FAIL — `undefined: New`, `undefined: ServerOptions`, etc.

- [ ] **Step 3: Write minimal server.go**

Create `internal/serve/server.go`:

```go
package serve

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

// ServerOptions configures a Server.
type ServerOptions struct {
	OutputDir string
	Bind      string
	Port      string // ":0" makes the OS pick a free port
	Logf      func(format string, args ...any)
}

// Server serves the built site over HTTP.
type Server struct {
	opts   ServerOptions
	logf   func(string, ...any)
	addrCh chan string // optional: tests read actual listen addr from here
}

func New(opts ServerOptions) *Server {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Server{opts: opts, logf: opts.Logf}
}

// Run blocks until ctx is cancelled or a SIGINT/SIGTERM arrives.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(s.opts.Bind, s.opts.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(s.opts.OutputDir)))

	httpSrv := &http.Server{Handler: mux}

	if s.addrCh != nil {
		s.addrCh <- listener.Addr().String()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s.logf("Serving at: http://%s/\n", listener.Addr())
		if err := httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logf("serve error: %v\n", err)
		}
	}()

	select {
	case <-ctx.Done():
	case <-sigCh:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}
```

Add the missing `time` import. (Editor's note: include `"time"` in the imports.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/serve/ -run TestServerServesStaticFiles -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/serve/server.go internal/serve/server_test.go
git commit -m "feat(serve): HTTP static file server skeleton"
```

---

### Task C2: Wire `runServe` to build + serve

**Files:**
- Create: `cmd/huan/serve.go`
- Modify: `cmd/huan/main.go` (remove stub `runServe`)

- [ ] **Step 1: Create serve.go**

Create `cmd/huan/serve.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/novel_ttl/huan/internal/build"
	"github.com/novel_ttl/huan/internal/config"
	"github.com/novel_ttl/huan/internal/serve"
	"github.com/spf13/cobra"
)

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	port, _ := cmd.Flags().GetString("port")
	bind, _ := cmd.Flags().GetString("bind")

	// Serve uses a temp directory, never the real publishDir (docs/).
	tmpDir, err := os.MkdirTemp("", "huan-serve-*")
	if err != nil {
		return fmt.Errorf("mkdtemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	includeDrafts, _ := cmd.Flags().GetBool("buildDrafts")

	if _, err := build.BuildSite(build.Options{
		SourceDir:     sourceDir,
		OutputDir:     tmpDir,
		IncludeDrafts: includeDrafts,
	}); err != nil {
		return err
	}

	fmt.Printf("Output:      %s\n", tmpDir)
	fmt.Printf("Serving at:  http://%s:%s/\n", bind, port)

	srv := serve.New(serve.ServerOptions{
		OutputDir: tmpDir,
		Bind:      bind,
		Port:      port,
		Logf:      func(format string, a ...any) { fmt.Printf(format, a...) },
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return srv.Run(ctx)
}
```

- [ ] **Step 2: Remove stub from main.go**

In `cmd/huan/main.go`, delete the entire `runServe` function (lines 842-858 per spec).

- [ ] **Step 3: Update serveCmd registration in main.go**

The `serveCmd` already exists with `--port` flag (lines 41-49 of original main.go). Add `--bind` and `-D/--buildDrafts`:

```go
serveCmd.Flags().String("port", "1313", "port to serve on")
serveCmd.Flags().String("bind", "127.0.0.1", "interface to bind")
serveCmd.Flags().BoolP("buildDrafts", "D", false, "include draft content")
```

- [ ] **Step 4: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles.

- [ ] **Step 5: Manual smoke test**

Run in one terminal:
```bash
./huan serve -s /Users/rong.zhu/Code/zhurongshuo
```
Expected: builds once, prints "Serving at: http://127.0.0.1:1313/", blocks.

In another terminal:
```bash
curl -s http://127.0.0.1:1313/ | head -5
```
Expected: HTML head of the site.

Ctrl+C in the first terminal: process exits, temp dir cleaned up (verify with `ls /var/folders/*/T/huan-serve-* 2>/dev/null` returning nothing).

- [ ] **Step 6: Commit**

```bash
git add cmd/huan/serve.go cmd/huan/main.go
git commit -m "feat(serve): wire build + HTTP serve (no watch yet)"
```

---

## Phase D — Vendor livereload.js

### Task D1: Fetch and embed livereload.js

**Files:**
- Create: `internal/serve/livereload.js`
- Create: `internal/serve/embed.go`

- [ ] **Step 1: Fetch the file**

Run:
```bash
cd /Users/rong.zhu/Code/huan/internal/serve
curl -fsSL -o livereload.js \
  https://raw.githubusercontent.com/livereload/livereload-js/v4.1.0/dist/livereload.js
```

(Use v4.1.0 or whatever the latest v4.x tag is — check `git ls-remote --tags https://github.com/livereload/livereload-js` first.)

- [ ] **Step 2: Compute SHA256**

Run: `shasum -a 256 livereload.js`
Record the hash for the file header comment.

- [ ] **Step 3: Prepend header comment**

Add at the very top of `internal/serve/livereload.js`:

```javascript
// Vendored from livereload-js v4.1.0 dist/livereload.js
// Source: https://github.com/livereload/livereload-js/tree/v4.1.0
// SHA256: <paste hash here>
// License: MIT (https://github.com/livereload/livereload-js/blob/v4.1.0/LICENSE)
```

Then the original file contents follow.

- [ ] **Step 4: Create embed.go**

Create `internal/serve/embed.go`:

```go
package serve

import _ "embed"

//go:embed livereload.js
var livereloadJS []byte
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: compiles.

- [ ] **Step 6: Commit**

```bash
git add internal/serve/livereload.js internal/serve/embed.go
git commit -m "feat(serve): vendor livereload.js v4"
```

---

### Task D2: Serve `/livereload.js` route

**Files:**
- Modify: `internal/serve/server.go`
- Test: `internal/serve/server_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/serve/server_test.go`:

```go
func TestServerServesLivereloadJS(t *testing.T) {
	tmp, err := os.MkdirTemp("", "huan-serve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	srv := New(ServerOptions{
		OutputDir: tmp,
		Bind:      "127.0.0.1",
		Port:      "0",
	})
	addrCh := make(chan string, 2)
	srv.addrCh = addrCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx) //nolint:errcheck

	<-addrCh // wait for first addr publication

	addr := <-addrCh
	resp, err := http.Get("http://" + addr + "/livereload.js")
	if err != nil {
		t.Fatalf("get /livereload.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "LiveReload") {
		t.Errorf("body does not look like livereload.js (no 'LiveReload' substring, len=%d)", len(body))
	}
}
```

- [ ] **Step 2: Run test, verify FAIL**

Run: `go test ./internal/serve/ -run TestServerServesLivereloadJS -v`
Expected: FAIL — 404 for `/livereload.js`.

- [ ] **Step 3: Add route in server.go**

In `internal/serve/server.go`, modify the `Run` method's mux setup:

```go
mux := http.NewServeMux()
mux.HandleFunc("/livereload.js", func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(livereloadJS)
})
mux.Handle("/", http.FileServer(http.Dir(s.opts.OutputDir)))
```

- [ ] **Step 4: Run test, verify PASS**

Run: `go test ./internal/serve/ -run TestServerServesLivereloadJS -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/serve/server.go internal/serve/server_test.go
git commit -m "feat(serve): route /livereload.js"
```

---

## Phase E — Inject LiveReload script at build time

### Task E1: Implement injection in `BuildSite`

**Files:**
- Modify: `internal/build/build.go`
- Test: `internal/build/build_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/build/build_test.go`:

```go
package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSiteInjectsLiveReload(t *testing.T) {
	// Minimal fixture: a config + one content file
	src, err := os.MkdirTemp("", "huan-build-test-src-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(src)

	out, err := os.MkdirTemp("", "huan-build-test-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(out)

	// Write minimal huan.yaml
	if err := os.WriteFile(filepath.Join(src, "huan.yaml"), []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "content", "_index.md"), []byte("# Home\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = BuildSite(Options{
		SourceDir:        src,
		OutputDir:        out,
		InjectLiveReload: true,
		LiveReloadURL:    "ws://localhost:1313/livereload",
		Logf:             func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("BuildSite: %v", err)
	}

	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(html), `src="/livereload.js?mindelay=10&v=2"`) {
		t.Errorf("index.html does not contain livereload script tag:\n%s", string(html))
	}
	if !strings.Contains(string(html), "ws://localhost:1313/livereload") {
		t.Errorf("index.html does not contain ws URL")
	}
}

const minimalConfig = `baseURL: "http://localhost:1313/"
title: "Test"
languageCode: "en-us"
publishDir: "public"
paginate: 10
minify: false
hasCJKLanguage: false
summaryLength: 70
enableEmoji: true

params:
  mainSections: ["posts"]
`
```

Note: if the fixture is too minimal to build (templates missing), expand it — copy minimal theme templates from a fixture dir. The point of this test is **verifying injection happened**, not exercising the full build pipeline. If the fixture is too painful to construct, switch to an integration test that uses the real zhurongshuo project — see Task K1.

- [ ] **Step 2: Run test, verify FAIL**

Run: `go test ./internal/build/ -run TestBuildSiteInjectsLiveReload -v`
Expected: FAIL — script tag not in output.

- [ ] **Step 3: Implement injection**

In `internal/build/build.go`, find the page-rendering loop. After each HTML render (around the existing `html, err := renderer.Render(tmplName, ctx)`), add injection:

```go
html, err := renderer.Render(tmplName, ctx)
if err != nil {
    // ... existing error handling
    continue
}

if opts.InjectLiveReload && opts.LiveReloadURL != "" {
    html = injectLiveReload(html, opts.LiveReloadURL)
}
```

Add the helper function in `internal/build/build.go`:

```go
// injectLiveReload inserts the livereload <script> before </head>.
// If </head> is absent, falls back to before </body>.
func injectLiveReload(html, wsURL string) string {
	tag := `<script src="/livereload.js?mindelay=10&v=2" data-livereload-port="` +
		portFromURL(wsURL) + `" data-livereload-host="` + hostFromURL(wsURL) +
		`"></script>`
	if idx := strings.Index(html, "</head>"); idx >= 0 {
		return html[:idx] + tag + html[idx:]
	}
	if idx := strings.Index(html, "</body>"); idx >= 0 {
		return html[:idx] + tag + html[idx:]
	}
	return html + tag
}

func portFromURL(wsURL string) string {
	// ws://localhost:1313/livereload -> 1313
	rest := strings.TrimPrefix(strings.TrimPrefix(wsURL, "ws://"), "wss://")
	rest = strings.TrimPrefix(rest, "//")
	host := rest
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		host = rest[:idx]
	}
	if idx := strings.LastIndexByte(host, ':'); idx >= 0 {
		return host[idx+1:]
	}
	return "1313"
}

func hostFromURL(wsURL string) string {
	rest := strings.TrimPrefix(strings.TrimPrefix(wsURL, "ws://"), "wss://")
	rest = strings.TrimPrefix(rest, "//")
	host := rest
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		host = rest[:idx]
	}
	if idx := strings.LastIndexByte(host, ':'); idx >= 0 {
		host = host[:idx]
	}
	return host
}
```

- [ ] **Step 4: Run test, verify PASS**

Run: `go test ./internal/build/ -run TestBuildSiteInjectsLiveReload -v`
Expected: PASS.

- [ ] **Step 5: Verify no regression on normal build**

Run: `./scripts/diff-build.sh`
Expected: zero differences (the build command does not set InjectLiveReload).

- [ ] **Step 6: Commit**

```bash
git add internal/build/build.go internal/build/build_test.go
git commit -m "feat(build): inject livereload script when InjectLiveReload=true"
```

---

### Task E2: Pass LiveReload options through `runServe`

**Files:**
- Modify: `cmd/huan/serve.go`

- [ ] **Step 1: Update BuildSite call in serve.go**

In `cmd/huan/serve.go`, change the BuildSite call to include LiveReload options:

```go
disableLR, _ := cmd.Flags().GetBool("disableLiveReload")
lrURL := ""
injectLR := false
if !disableLR {
	injectLR = true
	lrURL = "ws://" + bind + ":" + port + "/livereload"
}

if _, err := build.BuildSite(build.Options{
	SourceDir:        sourceDir,
	OutputDir:        tmpDir,
	IncludeDrafts:    includeDrafts,
	InjectLiveReload: injectLR,
	LiveReloadURL:    lrURL,
}); err != nil {
	return err
}
```

Also add `--disableLiveReload` flag registration in main.go's serveCmd setup:

```go
serveCmd.Flags().Bool("disableLiveReload", false, "disable browser auto-refresh")
```

- [ ] **Step 2: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles.

- [ ] **Step 3: Manual smoke test**

Run: `./huan serve -s /Users/rong.zhu/Code/zhurongshuo`
In another terminal: `curl -s http://127.0.0.1:1313/ | grep livereload`
Expected: shows the injected script tag.

- [ ] **Step 4: Commit**

```bash
git add cmd/huan/serve.go cmd/huan/main.go
git commit -m "feat(serve): pass LiveReload options to BuildSite"
```

---

## Phase F — File watcher

### Task F1: Create `internal/serve/watcher.go` with debounce

**Files:**
- Create: `internal/serve/watcher.go`
- Test: `internal/serve/watcher_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/serve/watcher_test.go`:

```go
package serve

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherFiresOnChange(t *testing.T) {
	dir, err := os.MkdirTemp("", "huan-watch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: dir,
		Debounce:  50 * time.Millisecond,
		OnChange: func() {
			atomic.AddInt32(&calls, 1)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	// Give watcher time to install hooks
	time.Sleep(100 * time.Millisecond)

	// Write a file
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing
	time.Sleep(200 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got < 1 {
		t.Errorf("OnChange called %d times, want >= 1", got)
	}
}

func TestWatcherDebouncesBursts(t *testing.T) {
	dir, err := os.MkdirTemp("", "huan-watch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	var calls int32
	w, err := NewWatcher(WatcherOptions{
		SourceDir: dir,
		Debounce:  100 * time.Millisecond,
		OnChange: func() {
			atomic.AddInt32(&calls, 1)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	// 5 rapid writes to the same file
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(filepath.Join(dir, "burst.txt"), []byte("x"), 0o644)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait past debounce
	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("OnChange called %d times, want exactly 1 (after debounce)", got)
	}
}
```

- [ ] **Step 2: Run test, verify FAIL**

Run: `go test ./internal/serve/ -run TestWatcher -v`
Expected: FAIL — `undefined: NewWatcher`.

- [ ] **Step 3: Implement watcher.go**

Create `internal/serve/watcher.go`:

```go
package serve

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatcherOptions configures a Watcher.
type WatcherOptions struct {
	SourceDir string
	Debounce  time.Duration
	OnChange  func()
	Logf      func(format string, args ...any)
}

// Watcher recursively watches SourceDir for changes and invokes OnChange
// after a debounce delay.
type Watcher struct {
	opts WatcherOptions
	fsw  *fsnotify.Watcher
	mu   sync.Mutex
	timer *time.Timer
}

func NewWatcher(opts WatcherOptions) (*Watcher, error) {
	if opts.Debounce == 0 {
		opts.Debounce = 400 * time.Millisecond
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{opts: opts, fsw: fsw}
	if err := w.addRecursive(opts.SourceDir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	return w, nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if w.isIgnored(path) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

func (w *Watcher) isIgnored(path string) bool {
	// Skip hidden dirs/files
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	return false
}

func (w *Watcher) Run(ctx context.Context) error {
	defer w.fsw.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if w.isIgnored(ev.Name) {
				continue
			}
			// If a new dir was created, watch it too
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(ev.Name)
				}
			}
			w.schedule()
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			w.opts.Logf("watcher error: %v\n", err)
		}
	}
}

func (w *Watcher) schedule() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.opts.Debounce, func() {
		if w.opts.OnChange != nil {
			w.opts.OnChange()
		}
	})
}
```

Add `"os"` to the imports (needed for `os.FileInfo` and `os.Stat`).

- [ ] **Step 4: Run tests, verify PASS**

Run: `go test ./internal/serve/ -run TestWatcher -v`
Expected: both tests PASS.

If flaky on CI: bump the test sleep durations — fsnotify on macOS can be slow to deliver events.

- [ ] **Step 5: Commit**

```bash
git add internal/serve/watcher.go internal/serve/watcher_test.go
git commit -m "feat(serve): recursive fsnotify watcher with debounce"
```

---

### Task F2: Wire watcher into `runServe`

**Files:**
- Modify: `cmd/huan/serve.go`
- Modify: `cmd/huan/main.go` (add `--debounce`, `--disableWatch` flags)

- [ ] **Step 1: Update serve.go to start watcher**

In `cmd/huan/serve.go`, after BuildSite, add watcher setup:

```go
disableWatch, _ := cmd.Flags().GetBool("disableWatch")
debounce, _ := cmd.Flags().GetDuration("debounce")

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if !disableWatch {
	watcher, err := serve.NewWatcher(serve.WatcherOptions{
		SourceDir: sourceDir,
		Debounce:  debounce,
		OnChange: func() {
			fmt.Println("[watch] change detected, rebuilding...")
			if _, err := build.BuildSite(build.Options{
				SourceDir:        sourceDir,
				OutputDir:        tmpDir,
				IncludeDrafts:    includeDrafts,
				InjectLiveReload: injectLR,
				LiveReloadURL:    lrURL,
				Logf:             func(format string, a ...any) { fmt.Printf(format, a...) },
			}); err != nil {
				fmt.Printf("[watch] rebuild error: %v\n", err)
				return
			}
			fmt.Println("[watch] rebuild complete")
			// LiveReload broadcast wired in Phase G
		},
		Logf: func(format string, a ...any) { fmt.Printf(format, a...) },
	})
	if err != nil {
		fmt.Printf("WARNING: file watcher unavailable: %v\n", err)
		fmt.Println("WARNING: use --disableWatch to suppress this message")
	} else {
		go watcher.Run(ctx) //nolint:errcheck
	}
}

fmt.Printf("Serving at:  http://%s:%s/\n", bind, port)
srv := serve.New(serve.ServerOptions{
	OutputDir: tmpDir,
	Bind:      bind,
	Port:      port,
	Logf:      func(format string, a ...any) { fmt.Printf(format, a...) },
})
return srv.Run(ctx)
```

Note: the OnChange closure references `injectLR` and `lrURL` from E2's edits. If those variable names differ in your serve.go, adjust.

- [ ] **Step 2: Add CLI flags**

In `cmd/huan/main.go` where serveCmd is configured:

```go
serveCmd.Flags().Duration("debounce", 400*time.Millisecond, "file change debounce delay")
serveCmd.Flags().Bool("disableWatch", false, "do not watch files for changes")
```

Add `"time"` import to main.go.

- [ ] **Step 3: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles.

- [ ] **Step 4: Manual smoke test**

Run: `./huan serve -s /Users/rong.zhu/Code/zhurongshuo`
In another terminal: `echo "test" >> /Users/rong.zhu/Code/zhurongshuo/content/posts/sample.md` (create or pick any existing md file)
Expected: terminal prints `[watch] change detected, rebuilding...` then `[watch] rebuild complete`.

- [ ] **Step 5: Commit**

```bash
git add cmd/huan/serve.go cmd/huan/main.go
git commit -m "feat(serve): wire file watcher to trigger rebuild"
```

---

## Phase G — LiveReload WebSocket

### Task G1: Create `internal/serve/livereload.go` hub

**Files:**
- Create: `internal/serve/livereload.go`
- Test: `internal/serve/livereload_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/serve/livereload_test.go`:

```go
package serve

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLiveReloadHubHandshakeAndReload(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a WS server on a free port
	listener := mustListenFreePort(t)
	serverReady := make(chan struct{})
	go func() {
		close(serverReady)
		_ = httpServeWS(ctx, listener, hub)
	}()
	<-serverReady

	// Connect a client
	c, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	// Expect hello from server
	_ = c.SetReadLimit(1 << 16)
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if !strings.Contains(string(msg), `"hello"`) {
		t.Errorf("first msg = %s, want hello", string(msg))
	}

	// Trigger a reload
	hub.BroadcastReload()

	// Expect reload msg
	_, msg, err = c.Read(ctx)
	if err != nil {
		t.Fatalf("read reload: %v", err)
	}
	if !strings.Contains(string(msg), `"reload"`) {
		t.Errorf("second msg = %s, want reload", string(msg))
	}
}
```

Helper functions for the test (put them in `livereload_test.go` or a shared `testing.go` file in the package):

```go
func mustListenFreePort(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func httpServeWS(ctx context.Context, ln net.Listener, hub *LiveReloadHub) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/livereload", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // test only; dev server uses loopback
		})
		if err != nil {
			return
		}
		defer c.CloseNow()
		hub.HandleConn(ctx, c)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	<-ctx.Done()
	return srv.Shutdown(context.Background())
}
```

Add imports: `"net"`, `"net/http"`, `"strings"`.

- [ ] **Step 2: Run test, verify FAIL**

Run: `go test ./internal/serve/ -run TestLiveReloadHub -v`
Expected: FAIL — `undefined: NewHub`, etc.

- [ ] **Step 3: Implement livereload.go**

Create `internal/serve/livereload.go`:

```go
package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type LiveReloadHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
	logf    func(format string, args ...any)
}

func NewHub() *LiveReloadHub {
	return &LiveReloadHub{
		clients: map[*websocket.Conn]struct{}{},
		logf:    func(string, ...any) {},
	}
}

// HandleConn runs the WS read loop for one client. Blocks until disconnect.
func (h *LiveReloadHub) HandleConn(ctx context.Context, c *websocket.Conn) {
	h.add(c)
	defer h.remove(c)

	// Send hello immediately.
	hello := map[string]any{
		"command":    "hello",
		"protocols":  []string{"http://livereload.com/protocols/official-7"},
		"serverName": "huan",
	}
	_ = h.writeJSON(ctx, c, hello)

	// Read loop (client may send hello back, or info messages)
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			return
		}
	}
}

func (h *LiveReloadHub) BroadcastReload() {
	h.broadcast(map[string]any{
		"command": "reload",
		"path":    "/",
		"liveCSS": true,
	})
}

func (h *LiveReloadHub) BroadcastAlert(message string) {
	h.broadcast(map[string]any{
		"command": "alert",
		"message": message,
	})
}

func (h *LiveReloadHub) broadcast(msg map[string]any) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_ = c.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}

func (h *LiveReloadHub) writeJSON(ctx context.Context, c *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, data)
}

func (h *LiveReloadHub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *LiveReloadHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = c.CloseNow()
}

// AcceptHTTP upgrades an HTTP request to a WebSocket and hands it to the hub.
// Convenience for wiring into a stdlib mux.
func (h *LiveReloadHub) AcceptHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow ws:// (not wss://) for localhost dev
	})
	if err != nil {
		return
	}
	defer c.CloseNow()
	h.HandleConn(r.Context(), c)
}
```

(If `fmt` isn't otherwise used in this file, drop the import and the dummy line. The Go compiler will tell you.)

- [ ] **Step 4: Run test, verify PASS**

Run: `go test ./internal/serve/ -run TestLiveReloadHub -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/serve/livereload.go internal/serve/livereload_test.go
git commit -m "feat(serve): LiveReload WebSocket hub with hello/reload/alert"
```

---

### Task G2: Wire `/livereload` WS route into server

**Files:**
- Modify: `internal/serve/server.go`
- Modify: `cmd/huan/serve.go` (call BroadcastReload after rebuild)

- [ ] **Step 1: Add hub field to Server**

In `internal/serve/server.go`, modify `ServerOptions` and `Server`:

```go
type ServerOptions struct {
	OutputDir string
	Bind      string
	Port      string
	Hub       *LiveReloadHub // optional; if nil, /livereload returns 404
	Logf      func(format string, args ...any)
}

type Server struct {
	opts   ServerOptions
	logf   func(string, ...any)
	addrCh chan string
	hub    *LiveReloadHub
}

func New(opts ServerOptions) *Server {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Server{
		opts: opts,
		logf: opts.Logf,
		hub:  opts.Hub,
	}
}
```

In `Run`, add the route:

```go
mux := http.NewServeMux()
mux.HandleFunc("/livereload.js", /* existing handler */)
if s.hub != nil {
	hub := s.hub
	mux.HandleFunc("/livereload", hub.AcceptHTTP)
}
mux.Handle("/", http.FileServer(http.Dir(s.opts.OutputDir)))
```

- [ ] **Step 2: Update serve.go to construct hub**

In `cmd/huan/serve.go`, create hub and wire to server + rebuild callback:

```go
var hub *serve.LiveReloadHub
if !disableLR {
	hub = serve.NewHub()
}

// In the OnChange callback, after successful rebuild:
if hub != nil {
	hub.BroadcastReload()
}
```

And pass hub to ServerOptions:

```go
srv := serve.New(serve.ServerOptions{
	OutputDir: tmpDir,
	Bind:      bind,
	Port:      port,
	Hub:       hub,
	Logf:      func(format string, a ...any) { fmt.Printf(format, a...) },
})
```

- [ ] **Step 3: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles.

- [ ] **Step 4: Manual smoke test (full LiveReload path)**

Run: `./huan serve -s /Users/rong.zhu/Code/zhurongshuo`
Open browser to `http://localhost:1313/`. Open dev tools Network tab. Confirm `livereload.js` loads and a WS connection to `/livereload` opens.

Edit any markdown file under `/Users/rong.zhu/Code/zhurongshuo/content/` and save.
Expected: browser auto-refreshes within ~1s.

- [ ] **Step 5: Commit**

```bash
git add internal/serve/server.go cmd/huan/serve.go
git commit -m "feat(serve): wire /livereload WS route and broadcast on rebuild"
```

---

## Phase H — Build error → broadcast alert

### Task H1: On rebuild error, broadcast alert (don't crash)

**Files:**
- Modify: `cmd/huan/serve.go`

- [ ] **Step 1: Update OnChange callback**

In `cmd/huan/serve.go`, the OnChange callback already catches the build error. Add an alert broadcast:

```go
OnChange: func() {
	fmt.Println("[watch] change detected, rebuilding...")
	if _, err := build.BuildSite(build.Options{
		// ... same as before
	}); err != nil {
		fmt.Printf("[watch] rebuild error: %v\n", err)
		if hub != nil {
			hub.BroadcastAlert(fmt.Sprintf("huan rebuild error: %v", err))
		}
		return
	}
	fmt.Println("[watch] rebuild complete")
	if hub != nil {
		hub.BroadcastReload()
	}
},
```

- [ ] **Step 2: Build**

Run: `go build -o huan ./cmd/huan`
Expected: compiles.

- [ ] **Step 3: Manual smoke test**

Run `./huan serve -s /Users/rong.zhu/Code/zhurongshuo`. Edit a markdown file to introduce invalid frontmatter (e.g., add a stray `:` to the YAML). Save.
Expected: terminal shows error; browser shows alert popup. Fix the file, save again → browser reloads normally.

- [ ] **Step 4: Commit**

```bash
git add cmd/huan/serve.go
git commit -m "feat(serve): broadcast alert on rebuild error"
```

---

## Phase I — Port conflict handling

### Task I1: Detect port-in-use and exit with helpful message

**Files:**
- Modify: `internal/serve/server.go`

- [ ] **Step 1: Update Run to surface port conflict**

In `internal/serve/server.go`'s `Run`, the `net.Listen` call already returns an error on conflict. Improve the error wrapping:

```go
listener, err := net.Listen("tcp", net.JoinHostPort(s.opts.Bind, s.opts.Port))
if err != nil {
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Op == "listen" {
		return fmt.Errorf("port %s already in use on %s (try --port <other>): %w",
			s.opts.Port, s.opts.Bind, err)
	}
	return fmt.Errorf("listen: %w", err)
}
```

(`errors` and `net` already imported.)

- [ ] **Step 2: Test manually**

Run two terminals:
```bash
./huan serve -s /Users/rong.zhu/Code/zhurongshuo            # terminal 1
./huan serve -s /Users/rong.zhu/Code/zhurongshuo            # terminal 2, same port
```
Expected: terminal 2 prints `ERROR: port 1313 already in use on 127.0.0.1 (try --port <other>): ...` and exits non-zero. Terminal 1 keeps running.

- [ ] **Step 3: Commit**

```bash
git add internal/serve/server.go
git commit -m "feat(serve): friendly error on port conflict"
```

---

## Phase J — End-to-end integration test

### Task J1: E2E test (serve + ws client + file change → reload received)

**Files:**
- Create: `internal/serve/e2e_test.go`

- [ ] **Step 1: Write the test**

Create `internal/serve/e2e_test.go`:

```go
package serve

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestE2EServeBuildWatchReload exercises the full pipeline:
// start server -> connect WS client -> modify file -> expect reload message.
//
// Uses a minimal source fixture to avoid depending on the zhurongshuo project.
func TestE2EServeBuildWatchReload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e in short mode")
	}

	src := mustWriteServeFixture(t)

	tmpOut, err := os.MkdirTemp("", "huan-e2e-out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpOut)

	// Build once
	if _, err := build.BuildSite(build.Options{
		SourceDir:        src,
		OutputDir:        tmpOut,
		InjectLiveReload: true,
		LiveReloadURL:    "ws://127.0.0.1:0/livereload", // port filled in after listen
		Logf:             func(string, ...any) {},
	}); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}

	hub := NewHub()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/livereload.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write(livereloadJS)
	})
	mux.HandleFunc("/livereload", hub.AcceptHTTP)
	mux.Handle("/", http.FileServer(http.Dir(tmpOut)))
	httpSrv := &http.Server{Handler: mux}
	go httpSrv.Serve(ln)
	defer httpSrv.Shutdown(context.Background())

	// Connect a client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://"+addr+"/livereload", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()

	// Read hello
	_, _, _ = c.Read(ctx)

	// Trigger reload
	hub.BroadcastReload()

	// Expect reload msg
	_ = c.SetReadLimit(1 << 16)
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read reload: %v", err)
	}
	if !strings.Contains(string(msg), `"reload"`) {
		t.Errorf("got %s, want reload", string(msg))
	}
}

func mustWriteServeFixture(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "huan-e2e-src-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	must := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("huan.yaml", minimalBuildConfig)
	must("content/_index.md", "# Home\n\nbody\n")
	must("layouts/_default/single.html", "<html><head><title>{{ .Title }}</title></head><body>{{ .Content }}</body></html>")
	must("layouts/_default/list.html", "<html><head><title>{{ .Title }}</title></head><body>{{ .Content }}</body></html>")
	must("layouts/index.html", "<html><head><title>{{ .Title }}</title></head><body>{{ .Content }}</body></html>")
	return dir
}

const minimalBuildConfig = `baseURL: "http://localhost:1313/"
title: "E2E Test"
languageCode: "en-us"
publishDir: "public"
paginate: 10
minify: false
hasCJKLanguage: false
summaryLength: 70
enableEmoji: true

params:
  mainSections: ["posts"]
`
```

(You'll need to import `"github.com/novel_ttl/huan/internal/build"` in the test file.)

**Note:** If the fixture is too minimal and BuildSite fails to render due to missing theme/templates, expand `mustWriteServeFixture` with more layouts. The test exists to verify wiring — adjust as needed.

- [ ] **Step 2: Run test**

Run: `go test ./internal/serve/ -run TestE2E -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/serve/e2e_test.go
git commit -m "test(serve): end-to-end build + serve + livereload"
```

---

## Phase K — Final verification

### Task K1: Acceptance checklist

- [ ] **Step 1: Full test suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 2: Hugo parity regression**

Run: `./scripts/diff-build.sh`
Expected: zero differences.

- [ ] **Step 3: Manual end-to-end against real project**

```bash
./huan serve -s /Users/rong.zhu/Code/zhurongshuo
```
Open `http://localhost:1313/` in a browser. Verify:
- Site loads
- A WS connection to `/livereload` appears in dev tools
- Editing any `.md` file under `content/` causes browser to refresh within ~1s
- Editing a `.go` file in theme templates (if zozo has any) causes refresh
- Editing `huan.yaml` causes refresh
- Ctrl+C cleans up (no orphan processes; temp dir gone)

- [ ] **Step 4: Verify flags**

Test each flag:
- `--port 8080` serves on 8080
- `--bind 0.0.0.0` is reachable from another machine on LAN (or just `curl http://$(hostname):1313/`)
- `-D` includes draft content (verify a known-draft page appears)
- `--disableLiveReload` serves without WS route (curl `/livereload` returns 404)
- `--disableWatch` serves without rebuilding on file change
- `--debounce 1s` makes rebuild wait 1s after last change

- [ ] **Step 5: main.go size final check**

Run: `wc -l cmd/huan/main.go`
Expected: under 80 lines.

- [ ] **Step 6: No commit**

This is verification only. The feature is complete.

---

## Summary of commits

After successful execution of this plan, you should have ~14 commits on top of `master`:

1. `refactor: move HTML summary helpers to internal/build`
2. `refactor: move template/context helpers to internal/build`
3. `refactor: extract BuildSite into internal/build`
4. `deps: add fsnotify and coder/websocket for serve`
5. `feat(serve): HTTP static file server skeleton`
6. `feat(serve): wire build + HTTP serve (no watch yet)`
7. `feat(serve): vendor livereload.js v4`
8. `feat(serve): route /livereload.js`
9. `feat(build): inject livereload script when InjectLiveReload=true`
10. `feat(serve): pass LiveReload options to BuildSite`
11. `feat(serve): recursive fsnotify watcher with debounce`
12. `feat(serve): wire file watcher to trigger rebuild`
13. `feat(serve): LiveReload WebSocket hub with hello/reload/alert`
14. `feat(serve): wire /livereload WS route and broadcast on rebuild`
15. `feat(serve): broadcast alert on rebuild error`
16. `feat(serve): friendly error on port conflict`
17. `test(serve): end-to-end build + serve + livereload`

(17 commits — slightly more than the ~14 estimate; OK.)
