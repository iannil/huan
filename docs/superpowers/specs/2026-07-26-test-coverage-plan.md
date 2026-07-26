# 设计文档：huan 测试覆盖增强计划

> **日期**：2026-07-26
> **状态**：草稿
> **目标**：系统性补齐 huan daemon 核心 + 插件系统 + 插件实现的测试覆盖，提升生产就绪度
> **工具链**：`testing` + `net/http/httptest` + `testify/assert`（仅 assert 包）
> **测试风格**：table-driven + subtests + interface-based manual mocking

---

## 一、范围与优先级

### P0（本周完成）— 出问题会导致 daemon 崩溃或数据丢失

| 优先级 | 包路径 | 当前状态 | 目标 |
|--------|--------|---------|------|
| P0 | `internal/daemon/eventbus/` | 已有基础测试 | 补并发安全、panic 恢复、关闭后行为、大量订阅者等边界 |
| P0 | `internal/daemon/dag/` | 已有 graph_test + bench | 补 DAG 规则测试、空图/循环依赖/并发安全 |
| P0 | `internal/daemon/cache/jit.go` | 已有 jit_test + bench | 补 TTL 过期、LRU 淘汰、并发读写、内存边界 |
| P0 | `internal/plugin/lifecycle.go` | 已有 lifecycle_test | **重写**—当前测试覆盖不到热插拔核心路径 |

### P1（下周完成）— 功能正确但边界未覆盖

| 优先级 | 包路径 | 当前状态 | 目标 |
|--------|--------|---------|------|
| P1 | `internal/daemon/sse/` | 已有 hub_test + handler_test | 补 maxClients 限制、断开重连、心跳超时 |
| P1 | `internal/daemon/contentindex/` | 已有 index_test + handler_test | 补增量更新、空索引、URL 推导 fallback |
| P1 | `internal/build/cache.go` + `incremental.go` | 已有 cache_test + incremental_test | 补模板变更检测、DAG 依赖传播、空变更场景 |
| P1 | `internal/build/jit.go` | 已有 jit_test | 补 JITCache 集成、并发 JIT 请求、不存在页面 |

### P2（两周内）— 插件实现集成测试

| 优先级 | 包路径 | 当前状态 | 目标 |
|--------|--------|---------|------|
| P2 | `plugins/cloudflare/` | 无测试 | 插件集成测试（mock HTTP client） |
| P2 | `plugins/qwen3/` | 无测试 | 插件集成测试（chunker/parse/quality） |
| P2 | `plugins/image-pipeline/` | 已有 4 个测试文件 | 补缺失场景（WebP/AVIF 跳过、空 HTML、大文件） |

### P3（视情况）— 边缘模块

| 优先级 | 包路径 | 当前状态 | 目标 |
|--------|--------|---------|------|
| P3 | `internal/admin/` | 已有 api_test + auth_test + audit_test | 补插件管理端点测试、权限边界 |
| P3 | `internal/seo/` (3 个子包) | 各有 plugin_test | 补配置注入、空配置行为 |

---

## 二、测试策略

### 2.1 分层策略

```
┌─────────────────────────────┐
│  集成测试 (daemon 全链路)    │  P3 — 视情况
├─────────────────────────────┤
│  模块集成测试 (插件 + daemon) │  P2 — 两周内
├─────────────────────────────┤
│  单元测试 (内部包)           │  P0/P1 — 核心
└─────────────────────────────┘
```

### 2.2 每个包的测试结构

每个目标包按以下模板组织测试文件：

```
internal/daemon/eventbus/
├── bus.go
├── bus_test.go          # P0 新增/增强
├── types.go
└── types_test.go        # 现有（保持）
```

### 2.3 Mock 策略

Go 隐式接口实现让 mock 极轻量。不引入 mock 框架，使用手写 mock struct：

```go
// mock_eventbus.go (在 _test.go 文件中或独立的 test 辅助文件)
type mockBus struct {
    publishFn func(ctx context.Context, event eventbus.Event) error
    subscribeFn func(eventType eventbus.EventType, handler eventbus.Handler) string
}

func (m *mockBus) Publish(ctx context.Context, event eventbus.Event) error {
    if m.publishFn != nil { return m.publishFn(ctx, event) }
    return nil
}
func (m *mockBus) Subscribe(eventType eventbus.EventType, handler eventbus.Handler) string {
    if m.subscribeFn != nil { return m.subscribeFn(eventType, handler) }
    return "mock-id"
}
// ... Unsubscribe, Close
```

### 2.4 测试数据

测试数据统一放在每个包的 `testdata/` 目录下。小数据集用 inline table 定义；大数据集（如完整的站点结构）放在 `testdata/` 文件里。

### 2.5 测试隔离

- 每个测试创建自己的依赖实例，不共享全局状态
- `t.Parallel()` 在独立测试间使用，共享资源（如 eventbus）的测试串行
- 使用 `t.Cleanup` 而非 defer 清理（即使 t.Fatalf 也能清理）

---

## 三、P0 详细测试计划

