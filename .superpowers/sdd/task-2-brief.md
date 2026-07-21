### Task 2: build.go — 支持 PipelineCache 填充

**Files:**
- Modify: `internal/build/build.go` — Options 新增 PipelineCache 字段，BuildSite 末尾填充
- Modify: `internal/build/pipeline.go` — 新增 pipeline.populateCache 方法

**Interfaces:**
- Consumes: `build.PipelineCache` (Task 1)
- Produces: `Options.PipelineCache` 字段, `BuildSite` 自动填充 cache

- [ ] **Step 1: 在 Options 中添加 PipelineCache 字段**

修改 `internal/build/build.go`，在 `Options` 结构体中（`AfterBuildSite` 字段之后）添加：

```go
	// PipelineCache, if non-nil, is populated with reusable build state
	// after BuildSite completes successfully. Used by daemon for
	// incremental builds. Pass a cache created by NewPipelineCache().
	// Experimental: API may change in future versions.
	PipelineCache *PipelineCache
```

- [ ] **Step 2: 在 pipeline 中添加 populateCache 方法**

在 `internal/build/pipeline.go` 末尾添加：

```go
// populateCache fills the PipelineCache with reusable rendering state
// after a successful full build. Called by BuildSite when opts.PipelineCache
// is non-nil.
func (p *pipeline) populateCache(cache *PipelineCache) {
	cache.Templates = p.tmpls
	cache.I18nBundle = p.i18nBundle
	cache.SCRegistry = p.scRegistry
	cache.MDRenderer = p.md
	cache.SiteCfg = p.cfg
	cache.Writer = p.writer
	cache.BuiltAt = time.Now()
}
```

注意：`pipeline.go` 顶部已 import `time`，无需新增。

- [ ] **Step 3: 在 BuildSite 中调用 populateCache**

修改 `internal/build/build.go` 的 `BuildSite` 函数，在 `AfterBuildSite` 回调之后、`return` 之前添加：

```go
	// Populate the pipeline cache if requested (for daemon incremental builds).
	if opts.PipelineCache != nil {
		p.populateCache(opts.PipelineCache)
	}

	return p.result, nil
```

- [ ] **Step 4: 编译验证**

```bash
go build ./internal/build/...
```
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行现有测试确保无回归**

```bash
go test ./internal/build/... -v
```
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/build/build.go internal/build/pipeline.go
git commit -m "feat(build): populate PipelineCache after full build"
```

---

