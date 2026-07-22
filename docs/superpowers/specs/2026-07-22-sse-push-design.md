# 设计文档：SSE 实时推送（Server-Sent Events）

- **日期**：2026-07-22
- **状态**：Draft
- **关联**：[内容查询 REST API](2026-07-22-content-query-api-design.md)、[增量构建](2026-07-21-incremental-build-design.md)、EventBus（`internal/daemon/eventbus/`）
- **实现阶段**：v0.12.0

## 1. 背景

daemon 作为持久化进程，已有完整的内部事件系统（EventBus：BuildCompleted / ContentChanged / PluginLoaded 等），但这些事件**只存在于 daemon 内部**，前端无法感知。

当前前端（Admin SPA 或任何消费 `/api/v1/*` 的客户端）只能**轮询**查询 API 检测变更——低效、延迟高。daemon 持久化进程的**实时性优势完全未利用**（这是 SSG 做不到的能力）。

已有基础：
- EventBus（`internal/daemon/eventbus/`）— 异步事件总线，已有构建/内容/插件事件
- dev 包的 `LiveReloadHub`（`internal/dev/livereload.go`）— WebSocket 广播模式可参考（coder/websocket）

## 2. 设计目标

1. **实时推送**：daemon 事件即时到达前端（构建完成、内容变更、插件加载）
2. **复用 EventBus**：事件源是已有的 daemon EventBus，不重新造事件源
3. **SSE 协议**：单向推送（服务器→客户端）匹配需求，HTTP 原生，浏览器 EventSource 原生支持 + 自动重连
4. **全量广播**：所有客户端收到所有事件（YAGNI，事件量小，客户端自行过滤）
5. **容量保护**：慢客户端不阻塞其他客户端，连接数上限防 DoS

## 3. 架构总览

```
daemon 内部事件
  BuildCompleted / ContentChanged / PluginLoaded / BuildFailed ...
        │
        ▼
   EventBus（已有）
        │ Subscribe（全量）
        ▼
   SSEHub（新增）─────── 广播 ──────┬──────┬──────┐
        │                            │      │      │
   GET /api/v1/events          客户端1 客户端2 客户端N
   (text/event-stream)         (EventSource)
```

## 4. 核心组件

### 4.1 SSEHub

`internal/daemon/sse/hub.go`（新增）：

```go
// SSEHub 管理 Server-Sent Events 客户端连接，广播 daemon 事件给所有连接的浏览器。
// 订阅 EventBus（全量），每条事件转发给所有客户端。
type SSEHub struct {
    mu      sync.RWMutex
    clients map[chan Event]struct{}
    logf    func(format string, args ...any)
}

func NewSSEHub(logf func(string, ...any)) *SSEHub

// SubscribeBus 订阅 daemon EventBus 的构建/内容/插件事件，转发给所有 SSE 客户端。
func (h *SSEHub) SubscribeBus(bus eventbus.EventBus)

// HandleSubscribe 是 HTTP handler，服务 GET /api/v1/events。
func (h *SSEHub) HandleSubscribe(w http.ResponseWriter, r *http.Request)

// Broadcast 向所有客户端广播事件（非阻塞，慢客户端丢弃）。
func (h *SSEHub) Broadcast(event Event)

// ClientCount 返回当前连接数。
func (h *SSEHub) ClientCount() int
```

### 4.2 Event（SSE 消息）

```go
// Event 是发送给客户端的 SSE 消息。
type Event struct {
    Type string `json:"type"`  // build_completed, content_changed, ...
    Data any    `json:"data"`  // 事件负载
}
```

SSE 线格式（`text/event-stream`）：

```
event: build_completed
data: {"pages":120,"duration_ms":3400}

event: content_changed
data: {"changed_files":["content/posts/hello.md"]}

```

### 4.3 关键设计决策

