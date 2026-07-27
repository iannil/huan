# Task 5 Report: Integrate ShortcodeProvider into Build Pipeline

**Date:** 2026-07-27

## Summary

Modified `renderMarkdownAndTree()` in `internal/build/pipeline.go` to check if the active theme implements `ShortcodeProvider` and, if so, register its shortcodes into the shortcode registry.

## Changes

- **`internal/build/pipeline.go`**: In `renderMarkdownAndTree()`, after creating `p.scRegistry` and `p.md`, added a block that:
  - Checks if `p.themeManager` is non-nil
  - If so, calls `p.themeManager.Active()` to get the active theme plugin
  - If the active plugin implements `theme.ShortcodeProvider`, iterates over its `Shortcodes()` map and registers each handler via `p.scRegistry.Register()`

```go
// Register theme shortcodes if the active theme provides them.
if p.themeManager != nil {
    if tp := p.themeManager.Active(); tp != nil {
        if sp, ok := tp.(theme.ShortcodeProvider); ok {
            for name, handler := range sp.Shortcodes() {
                p.scRegistry.Register(name, handler)
                p.logf("  shortcode: theme registered %q\n", name)
            }
        }
    }
}
```

## Verification

- `go test ./internal/build/ -v` — ALL PASS (47 passed, 1 skipped)
- `go build ./...` — clean compilation
