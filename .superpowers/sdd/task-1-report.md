# Task 1 Report: PipelineCache + HasTemplateChanges

## Status
COMPLETE — all tests pass, committed.

## Commits
- `24f094c` feat(build): add PipelineCache and HasTemplateChanges

## Files Created
- `/Users/rong.zhu/Code/zhurong/huan/internal/build/cache.go`
- `/Users/rong.zhu/Code/zhurong/huan/internal/build/cache_test.go`

## TDD Cycle
1. Wrote `cache_test.go` (9 tests, verbatim from brief).
2. Ran tests → confirmed RED (compilation error: `undefined: NewPipelineCache`, `undefined: HasTemplateChanges`).
3. Implemented `cache.go` (verbatim from brief).
4. Ran tests → all 9 PASS.
5. Verified no regressions: `go test ./internal/build/` ok; `go vet ./internal/build/` clean.
6. Committed both files.

## Test Summary
```
=== RUN   TestNewPipelineCache_Empty              --- PASS
=== RUN   TestHasTemplateChanges_LayoutsFile      --- PASS
=== RUN   TestHasTemplateChanges_ContentFile      --- PASS
=== RUN   TestHasTemplateChanges_HuanYaml         --- PASS
=== RUN   TestHasTemplateChanges_I18nFile         --- PASS
=== RUN   TestHasTemplateChanges_ThemesFile       --- PASS
=== RUN   TestHasTemplateChanges_MixedFiles       --- PASS
=== RUN   TestHasTemplateChanges_EmptyList        --- PASS
=== RUN   TestHasTemplateChanges_OutsideSourceDir --- PASS
PASS
ok  github.com/iannil/huan/internal/build  0.529s
```
9/9 pass. Full package `go test ./internal/build/` also ok (no regressions).

## What Was Implemented
- `PipelineCache` struct with exactly the 7 fields from the brief: `Templates`, `I18nBundle`, `SCRegistry`, `MDRenderer`, `SiteCfg`, `Writer`, `BuiltAt`.
- `NewPipelineCache() *PipelineCache` — returns empty cache with `BuiltAt = time.Now()`.
- `HasTemplateChanges(changedFiles []string, sourceDir string) bool` — returns true if any changed file lies under `layouts/`, `i18n/`, `themes/`, or is `huan.yaml` relative to `sourceDir`. Files outside `sourceDir` (resolved via `filepath.Rel` producing a `../` prefix) are ignored.

## Type Verification
Before implementing, confirmed all referenced types exist in their packages:
- `i18n.Bundle` — internal/i18n/i18n.go:13
- `shortcode.Registry` — internal/shortcode/shortcode.go:25
- `markdown.Renderer` — internal/markdown/renderer.go:28
- `config.Config` — internal/config/config.go:9
- `output.Writer` — internal/output/writer.go:14

## Concerns / Notes for Downstream Tasks
- `PipelineCache` is defined but not yet wired into the build pipeline. A later task must populate it at the end of a full build and consume it in the incremental path.
- `HasTemplateChanges` uses `filepath.Rel` + `strings.HasPrefix(rel, "../")` to exclude out-of-tree paths. This works for the test cases but relies on `sourceDir` being a lexical prefix of the changed file. If the incremental watcher ever reports relative paths or paths with different normalization (e.g. symlinks), the caller must canonicalize first. Not a blocker for Task 1.
- `static/` directory is not explicitly tested as "not a template change"; it falls through all switch cases and correctly returns false. If a future task wants explicit handling it should add a test.