1. **SSE（非 WebSocket）**：单向推送（服务器→客户端）正好匹配需求；HTTP 原生（text/event-stream），浏览器 EventSource API 原生支持 + 自动重连；无握手/ping-pong 复杂度。
2. **全量广播**：daemon 事件量小（构建/内容/插件都是低频），全量广播无带宽问题；无订阅状态 = 实现最简。
3. **心跳保活**：每 15 秒发送 SSE 注释行（`:heartbeat\n\n`），防止代理/负载均衡器超时断开空闲连接。
4. **慢客户端保护**：每客户端带缓冲通道（容量 16），满则丢弃（不阻塞其他客户端）。
5. **连接数上限**：maxClients（1000）防 DoS，超限返回 503。

## 5. 客户端连接生命周期

```
1. 浏览器: new EventSource("/api/v1/events")
2. daemon: SSEHub.HandleSubscribe
   a. 检查连接数上限（maxClients）
   b. 设置 SSE headers（Content-Type: text/event-stream, Cache-Control: no-cache, Connection: keep-alive, X-Accel-Buffering: no）
   c. 创建客户端通道 ch（缓冲 16），注册到 clients map
   d. 循环：
      - 从 ch 读事件 → fmt.Fprintf(w, "event: %s\ndata: %s\n\n", type, json)
      - http.Flusher 立即推送
      - <-r.Context().Done()（客户端断开）→ 注销 ch，退出
3. 浏览器: EventSource.onmessage / addEventListener 收到事件
```

## 6. EventBus → SSEHub 桥接

```go
// SubscribeBus 订阅 daemon EventBus 事件并转发给所有 SSE 客户端。
func (h *SSEHub) SubscribeBus(bus eventbus.EventBus) {
    for _, eventType := range []eventbus.EventType{
        eventbus.EventBuildCompleted,
        eventbus.EventBuildFailed,
        eventbus.EventContentChanged,
        eventbus.EventPluginLoaded,
        eventbus.EventPluginUnloaded,
    } {
        bus.Subscribe(eventType, func(ctx context.Context, ev eventbus.Event) error {
            h.Broadcast(Event{Type: ev.Type.String(), Data: ev.Payload})
            return nil
        })
    }
}
```

daemon.go 只需调一行 `sseHub.SubscribeBus(bus)`。

## 7. 心跳与重连

```go
// startHeartbeat 每 15 秒广播 SSE 注释行（:heartbeat\n\n）。
// SSE 规范：以 ":" 开头的行是注释，客户端忽略，但保持连接活跃。
func (h *SSEHub) startHeartbeat(ctx context.Context)
```

浏览器 `EventSource` 断开后自动重连（协议内置）。daemon 重启后客户端自动重新连接 `/api/v1/events`。

## 8. 容量保护

```go
const (
    clientBufferSize = 16   // 每客户端缓冲事件数
    maxClients       = 1000 // 最大并发连接数
)

// Broadcast 非阻塞发送给每个客户端通道。
// 通道满（慢客户端）→ 丢弃该事件 + log warn（不影响其他客户端）。
func (h *SSEHub) Broadcast(event Event) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for ch := range h.clients {
        select {
        case ch <- event:
        default:
            h.logf("sse: client buffer full, dropping event")
        }
    }
}
```

## 9. daemon 集成

### ServingOptions 新增

```go
type ServingOptions struct {
    // ... 原有字段
    SSEHub *sse.SSEHub  // 可选，启用 /api/v1/events 实时推送
}
```

### serving.go 注册端点

```go
if s.opts.SSEHub != nil {
    mux.HandleFunc("/api/v1/events", s.opts.SSEHub.HandleSubscribe)
}
```

### daemon.go 创建 + 订阅

```go
// 创建 SSEHub + 订阅 EventBus
sseHub := sse.NewSSEHub(log.Printf)
sseHub.SubscribeBus(d.bus)

// 注入 Serving
ServingOptions{
    // ...
    SSEHub: sseHub,
}
```

## 10. SSE 事件类型

