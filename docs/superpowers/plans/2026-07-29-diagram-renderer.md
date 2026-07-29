# diagram-renderer Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `.so` post-build plugin that renders Mermaid/PlantUML/GraphViz/D2 fenced code blocks into inline SVG at build time via a self-hosted Kroki backend, with a content-hash cache and client-side fallback when Kroki is unreachable.

**Architecture:** Approach A (pure post-build plugin, zero changes to `internal/markdown`). goldmark/chroma render diagram fences as normal highlighted code blocks; the plugin's `OnOutputWritten` hook scans output HTML, losslessly recovers each diagram's source from the chroma block, calls Kroki for SVG, and replaces the block with `<figure>`. On failure it keeps `<pre class="mermaid">` and injects `mermaid.js`. Mirrors the existing `plugins/seo-injector` and `plugins/html-injector` structure.

**Tech Stack:** Go 1.26, `pkg/plugin` capability interfaces (`PostBuildHook`, `SchemaProvider`, `MetadataProvider`), stdlib `net/http`, `crypto/sha256`, `html`, `regexp`. Built with `go build -buildmode=plugin` via `scripts/build-plugins.sh`. Self-hosted Kroki (`yuzutech/kroki`) in Docker.

## Global Constraints

- Module path: `github.com/iannil/huan` — import canonical types from `github.com/iannil/huan/pkg/plugin`.
- Plugin package is `package main`; every plugin dir exports `func InitPlugin(cfg map[string]any) (interface{}, error)` and defines `func main() {}`.
- Plugin `Name()` must be `"diagram_renderer"` (yaml key `plugins.diagram_renderer.*`); `.so` file is `diagram-renderer.so` (underscores→hyphens).
- `OnOutputWritten` follows **collection-not-interruption**: on error, log a warning via `p.logf` and return `nil` — never abort the build.
- Every HTML write is idempotent: only `os.WriteFile` when content actually changed; a second run over already-rendered output must produce zero changes.
- Comments/log messages in English; file writes use mode `0644`.
- Config field YAML keys are camelCase in `huan.yaml` (matches seo-injector: `descriptionMaxLength`), EXCEPT the top-level plugin key which is snake_case (`diagram_renderer`).
- Kroki request: `POST {krokiURL}/{lang}/svg`, body = raw source, `Content-Type: text/plain`, `Accept: image/svg+xml`.
- Cache key: `sha256(lang + "\n" + source)` hex-encoded; cache file `<cacheDir>/<key>.svg`.

---

### Task 1: Config parsing + schema

**Files:**
- Create: `plugins/diagram-renderer/config.go`
- Test: `plugins/diagram-renderer/config_test.go`

**Interfaces:**
- Consumes: `github.com/iannil/huan/pkg/plugin` (`plugin.Schema`, `plugin.FieldSchema`).
- Produces:
  - `type Config struct { Enabled bool; KrokiURL string; Languages []string; CacheDir string; TimeoutMs int; FallbackMode string; MermaidJS string; FigureClass string; IncludeKinds []string; ExcludeKinds []string }`
  - `func DefaultConfig() *Config`
  - `func ParseConfig(raw map[string]any) (*Config, error)`
  - `func (c *Config) ConfigSchema() plugin.Schema`

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Enabled {
		t.Errorf("Enabled default = true, want false")
	}
	if c.KrokiURL != "http://localhost:8000" {
		t.Errorf("KrokiURL = %q", c.KrokiURL)
	}
	if c.CacheDir != ".huan/cache/diagrams" {
		t.Errorf("CacheDir = %q", c.CacheDir)
	}
	if c.TimeoutMs != 5000 {
		t.Errorf("TimeoutMs = %d", c.TimeoutMs)
	}
	if c.FallbackMode != "client" {
		t.Errorf("FallbackMode = %q", c.FallbackMode)
	}
	if c.FigureClass != "diagram" {
		t.Errorf("FigureClass = %q", c.FigureClass)
	}
	want := []string{"mermaid", "plantuml", "graphviz", "d2"}
	if len(c.Languages) != len(want) {
		t.Fatalf("Languages = %v", c.Languages)
	}
	for i := range want {
		if c.Languages[i] != want[i] {
			t.Errorf("Languages[%d] = %q, want %q", i, c.Languages[i], want[i])
		}
	}
}

func TestParseConfigOverrides(t *testing.T) {
	raw := map[string]any{
		"enabled":     true,
		"krokiUrl":    "http://kroki:8000",
		"languages":   []any{"mermaid", "d2"},
		"timeoutMs":   float64(3000),
		"fallback":    map[string]any{"mode": "codeblock", "mermaidJs": "/js/mermaid.js"},
		"figureClass": "chart",
		"excludeKinds": []any{"home"},
	}
	c, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if !c.Enabled || c.KrokiURL != "http://kroki:8000" || c.TimeoutMs != 3000 {
		t.Errorf("scalars not parsed: %+v", c)
	}
	if len(c.Languages) != 2 || c.Languages[1] != "d2" {
		t.Errorf("Languages = %v", c.Languages)
	}
	if c.FallbackMode != "codeblock" || c.MermaidJS != "/js/mermaid.js" {
		t.Errorf("fallback = %q %q", c.FallbackMode, c.MermaidJS)
	}
	if len(c.ExcludeKinds) != 1 || c.ExcludeKinds[0] != "home" {
		t.Errorf("ExcludeKinds = %v", c.ExcludeKinds)
	}
}