### 3.1 eventbus — 边界与并发安全

当前测试覆盖：发布/订阅、取消订阅、关闭后发布报错、handler 超时、多 handler

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestConcurrentPublishSubscribe` | 10 goroutine 并发发布/订阅 | 无 data race，handler 计数正确 |
| `TestPublishOnClosedBus` | 关闭后 publish | 已有，补 Race 检测 |
| `TestUnsubscribeNonExistent` | 取消不存在的 handler | 不 panic |
| `TestSubscribeAfterClose` | 关闭后 subscribe | 不 panic（当前允许，验证行为） |
| `TestHandlerPanic` | handler 中 panic | 不崩 daemon，recover 并继续 |
| `TestManySubscribers` | 1000 个 handler 订阅同一事件 | 性能不退化，不漏 handler |
| `TestPublishNoSubscribers` | 发布无订阅者的事件 | 不 panic，无 goroutine leak |
| `TestBusRace` | `go test -race` 全部通过 | 新增一个专门的 race test |

**关键实现注意事项**：
- `Publish` 中 go func 里的 panic 必须 recover，否则一个 handler 崩了整条 daemon
- 当前代码没有 recover，这是 bug

### 3.2 DAG — 依赖图规则与并发

当前测试覆盖：基本 graph 操作、benchmark

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestEmptyDAG` | 空图拓扑排序 | 返回空切片 |
| `TestSingleNode` | 单节点排序 | 返回该节点 |
| `TestCyclicDependency` | 循环依赖检测 | 返回错误 |
| `TestDependencyOrdering` | 多节点依赖排序 | 拓扑序正确 |
| `TestDependsOnDependedBy` | DependsOn/DependedBy 双向一致 | 双向引用正确 |
| `TestConcurrentAccess` | 并发读写 | 无 data race |
| `TestOrderByDependency` | OrderByDependency 完整场景 | 按依赖序输出 |

### 3.3 JITCache — LRU + TTL 过期

当前测试覆盖：基本 Get/Set、benchmark

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestLRUEviction` | 超过 maxEntries 时淘汰最久未访问 | 最旧条目被淘汰 |
| `TestTTLExpiry` | TTL 过期后 Get 返回 nil | 过期后不可访问 |
| `TestGetRefreshesLRU` | Get 操作刷新 LRU 顺序 | 被 Get 的条目排在后面 |
| `TestClear` | Clear 后所有条目不可访问 | 内存释放 |
| `TestRemove` | Remove 特定 key | 仅该 key 被移除 |
| `TestConcurrentGetSet` | 并发读写 | 无 data race |
| `TestZeroMaxEntries` | maxEntries=0 或负数 | 退化为 no-op cache |
| `TestZeroTTL` | TTL=0 不缓存 | 每次 Get 返回 nil |

### 3.4 LifecycleManager — 热插拔核心

当前测试状态：`lifecycle_test.go` 存在但覆盖不足。需重写。

**测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestStart_CompiledPlugins` | Start 时已注册的编译期插件 | 被 tracked，published EventPluginLoaded |
| `TestStart_LoadSOPlugins` | Start 扫描 .so 目录加载 | 成功注册，published 事件 |
| `TestLoad_SOPathValidation` | 路径在插件目录外 | 拒绝，报错 |
| `TestLoad_NameConflict` | 同名插件已存在 | 返回 ErrPluginNameConflict |
| `TestLoad_SubscribeEvents` | 插件实现 EventSubscriber | 自动订阅声明的事件 |
| `TestUnload_RemovePlugin` | 卸载运行时插件 | 从 registry 移除，取消订阅，publish 事件 |
| `TestUnload_CompiledPlugin` | 卸载编译期插件 | 拒绝 |
| `TestUnload_NonExistent` | 卸载不存在的插件 | 返回 ErrPluginNotFound |
| `TestReload_Success` | 热重载插件 | 新插件替换旧插件，publish 事件 |
| `TestReload_Rollback` | 热重载失败 | 旧插件保留，publish 错误事件 |
| `TestReload_NameChanged` | 重载时插件名变更 | 返回错误，提示使用 Unload+Load |
| `TestReload_CompiledPlugin` | 重载编译期插件 | 拒绝 |
| `TestSubscribeEvents_NoSubscriber` | 插件未实现 EventSubscriber | 不 panic，不注册 |
| `TestSubscribeEvents_EmptyEvents` | SubscribedEvents 返回空 | 不注册 |
| `TestList_IncludesCompiled` | List 包含编译期插件 | 元数据正确 |
| `TestList_MetadataProvider` | 插件实现 MetadataProvider | 版本/作者/标签等正确 |
| `TestStop_Cleanup` | Stop 卸载 + 取消订阅 | 所有运行时插件移除，订阅清理 |

**LifecycleManager 测试需要 mock 以下依赖**：
- `eventbus.EventBus`（mock bus，验证发布/订阅行为）
- `*Loader`（mock LoadPlugin 返回测试插件，不加载真实 .so）
- 文件系统（用 temp dir 测试路径验证，不依赖真实插件目录）

