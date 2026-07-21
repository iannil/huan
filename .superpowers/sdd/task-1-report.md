# Task 1 Report: 创建 ImageProcessor capability 接口

## Status

**Done.** Completed as specified.

## Commits

```
dc07e60 feat(image): add ImageProcessor capability interface
```

## Files Created

- `internal/image/processor.go` — ImageProcessor interface (embeds plugin.Plugin, adds Process)
- `internal/image/processor_test.go` — TDD tests

## Test Summary

```
=== RUN   TestImageProcessorEmbedsPlugin --- PASS: 0.00s
=== RUN   TestImageProcessorHasProcessMethod --- PASS: 0.00s
=== RUN   TestImageProcessorCanBeRegistered --- PASS: 0.00s
```

3/3 tests pass. Covers compile-time interface satisfaction, method signature, and plugin.Registry integration.

## Concerns

None.