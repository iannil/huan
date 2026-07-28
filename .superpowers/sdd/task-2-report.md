# Task 2 Report: 扩展 Loader 支持按 Category 加载

**Status:** Completed

**Date:** 2026-07-27

## Summary

在 `internal/plugin/loader.go` 中添加了三个 Category 常量和 `ShouldLoadInCategory` 辅助函数，并在 `internal/plugin/loader_test.go` 中添加了完整的 table-driven 测试。

## Files Modified

1. **`internal/plugin/loader.go`** — 新增：
   - `CategoryStatic = "static"` 常量
   - `CategoryDynamic = "dynamic"` 常量
   - `CategoryMixed = "mixed"` 常量
   - `ShouldLoadInCategory(pluginCategory, mode string) bool` 函数

2. **`internal/plugin/loader_test.go`** — 新增：
   - `TestShouldLoadInCategory` — 覆盖 10 个 case：static/build, static/daemon, dynamic/build, dynamic/daemon, mixed/build, mixed/daemon, 空/build, 空/daemon, unknown/build, unknown/daemon

## Category 加载规则

| category | build 时 | daemon 时 |
|----------|----------|-----------|
| static   | 加载     | 不加载    |
| dynamic  | 不加载   | 加载      |
| mixed    | 加载     | 加载      |
| 空       | 不加载   | 加载 (默认 dynamic) |
| unknown  | 不加载   | 加载 (默认 dynamic) |

## Test Results

```
go test ./internal/plugin/ -v  => ALL PASS (48 tests)
go build ./...                  => success
```

## Commit

```
4b31f8f feat(plugin): add category constants and ShouldLoadInCategory
```
