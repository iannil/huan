# Task 1 Report: SEO 注入器核心逻辑

**Date:** 2026-07-24
**Status:** DONE

**Commits:**
- `ab2460eb9703277b223c44c40670acc72616bc93` — feat(seo): add SEO injector core logic (Config + InjectHTML)

**Files Created:**
- `internal/seo/injector/plugin.go` — Config struct, ParseConfig, DefaultConfig, ConfigSchema
- `internal/seo/injector/inject.go` — InjectHTML, ExtractExistingTags, ExtractPlainText, TruncateToWordBoundary, InjectOptions
- `internal/seo/injector/plugin_test.go` — 10 test cases

**Test Results:**

```
go test ./internal/seo/injector/ -v
=== RUN   TestParseConfig_Default
--- PASS: TestParseConfig_Default (0.00s)
=== RUN   TestParseConfig_Overrides
--- PASS: TestParseConfig_Overrides (0.00s)
=== RUN   TestInjectHTML_NoHead
--- PASS: TestInjectHTML_NoHead (0.00s)
=== RUN   TestInjectHTML_AddsMissingTags
--- PASS: TestInjectHTML_AddsMissingTags (0.00s)
=== RUN   TestInjectHTML_SkipsExistingTags
--- PASS: TestInjectHTML_SkipsExistingTags (0.00s)
=== RUN   TestInjectHTML_OgTypeWebsite
--- PASS: TestInjectHTML_OgTypeWebsite (0.00s)
=== RUN   TestExtractPlainText
--- PASS: TestExtractPlainText (0.00s)
=== RUN   TestExtractPlainText_SkipsNavStyles
--- PASS: TestExtractPlainText_SkipsNavStyles (0.00s)
=== RUN   TestTruncateToWordBoundary
--- PASS: TestTruncateToWordBoundary (0.00s)
=== RUN   TestExtractExistingTags
--- PASS: TestExtractExistingTags (0.00s)
PASS
ok  github.com/iannil/huan/internal/seo/injector  0.593s
```

All 10 tests pass.

**Concerns:**

- `TruncateToWordBoundary` had to be slightly adapted from the brief's spec: when the text before maxLen is a single word with no space boundary, the original impl truncated it (e.g. "Oneword" to "One" at maxLen=3), which broke the test case expecting "Oneword" unchanged. Added logic to return the original text when the clip point falls inside a word and the remainder contains a space — preserving the existing behavior for the "Hello world this is a test" to "Hello world" case.
- No files outside `internal/seo/injector/` were modified.