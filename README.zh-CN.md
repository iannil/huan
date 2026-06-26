# huan（幻）

**中文** | [English](./README.md)

> 一个用 Go 编写的一体化内容引擎——基于文件管理内容，内置管理后台，替代所有 CMS。

`huan` 将 Markdown + 单个 YAML 配置 + Go 模板编译为静态网站，其输出**可与 Hugo 逐字节对比验证**（在参照站点上 99.7% 字节一致，SEO 与 AI 两个维度 0 差异）。它是一个零运行时依赖的单一二进制文件，使用与 Hugo 同源的 goldmark 引擎，将 CJK 内容视为一等公民，并把部署、发布、LLM 翻译都集成在同一个 CLI 中。

---

## 目录

- [huan 是什么？](#huan-是什么)
- [为什么是 huan？](#为什么是-huan)
- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [翻译与 i18n](#翻译与-i18n)
- [部署与发布](#部署与发布)
- [项目状态](#项目状态)
- [项目结构](#项目结构)
- [文档](#文档)
- [路线图](#路线图)
- [参与贡献](#参与贡献)
- [许可证](#许可证)

---

## huan 是什么？

`huan` 是一个用 Go 编写的一体化内容引擎。它最初的目标是完全替代 Hugo 来构建 [zhurongshuo.com](https://zhurongshuo.com)——每一个 HTML、RSS、sitemap、搜索索引的字节都必须可复现、可与 Hugo 输出对比验证。在该等价目标基本达成后（99.7% 字节一致，SEO/AI 维度 0 差异），huan 已进化为**全功能 CMS 替代品**——基于文件管理内容，内置管理后台，保留 SSG 全部能力。

核心特征：

- **单一二进制**，零运行时依赖，冷启动快
- 使用 **goldmark** 渲染 Markdown——与 Hugo 同一个库
- 使用 **`huan.yaml`** 作为配置（YAML，而非 TOML）
- **CJK 友好**：字数统计、标题 ID、摘要截断都能正确处理中文 / 日文 / 韩文，无需额外配置
- **默认 AI 友好**：内置 `llms.txt`、内容 API（`/api/{section}.json`）、每页 Markdown 镜像——为 LLM 爬虫和 AI 消费者而设计，而不仅是 SEO 爬虫
- **开箱即用的双语能力**：一套 i18n 构建系统将 `.zh-cn`/`.en` 边车文件渲染为完整的本地化站点，并内置一个翻译插件用本地 LLM 补齐缺口
- **统一插件系统**（[ADR 0003](docs/adr/0003-unified-plugin-system.md)）：基于能力（capability）的扩展——`Deployer`（Cloudflare）和 `Translator`（Qwen3）内置并共享同一个注册表
- **自包含的发布与部署**：`huan release` 跨平台打包，`huan deploy` 通过直连 API 发布到 Cloudflare，并支持 tag 推送触发 GitHub Actions 自动发版
- **等同 `hugo serve` 的开发体验**：HTTP 服务器 + fsnotify 文件监听 + LiveReload WebSocket，亚秒级浏览器刷新
- **可对 Hugo 验证**：一条 diff 流水线将 huan 输出与 Hugo 逐字节对比，并在三个维度（肉眼 / SEO / AI）上拦截回归

`huan` **不是** Hugo 的 drop-in 替换。模板迁移一次即可，此后由 huan 接管整个构建流水线。

---

## 为什么是 huan？

Hugo 很优秀，但对于单一站点的需求而言，它携带了大量用不上的表面积。`huan` 存在的理由：

1. **把 SSG 精简到真正会上线的子集。** 没有主题市场，没有 HTML/RSS/sitemap/search 之外的输出格式矩阵——只保留进入生产的部分。
2. **把 CJK 内容当作一等公民。** `hasCJKLanguage`、字数统计、摘要长度、标题 ID 生成默认就考虑中文。
3. **默认 AI 友好。** 自动生成 `llms.txt`、JSON 内容 API、每页 Markdown 镜像，让 AI agent 和 LLM 爬虫获得干净、结构化的内容访问。
4. **让双语站点成为构建期问题，而非手工苦差。** 用一种语言写作，放入 `.en.md` 边车文件（手写或 LLM 生成），huan 即产出带 parity 审计的本地化站点。
5. **保持对 Hugo 可验证。** diff 流水线（`scripts/diff-build.sh`）将 huan 输出与 Hugo 逐字节对比；2026/2032（99.7%）字节一致基线作为回归门禁被持续跟踪。
6. **保持开发循环快速。** `huan serve` 原子重建（重建期间无 404），保存 Markdown 文件后约 1 秒内刷新浏览器。

---

## 功能特性

### 命令

| 命令 | 用途 |
|---|---|
| `huan build` | 构建站点到 `publishDir` |
| `huan serve` | 启动带文件监听 + LiveReload 的开发服务器 |
| `huan new <kind>/<path>` | 从 `archetypes/<kind>.md` 脚手架内容（多 archetype） |
| `huan sync gallery` | 为新图片脚手架 `content/gallery/<name>.md` |
| `huan toc` | 为 books / practices / products 生成 TOC markdown |
| `huan export` | 导出内容为 CSV 归档（通过 i18n collator 复现 zh_CN 排序） |
| `huan translate qwen3` | 通过本地 Qwen3 LLM 将源 markdown 翻译为 `.en.md` 边车文件 |
| `huan translate status` | 报告所有源文件的翻译状态（已缓存 / 过期 / 缺失） |
| `huan translate audit` | 对运行中的 `serve` 审计 zh/en parity 与逐页语言正确性 |
| `huan deploy cloudflare {pages,r2,worker}` | 部署到 Cloudflare Pages / R2 / Workers |
| `huan plugin {list,info}` | 查看已注册的插件 |
| `huan release` | 交叉编译 + 打包 + 校验和到 `release/<version>/` |
| `huan version` / `env` / `config` / `list` | 自省命令 |

`huan serve` 参数：

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--port` | `1313` | 监听端口 |
| `--bind` | `127.0.0.1` | 绑定地址（支持 `0.0.0.0`、`::`） |
| `-D` / `--buildDrafts` | `false` | 包含草稿内容 |
| `--disableLiveReload` | `false` | 关闭浏览器自动刷新 |
| `--disableWatch` | `false` | 不监听文件变化 |
| `--debounce` | `400ms` | 文件变化防抖延迟 |

### 渲染流水线

- **Markdown**：goldmark，`unsafe: true` + 可配置 typographer；标题 ID 与 Hugo 算法对齐（处理 CJK + 中文标点 + HTML 实体）
- **Shortcode**：内置 `audio`、`img`；可扩展注册表
- **模板**：Go `html/template`，约 40 个 Hugo 兼容函数（`urlize`、`safeHTML`、`markdownify`、`Scratch`、`partial`、`where`、`sort`、`index`、`len`，以及数学/字符串/路径辅助函数等）
- **分类**：tags 与 categories，含列表页与每个 term 页
- **分页**：`/page/N/`，`/page/1/` 重定向到 `/`
- **输出**：HTML、RSS（按 section / 按 taxonomy / 按 term）、`sitemap.xml`、`search.json`
- **AI 输出**：`llms.txt`、`/api/{section}.json`、每页 `index.md` 镜像
- **压缩**：通过 `tdewolff/minify` 压缩 HTML / CSS / JS / JSON / SVG / XML
- **canonifyURLs**：将根相对 URL 后处理为绝对 URL
- **i18n**：YAML 消息包 + 完整双语构建系统（[ADR 0007](docs/adr/0007-i18n-build-system.md)）

### 开发服务器内部

- 带自定义 404 的 HTTP 静态文件服务器
- 递归 fsnotify 监听器，可配置防抖
- LiveReload WebSocket hub，每客户端独立广播通道（慢客户端不阻塞）
- 原子重建：先写入同级 staging 目录，再用 `rename(2)` 换入——重建期间继续服务旧内容，无 404
- 串行化重建（`atomic.Bool` busy + pending 标志）——连续编辑合并为一次重建
- 重建错误以 LiveReload `alert` 消息广播；开发服务器继续运行
- 始终从临时目录服务；绝不触碰真实的 `publishDir`

---

## 快速开始

### 安装

**从源码（目前推荐）：**

```bash
git clone https://github.com/iannil/huan.git
cd huan
go build -o huan ./cmd/huan
```

**通过 `go install`：**

```bash
go install github.com/iannil/huan/cmd/huan@latest
```

**从发布 tarball**（无需 Go 工具链）：

```bash
# 1. 从 /release/<version>/ 或 GitHub Releases 下载 huan_<version>_<os>_<arch>.tar.gz
# 2. 校验 checksum（可选但推荐）：
shasum -a 256 -c huan_<version>_checksums.txt
# 3. 解压：
tar xzf huan_<version>_darwin_arm64.tar.gz   # 产出 ./huan、./LICENSE、./README*.md
# 4. 移入 PATH：
sudo mv huan /usr/local/bin/
huan version                                  # 确认："huan <version> (<git sha>)"
```

Windows 用户：下载 `huan_<version>_windows_amd64.zip` 并解压 `huan.exe`。

`go install` / `go build` 路径需要 Go 1.26+；预编译 tarball 无 Go 依赖。

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
│   │           └── hello.en.md    # → /en/posts/2026/06/hello/（可选边车）
│   └── _index.md                  # 首页
├── layouts/                       # Go html/templates
│   ├── _default/
│   │   ├── single.html
│   │   └── list.html
│   └── partials/
└── static/                        # 原样拷贝
```

### 构建与预览

```bash
# 构建到 publishDir（默认 ./public）
./huan build

# 启动开发服务器（默认 http://localhost:1313）
./huan serve

# 常见 serve 变体
./huan serve --port 8080 --bind 0.0.0.0 -D
./huan serve --disableLiveReload    # 无 WS，仅静态文件
./huan serve --disableWatch         # 文件变化时不重建
```

### 对 Hugo 验证（回归门禁）

```bash
./scripts/diff-build.sh             # 完整重建 + 与 Hugo 字节 diff
./scripts/diff-summary.sh           # 仅结构化报告
./scripts/diff-patterns.sh          # 按模式归类 diff
```

---

## 翻译与 i18n

huan 把双语站点变成构建期问题（[ADR 0007](docs/adr/0007-i18n-build-system.md)、[ADR 0008](docs/adr/0008-translator-capability-qwen3-plugin.md)）：

1. **写一次。** 用源语言撰写内容（如 `hello.md`）。
2. **加边车。** 在旁边放一个 `hello.en.md`——手写，或由翻译插件生成。huan 会将其渲染到本地化的 URL 前缀下（如 `/en/...`）。
3. **用 LLM 补缺口。** `huan translate qwen3` 遍历每个源文件，通过 Ollama HTTP API 调用本地 **Qwen3** 模型，写出 `.en.md` 边车。它是增量的（按 source-hash 缓存）、结构感知的（校验 markdown chunk 结构往返一致）、可观测的（`--progress-every` 在长任务中打印吞吐 + ETA）。
4. **审计 parity。** `huan translate audit` 爬取运行中的 `huan serve`，枚举 zh 与 en sitemap，报告缺失/孤立边车以及逐页语言正确性（英文页里残留未翻译中文，或反之）。只读，绝不修改内容。

```bash
# 翻译所有新增或变更的内容（增量）
./huan translate qwen3

# 预览将翻译哪些文件，不调用 LLM
./huan translate qwen3 --dry-run

# 翻译单个文件，强制重跑
./huan translate qwen3 --file posts/2026/06/hello.md --force

# 报告已缓存 / 过期 / 缺失的翻译状态
./huan translate status

# 对运行中的开发服务器审计 zh/en parity
./huan serve &
./huan translate audit --fail      # 存在任何 parity/语言问题则非零退出
```

`Translator` 能力是统一插件系统的一部分，因此可在 `internal/translate/<provider>/` 下新增其他 provider（云端 API、其他本地模型），无需触碰构建流水线。

---

## 部署与发布

- **统一插件系统**（[ADR 0003](docs/adr/0003-unified-plugin-system.md)）：基于能力的扩展共享同一注册表；当前能力为 `Deployer` 与 `Translator`。
- **Cloudflare 部署**（[ADR 0002](docs/adr/0002-cloudflare-deploy-plugin.md)）：纯 Go 直连 API（不 shell out 到 wrangler）。Pages 用 blake3 哈希 + 直传，R2 用 minio-go（S3 兼容，MD5 etag），Worker 用 multipart modules API。部署自包含（[ADR 0009](docs/adr/0009-self-contained-downstream-deploys.md)）。
- **本地打包**（[ADR 0004](docs/adr/0004-release-command.md)）：`huan release` 用 `CGO_ENABLED=0 -trimpath -ldflags=-s -w` 交叉编译标准平台，产出 tarball/zip + sha256 校验和 + JSON manifest。确定性构建：相同 commit + Go 版本 → 相同 sha256。
- **CI 自动发版**（[ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)）：GitHub Actions workflow 在 `v*` tag 推送时运行，通过 `go run ./cmd/huan release` 构建产物，并创建附带所有 tarball 的 GitHub Release。

---

## 项目状态

**当前版本：v0.3.3。**

**阶段一（Hugo 等价）：已完成。** 在参照站点（[zhurongshuo.com](https://zhurongshuo.com)）上，三维度等价门禁通过：

| 维度 | 结果 | 状态 |
|---|---|---|
| **SEO** | 0 差异 | ✅ 通过 |
| **AI** | 0 差异 | ✅ 通过 |
| 字节一致文件 | **2026 / 2032** | 99.7% |
| 仅 Hugo / 仅 huan 独有文件 | 0 / 0 | ✅ |

剩余少量 normalized/byte 差异全部是 chroma 语法高亮器的版本差异（huan 使用 chroma v2.26.1，与 Hugo 内置版本不同）或非可见 artifact（RSS 描述缩进、sitemap URL 排序）。**肉眼、SEO、AI 三个维度全部无差异。** 详见 [`docs/standards/equivalence.md`](docs/standards/equivalence.md) 与 [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md)。

**阶段二（AI 友好输出 + i18n + 翻译）：已交付。** `llms.txt`、内容 API、Markdown 镜像、双语构建系统、Qwen3 翻译插件均在 v0.2.x–v0.3.x 线落地。

---

## 项目结构

```
huan/
├── cmd/
│   ├── huan/              # CLI 入口（main.go + 各命令文件）
│   └── equiv-check/       # 等价检查辅助二进制
├── internal/
│   ├── build/             # BuildSite 核心 + 原子换入
│   ├── config/            # huan.yaml 解析
│   ├── content/           # 内容加载 + tree + frontmatter
│   ├── markdown/          # goldmark 流水线
│   ├── shortcode/         # audio / img
│   ├── template/          # html/template 加载 + funcmap
│   ├── taxonomy/          # tags / categories
│   ├── pagination/
│   ├── output/            # writer + canonify + minify + AI 输出
│   ├── i18n/              # 消息包 + collator + audit + langdetect
│   ├── translate/         # Translator 能力 + qwen3 provider
│   ├── plugin/            # 统一插件注册表
│   ├── deploy/            # Deployer 能力 + cloudflare provider
│   ├── release/           # 交叉编译 + 打包
│   ├── equiv/             # Hugo 等价检查
│   ├── observability/     # 结构化日志 / 追踪
│   ├── version/           # 构建版本信息
│   └── serve/             # HTTP 服务器 + 监听 + LiveReload
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
- [`docs/standards/equivalence.md`](docs/standards/equivalence.md) —— 三维度等价定义与登记簿
- [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) —— 实时进展与剩余差异
- [`docs/adr/`](docs/adr/) —— 架构决策记录（0001–0009）
- [`CLAUDE.md`](CLAUDE.md) —— 贡献者指南（编码约定、可观测性、记忆系统；中文）

---

## 路线图

**阶段一打磨：**

- 收尾剩余的 chroma/版本边缘差异 vs Hugo
- 扩展 `internal/{config,content,markdown,output,template,i18n}` 的测试覆盖

**更远：**

- 在现有 `Translator` 能力下新增翻译 provider（云端 API、其他本地模型）
- 在现有 `Deployer` 能力下新增部署目标
- [`docs/technical-plan.md`](docs/technical-plan.md) 中勾勒的更多插件能力

---

## 参与贡献

欢迎向 `master` 分支提交 PR。

**硬性规则：** 每次变更都必须让 `./scripts/diff-build.sh` 在 SEO/AI 维度上相对 Hugo 零新增差异（或在 PR 描述和 [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) 中明确记录预期差异）。

工作流：

```bash
# 1. 进行你的修改
# 2. 验证
go build -o huan ./cmd/huan
go test ./...
./scripts/diff-build.sh

# 3. 提交（推荐小而聚焦的 commit）
```

编码约定、可观测性要求、记忆系统详见贡献前必读的 [`CLAUDE.md`](CLAUDE.md)。

---

## 许可证

[MIT](./LICENSE) © 2026 iannil
