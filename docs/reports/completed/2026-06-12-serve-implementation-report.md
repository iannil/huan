# huan serve 实现完成报告

> 完成日期：2026-06-12  ·  对标：`hugo serve`
> 计划与设计原文：[`2026-06-11-serve-implementation-plan.md`](2026-06-11-serve-implementation-plan.md)、[`2026-06-11-serve-design-spec.md`](2026-06-11-serve-design-spec.md)

## 1. 概述

`huan serve` 已完整落地，行为对标 `hugo serve`：本地 HTTP 文件服务器 + fsnotify 文件监听 + LiveReload WebSocket，编辑 markdown/模板/配置后浏览器在 ~1 秒内自动刷新。

实现拆分为 11 个 Phase（A–K），对应 17 个 commit，全部已合入 `master`。

## 2. 新增依赖

| 依赖 | 版本 | 用途 |
|---|---|---|
| `github.com/fsnotify/fsnotify` | v1.9.0 | 跨平台文件系统事件 |
| `github.com/coder/websocket` | v1.8.12 | LiveReload WebSocket |

依赖记录在 `go.mod`，由 cobra/goldmark/minify 等既有依赖并存。

## 3. 新增包：`internal/serve/`

| 文件 | 职责 |
|---|---|
| `server.go` | HTTP 静态文件服务器（含 `/livereload.js`、`/livereload` WS 路由、自定义 404） |
| `watcher.go` | 递归 fsnotify watcher + debounce |
| `livereload.go` | LiveReload hub（per-client broadcast；hello/reload/alert 消息） |
| `embed.go` | `//go:embed` 内嵌 `livereload.js` v4.0.2 |
| `livereload.js` | 第三方 vendored 资源（156KB，原样保留） |
| `*_test.go` ×4 | server / watcher / livereload / e2e 集成测试 |

## 4. 新增 CLI flags（`cmd/huan/serve.go`）

| Flag | 默认 | 说明 |
|---|---|---|
| `--port` | `1313` | 监听端口 |
| `--bind` | `127.0.0.1` | 绑定地址，支持 `0.0.0.0` / `::` |
| `-D` / `--buildDrafts` | `false` | 包含 draft 内容 |
| `--disableLiveReload` | `false` | 关闭浏览器自动刷新（不注入脚本、不注册 WS 路由） |
| `--disableWatch` | `false` | 不监听文件变化 |
| `--debounce` | `400ms` | 文件变化 debounce 延迟 |

## 5. 关键设计决策

1. **BaseURL override**：`serve` 模式下用 `devBaseURL = http://{browserHost}:{port}/` 覆盖 `cfg.BaseURL`，保证 canonify/permalinks/RSS/sitemap 中的站内链接指向 dev server 而非生产域名（`serve.go:34`）。

2. **原子 rebuild swap**：rebuild 写入 sibling 临时目录（`tmpDir + ".next"`），构建成功后用 `build.SwapBuildDir` 原子重命名替换。rebuild 期间继续服务旧内容（无 404），rebuild 失败保留旧内容；`serve.go:79-127`。

3. **重建串行化**：`BuildSite` 不是并发安全（修改包级 template/i18n 状态、写入同一 `OutputDir`），用 `atomic.Bool` 实现 busy + pending 双标记，burst 编辑合并为一次重建（`serve.go:74-127`）。

4. **per-client broadcast**：LiveReload hub 给每个连接的 WS client 独立发送 channel，慢客户端不会阻塞广播。

5. **IPv6 URL 处理**：`bind="::"` 时浏览器 URL 用 `localhost`，避免 `ws://[::]:1313/livereload` 这种浏览器不友好形式。

6. **port 冲突友好提示**：`EADDRINUSE` 时退出码非零并打印明确的"端口被占用，请用 --port 指定其他端口"。

7. **rebuild error 不崩溃**：构建失败时通过 `hub.BroadcastAlert` 推送错误到浏览器，dev server 继续运行。

8. **临时目录隔离**：serve 始终用 `os.MkdirTemp("", "huan-serve-*")`，绝不写入真实的 `publishDir`（即 `docs/`），避免污染生产构建。

## 6. 验收记录

```bash
$ go test ./...
ok  github.com/novel_ttl/huan/internal/build    (含 BuildSite + SwapBuildDir)
ok  github.com/novel_ttl/huan/internal/serve    (server + watcher + livereload + e2e)
ok  github.com/novel_ttl/huan/internal/encrypt
ok  github.com/novel_ttl/huan/internal/pagination
...（其余包同上）
```

`./scripts/diff-build.sh` 维持零回归（serve 改动不影响 build 输出路径）。

### 浏览器手动验收点（来自原 plan K1 步骤 3-4）

- ✅ `./huan serve -s /path/to/zhurongshuo` 启动后 `http://localhost:1313/` 可访问
- ✅ DevTools Network 看到 `/livereload` WS 连接建立
- ✅ 编辑任意 `content/**/*.md` → 浏览器 ~1 秒内刷新
- ✅ 编辑 `huan.yaml` → 触发 rebuild
- ✅ Ctrl+C 干净退出，临时目录被 `defer os.RemoveAll` 清理
- ✅ `--port 8080` / `--bind 0.0.0.0` / `-D` / `--disableLiveReload` / `--disableWatch` / `--debounce 1s` 各 flag 行为符合预期

## 7. 文件大小核验

`wc -l cmd/huan/main.go` = **67 行**（< 80，符合原计划 K1 步骤 5 验收阈值）。

## 8. 后续优化项（不在 stage 1 范围）

- HTTP/2 / HTTPS / TLS 证书支持
- 反向代理模式（X-Forwarded-* 处理）
- 浏览器端错误显示组件（当前 alert 消息依赖 livereload.js 默认渲染）
- 多源项目同时 serve（当前一次只服务一个 `-s` 源）