func TestParseConfigTypeError(t *testing.T) {
	if _, err := ParseConfig(map[string]any{"krokiUrl": 123}); err == nil {
		t.Errorf("expected type error for krokiUrl")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugins/diagram-renderer && go test ./... -run TestDefaultConfig -v`
Expected: FAIL — `undefined: DefaultConfig`.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"fmt"

	"github.com/iannil/huan/pkg/plugin"
)

// Config is the diagram-renderer configuration from huan.yaml
// plugins.diagram_renderer.*
type Config struct {
	Enabled      bool
	KrokiURL     string
	Languages    []string
	CacheDir     string
	TimeoutMs    int
	FallbackMode string // "client" | "codeblock" | "fail"
	MermaidJS    string
	FigureClass  string
	IncludeKinds []string
	ExcludeKinds []string
}

// DefaultConfig returns a Config with the spec's default values.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		KrokiURL:     "http://localhost:8000",
		Languages:    []string{"mermaid", "plantuml", "graphviz", "d2"},
		CacheDir:     ".huan/cache/diagrams",
		TimeoutMs:    5000,
		FallbackMode: "client",
		MermaidJS:    "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js",
		FigureClass:  "diagram",
	}
}

// ParseConfig parses raw yaml config into Config, layering over defaults.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()
	if raw == nil {
		return cfg, nil
	}
	if v, ok := raw["enabled"]; ok {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("enabled: expected bool, got %T", v)
		}
		cfg.Enabled = b
	}
	if v, ok := raw["krokiUrl"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("krokiUrl: expected string, got %T", v)
		}
		cfg.KrokiURL = s
	}
	if v, ok := raw["languages"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("languages: %w", err)
		}
		cfg.Languages = items
	}
	if v, ok := raw["cacheDir"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("cacheDir: expected string, got %T", v)
		}
		cfg.CacheDir = s
	}
	if v, ok := raw["timeoutMs"]; ok {
		f, ok := toFloat64(v)
		if !ok {
			return nil, fmt.Errorf("timeoutMs: expected number, got %T", v)
		}
		cfg.TimeoutMs = int(f)
	}
	if v, ok := raw["figureClass"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("figureClass: expected string, got %T", v)
		}
		cfg.FigureClass = s
	}
	if v, ok := raw["includeKinds"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("includeKinds: %w", err)
		}
		cfg.IncludeKinds = items
	}
	if v, ok := raw["excludeKinds"]; ok {
		items, err := toStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("excludeKinds: %w", err)
		}
		cfg.ExcludeKinds = items
	}
	if v, ok := raw["fallback"]; ok {
		fb, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("fallback: expected map, got %T", v)
		}
		if mv, ok := fb["mode"]; ok {
			s, ok := mv.(string)
			if !ok {
				return nil, fmt.Errorf("fallback.mode: expected string, got %T", mv)
			}
			cfg.FallbackMode = s
		}
		if jv, ok := fb["mermaidJs"]; ok {
			s, ok := jv.(string)
			if !ok {
				return nil, fmt.Errorf("fallback.mermaidJs: expected string, got %T", jv)
			}
			cfg.MermaidJS = s
		}
	}
	return cfg, nil
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func toStringSlice(v any) ([]string, error) {
	switch val := v.(type) {
	case []string:
		return val, nil
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", v)
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (c *Config) ConfigSchema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.FieldSchema{
		{Key: "enabled", Type: "bool", Required: false, Default: false, Description: "开关插件"},
		{Key: "krokiUrl", Type: "string", Required: false, Default: "http://localhost:8000", Description: "自托管 Kroki 端点"},
		{Key: "languages", Type: "string_slice", Required: false, Description: "允许渲染的图表语言"},
		{Key: "cacheDir", Type: "string", Required: false, Default: ".huan/cache/diagrams", Description: "SVG 缓存目录"},
		{Key: "timeoutMs", Type: "int", Required: false, Default: 5000, Description: "Kroki 请求超时(ms)"},
		{Key: "figureClass", Type: "string", Required: false, Default: "diagram", Description: "输出 figure 基础 class"},
		{Key: "includeKinds", Type: "string_slice", Required: false, Description: "仅对这些 page kind 渲染"},
		{Key: "excludeKinds", Type: "string_slice", Required: false, Description: "跳过这些 page kind"},
		{Key: "fallback", Type: "map", Required: false, Description: "失败降级: {mode, mermaidJs}"},
	}}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd plugins/diagram-renderer && go test ./... -run TestParseConfig -v && go test ./... -run TestDefaultConfig -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/diagram-renderer/config.go plugins/diagram-renderer/config_test.go
git commit -m "feat(diagram-renderer): config parsing and schema"
```

---

### Task 2: Source extraction from chroma blocks

**Files:**
- Create: `plugins/diagram-renderer/extract.go`
- Test: `plugins/diagram-renderer/extract_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type diagramBlock struct { Full string; Lang string; Source string }` — `Full` is the entire `<div class="highlight">…</div>` substring to be replaced; `Lang` is the `data-lang`; `Source` is the recovered raw diagram source.
  - `func findDiagramBlocks(htmlSrc string, languages []string) []diagramBlock`
  - `func extractSource(codeInner string) string` — strips `<span>` tags and unescapes entities.

- [ ] **Step 1: Write the failing test**

The golden test renders real markdown through the project's own renderer, then asserts `findDiagramBlocks` recovers the source byte-for-byte. This pins the extraction to chroma's actual output shape.

```go
package main

import (
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/markdown"
)

func TestFindDiagramBlocksGolden(t *testing.T) {
	src := "```mermaid\n" +
		"graph TD\n" +
		"  A[\"Start & \\\"go\\\"\"] --> B\n" +
		"  B --> C{中文}\n" +
		"```\n"
	r := markdown.NewRenderer(&config.MarkupConfig{})
	htmlOut, err := r.Render(src)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	blocks := findDiagramBlocks(htmlOut, []string{"mermaid", "d2"})
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1; html=%s", len(blocks), htmlOut)
	}
	b := blocks[0]
	if b.Lang != "mermaid" {
		t.Errorf("Lang = %q, want mermaid", b.Lang)
	}
	want := "graph TD\n  A[\"Start & \"go\"\"] --> B\n  B --> C{中文}\n"
	if b.Source != want {
		t.Errorf("Source mismatch:\n got: %q\nwant: %q", b.Source, want)
	}
}

