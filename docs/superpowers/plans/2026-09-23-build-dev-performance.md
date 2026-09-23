# build / dev 性能优化第一批实施计划

**Goal:** 减少重复 Markdown 计算和多语言输入读取，并为 dev 后续页面级增量保留完整变更集合。

**Architecture:** 有界 Markdown 值缓存通过 build.Options 注入；dev 生命周期内持有缓存。Watcher 传完整路径，串行队列合并构建中事件，只有内容变更保留缓存。多语言 stale check 观察内容加载的原始字节，生成轻量快照。

**Tech Stack:** Go 标准库、现有 fsnotify / goldmark；不新增依赖。

**Spec:** `docs/superpowers/specs/2026-09-23-build-dev-performance-design.md`

## 约束

完整发布输出、插件调用、原始 Markdown 与 shortcode 语义不变；不修改已有脚注/CSS 工作；不覆盖系统安装。执行采用 subagent-driven-development 分工，主代理集成、独立审查。

## 1. Markdown 缓存

文件：`internal/build/markdown_cache.go`、`markdown_cache_test.go`、`build.go`、`pipeline.go`。
接口：`NewMarkdownCache(maxBytes int64) *MarkdownCache`、`Clear()`、`Stats() MarkdownCacheStats`、`Options.MarkdownCache`。

- [x] 写缺失 API 测试并运行失败：`go test ./internal/build -run '^TestMarkdownCache'`。
- [x] 实现 LRU、128 MiB dev 预算；固定开销计入预算，clone 字符串，错误不存。
- [x] 每轮重新 shortcode Expand；原始正文、展开正文、配置摘要共同构成键。
- [x] 验证正文/配置/动态 shortcode 失效、淘汰、并发、无 Page 指针复用。

## 2. Watcher 与串行调度

文件：`internal/dev/watcher.go`、`watcher_test.go`、`rebuild.go`、`rebuild_test.go`。
接口：`OnChanges func([]string)`；`NewRebuildQueue(func([]string))`、`Submit([]string)`、`Close()`、`Wait()`。

- [x] 先写超过五条路径、去重、取消测试；队列并发/递归 submit/nil 全量测试。
- [x] 完整保留路径，日志独立限长，generation 拒绝已失效 timer 回调。
- [x] 在同一锁内完成队列空检查和所有权释放；nil 全量覆盖待办路径。
- [x] 取消停止 timer 并等待已开始回调；队列关闭拒绝新请求、排空已接受请求。

## 3. 多语言共享读取

文件：`internal/content/load.go`、`load_observer_test.go`、`internal/build/inputs.go`、`i18n_strict.go`、`i18n_snapshot_test.go`。
接口：`LoadDirWithObserver(contentDir string, observe func(string, []byte)) ([]*Page,error)`。

- [x] 先验证缺失 observer/snapshot API 失败。
- [x] snapshot 仅保存原始字节 hash、空正文标志、sidecar source_hash，诊断顺序不变。
- [x] 与旧磁盘检查逐项对比；共享读取与旧双读取标准 benchmark。

## 4. CLI 集成与验收

文件：`cmd/huan/dev.go`、`dev_cache_test.go`、`internal/build/markdown_rebuild_test.go`、`scripts/bench-build.py`。

- [x] 分类测试先红后绿；dev 注入缓存、新队列、shutdown join。
- [x] 双语完整输出对比：无变化、正文、元数据/slug、改名、删除、Markup 配置。
- [x] `go test ./...`。
- [x] `go test -race ./internal/build ./internal/content ./internal/dev ./cmd/huan`。
- [x] 实际站点交替整站 build、dev 保存到 HTTP 新内容的延迟测量。
- [x] 独立审查、完成报告和 memory 更新。

## 后续阶段

页面级依赖失效、搜索条目缓存、跨进程缓存、多语言并行另分阶段。第一批没有跳过页面模板/聚合输出/静态插件，不能称为完整页面级增量引擎。

## 追加验收：异步旧目录清理

- [x] BuildDirSwapper 的阻塞清理、等待、第二轮复用与回滚测试先红后绿。
- [x] dev shutdown 在 queue.Wait 后调用 swapper.Wait；全量回归与四包 race 再次通过。
- [x] dev 两组交替会话、每组预热一次加三次测量，逐次检查 HTTP 新正文，完成报告记录全部数值。
