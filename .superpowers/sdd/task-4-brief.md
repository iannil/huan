### Task 4: daemon 集成（serving + daemon.go）

**Files:**
- Modify: `internal/daemon/serving.go` — ServingOptions 新增 SSEHub，注册 /api/v1/events
- Modify: `internal/daemon/daemon.go` — 创建 SSEHub + SubscribeBus + Start + 注入

**Interfaces:**
- Consumes: SSEHub, SubscribeBus, HandleSubscribe, Start（Task 1-3）

- [ ] **Step 1: ServingOptions 新增 SSEHub 字段**

在 `internal/daemon/serving.go` 的 `ServingOptions` 结构体添加：

```go
	// SSEHub, if non-nil, enables the /api/v1/events real-time push endpoint.
	SSEHub *sse.SSEHub
```

并在 serving.go 顶部 import 块加入：

```go
	"github.com/iannil/huan/internal/daemon/sse"
```

- [ ] **Step 2: serving.go 注册 /api/v1/events**

在 `Start()` 中，ContentAPI handler 注册之后，添加：

```go
	// SSE real-time push — /api/v1/events
	if s.opts.SSEHub != nil {
		mux.HandleFunc("/api/v1/events", s.opts.SSEHub.HandleSubscribe)
	}
```

- [ ] **Step 3: daemon.go 创建 SSEHub + 订阅 + 启动心跳**

在 `internal/daemon/daemon.go` 的 `Run()` 中，创建 Serving 之前（EventBus d.bus 已就绪），添加：

```go
	// Init SSEHub (real-time push via /api/v1/events)
	sseHub := sse.NewSSEHub(log.Printf)
	sseHub.SubscribeBus(d.bus)
	sseWatchCtx, sseWatchCancel := context.WithCancel(context.Background())
	defer sseWatchCancel()
	sseHub.Start(sseWatchCtx)
	log.Println("daemon: SSE push enabled (/api/v1/events)")
```

在 Serving 创建时注入：

```go
	ServingOptions{
		// ... 原有字段
		SSEHub: sseHub,
	}
```

在 daemon.go 顶部 import 块加入：

```go
	"github.com/iannil/huan/internal/daemon/sse"
```

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```
Expected: BUILD SUCCESS

- [ ] **Step 5: 运行 daemon 测试确保无回归**

```bash
go test ./internal/daemon/... -v
```
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/daemon/serving.go internal/daemon/daemon.go
git commit -m "feat(daemon): wire SSEHub and /api/v1/events into daemon"
```

---