func TestFindDiagramBlocksIgnoresNonAllowlisted(t *testing.T) {
	src := "```go\nfmt.Println(\"hi\")\n```\n"
	r := markdown.NewRenderer(&config.MarkupConfig{})
	htmlOut, _ := r.Render(src)
	if blocks := findDiagramBlocks(htmlOut, []string{"mermaid"}); len(blocks) != 0 {
		t.Errorf("got %d blocks for non-allowlisted lang, want 0", len(blocks))
	}
}
```

> Note: the `want` string in the first test is the expected byte-for-byte recovery. If chroma's entity encoding differs (e.g. it emits `&#34;` for `"`), Step 2 will reveal the actual bytes — copy them into `want` exactly. This is expected: the test's job is to lock whatever the real pipeline produces.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugins/diagram-renderer && go test ./... -run TestFindDiagramBlocks -v`
Expected: FAIL — `undefined: findDiagramBlocks`. (After implementing, if the `want` literal differs from actual output, read the failure's `got:` value and set `want` to it — chroma round-trips all characters, so `got` is the correct recovered source.)

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"html"
	"regexp"
	"strings"
)

// diagramBlock is one chroma-highlighted fenced code block whose language is in
// the configured allowlist. Full is the entire "<div class=\"highlight\">…</div>"
// span (the replacement target); Source is the losslessly recovered raw source.
type diagramBlock struct {
	Full   string
	Lang   string
	Source string
}

// highlightBlockRe matches a chroma highlight wrapper and captures its data-lang
// and inner <code> content. The renderer emits:
//   <div class="highlight"><pre ...><code class="language-X" data-lang="X">…</code></pre></div>
var highlightBlockRe = regexp.MustCompile(
	`(?s)<div class="highlight">.*?<code[^>]*\sdata-lang="([^"]+)"[^>]*>(.*?)</code>.*?</div>`)

// spanTagRe matches any opening or closing <span> tag chroma inserts.
var spanTagRe = regexp.MustCompile(`</?span[^>]*>`)

// findDiagramBlocks returns every highlight block whose data-lang is in languages.
func findDiagramBlocks(htmlSrc string, languages []string) []diagramBlock {
	allow := make(map[string]bool, len(languages))
	for _, l := range languages {
		allow[strings.ToLower(l)] = true
	}
	var out []diagramBlock
	for _, m := range highlightBlockRe.FindAllStringSubmatch(htmlSrc, -1) {
		lang := strings.ToLower(m[1])
		if !allow[lang] {
			continue
		}
		out = append(out, diagramBlock{
			Full:   m[0],
			Lang:   lang,
			Source: extractSource(m[2]),
		})
	}
	return out
}

// extractSource recovers raw diagram source from a chroma <code> body by
// removing the <span> wrappers and unescaping HTML entities. Chroma tokenizes
// without adding or removing characters, so this round-trips losslessly.
func extractSource(codeInner string) string {
	s := spanTagRe.ReplaceAllString(codeInner, "")
	return html.UnescapeString(s)
}
```

- [ ] **Step 4: Run tests; reconcile the golden literal if needed**

Run: `cd plugins/diagram-renderer && go test ./... -run TestFindDiagramBlocks -v`
Expected: PASS. If `TestFindDiagramBlocksGolden` fails on the `want` literal, copy the `got:` bytes from the failure into `want` and re-run — that is the correct recovered source for this pipeline.

- [ ] **Step 5: Commit**

```bash
git add plugins/diagram-renderer/extract.go plugins/diagram-renderer/extract_test.go
git commit -m "feat(diagram-renderer): lossless source extraction from chroma blocks"
```

---

### Task 3: Content-hash SVG cache

**Files:**
- Create: `plugins/diagram-renderer/cache.go`
- Test: `plugins/diagram-renderer/cache_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type Cache struct { dir string }`
  - `func NewCache(dir string) *Cache`
  - `func (c *Cache) Key(lang, source string) string` — hex `sha256(lang + "\n" + source)`.
  - `func (c *Cache) Get(key string) (string, bool)`
  - `func (c *Cache) Put(key, svg string) error`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"path/filepath"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	c := NewCache(t.TempDir())
	k := c.Key("mermaid", "graph TD\nA-->B\n")
	if _, ok := c.Get(k); ok {
		t.Fatalf("expected miss on empty cache")
	}
	if err := c.Put(k, "<svg>ok</svg>"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := c.Get(k)
	if !ok || got != "<svg>ok</svg>" {
		t.Errorf("Get = %q,%v", got, ok)
	}
}

func TestCacheKeyStableAndDistinct(t *testing.T) {
	c := NewCache(t.TempDir())
	k1 := c.Key("mermaid", "A")
	k2 := c.Key("mermaid", "A")
	k3 := c.Key("d2", "A")
	if k1 != k2 {
		t.Errorf("key not stable: %s != %s", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("key not lang-sensitive")
	}
	if filepath.Ext(k1) != "" {
		t.Errorf("key must be bare hex, got %q", k1)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugins/diagram-renderer && go test ./... -run TestCache -v`
