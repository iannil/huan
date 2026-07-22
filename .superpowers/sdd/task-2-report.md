# Task 2 Report — ContentIndex 查询方法

**Date:** 2026-07-22
**Status:** Complete
**Commit:** `ea23dae` — `feat(contentindex): add Query/Tags/Sections with filtering and pagination`

## Summary

为 `internal/daemon/contentindex/index.go` 增加了查询能力：`Filter` / `Result` 结构体、`Query(filter)` 方法、`Tags()` / `Sections()` 聚合方法，以及私有辅助函数 `containsString`、`containsLower`、`sortItemsByDateDesc`。严格遵循 TDD 红-绿流程：先加测试，确认编译失败，再加实现，全量通过后提交。

## Files Modified

- `internal/daemon/contentindex/index.go` (+138 行)
  - `Filter` / `Result` 结构体（带 JSON tag）
  - `Query(Filter) Result`：section/tag/full-text 过滤、按日期倒序稳定排序、分页（Page 默认 1、Limit 默认 10、上限 50）、空结果返回 `[]Item{}` 而非 `nil`
  - `Tags() map[string]int`、`Sections() map[string]int`
  - 辅助函数：`containsString`、`containsLower`、`sortItemsByDateDesc`（稳定插入排序）
- `internal/daemon/contentindex/index_test.go` (+107 行)
  - 11 个新测试：`TestQuery_SectionFilter` / `TagFilter` / `FullTextSearch` / `Pagination` / `LimitCapped` / `Defaults` / `SortByDateDesc` / `NoMatch`、`TestGetByURL_NotFound`、`TestTags`、`TestSections`
  - `loadTestIndex(t)` 内存索引 helper（直接填充 `ci.items`，不经磁盘）
  - 测试包内 `contains` 辅助（仅用于断言，与包内 `containsString` 区分命名）

## TDD 流程

1. **RED** — 追加测试后运行 `go test`：
   ```
   ci.Query undefined (type *ContentIndex has no field or method Query)
   undefined: Filter
   ...
   FAIL [build failed]
   ```
2. **GREEN** — 添加实现后运行 `go test ./internal/daemon/contentindex/ -v`：16 个测试全部 PASS（原有 5 + 新增 11）。
3. **回归** — `go build ./... && go test ./...`：全项目通过，无回归。

## Test Summary

```
=== RUN   TestLoadFromDir_LoadsAllSections          PASS
=== RUN   TestLoadFromDir_RelativeURL               PASS
=== RUN   TestLoadFromDir_MalformedJSON             PASS
=== RUN   TestLoadFromDir_EmptyDir                  PASS
=== RUN   TestLoadFromDir_NoAPIDir                  PASS
=== RUN   TestQuery_SectionFilter                   PASS
=== RUN   TestQuery_TagFilter                       PASS
=== RUN   TestQuery_FullTextSearch                  PASS
=== RUN   TestQuery_Pagination                      PASS
=== RUN   TestQuery_LimitCapped                     PASS
=== RUN   TestQuery_Defaults                        PASS
=== RUN   TestQuery_SortByDateDesc                  PASS
=== RUN   TestQuery_NoMatch                         PASS
=== RUN   TestGetByURL_NotFound                     PASS
=== RUN   TestTags                                  PASS
=== RUN   TestSections                              PASS
ok  github.com/iannil/huan/internal/daemon/contentindex  0.498s
```

16/16 通过；全项目 `go test ./...` 通过。

## 行为契约

- `Filter.Page < 1` → 默认 1；`Filter.Limit < 1` → 默认 10；`Filter.Limit > 50` → 截断到 50。
- `Filter.Query` 对 `Title` / `Summary` / `Description` 做大小写不敏感子串匹配（任一命中即保留）；空 query 跳过全文过滤。
- `Filter.Tag` 要求 item 至少包含该标签；`Filter.Section` 要求严格相等。
- 排序：按 `Date` 字符串字典序倒序（ISO-8601 日期即等同时间倒序），稳定插入排序。
- 分页越界安全：`start` / `end` 被 clamp 到 `[0, total]`；空页返回 `[]Item{}`（非 nil），便于 JSON 序列化为 `[]`。

## Concerns / Notes

- **排序算法**：插入排序 O(n²)。当前数据规模（单站点通常 < 10k 内容项）下足够；若未来索引规模显著增长，可替换为 `sort.SliceStable`。`Sort` 字段当前仅识别 `"date"`（默认），其他值同样走日期倒序——为后续扩展（如按标题）预留。
- **全文匹配范围**：brief 指定 Title/Summary/Description；`Item` 中 `Plain` 已被 Task 1 在解码阶段丢弃，不在匹配范围内——与契约一致。
- **测试 `contains` 命名**：测试包内新增的 `contains` 与包内 `containsString` 同义，刻意区分命名以避免与未来可能导出的 API 冲突，仅用于断言。
- 无阻塞问题，可进入 Task 3（HTTP handler 层 `/api/v1/*`）。
