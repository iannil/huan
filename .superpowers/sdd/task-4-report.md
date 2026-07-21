# Task 4 Report — DAG OrderByDependency

**Date:** 2026-07-21
**Commit:** `7a60e5b` — `feat(dag): add OrderByDependency for incremental build ordering`

## Status

COMPLETE — all tests pass, no regressions, repo builds clean.

## Files Modified

- `/Users/rong.zhu/Code/zhurong/huan/internal/daemon/dag/graph.go` — added `OrderByDependency` method (after `PagePathFromSource`)
- `/Users/rong.zhu/Code/zhurong/huan/internal/daemon/dag/graph_test.go` — added 4 new test cases plus `indexOf` helper

## Implementation

`OrderByDependency(pagePaths []string) []string` returns page paths in the correct incremental-rendering order using Kahn's algorithm with stable (input-order-preserving) tie-breaking.

### Algorithm

1. Build the subgraph induced by `pagePaths`.
2. For each dependency edge `A -> B` (A's `DependsOn` contains B) where both endpoints are in the input set, record that **A must render before B** (B's `dependers` list includes A).
3. Repeatedly scan in input order, emitting any node whose dependers have all been emitted already.
4. On cycle / no-progress, flush remaining nodes in input order (preserves stability and never deadlocks).

## Test Summary

```
=== RUN   TestOrderByDependency_LeafBeforeParent        --- PASS
=== RUN   TestOrderByDependency_SinglePage              --- PASS
=== RUN   TestOrderByDependency_Empty                   --- PASS
=== RUN   TestOrderByDependency_UnknownPathsPreserved   --- PASS
```

Full DAG suite (8 tests) all pass. `go build ./...` clean.

## TDD Trail

- Red: added the 4 test cases verbatim from the brief; first run failed with compilation error (`OrderByDependency undefined`).
- First implementation attempt followed the brief's prose ("depended-upon pages first") — `TestOrderByDependency_LeafBeforeParent` failed because that direction contradicts the brief's own test assertions.
- Reconciled implementation to match the test assertions (dependents first). All 4 tests green.

## Concerns / Spec Note

The task brief's description and its verbatim test code are internally inconsistent on dependency direction, and this required reconciling during implementation:

- **Brief's prose** (line 10): "返回拓扑排序后的页面路径（被依赖的页面在前，依赖它们的页面在后）" — *depended-upon pages first, dependents later*. Under this rule, with the test graph where `/posts/hello/` `DependsOn` `[/posts/, /]`, the expected output would be `[/, /posts/, /posts/hello/]`.
- **Brief's example** (also line 10): "先渲染 `/posts/hello/`，再渲染引用它的 `/posts/`（section）和 `/`（home）" — *render the leaf first, then the section/home that reference it*. This is the opposite direction.
- **Brief's test assertions** require `helloIdx < postsIdx` and `helloIdx < homeIdx`, i.e. **dependents first, depended-upon later** — matching the example, not the prose.

Since the test code was specified verbatim and is the load-bearing artifact, the implementation was written to pass the tests: **dependents render first, depended-upon render later**. This also matches the real incremental-build rationale (an article's HTML should be regenerated before the list pages that reference it, so the lists reflect the updated article).

The godoc on the implemented method documents the actual behavior so future readers are not misled by the original brief's contradictory prose. If the original prose direction is actually what's desired, the test assertions in the brief would need to flip.

## Commits

- `7a60e5b` feat(dag): add OrderByDependency for incremental build ordering