| SSE event | 来源 EventBus 事件 | data 示例 |
|-----------|-------------------|----------|
| `build_completed` | EventBuildCompleted | `{"pages":120,"duration_ms":3400,"incremental":false}` |
| `build_failed` | EventBuildFailed | `{"error":"..."}` |
| `content_changed` | EventContentChanged | `{"changed_files":["content/posts/hello.md"]}` |
| `plugin_loaded` | EventPluginLoaded | `{"name":"cloudflare","source":"loaded"}` |
| `plugin_unloaded` | EventPluginUnloaded | `{"name":"cloudflare"}` |

## 11. 安全考虑

- `/api/v1/events` 公开只读（与查询 API 一致，无 token）
- **不推送敏感负载**：构建事件只含统计，内容变更只含文件路径，不含内容本身
- **Payload 脱敏**：不暴露 draft、不暴露完整正文
- **连接数上限**：maxClients（1000）防 DoS

## 12. 测试策略

### 12.1 单元测试

| 测试 | 说明 |
|------|------|
| `TestHub_Broadcast_DeliversToClient` | 注册客户端 → Broadcast → 收到事件 |
| `TestHub_Broadcast_MultipleClients` | 多客户端都收到 |
| `TestHub_Broadcast_SlowClientDrops` | 通道满 → 丢弃 + 不阻塞其他 |
| `TestHub_ClientCount` | 连接/断开后计数正确 |
| `TestHub_MaxClientsRejected` | 超限 → 503 |
| `TestHub_SubscribeBus_Bridges` | EventBus 事件 → Hub 广播 |
| `TestHandleSubscribe_SSEHeaders` | 响应头正确 |
| `TestHandleSubscribe_EventFormat` | SSE 线格式正确 |
| `TestHandleSubscribe_Heartbeat` | 心跳注释行定时 |
| `TestHandleSubscribe_DisconnectCleanup` | 断开后注销 |

### 12.2 集成测试

| 测试 | 说明 |
|------|------|
| `TestDaemon_SSE_BuildEventPushed` | 触发构建 → SSE 收到 build_completed |
| `TestDaemon_SSE_ContentChangedPushed` | 内容变更 → 收到 content_changed |

## 13. 文件变更清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/daemon/sse/hub.go` | 新增 | SSEHub + Event + SubscribeBus + Broadcast + 心跳 |
| `internal/daemon/sse/hub_test.go` | 新增 | Hub 单元测试 |
| `internal/daemon/sse/handler.go` | 新增 | HandleSubscribe HTTP handler |
| `internal/daemon/sse/handler_test.go` | 新增 | Handler 单元测试 |
| `internal/daemon/serving.go` | 修改 | 注册 /api/v1/events，ServingOptions 新增 SSEHub |
| `internal/daemon/daemon.go` | 修改 | 创建 SSEHub + 订阅 EventBus，注入 Serving |

## 14. 使用示例

```javascript
// 浏览器端（EventSource 原生 API）
const es = new EventSource("/api/v1/events");

es.addEventListener("build_completed", (e) => {
    const data = JSON.parse(e.data);
    console.log(`Build done: ${data.pages} pages in ${data.duration_ms}ms`);
});

es.addEventListener("content_changed", (e) => {
    const data = JSON.parse(e.data);
    console.log("Content changed:", data.changed_files);
});

es.addEventListener("plugin_loaded", (e) => {
    console.log("Plugin loaded:", JSON.parse(e.data).name);
});
```

## 15. 未来扩展

- **按类型订阅**：`?types=build,content` 服务端过滤（当前全量广播，客户端自行过滤）
- **认证 SSE**：受保护事件需 token（会员/付费场景）
- **WebSocket 双向**：客户端可发指令（如触发构建、订阅特定页面变更）
- **跨域支持**：CORS 头允许跨域 EventSource 消费
- **消息压缩**：gzip 压缩 SSE 流（大 payload）
