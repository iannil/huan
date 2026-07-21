### Task 1: PipelineCache 结构体 + hasTemplateChanges

**Files:**
- Create: `internal/build/cache.go` — PipelineCache 结构体 + NewPipelineCache + hasTemplateChanges
- Create: `internal/build/cache_test.go` — 测试

**Interfaces:**
- Produces: `build.PipelineCache` 结构体, `build.NewPipelineCache()`, `build.HasTemplateChanges(changedFiles []string, sourceDir string) bool`

- [ ] **Step 1: 编写测试**

`internal/build/cache_test.go`：

```go
package build

import (
	"testing"
)

func TestNewPipelineCache_Empty(t *testing.T) {
	c := NewPipelineCache()
	if c == nil {
		t.Fatal("NewPipelineCache returned nil")
	}
	if c.Templates != nil {
		t.Error("Templates should be nil for fresh cache")
	}
	if c.BuiltAt.IsZero() {
		t.Error("BuiltAt should be set")
	}
}

func TestHasTemplateChanges_LayoutsFile(t *testing.T) {
	changed := []string{"/site/layouts/_default/single.html"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("layouts/ change should trigger full build")
	}
}

func TestHasTemplateChanges_ContentFile(t *testing.T) {
	changed := []string{"/site/content/posts/hello.md"}
	if HasTemplateChanges(changed, "/site") {
		t.Error("content/ change should NOT trigger full build")
	}
}

func TestHasTemplateChanges_HuanYaml(t *testing.T) {
	changed := []string{"/site/huan.yaml"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("huan.yaml change should trigger full build")
	}
}

func TestHasTemplateChanges_I18nFile(t *testing.T) {
	changed := []string{"/site/i18n/en.yaml"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("i18n/ change should trigger full build")
	}
}

func TestHasTemplateChanges_ThemesFile(t *testing.T) {
	changed := []string{"/site/themes/zozo/layouts/baseof.html"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("themes/ change should trigger full build")
	}
}

func TestHasTemplateChanges_MixedFiles(t *testing.T) {
	// One content, one template → should trigger (template change wins)
	changed := []string{
		"/site/content/posts/hello.md",
		"/site/layouts/_default/list.html",
	}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("mixed files with one template change should trigger full build")
	}
}

func TestHasTemplateChanges_EmptyList(t *testing.T) {
	if HasTemplateChanges([]string{}, "/site") {
		t.Error("empty change list should not trigger full build")
	}
}

func TestHasTemplateChanges_OutsideSourceDir(t *testing.T) {
	// File outside sourceDir (Rel fails or gives long path) → not a template change
	changed := []string{"/other/path/file.html"}
	if HasTemplateChanges(changed, "/site") {
		t.Error("file outside sourceDir should not trigger full build")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/build/ -run "TestNewPipelineCache|TestHasTemplateChanges" -v
```
Expected: COMPILATION ERROR (no cache.go yet)

- [ ] **Step 3: 实现 cache.go**

`internal/build/cache.go`：

```go
package build

import (
	"html/template"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/i18n"
	"github.com/iannil/huan/internal/markdown"
	"github.com/iannil/huan/internal/output"
	"github.com/iannil/huan/internal/shortcode"
)

// PipelineCache holds reusable build state across incremental builds.
// Populated after a full build completes. Incremental builds reuse this
// state to avoid re-parsing templates, reloading i18n bundles, and
// re-initializing the writer.
//
// DESIGN NOTE: This cache intentionally does NOT hold per-page content or
// template contexts. Those reference page pointers that change on every
// content edit, so caching them would produce stale output for list pages.
// Instead, incremental builds reload content + rebuild contexts (cheap)
// and reuse only the rendering infrastructure here (expensive to rebuild).
type PipelineCache struct {
	// Rendering infrastructure (valid until template/i18n/config changes)
	Templates  *template.Template
	I18nBundle *i18n.Bundle
	SCRegistry *shortcode.Registry
	MDRenderer *markdown.Renderer

	// Site config (valid until huan.yaml changes)
	SiteCfg *config.Config

	// Output writer (valid across builds; writes to the same OutputDir)
	Writer *output.Writer

	// BuiltAt records when this cache was populated (the last full build time).
	BuiltAt time.Time
}

// NewPipelineCache returns an empty PipelineCache with BuiltAt set to now.
func NewPipelineCache() *PipelineCache {
	return &PipelineCache{
		BuiltAt: time.Now(),
	}
}

// HasTemplateChanges reports whether any changed file invalidates the
// pipeline cache. Files under layouts/, i18n/, themes/, or the huan.yaml
// config itself require a full rebuild because they affect the cached
// templates/i18n/config.
//
// Returns false for content/ and static/ changes (those are handled
// incrementally).
func HasTemplateChanges(changedFiles []string, sourceDir string) bool {
	for _, f := range changedFiles {
		rel, err := filepath.Rel(sourceDir, f)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		// If the file is outside sourceDir, Rel returns a "../" path.
		// Those are not template changes relative to this site.
		if strings.HasPrefix(rel, "../") {
			continue
		}
		switch {
		case strings.HasPrefix(rel, "layouts/"):
			return true
		case strings.HasPrefix(rel, "i18n/"):
			return true
		case rel == "huan.yaml":
			return true
		case strings.HasPrefix(rel, "themes/"):
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/build/ -run "TestNewPipelineCache|TestHasTemplateChanges" -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/build/cache.go internal/build/cache_test.go
git commit -m "feat(build): add PipelineCache and HasTemplateChanges"
```

---

