# huan

**中文** | [English](./README.md)

> 一个用 Go 编写的静态站点生成器，旨在替代 Hugo 构建 [zhurongshuo.com](https://zhurongshuo.com)。

`huan` 从 Markdown + YAML 配置 + Go 模板构建静态站点，输出可与 Hugo **逐字节对比**。它以单二进制发布、零运行时依赖，使用与 Hugo 同源的 goldmark Markdown 引擎，并对 CJK 内容、deploy/release 工具链、`hugo serve` 风格的 LiveReload 开发服务器提供一等支持。

---

## 目录

- [huan 是什么？](#huan-是什么)
- [为什么选择 huan？](#为什么选择-huan)
- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [项目状态](#项目状态)
- [项目结构](#项目结构)
- [文档](#文档)
- [路线图](#路线图)
- [贡献](#贡献)

---

## huan 是什么？

`huan` 是用 Go 编写的静态站点生成器。阶段一目标是**完整替代 Hugo** 构建 [zhurongshuo.com](https://zhurongshuo.com) —— 每一份 HTML、RSS、sitemap、search-index 字节都必须与 Hugo 输出一致。

关键特点：

- **单二进制**，零运行时依赖，冷启动快
- **goldmark** 渲染 Markdown —— 与 Hugo 同一库
- **`huan.yaml`** 配置（YAML，非 TOML）
- **CJK 友好**：字数统计、heading ID、摘要截断都正确处理中、日、韩文本
- **统一插件系统**（[ADR 0003](docs/adr/0003-unified-plugin-system.md)）：内置 `huan deploy cloudflare {pages,r2,worker}`，deploy 是首个 capability，未来插件同机制扩展
- **`huan release` 本地打包**（[ADR 0004](docs/adr/0004-release-command.md)）+ GitHub Actions 自动发版（[ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)）
- **内容运维子命令**：`huan new`（multi-archetype）、`huan sync gallery`、`huan toc`、`huan export` —— 替代 zhurongshuo 的 Node.js/bash 编排脚本
- **`hugo serve` 同等开发体验**：HTTP 服务器 + fsnotify 文件监听 + LiveReload WebSocket，保存 Markdown 后约 1 秒浏览器刷新

`huan` **不是** Hugo 的 drop-in 替换。模板一次性迁移，之后由 huan 接管构建管线。

---

## 为什么选择 huan？

Hugo 非常优秀，但对 [zhurongshuo.com](https://zhurongshuo.com) 的需求来说，它带了很多用不到的功能。`huan` 的存在意义是：

1. **只保留 zhurongshuo 真正用到的 Hugo 子集。** 没有主题系统、没有 taxonomy-of-taxonomy、没有 HTML/RSS/sitemap/search 之外的多种输出格式 —— 只保留发到生产的那部分。
2. **把 CJK 内容当一等公民。** `hasCJKLanguage`、字数统计、摘要长度、heading ID 生成都默认考虑中文文本，无需额外配置。
3. **加密功能已移除。** zhurongshuo 早期规划的页面级加密（`access: protected`、`encryptGroups`、`redact` shortcode）实际从未启用，已于 v0.2.0 移除以减负（详见 [ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)）。
4. **保持与 Hugo 的可对比性。** 一条 diff 管线（`scripts/diff-build.sh`）逐字节对比 huan 与 Hugo 输出。905/2028（44.5%）字节一致的基线作为回归闸门。
5. **保持开发循环快。** `huan serve` 原子重建（rebuild 期间无 404），保存 Markdown 后约 1 秒推送 LiveReload 信号刷新浏览器。

---

## 功能特性

### 命令

| 命令 | 用途 |
|---|---|
| `huan build` | 构建站点到 `publishDir` |
| `huan serve` | 启动开发服务器（文件监听 + LiveReload） |

`huan serve` flags：

| Flag | 默认 | 说明 |
|---|---|---|
| `--port` | `1313` | 监听端口 |
| `--bind` | `127.0.0.1` | 绑定地址（支持 `0.0.0.0`、`::`） |
| `-D` / `--buildDrafts` | `false` | 包含 draft 内容 |
| `--disableLiveReload` | `false` | 关闭浏览器自动刷新 |
| `--disableWatch` | `false` | 不监听文件变化 |
| `--debounce` | `400ms` | 文件变化 debounce 延迟 |

### 渲染管线

- **Markdown**：goldmark，`unsafe: true` + 可配置 typographer；heading ID 算法对齐 Hugo（处理 CJK + 中文标点 + HTML 实体）
- **Shortcode**：内置 `audio`、`img`；可扩展注册（legacy `redact` shortcode 已于 v0.2.0 移除，详见 [ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)）
- **模板**：Go `html/template`，约 40 个 Hugo 兼容函数（`urlize`、`safeHTML`、`markdownify`、`Scratch`、`partial`、`where`、`sort`、`index`、`len`，以及数学/字符串/路径辅助函数……）
- **Taxonomy**：标签与分类，含列表页与每个 term 的页面
- **分页**：`/page/N/`，`/page/1/` redirect 到 `/`
- **输出**：HTML、RSS（按 section / taxonomy / term）、`sitemap.xml`、`search.json`
- **Minify**：HTML / CSS / JS / JSON / SVG / XML（基于 `tdewolff/minify`）
- **canonifyURLs**：将 root-relative URL 后处理为绝对 URL
- **i18n**：基于 YAML 的 message bundle（如 `zh-cn.yaml`）

### 加密与涂黑（v0.2.0 已移除）

页面级加密与 `redact` shortcode 已于 v0.2.0 移除——zhurongshuo 实际从未启用。详见 [ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)。`huan.yaml` 的 `params.encryptGroups` 保留为 dead config（向后兼容，huan 不消费）。

### 部署与发布

- **统一插件系统**（[ADR 0003](docs/adr/0003-unified-plugin-system.md)）：capability-based 扩展，首个 capability = `Deployer`
- **Cloudflare 部署**（[ADR 0002](docs/adr/0002-cloudflare-deploy-plugin.md)）：纯 Go 直连 API（不 shell out wrangler）；Pages 用 blake3 hash + 5-endpoint 直接上传，R2 用 minio-go（S3 兼容，MD5 etag），Worker 用 multipart modules API
- **本地打包**（[ADR 0004](docs/adr/0004-release-command.md)）：`huan release` 跨编译 5 标准平台，`CGO_ENABLED=0 -trimpath -ldflags=-s -w`，产出 tarball/zip + sha256 checksums + JSON manifest；确定性构建（同 commit + Go 版本 → 二进制 sha256 一致）
- **CI 自动发版**（[ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)）：GitHub Actions 在 `v*` tag push 时自动 `go run ./cmd/huan release` + `gh release create`

### 开发服务器内部

- HTTP 静态文件服务器，含自定义 404
- 递归 fsnotify 监听器，可配置 debounce
- LiveReload WebSocket hub，每客户端独立广播 channel（慢客户端不阻塞广播）
- **原子重建**：写入 sibling 临时目录后用 `rename(2)` 原子替换 —— 多秒级 rebuild 期间继续服务旧内容，无 404
- 重建串行化（`atomic.Bool` busy + pending 标记）—— burst 编辑合并为一次 rebuild
- rebuild 错误以 LiveReload `alert` 消息广播；dev server 继续运行
- 端口冲突检测，友好错误提示
- 始终从临时目录服务，绝不污染真实 `publishDir`

---

## 快速开始

### 安装

**从源码构建（当前推荐）：**

```bash
git clone https://github.com/iannil/huan.git
cd huan
go build -o huan ./cmd/huan
```

**通过 `go install`：**

```bash
go install github.com/iannil/huan/cmd/huan@latest
```

**从 release tarball 安装**（无需 Go 工具链）：

```bash
# 1. 从 /release/0.2.2/ 或 GitHub Releases 下载 huan_0.2.2_<os>_<arch>.tar.gz
# 2. 校验 checksum（推荐）：
shasum -a 256 -c huan_0.2.2_checksums.txt   # 对你下载的那个归档报告 OK
# 3. 解压：
tar xzf huan_0.2.2_darwin_arm64.tar.gz      # 产出 ./huan、./LICENSE、./README*.md
# 4. 移入 PATH：
sudo mv huan /usr/local/bin/
huan version                                  # 确认："huan 0.2.2 (<git sha>)"
```

Windows 用户：下载 `huan_0.2.2_windows_amd64.zip`，解压得到 `huan.exe`。

`go install` / `go build` 路径需要 Go 1.26+；预编译 tarball 不依赖 Go。

### 最小 `huan.yaml`

```yaml
baseURL: "https://example.com/"
title: "My Site"
languageCode: "zh-cn"
publishDir: "public"
paginate: 10
hasCJKLanguage: true
summaryLength: 120

markup:
  goldmark:
    renderer:
      unsafe: true
    extensions:
      typographer: false
```

### 内容布局

```
my-site/
├── huan.yaml
├── content/
│   ├── posts/
│   │   └── 2026/
│   │       └── 06/
│   │           └── hello.md       # → /posts/2026/06/hello/
│   └── _index.md                  # 首页
├── layouts/                       # Go html/templates
│   ├── _default/
│   │   ├── single.html
│   │   └── list.html
│   └── partials/
└── static/                        # 原样复制
```

### 构建与开发服务

```bash
# 构建到 publishDir（默认 ./public）
./huan build

# 启动开发服务器（默认 http://localhost:1313）
./huan serve

# 常用 serve 变体
./huan serve --port 8080 --bind 0.0.0.0 -D
./huan serve --disableLiveReload    # 不开 WS，只做静态文件服务
./huan serve --disableWatch         # 文件变化不触发 rebuild
```

### 与 Hugo 对比验证（回归闸门）

```bash
./scripts/diff-build.sh             # 完整重建 + 与 Hugo 逐字节 diff
./scripts/diff-summary.sh           # 仅生成结构化报告
./scripts/diff-patterns.sh          # 按模式归类差异
```

---

## 项目状态

**阶段一（Hugo 兼容）：基本完成。**

- 里程碑 1–9 全部落地：CLI / 内容加载 / 模板 / Shortcode / 列表+Taxonomy+分页 / 辅助输出（RSS、sitemap、search）/ Minify / 验证 / 开发服务器
- Hugo 输出一致性：**2028 个共有文件中 905 个字节完全相同（44.5%）**，缺失 0 个，多出 8 个（有意为之）
- 剩余 5 类已知边缘差异（CJK 字数统计精度、RSS items 排序 / description 截断、summary block-level 空白处理）—— 实时状态见 [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md)

**阶段二（插件架构）：已规划，未开始。**

`internal/{pipeline,plugin,search}/` 与 `pkg/` 当前刻意不存在 —— 它们属于阶段二，启动时从零创建（接口草案见 [`docs/technical-plan.md`](docs/technical-plan.md) §4.11）。

---

## 项目结构

```
huan/
├── cmd/huan/              # CLI 入口（main.go、serve.go）
├── internal/
│   ├── build/             # BuildSite 核心 + 原子 swap
│   ├── config/            # huan.yaml 解析
│   ├── content/           # 内容加载 + 内容树 + frontmatter
│   ├── markdown/          # goldmark 渲染管线
│   ├── shortcode/         # audio / img（redact 已于 v0.2.0 移除）
│   ├── plugin/            # 统一插件宿主（ADR 0003）
│   ├── deploy/cloudflare/ # Pages / R2 / Worker 三 target（ADR 0002）
│   ├── observability/     # 跨包 JSON Logger
│   ├── release/           # 跨平台打包（ADR 0004）
│   ├── version/           # VCS info
│   └── equiv/             # 三维度等价算法
│   ├── template/          # html/template 加载与函数注册
│   ├── taxonomy/          # 标签 / 分类
│   ├── pagination/        # 分页器
│   ├── output/            # 写入 + canonify + minify
│   ├── i18n/              # message bundle
│   └── serve/             # HTTP 服务器 + watcher + LiveReload
├── scripts/               # diff-build.sh + diff-summary.sh + diff-patterns.*
├── docs/                  # 见 docs/INDEX.md
├── memory/                # 项目记忆（MEMORY.md + 每日笔记）
├── huan.yaml              # 示例配置
├── go.mod / go.sum
└── CLAUDE.md              # 贡献者指南（中文）
```

---

## 文档

- [`docs/INDEX.md`](docs/INDEX.md) —— 文档索引（从这里开始）
- [`docs/technical-plan.md`](docs/technical-plan.md) —— 完整架构蓝图
- [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) —— 实时进展与剩余差异
- [`docs/standards/documentation.md`](docs/standards/documentation.md) —— 文档规范
- [`docs/reports/completed/`](docs/reports/completed/) —— 实现报告（serve 设计 / 计划 / 完成报告）
- [`CLAUDE.md`](CLAUDE.md) —— 贡献者指南（编码约定、可观测性、记忆系统；中文）

---

## 路线图

**阶段一收尾：**

- 闭合剩余 5 类 Hugo 兼容边缘差异
- 扩充 `internal/{config,content,markdown,output,template,i18n}` 的测试覆盖

**阶段二 —— 插件架构（草案）：**

- `AuthPlugin` —— 受保护内容的 JWT 鉴权
- `PaymentPlugin` —— 付费内容校验
- `MemberPlugin` —— 会员等级与权益管理
- `DynamicRenderPlugin` —— 动态渲染受保护内容的 HTTP server
- `SearchPlugin` —— 服务端全文搜索
- `ContentRelationPlugin` —— 内容关系图与交叉引用
- `CustomTemplatePlugin` —— 可插拔模板引擎

插件接口草案见 [`docs/technical-plan.md`](docs/technical-plan.md) §4.11。

---

## 贡献

欢迎向 `master` 分支提交 Pull Request。

**硬性规则**：每次改动必须保证 `./scripts/diff-build.sh` 与 Hugo 对比无新增差异（或在 PR 描述与 [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) 中明确说明预期差异）。

工作流：

```bash
# 1. 修改代码
# 2. 验证
go build -o huan ./cmd/huan
go test ./...
./scripts/diff-build.sh

# 3. 提交（推荐小步、聚焦的 commit）
```

贡献前请先阅读 [`CLAUDE.md`](CLAUDE.md)，了解编码约定、可观测性要求与记忆系统。