**插件 watcher 测试**：
- `TestWatcher_CreateLoad`：创建 .so 文件 → 自动加载
- `TestWatcher_RemoveUnload`：删除 .so 文件 → 自动卸载
- `TestWatcher_Debounce`：连续修改 → 500ms 后只触发一次
- `TestWatcher_NonSOIgnored`：非 .so 文件变更 → 忽略

---

## 四、P1 详细测试计划

### 4.1 SSE Hub

当前已有 hub_test.go 和 handler_test.go。

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestMaxClients` | 超过 maxClients 上限 | 新连接被拒绝 |
| `TestClientDisconnect` | 客户端断开后清理 | 不泄漏 goroutine |
| `TestHeartbeat` | 15s 心跳 | 定时发送 keepalive |
| `TestBroadcastFilter` | 广播时过滤特定事件类型 | 只广播匹配类型 |
| `TestConcurrentBroadcast` | 并发广播 | 无 data race |

### 4.2 ContentIndex

当前已有 index_test.go 和 handler_test.go。

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestIncrementalUpdate` | 增量更新索引 | 新增/删除/修改条目正确 |
| `TestEmptyIndex` | 空索引 | 查询返回空 |
| `TestResolveSourceFallback` | URL 推导 fallback | 多层 fallback 正确 |
| `TestConcurrentReadWrite` | 并发读写 | 无 data race |

### 4.3 增量构建

当前已有 cache_test.go 和 incremental_test.go。

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestHasTemplateChanges` | 模板变更检测 | 正确返回 true/false |
| `TestEmptyChangeSet` | 无变更时增量构建 | 返回 0 pages rendered |
| `TestDAGPropagation` | 单文章变更传播到聚合页 | section/home/tag 被标记为脏 |
| `TestFullRebuildTrigger` | 模板/i18n/config 变更触发全量 | 回退到全量构建 |

### 4.4 JIT 渲染

当前已有 jit_test.go。

**新增测试场景**：

| 测试名 | 场景 | 验证点 |
|--------|------|--------|
| `TestRenderNonExistentPage` | 请求不存在的页面 | 返回 404 |
| `TestJITCacheIntegration` | JIT 结果被缓存 | 第二次请求命中缓存 |
| `TestConcurrentJIT` | 并发请求同一页面 | 只渲染一次，不重复 |
| `TestJITAfterContentChange` | 内容变更后 JIT 缓存失效 | 返回新内容 |

---

## 五、P2 详细测试计划

### 5.1 插件集成测试（概要）

三个 .so 插件的集成测试策略相同：**不加载真实 .so**，而是直接调用插件代码的导出函数，验证其逻辑。

**cloudflare 插件**：
- Mock HTTP client（Cloudflare API 响应）
- 测试 pages deploy / r2 upload / worker deploy
- 测试重试逻辑、并发控制、manifest 生成

**qwen3 插件**：
- 测试 chunker（段落切块 + sliding window）
- 测试 parse（响应解析）
- 测试 quality gate
- Mock API client（模拟 Qwen3 响应）

**image-pipeline 插件**：
- 已有测试，补缺失场景
- 空 HTML 输入
- 图片 URL 解析边界（相对/绝对路径、base64 data URL）
- srcset/picture 注入正确性

---

## 六、非功能性要求

### 6.1 Race 检测

所有新增测试必须通过 `go test -race ./internal/daemon/...` 和 `go test -race ./internal/plugin/...`。

### 6.2 测试性能

- 单元测试：单个测试 < 100ms
- 集成测试：单个测试 < 2s
- 整个 P0+P1 测试套件 < 30s

### 6.3 代码组织

- 每个目标包新增一个 `*_test.go` 文件（或扩展现有）
- Mock 类型放在 `*_test.go` 文件内（Go 允许 _test.go 包内定义类型）
- 如果需要跨测试文件共享 helper，放在 `helpers_test.go` 中

---

## 七、实施顺序

1. **eventbus** — 最小依赖，先加固事件基础
2. **DAG** — 独立的数据结构，无外部依赖
3. **JITCache** — 独立的缓存实现，无外部依赖
4. **LifecycleManager** — 依赖 eventbus + Loader，需要 mock
5. **SSE** — 依赖 eventbus，需要 mock
6. **ContentIndex** — 依赖构建输出
7. **增量构建** — 依赖 DAG + Cache，需要 mock
8. **JIT 渲染** — 依赖增量构建 + JITCache
9. **插件集成测试** — 依赖插件代码
10. **Admin API / SEO** — 视情况

---

## 八、验收标准

- [ ] `go test -race ./...` 通过
- [ ] P0 包测试覆盖率 ≥ 80%（语句覆盖）
- [ ] P1 包测试覆盖率 ≥ 60%
- [ ] 所有测试用例使用 table-driven + subtests 风格
- [ ] 无 testify/mock 或 testify/suite 依赖
- [ ] 测试文件按 `docs/standards/documentation.md` 规范组织