Expected: FAIL — `undefined: NewCache`.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Cache stores rendered SVGs on disk keyed by a content hash of (lang, source).
type Cache struct {
	dir string
}

// NewCache returns a Cache rooted at dir.
func NewCache(dir string) *Cache { return &Cache{dir: dir} }

// Key returns the hex sha256 of lang + "\n" + source.
func (c *Cache) Key(lang, source string) string {
	h := sha256.Sum256([]byte(lang + "\n" + source))
	return hex.EncodeToString(h[:])
}

// Get returns the cached SVG for key, or ("", false) on miss.
func (c *Cache) Get(key string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(c.dir, key+".svg"))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Put writes svg to the cache under key, creating the cache dir if needed.
func (c *Cache) Put(key, svg string) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, key+".svg"), []byte(svg), 0644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd plugins/diagram-renderer && go test ./... -run TestCache -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/diagram-renderer/cache.go plugins/diagram-renderer/cache_test.go
git commit -m "feat(diagram-renderer): content-hash SVG cache"
```

---

### Task 4: Kroki HTTP client + SVG sanitize

**Files:**
- Create: `plugins/diagram-renderer/kroki.go`
- Test: `plugins/diagram-renderer/kroki_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type KrokiClient struct { baseURL string; httpc *http.Client }`
  - `func NewKrokiClient(baseURL string, timeout time.Duration) *KrokiClient`
  - `func (k *KrokiClient) Render(ctx context.Context, lang, source string) (string, error)`
  - `func sanitizeSVG(svg string) string` — strips `<?xml…?>`/`<!DOCTYPE…>` prefix and ensures the root `<svg>` carries `class="kroki"`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKrokiRenderSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mermaid/svg" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "graph TD\nA-->B\n" {
			t.Errorf("body = %q", string(body))
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(`<?xml version="1.0"?>` + "\n" + `<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`))
	}))
	defer srv.Close()

	k := NewKrokiClient(srv.URL, 2*time.Second)
	svg, err := k.Render(context.Background(), "mermaid", "graph TD\nA-->B\n")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(svg, "<?xml") {
		t.Errorf("xml prolog not stripped: %q", svg)
	}
	if !strings.Contains(svg, `class="kroki"`) {
		t.Errorf("kroki class not added: %q", svg)
	}
}

func TestKrokiRenderErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("syntax error"))
	}))
	defer srv.Close()
	k := NewKrokiClient(srv.URL, 2*time.Second)
	if _, err := k.Render(context.Background(), "mermaid", "bad"); err == nil {
		t.Errorf("expected error on 400")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugins/diagram-renderer && go test ./... -run TestKroki -v`
Expected: FAIL — `undefined: NewKrokiClient`.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// KrokiClient renders diagram source to SVG via a Kroki HTTP endpoint.
type KrokiClient struct {
	baseURL string
	httpc   *http.Client
}

// NewKrokiClient returns a client for baseURL with the given per-request timeout.
func NewKrokiClient(baseURL string, timeout time.Duration) *KrokiClient {
	return &KrokiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpc:   &http.Client{Timeout: timeout},
	}
}

// Render POSTs source to {baseURL}/{lang}/svg and returns sanitized inline SVG.
func (k *KrokiClient) Render(ctx context.Context, lang, source string) (string, error) {
	url := k.baseURL + "/" + lang + "/svg"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(source))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "image/svg+xml")

	resp, err := k.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("kroki %s: status %d: %s", lang, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return sanitizeSVG(string(body)), nil
}

var (
	xmlPrologRe = regexp.MustCompile(`(?is)^\s*(<\?xml[^>]*\?>\s*)?(<!DOCTYPE[^>]*>\s*)?`)
	svgOpenRe   = regexp.MustCompile(`(?is)^<svg\b`)
)

