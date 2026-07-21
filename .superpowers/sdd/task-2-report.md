# Task 2 Report: build.go — 支持 PipelineCache 填充

## 状态：已完成

## 提交

- **Commit:** `40f04a9` feat(build): populate PipelineCache after full build
- **分支:** master

## 变更内容

- `internal/build/build.go`
  - 在 `Options` 结构体中（`AfterBuildSite` 之后）新增 `PipelineCache *PipelineCache` 字段，注释标注为 experimental。
  - `BuildSite` 中，在 `AfterBuildSite` 回调之后、`return` 之前，新增 `p.populateCache(opts.PipelineCache)` 调用，带 nil 守卫。
- `internal/build/pipeline.go`
  - 新增 `(*pipeline).populateCache(cache *PipelineCache)` 方法，填充 `Templates / I18nBundle / SCRegistry / MDRenderer / SiteCfg / Writer`，并盖上 `BuiltAt = time.Now()`。
  - `time` 包已在 pipeline.go 顶部 import，无需新增。

## 验证

- `go build ./internal/build/...` → BUILD SUCCESS
- `go build ./...` → 整模块编译通过（新 Options 字段未破坏下游）
- `go vet ./internal/build/...` → clean
- `go test ./internal/build/...` → `ok github.com/iannil/huan/internal/build 0.657s`（全部既有测试通过，无回归）

## 测试结果

本任务为纯接线（pure wiring），未新增单元测试。理由：

- 核心行为是 nil 守卫 + 结构体字段拷贝，自身没有值得单独覆盖的分支逻辑。
- 默认路径（`PipelineCache == nil`）已被既有测试套件覆盖，确保无回归。
- 真正消费该缓存的路径（增量构建 / daemon）将在后续 Task 中实现，届时会有端到端测试一并覆盖 populateCache 的产出。

## 注意事项 / 遗留

- populateCache 仅在 `BuildSite` 调用，`RenderPage` / `RenderPageToBytes`（单页入口）不填充缓存——与 brief 一致。
- 字段填充顺序与 `cache.go` 的 `PipelineCache` 结构体声明顺序一致，`BuiltAt` 最后赋值（每次全量构建覆盖，符合设计）。
- 本次 commit 仅包含 `internal/build/build.go` 和 `internal/build/pipeline.go` 两个源文件（按 brief 要求）。
- 工作区未提交的内容（不属于本 Task 范围，保持原样）：`.superpowers/sdd/task-{1,2}-{brief,report}.md` 的修改，以及 `docs/superpowers/plans/2026-07-21-incremental-build-plan.md`（SDD/process 产物）。
