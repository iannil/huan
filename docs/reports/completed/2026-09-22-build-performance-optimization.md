# 全量构建性能优化（zhurongshuo 站点）完成报告

> 完成日期：2026-09-22  ·  对标：`huan build` 全量构建 zhurongshuo（2959 个 md，双语）
> 前置诊断：[`2026-09-13-full-build-hotspots.md`](2026-09-13-full-build-hotspots.md)（Canonify/replaceRE 优化已于上轮落地）

## 1. 概述

针对 zhurongshuo 全量构建过慢的问题，实测 `--timings` 定位出五类热点（串行 Markdown 渲染、串行 taxonomy term 渲染、seo_injector 每文件三次全量 HTML 解析、同步清理 216MB 发布目录、Writer 逐文件 MkdirAll），逐项优化后同环境交替实测 **6.2s → 3.5s（约 -44%）**，输出保持字节级等价。

## 2. 新增依赖

无。

## 3. 新增 / 修改的包

| 路径 | 职责 | 关键文件 |
|---|---|---|
| `internal/build` | Markdown 渲染并行化；taxonomy term 页并行渲染；发布目录异步清理；feeds 子阶段计时 | `pipeline.go`、`pipeline_feeds.go`、`pipeline_setup.go`、`build.go` |
| `internal/output` | Writer 目录创建缓存（`dirs sync.Map` + `ensureDir`） | `writer.go` |
| `plugins/seo-injector` | 单次 `html.Parse` 采集 meta/title/body 纯文本（原三次解析）；文件级并行处理 | `inject.go`、`plugin.go` |

## 4. CLI / API 变更

无 CLI 变更。`--timings` 新增 feeds 子阶段观测点（`feeds/taxonomy`、`feeds/paginated home`、`feeds/search index` 等），复用既有 `measureVoid`。

## 5. 关键设计决策

1. **Markdown 有界并行（GOMAXPROCS semaphore）** —— goldmark `Convert` 每次调用独立 context、shortcode registry 注册完成后只读、每页只写自身 `Page` 字段，并行安全；错误语义保持"首个失败即中止"。`go test -race` 验证通过。
2. **taxonomy term 并行渲染** —— zh-cn 有 431 个 tag，串行渲染 HTML+RSS 占 949ms；term 间相互独立，renderer/writer 已被 `renderPages` 证明可并发。优化后 374ms。
3. **seo_injector 单次解析而非正则hack** —— 保持 `golang.org/x/net/html` 解析语义不变（注入内容、标签顺序、`</head>` 插入位置逐字节一致），仅把三次 `html.Parse` 合并为一次遍历；叠加文件级 worker pool，698+610ms → 102+87ms。
4. **异步清理 = rename + 后台删除，而非延迟清理** —— `os.Rename` 瞬时完成，渲染立即写入新目录，删除与 ~3s 渲染重叠；构建返回前 `cleanupWg.Wait()` 保证进程退出不留 trash。`output.CleanPublishDir` 同步语义不变，供独立调用方使用。
5. **不做双语言并行** —— zh-cn 清理 docs/ 根目录与 en 写 docs/en 存在顺序依赖，沿用上一轮报告结论。

## 6. 验收记录

```
$ go test ./...                                    # 全部通过
$ go test -race ./internal/build/ ./internal/output/ ./internal/markdown/ ./internal/template/ ./internal/shortcode/
ok  github.com/iannil/huan/internal/build     2.238s
ok  github.com/iannil/huan/internal/output    2.079s
ok  github.com/iannil/huan/internal/markdown  1.751s
ok  github.com/iannil/huan/internal/template  2.112s
ok  github.com/iannil/huan/internal/shortcode 1.952s
$ (cd plugins/seo-injector && go test ./...)  # ok

# 输出等价性：基线二进制 vs 优化二进制，docs/ 全量 sha256 清单对比
# 7237 个文件完全一致；仅 8 个文件（posts/2023/04/1401、posts/2025/02/2202 双语的
# index.html/index.md）存在差异 —— 已证明为基线二进制自身两次运行即不一致的
# 既有重复 slug URL 冲突（2025/02/2203.md 的 slug 为 "2202"、2023/04/1402.md 的
# slug 为 "1401"），与本次改动无关。

# 性能：同环境交替 3 次
baseline  6.19s / 6.07s / 6.63s
optimized 3.46s / 3.49s / 3.54s
```

## 7. 已知限制

- 两个既有重复 slug 冲突页面（`/posts/2023/04/1401/`、`/posts/2025/02/2202/`）输出依旧不确定——这是内容数据问题（`2203.md`/`1402.md` 填错了 slug），需修正内容源文件才能根除。
- 剩余热点：`pages render + write`（两语言各 ~800ms，模板渲染本身）、`search index`（265/388ms，plainify+truncate 全文处理）。本轮未动。

## 8. 后续优化项（不在当前阶段范围）

- search index 模板侧优化（plainify 全文 → 截断后处理）。
- 模板渲染本身的热点剖析（`html/template` 执行占大头后的进一步手段）。
- 双语言并行构建（需先解决清理顺序依赖）。
- i18n stale check（330ms）并行化。