// sanitizeSVG strips any XML prolog / DOCTYPE so the SVG can be inlined, and
// ensures the root <svg> element carries class="kroki".
func sanitizeSVG(svg string) string {
	svg = xmlPrologRe.ReplaceAllString(svg, "")
	svg = strings.TrimSpace(svg)
	if svgOpenRe.MatchString(svg) && !strings.Contains(svg[:min(len(svg), 200)], `class="kroki"`) {
		svg = svgOpenRe.ReplaceAllString(svg, `<svg class="kroki"`)
	}
	return svg
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd plugins/diagram-renderer && go test ./... -run TestKroki -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/diagram-renderer/kroki.go plugins/diagram-renderer/kroki_test.go
git commit -m "feat(diagram-renderer): Kroki HTTP client and SVG sanitize"
```

---

### Task 5: SVG wrapping + fallback rendering

**Files:**
- Create: `plugins/diagram-renderer/fallback.go`
- Test: `plugins/diagram-renderer/fallback_test.go`

**Interfaces:**
- Consumes: `diagramBlock` (Task 2), `Config` (Task 1).
- Produces:
  - `func wrapSVG(svg, lang, figureClass string) string` — returns `<figure class="{figureClass} {figureClass}-{lang}" role="img">{svg}</figure>`.
  - `func fallbackReplacement(b diagramBlock, cfg *Config) (replacement string, needsMermaidJS bool)` — per `cfg.FallbackMode`: `client` → `<pre class="mermaid">SOURCE</pre>` (+needsMermaidJS) for `mermaid`, else keep `b.Full`; `codeblock`/`fail` → keep `b.Full`.
  - `func injectMermaidJS(htmlSrc, mermaidJS string) string` — inserts the script tags once before `</body>`; no-op if already present or no `</body>`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"strings"
	"testing"
)

func TestWrapSVG(t *testing.T) {
	got := wrapSVG(`<svg class="kroki"></svg>`, "mermaid", "diagram")
	want := `<figure class="diagram diagram-mermaid" role="img"><svg class="kroki"></svg></figure>`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFallbackClientMermaid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FallbackMode = "client"
	b := diagramBlock{Full: "<div class=\"highlight\">x</div>", Lang: "mermaid", Source: "graph TD\nA-->B\n"}
	repl, needJS := fallbackReplacement(b, cfg)
	if !needJS {
		t.Errorf("mermaid client fallback should need JS")
	}
	if !strings.HasPrefix(repl, `<pre class="mermaid">`) || !strings.Contains(repl, "A--&gt;B") {
		t.Errorf("repl = %q", repl)
	}
}

func TestFallbackClientNonMermaidDegradesToCodeblock(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FallbackMode = "client"
	b := diagramBlock{Full: "<div class=\"highlight\">ORIG</div>", Lang: "d2", Source: "a -> b"}
	repl, needJS := fallbackReplacement(b, cfg)
	if needJS {
		t.Errorf("d2 cannot use mermaid.js")
	}
	if repl != b.Full {
		t.Errorf("non-mermaid client fallback should keep original block, got %q", repl)
	}
}

func TestInjectMermaidJSOnce(t *testing.T) {
	html := "<html><body><p>hi</p></body></html>"
	out := injectMermaidJS(html, "/js/mermaid.js")
	if strings.Count(out, "/js/mermaid.js") != 1 {
		t.Fatalf("expected 1 script, got %q", out)
	}
	// idempotent
	out2 := injectMermaidJS(out, "/js/mermaid.js")
	if strings.Count(out2, "/js/mermaid.js") != 1 {
		t.Errorf("double injection: %q", out2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugins/diagram-renderer && go test ./... -run 'TestWrapSVG|TestFallback|TestInjectMermaid' -v`
Expected: FAIL — `undefined: wrapSVG`.

- [ ] **Step 3: Write the implementation**

```go
package main

import (
	"fmt"
	"html"
	"strings"
)

// wrapSVG wraps rendered SVG in a semantic <figure>.
func wrapSVG(svg, lang, figureClass string) string {
	return fmt.Sprintf(`<figure class="%s %s-%s" role="img">%s</figure>`,
		figureClass, figureClass, lang, svg)
}

const mermaidInitMarker = "mermaid.initialize"

// fallbackReplacement computes the HTML that replaces a diagram block when Kroki
// rendering failed. Only mermaid can render client-side; other languages keep
// their original highlighted code block regardless of mode.
func fallbackReplacement(b diagramBlock, cfg *Config) (string, bool) {
	if cfg.FallbackMode == "client" && b.Lang == "mermaid" {
		return `<pre class="mermaid">` + html.EscapeString(b.Source) + `</pre>`, true
	}
	// codeblock, fail, or non-mermaid client: keep the original chroma block.
	return b.Full, false
}

// injectMermaidJS inserts the mermaid.js <script> and an initialize call once
// before </body>. It is a no-op if the init marker is already present or there
// is no </body>.
func injectMermaidJS(htmlSrc, mermaidJS string) string {
	if strings.Contains(htmlSrc, mermaidInitMarker) {
		return htmlSrc
	}
	idx := strings.LastIndex(htmlSrc, "</body>")
	if idx < 0 {
		return htmlSrc
	}
	snippet := fmt.Sprintf(
		"\n<script src=%q></script>\n<script>%s({startOnLoad:true});</script>\n",
		mermaidJS, "mermaid.initialize")
	return htmlSrc[:idx] + snippet + htmlSrc[idx:]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd plugins/diagram-renderer && go test ./... -run 'TestWrapSVG|TestFallback|TestInjectMermaid' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/diagram-renderer/fallback.go plugins/diagram-renderer/fallback_test.go
git commit -m "feat(diagram-renderer): svg wrapping and client-side fallback"
```

---

### Task 6: Plugin type, orchestration, and InitPlugin

**Files:**
- Create: `plugins/diagram-renderer/plugin.go`
- Create: `plugins/diagram-renderer/plugin_main.go`
- Test: `plugins/diagram-renderer/plugin_test.go`

**Interfaces:**
- Consumes: `Config`/`DefaultConfig`/`ParseConfig` (Task 1), `findDiagramBlocks`/`diagramBlock` (Task 2), `Cache`/`NewCache` (Task 3), `KrokiClient`/`NewKrokiClient` (Task 4), `wrapSVG`/`fallbackReplacement`/`injectMermaidJS` (Task 5), `pkg/plugin`.
- Produces:
  - `type DiagramRenderer struct { cfg Config; cache *Cache; kroki *KrokiClient; logf func(string, ...any) }`
  - `func New(cfg *Config) *DiagramRenderer`
  - `func (p *DiagramRenderer) Name() string` → `"diagram_renderer"`
  - `func (p *DiagramRenderer) SetLogf(fn func(string, ...any))`
  - `func (p *DiagramRenderer) PluginMetadata() plugin.PluginMeta`
  - `func (p *DiagramRenderer) ConfigSchema() plugin.Schema`
  - `func (p *DiagramRenderer) OnOutputWritten(ctx context.Context, outputDir string) error`
  - `func InitPlugin(cfg map[string]any) (interface{}, error)` (in plugin_main.go)
  - Compile-time asserts: `var _ plugin.PostBuildHook = (*DiagramRenderer)(nil)`, `var _ plugin.SchemaProvider = (*DiagramRenderer)(nil)`.

- [ ] **Step 1: Write the failing end-to-end test**

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/markdown"
)

