# build / dev 性能优化第一批完成报告

日期：2026-09-23。基线：`7609eef`，保留本轮开始前工作区已有的脚注和主题 CSS 修改，两组程序使用对应源码重新编译插件。本轮没有发布或替换系统 huan。

## 结果

环境：Apple M5 Max，Go 1.27.1，zhurongshuo 双语站点。build 原始语料 2,959 个 Markdown、7,245 个输出文件。dev 在临时输入副本中增加一篇探测文章，每次修改正文并验证 HTTP 返回新内容。

| 指标 | 基线中位数 | 优化后中位数 | 耗时变化 |
|---|---:|---:|---:|
| 全量 build，CLI 全程 | 2.296s | 2.264s | -1.4%，收益较小，不宣称显著提速 |
| dev 重建执行 | 2.734s | 1.912s | -30.1% |
| dev 保存到 HTTP 新内容，含默认 400ms 防抖 | 3.136s | 2.315s | -26.2% |

build 预热一次后交替运行五轮；dev 按 baseline/candidate/baseline/candidate 四个会话测量，每会话预热一次、记录三次修改，共每版本六个样本。正式整站测量未并行运行其他基准。HTTP 验证不包含浏览器绘制和 WebSocket 送达时间；未清理操作系统文件缓存，不代表真正冷启动。

实际 dev 每次单篇编辑有 2,822 次 Markdown 缓存命中、1 次重算。双语 Markdown 阶段合计约几毫秒。旧目录删除之前阻塞约 550ms；现在目录交换本身约 0.24–0.29ms，删除仍然执行，只是移出了刷新关键路径。

缓存预算为 128MiB；该站点日志报告缓存约 95.9MiB（字符串与保守条目开销）。这是以额外常驻内存换重复构建延迟，不是降低峰值内存，也不是完整进程 RSS 测量。

## 实现

1. `internal/build/markdown_cache.go`：并发安全、有界 LRU。只保存 HTML、纯文本、摘要、字数，不保留 Page 指针。原始正文、shortcode 展开正文、完整 Markup 配置的摘要构成键。每次仍先执行 shortcode，错误不缓存；字符串独立复制以限制底层内存保留。
2. `cmd/huan/dev.go`：dev 持有跨构建 Markdown 缓存。全部变更都是 content 下 Markdown 时保留缓存；未知、资源、配置、模板及手工全量请求清空缓存。所有模板、聚合输出、静态处理和插件调用继续执行。
3. `internal/dev/watcher.go`：新增 `OnChanges func([]string)`，保留旧回调兼容；传完整去重路径，五条上限仅用于日志。取消会停止 timer 并等待已开始回调。
4. `internal/dev/rebuild.go`：串行调度、合并在途变更，nil 全量请求优先；待办检查和执行权释放在同一锁内，消除原 busy/pending 交接丢事件窗口。关闭后拒绝新请求并排空已接受工作。
5. `internal/content/load.go` 和 `internal/build/inputs.go`：内容读取时观察原始字节，形成轻量 stale 检查快照，避免多语言构建重复读取。hash 仍计算全部原始字节，空正文/缺源/source_hash 格式和诊断顺序保留。
6. `internal/build/swap.go`：保留 `SwapBuildDir` 原同步语义，新增 dev 使用的 `BuildDirSwapper`。一次最多一个后台删除，后续交换前等待上轮清理，退出时等待回收；双 rename 失败仍走既有 best-effort 回滚。

没有新增第三方依赖或 CLI 参数，默认 debounce 未改变。后台清理后的整个进程退出仍等待删除完成，没有把未完成工作留给下次启动。

## 正确性与验收

- `go test ./...` 通过。
- `go test -race ./internal/build ./internal/content ./internal/dev ./cmd/huan` 通过；异步清理追加后再次运行通过。
- 单元测试覆盖动态 shortcode、正文/配置失效、LRU 淘汰、容量限制、并发、完整 watcher 批次、取消、并发排队、全量请求合并、shutdown、目录交换等待和失败回滚。
- 双语缓存构建与干净全量构建逐文件比较：初次、无变化、正文、metadata/slug、改名、删除、Markup 配置变更全部一致。
- 真实站点 build SHA-256 对比：7,236 个稳定输出文件完全一致；9 个既有不稳定文件由基线自身多次运行确认，包括两组重复 slug 对应中英 HTML/Markdown，以及 `en/sitemap.xml`。优化版没有新增差异路径。不能宣称全部 7,245 文件稳定逐字节等价。
- dev 每次修改都检查 HTTP 正文包含新标记；最终会话退出后临时 live 输出已删除。独立源码审查未发现阻塞问题。
- 新增标准 Go benchmark：`BenchmarkMarkdownCache`、`BenchmarkStaleContentLoad`、`BenchmarkMultiSiteRebuild`。微基准反映局部成本，不直接外推整站倍数。

## 复现

完整机器可读样本：[性能数据](../artifacts/2026-09-23-build-dev-performance.json)。

```bash
python3 scripts/bench-build.py --source /path/to/site \
  --baseline /path/to/base/huan --candidate /path/to/new/huan \
  --baseline-plugins /path/to/base/plugins --candidate-plugins /path/to/new/plugins \
  --rounds 5

python3 scripts/bench-dev.py --source /path/to/site \
  --baseline /path/to/base/huan --candidate /path/to/new/huan \
  --baseline-plugins /path/to/base/plugins --candidate-plugins /path/to/new/plugins \
  --rounds 3 --port 18573

go test ./internal/build -run '^$' \
  -bench '^Benchmark(MarkdownCache|StaleContentLoad|MultiSiteRebuild)$' -benchmem
```

build 脚本只读站点、输出到独立临时目录；dev 脚本复制站点（忽略 `.git`），仅修改副本。两者隔离 HUAN_HOME，防止本机已安装插件污染对照。dev 脚本已用最小站点验证启动、变更、新 HTML 检查与退出流程。主程序与插件必须从匹配源码及工具链构建。

本次原始 build 日志在 `/var/folders/qt/v_t3_fms0ts0kfdz1bv65qyr0000gn/T/huan-build-bench-909ugv5y/`；程序、插件、dev 日志在 `/var/folders/qt/v_t3_fms0ts0kfdz1bv65qyr0000gn/T/huan-perf-20260923-odkqmjgk/`。这些临时文件不作为持久成果，数据摘要已保存在仓库。

## 尚未实施

本批完成 Markdown 阶段复用及调度基础，dev 仍加载全量内容、重建树/上下文、执行所有页面模板、聚合输出与插件。真正页面级依赖失效、搜索条目缓存、跨进程磁盘缓存、模板级持久会话、多语言并行仍属后续阶段。

切换语言模式或主题仍受启动配置/主题实例影响，需要重启，这是原有行为。全量 build 收益较小，后续要继续看页面模板、搜索全文处理和插件文件往返，不能用本轮局部微基准代替这些阶段的实测。
