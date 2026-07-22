### Task 4: daemon 集成（serving + builder + daemon.go）

**Files:**
- Modify: `internal/daemon/serving.go` — 注册 /api/v1/ handler，ServingOptions 新增 ContentAPI
- Modify: `internal/daemon/builder.go` — ContentIndex 刷新钩子，BuilderOptions 新增 ContentIndex
- Modify: `internal/daemon/daemon.go` — 创建 ContentIndex + Handler，注入

**Interfaces:**
- Consumes: ContentIndex, Handler (Task 1-3)
- Produces: ServingOptions.ContentAPI, BuilderOptions.ContentIndex, daemon wiring

- [ ] **Step 1: ServingOptions 新增 ContentAPI 字段**

在 `internal/daemon/serving.go` 的 `ServingOptions` 结构体中添加字段：

```go
	ContentAPI    http.Handler              // optional /api/v1/* content query handler
```

- [ ] **Step 2: serving.go Start() 注册 /api/v1/**

在 `internal/daemon/serving.go` 的 `Start()` 方法中，注册 admin handler 之前（或 health 之后），添加：

```go
	// Content query API (public, read-only) — /api/v1/*
	if s.opts.ContentAPI != nil {
		mux.Handle("/api/v1/", s.opts.ContentAPI)
	}
```

放置位置：在 metrics handler 之后、admin handler 之前（确保 `/api/v1/` 精确匹配优先于 `/` catch-all）。

- [ ] **Step 3: BuilderOptions 新增 ContentIndex 字段**

在 `internal/daemon/builder.go` 的 `BuilderOptions` 结构体中添加：

```go
	// ContentIndex is reloaded from /api/*.json after each build so the
	// query API serves fresh data.
	ContentIndex *contentindex.ContentIndex
```

并在 builder.go 顶部添加 import（若不存在）：

```go
	"github.com/iannil/huan/internal/daemon/contentindex"
```

- [ ] **Step 4: builder.go 添加 ContentIndex 刷新钩子**

在 `executeFullBuild` 成功日志后（JITCache.Clear 附近）添加：

```go
	if b.opts.ContentIndex != nil {
		if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
			b.opts.Logf("builder: content index reload: %v", err)
		}
	}
```

在 `IncrementalBuild` 的 JITCache.Remove 循环之后添加同样代码：

```go
	if b.opts.ContentIndex != nil {
		if err := b.opts.ContentIndex.LoadFromDir(b.opts.OutputDir); err != nil {
			b.opts.Logf("builder: content index reload: %v", err)
		}
	}
```

- [ ] **Step 5: daemon.go 创建并注入 ContentIndex + Handler**

在 `internal/daemon/daemon.go` 的 `Run()` 中，创建 Builder 之前，添加：

```go
	// 7.5 Init ContentIndex (for /api/v1/* query API)
	var contentAPI http.Handler
	var contentIdx *contentindex.ContentIndex
	if cfg.AI.ContentAPI {
		contentIdx = contentindex.NewContentIndex(cfg.BaseURL)
		if err := contentIdx.LoadFromDir(tmpDir); err != nil {
			log.Printf("daemon: content index load: %v", err)
		}
		contentAPI = contentindex.NewHandler(contentIdx)
		log.Println("daemon: content query API enabled (/api/v1/*)")
	} else {
		log.Println("daemon: content query API disabled (ai.contentAPI not set)")
	}
```

在 Builder 创建时注入 `ContentIndex: contentIdx`。

在 Serving 创建时注入 `ContentAPI: contentAPI`。

在 daemon.go 顶部添加 import：

```go
	"github.com/iannil/huan/internal/daemon/contentindex"
```

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 7: 运行 daemon 测试确保无回归**

```bash
go test ./internal/daemon/... -v
```
Expected: ALL PASS

- [ ] **Step 8: 提交**

```bash
git add internal/daemon/serving.go internal/daemon/builder.go internal/daemon/daemon.go
git commit -m "feat(daemon): wire ContentIndex and /api/v1/ query API into daemon"
```

---