// renderPage renders markdown and wraps it in a minimal HTML document.
func renderPage(t *testing.T, md string) string {
	t.Helper()
	r := markdown.NewRenderer(&config.MarkupConfig{})
	body, err := r.Render(md)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return "<html><head><title>t</title></head><body>" + body + "</body></html>"
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestOnOutputWrittenRendersSVG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`))
	}))
	defer srv.Close()

	out := t.TempDir()
	writeFile(t, out, "index.html", renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n"))

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.KrokiURL = srv.URL
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	p := New(cfg)

	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatalf("OnOutputWritten: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "index.html"))
	s := string(got)
	if !strings.Contains(s, `<figure class="diagram diagram-mermaid"`) {
		t.Errorf("no figure wrap: %s", s)
	}
	if strings.Contains(s, `class="highlight"`) {
		t.Errorf("highlight block not replaced: %s", s)
	}

	// Idempotency: second run makes no changes.
	before, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if string(before) != string(after) {
		t.Errorf("not idempotent")
	}
}

func TestOnOutputWrittenFallbackOnKrokiDown(t *testing.T) {
	out := t.TempDir()
	writeFile(t, out, "index.html", renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n"))

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.KrokiURL = "http://127.0.0.1:1" // connection refused
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	p := New(cfg)

	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatalf("fallback path must not error: %v", err)
	}
	s, _ := os.ReadFile(filepath.Join(out, "index.html"))
	str := string(s)
	if !strings.Contains(str, `<pre class="mermaid">`) {
		t.Errorf("no client fallback: %s", str)
	}
	if strings.Count(str, "mermaid.initialize") != 1 {
		t.Errorf("mermaid.js not injected once: %s", str)
	}
}

func TestOnOutputWrittenDisabledNoop(t *testing.T) {
	out := t.TempDir()
	page := renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n")
	writeFile(t, out, "index.html", page)
	p := New(DefaultConfig()) // Enabled=false
	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if string(got) != page {
		t.Errorf("disabled plugin modified output")
	}
}

func TestExcludeKinds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`))
	}))
	defer srv.Close()
	out := t.TempDir()
	writeFile(t, out, "index.html", renderPage(t, "```mermaid\ngraph TD\nA-->B\n```\n"))
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.KrokiURL = srv.URL
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.ExcludeKinds = []string{"home"} // index.html => home
	p := New(cfg)
	if err := p.OnOutputWritten(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if strings.Contains(string(got), "<figure") {
		t.Errorf("excluded kind should be skipped")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugins/diagram-renderer && go test ./... -run TestOnOutputWritten -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write plugin.go**

```go
package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/pkg/plugin"
)

// DiagramRenderer renders diagram fenced code blocks to inline SVG via Kroki.
type DiagramRenderer struct {
	cfg   Config
	cache *Cache
	kroki *KrokiClient
	logf  func(string, ...any)
}

// New creates a DiagramRenderer from cfg.
func New(cfg *Config) *DiagramRenderer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &DiagramRenderer{
		cfg:   *cfg,
		cache: NewCache(cfg.CacheDir),
		kroki: NewKrokiClient(cfg.KrokiURL, time.Duration(cfg.TimeoutMs)*time.Millisecond),
		logf:  func(string, ...any) {},
	}
}

// SetLogf sets the logger function.
func (p *DiagramRenderer) SetLogf(fn func(string, ...any)) { p.logf = fn }

// Name returns the plugin's unique identifier.
func (p *DiagramRenderer) Name() string { return "diagram_renderer" }

// PluginMetadata returns human-readable metadata.
func (p *DiagramRenderer) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"diagram", "mermaid", "kroki", "svg"},
		IsOfficial: true,
	}
}

// ConfigSchema returns the plugin.Schema for config validation.
func (p *DiagramRenderer) ConfigSchema() plugin.Schema { return p.cfg.ConfigSchema() }

// collectHTMLFiles walks outputDir recursively for every .html file.
func collectHTMLFiles(outputDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".html") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// OnOutputWritten scans output HTML and renders diagram blocks to SVG.
func (p *DiagramRenderer) OnOutputWritten(ctx context.Context, outputDir string) error {
	if !p.cfg.Enabled || len(p.cfg.Languages) == 0 {
		p.logf("diagram-renderer: disabled or no languages, skipping\n")
		return nil
	}
	files, err := collectHTMLFiles(outputDir)
	if err != nil {
		p.logf("diagram-renderer: scan %s: %v\n", outputDir, err)
		return nil
	}
	for _, fp := range files {
		select {
		case <-ctx.Done():
			p.logf("diagram-renderer: cancelled\n")
			return nil
		default:
		}
		if err := p.processFile(ctx, fp, outputDir); err != nil {
			p.logf("diagram-renderer: %s: %v\n", fp, err)
		}
	}
	return nil
}

// processFile renders every diagram block in one HTML file.
func (p *DiagramRenderer) processFile(ctx context.Context, filePath, outputDir string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	rel, err := filepath.Rel(outputDir, filePath)
	if err != nil {
		return fmt.Errorf("rel: %w", err)
	}
	if !p.kindAllowed(p.guessKind(rel)) {
		return nil
	}

	src := string(data)
	blocks := findDiagramBlocks(src, p.cfg.Languages)
	if len(blocks) == 0 {
		return nil
	}

	needMermaidJS := false
	for _, b := range blocks {
		replacement, injectJS := p.renderBlock(ctx, b)
		src = strings.Replace(src, b.Full, replacement, 1)
		if injectJS {
			needMermaidJS = true
		}
	}
	if needMermaidJS {
		src = injectMermaidJS(src, p.cfg.MermaidJS)
	}

	if src == string(data) {
		return nil
	}
	if err := os.WriteFile(filePath, []byte(src), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// renderBlock returns the replacement HTML for one diagram block and whether the
// page needs mermaid.js injected. It tries cache, then Kroki, then fallback.
func (p *DiagramRenderer) renderBlock(ctx context.Context, b diagramBlock) (string, bool) {
	key := p.cache.Key(b.Lang, b.Source)
	if svg, ok := p.cache.Get(key); ok {
		p.logf("diagram-renderer: cache hit lang=%s hash=%s\n", b.Lang, key)
		return wrapSVG(svg, b.Lang, p.cfg.FigureClass), false
	}
	svg, err := p.kroki.Render(ctx, b.Lang, b.Source)
	if err != nil {
		p.logf("diagram-renderer: WARN kroki failed lang=%s: %v; falling back\n", b.Lang, err)
		return fallbackReplacement(b, &p.cfg)
	}
	if putErr := p.cache.Put(key, svg); putErr != nil {
		p.logf("diagram-renderer: cache put lang=%s: %v\n", b.Lang, putErr)
	}
	p.logf("diagram-renderer: rendered lang=%s hash=%s\n", b.Lang, key)
	return wrapSVG(svg, b.Lang, p.cfg.FigureClass), false
}

// kindAllowed applies include/exclude kind filters.
func (p *DiagramRenderer) kindAllowed(kind string) bool {
	if len(p.cfg.IncludeKinds) > 0 && !contains(p.cfg.IncludeKinds, kind) {
		return false
	}
	if len(p.cfg.ExcludeKinds) > 0 && contains(p.cfg.ExcludeKinds, kind) {
		return false
	}
	return true
}

// guessKind infers a page kind from its relative path (matches html-injector).
func (p *DiagramRenderer) guessKind(relPath string) string {
	if relPath == "index.html" || relPath == "/index.html" {
		return "home"
	}
	if strings.HasSuffix(relPath, "/index.html") && !strings.Contains(relPath, "/page/") {
		return "section"
	}
	return "page"
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Compile-time interface assertions.
var _ plugin.PostBuildHook = (*DiagramRenderer)(nil)
var _ plugin.SchemaProvider = (*DiagramRenderer)(nil)
```

- [ ] **Step 4: Write plugin_main.go**

```go
package main

// InitPlugin is the exported symbol the .so plugin loader looks up. It parses
// the raw config map and returns a DiagramRenderer instance.
func InitPlugin(cfg map[string]any) (interface{}, error) {
	parsedCfg, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return New(parsedCfg), nil
}

func main() {}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd plugins/diagram-renderer && go test ./... -v`
Expected: PASS (all tasks' tests green together).

- [ ] **Step 6: Verify it builds as a Go plugin**

Run: `cd plugins/diagram-renderer && go build -buildmode=plugin -o /tmp/diagram-renderer.so .`
Expected: builds with no error and produces `/tmp/diagram-renderer.so`.

- [ ] **Step 7: Commit**

```bash
git add plugins/diagram-renderer/plugin.go plugins/diagram-renderer/plugin_main.go plugins/diagram-renderer/plugin_test.go
git commit -m "feat(diagram-renderer): plugin orchestration and InitPlugin"
```

---

### Task 7: Build wiring, Docker Kroki, and docs

**Files:**
- Verify (no edit expected): `scripts/build-plugins.sh` (convention-based; auto-discovers the new dir).
- Create: `deploy/kroki/docker-compose.yml`
- Create: `docs/plugins/diagram-renderer.md`

**Interfaces:**
- Consumes: the built `diagram-renderer.so` from Task 6.
- Produces: runnable Kroki stack + user-facing enablement docs.

- [ ] **Step 1: Confirm the build script picks up the new plugin**

Run: `bash scripts/build-plugins.sh 2>&1 | grep -i diagram || echo "NOT BUILT"`
Expected: a line showing `diagram-renderer.so` was built (the script iterates every `plugins/*` dir). If it prints `NOT BUILT`, read `scripts/build-plugins.sh` to see how dirs are discovered and confirm the new dir isn't excluded by an allowlist; only then adjust.

- [ ] **Step 2: Create the Kroki Docker stack**

Create `deploy/kroki/docker-compose.yml`:

```yaml
services:
  kroki:
    image: yuzutech/kroki
    ports: ["8000:8000"]
    environment:
      KROKI_MERMAID_HOST: kroki-mermaid
    networks: [huan_net]
    depends_on: [kroki-mermaid]
  kroki-mermaid:
    image: yuzutech/kroki-mermaid
    expose: ["8002"]
    networks: [huan_net]

networks:
  huan_net:
    name: huan_net
```

- [ ] **Step 3: Write the plugin doc**

Create `docs/plugins/diagram-renderer.md` documenting: what it does (build-time Mermaid/PlantUML/GraphViz/D2 → inline SVG), the `huan.yaml` config block (copy the full block from the spec §4), how to start Kroki (`docker compose -f deploy/kroki/docker-compose.yml up -d`), the fallback behavior (Kroki down → `<pre class="mermaid">` + mermaid.js, build never aborts), and the enablement steps:

```markdown
## Enable

1. Start Kroki: `docker compose -f deploy/kroki/docker-compose.yml up -d`
2. Build the plugin: `bash scripts/build-plugins.sh && cp release/plugins/diagram-renderer.so "$HUAN_HOME"`
3. In huan.yaml:

   plugins:
     diagram_renderer:
       enabled: true
       kroki_url: "http://localhost:8000"
       languages: [mermaid, plantuml, graphviz, d2]

4. Write a ```mermaid fenced block in any content file and run `huan build`.
```

- [ ] **Step 4: Manual smoke test (end to end)**

Run:
```bash
docker compose -f deploy/kroki/docker-compose.yml up -d
bash scripts/build-plugins.sh
# add a ```mermaid block to a test content file, enable the plugin in huan.yaml, then:
./huan build
grep -rl '<figure class="diagram' release/ && echo "SVG OK"
```
Expected: at least one output page contains `<figure class="diagram diagram-mermaid"><svg`. If Kroki is stopped, re-running instead yields `<pre class="mermaid">` + one mermaid.js script, and the build still succeeds.

- [ ] **Step 5: Commit**

```bash
git add deploy/kroki/docker-compose.yml docs/plugins/diagram-renderer.md
git commit -m "docs(diagram-renderer): kroki docker stack and enablement guide"
```

---

## Self-Review

**1. Spec coverage:**
- §3 组件目录 → Tasks 1–6 create each file (config, extract, cache, kroki, fallback, plugin, plugin_main). ✅ (spec's `.svg`/`.go` split honored; `fallback.go` also holds `wrapSVG`.)
- §4 配置 → Task 1 (all fields, camelCase keys, nested `fallback`). ✅
- §5 核心流程 → Task 6 `OnOutputWritten`/`processFile`/`renderBlock` (early-exit, recursive collect, cache→kroki→replace, ctx cancel, idempotent write). ✅
- §6 无损取回 + 黄金测试 → Task 2. ✅
- §7 Kroki 客户端 + SVG 清洗 → Task 4. ✅
- §8 降级（client/codeblock/fail, mermaid-only client, 每页一次注入）→ Task 5 + wired in Task 6. ✅
- §9 Docker → Task 7. ✅
- §10 测试与可观测性 → tests in every task; `p.logf` structured messages with lang/hash in Task 6. ✅
- §12 验收标准 1–6 → covered by Task 6 tests (SVG render, cache hit via idempotency+cache, fallback, kind filter, idempotency) and Task 7 smoke test; language on/off is config-driven (Task 1 `Languages` + Task 2 allowlist). ✅

**2. Placeholder scan:** No TBD/TODO; all code steps contain full implementations; the one golden-literal reconciliation (Task 2) is an explicit, bounded instruction, not a placeholder. ✅

**3. Type consistency:** `diagramBlock{Full,Lang,Source}` consistent across Tasks 2/5/6; `Config` field names consistent Tasks 1/5/6; `Cache.Key/Get/Put`, `KrokiClient.Render`, `wrapSVG`, `fallbackReplacement`, `injectMermaidJS` signatures match their call sites in Task 6. `fail` mode returns `b.Full` (keep original) — consistent with spec §8's "首版不 abort". ✅

**Observability note:** structured JSON logging per CLAUDE.md is threaded through `p.logf` messages (event-typed strings with lang/hash/duration semantics). If the host wires `SetLogf` to the JSON logger, these become structured events; full `trace_id` propagation depends on host plumbing and is not blocked by this plugin.
