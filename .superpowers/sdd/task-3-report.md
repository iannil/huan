# Task 3 Report: Create qwen3_translate independent plugin repository

**Date:** 2026-07-21

## Status: COMPLETED

## Steps Performed

### 1. Plugin Repository Setup
- Created directory `/Users/rong.zhu/Code/zhurong/huan-plugin-qwen3`
- Created `go.mod` as self-contained module (no `replace` directive pointing to huan, following the pattern established in Task 2)

### 2. Self-Contained Repository Approach
Following the pattern from Task 2 (cloudflare plugin), the plugin repo is fully self-contained with copies of required interface/type packages:

- **`translate/types.go`** — copied from `internal/translate/types.go` (Translator interface, Request, Response, QualityResult)
- **`plugin/plugin.go`** — copied from `internal/plugin/plugin.go` (Plugin interface, Registry)
- **`observability/logging.go`** — copied from `internal/observability/logging.go` (Logger)
- **`i18n/langdetect/langdetect.go`** — copied from `internal/i18n/langdetect/langdetect.go` (CJK detection helpers used by quality.go)

### 3. Qwen3 Source Files
All 8 `.go` files (excluding `_test.go`) copied from `internal/translate/qwen3/`:
- `plugin.go`, `options.go`, `client.go`, `parse.go`, `prompt.go`, `quality.go`, `chunker.go`, `context.go`

Changes made:
- Package renamed from `qwen3` to `main`
- Import paths updated from `github.com/iannil/huan/internal/...` to `github.com/iannil/huan-plugin-qwen3/...`

### 4. Entry Point
Created `plugin_main.go` with `InitPlugin` export:

```go
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    projectRoot := ""
    if v, ok := cfg["_project_root"].(string); ok {
        projectRoot = v
    }
    return New(parsedCfg, projectRoot)
}
```

The `_project_root` key allows the loader to pass the project root directory so the plugin can resolve relative paths (system_prompt_file, glossary_file).

### 5. Build Verification
```bash
cd /Users/rong.zhu/Code/zhurong/huan-plugin-qwen3
go build -buildmode=plugin -o qwen3.so .
```
**BUILD SUCCESS** — `qwen3.so` created (13 MB)

### 6. Deletion from Huan Main Repo
- Removed `internal/translate/qwen3/` directory (all 8 `.go` source files + 5 `_test.go` files)

### 7. Build Verification of Huan Main Repo
```bash
cd /Users/rong.zhu/Code/zhurong/huan
go build ./...
```
**BUILD SUCCESS**

### 8. Test Verification
```bash
go test ./internal/translate/...
```
**PASS** — 3 tests in `internal/translate` (types_test.go)

```bash
go test ./...
```
**ALL PASS** — 26 packages, 0 failures

## Commit
`69171dc` — `refactor(translate): extract qwen3 plugin to external repository`

## Concerns
- The plugin repo now maintains copies of `translate/types.go`, `plugin/plugin.go`, `observability/logging.go`, and `i18n/langdetect/langdetect.go`. These need to be kept in sync if interfaces change.
- The `_project_root` key convention must be respected by the plugin loader — the loader should inject this key into the config map before calling `InitPlugin`.
- Tests from the original `internal/translate/qwen3/` were deleted with the source code. They would need to be recreated in the plugin repo to maintain coverage on the plugin side.
- The `cmd/huan/translate_cmd.go` still references `qwen3_translate` by name string and `translate.Translator` interface — these remain valid since the interface lives in huan's own `internal/translate` package.