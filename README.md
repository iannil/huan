# huan（幻）

[中文](./README.zh-CN.md) | **English**

> A Go-based all-in-one content engine — replaces traditional CMS systems with file-based content management, a built-in admin panel, and a static site generation pipeline.

`huan` turns Markdown + a single YAML config + Go templates into a static website whose output is **byte-for-byte verifiable against Hugo** (99.7% identical on the reference site, 0 differences on the SEO and AI dimensions). It ships as a single binary with zero runtime dependencies, uses the same goldmark engine as Hugo, treats CJK content as a first-class citizen, and bundles deploy, release, and LLM-powered translation into the same CLI.

---

## Table of Contents

- [What is huan?](#what-is-huan)
- [Why huan?](#why-huan)
- [Features](#features)
- [Quick Start](#quick-start)
- [Translation & i18n](#translation--i18n)
- [Deploy & Release](#deploy--release)
- [Project Status](#project-status)
- [Project Structure](#project-structure)
- [Documentation](#documentation)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## What is huan?

`huan` is an all-in-one content engine written in Go. It was born to fully replace Hugo for building [zhurongshuo.com](https://zhurongshuo.com) — with the hard requirement that every HTML, RSS, sitemap, and search-index byte be reproducible and verifiable against Hugo's output. With that parity goal essentially met (99.7% byte-identical, 0 differences on SEO and AI dimensions), huan has evolved into a **full CMS replacement** — a file-based content engine with a built-in admin panel that manages your content right where it lives.

Key characteristics:

- **Single binary**, no runtime dependencies, fast cold start
- **goldmark** for Markdown rendering — the same library Hugo uses
- **`huan.yaml`** for configuration (YAML, not TOML)
- **CJK-aware**: word counting, heading IDs, and summary truncation all handle Chinese / Japanese / Korean correctly without extra configuration
- **AI-friendly by default**: built-in `llms.txt`, content API (`/api/{section}.json`), and per-page Markdown mirrors — designed for LLM crawlers and AI consumers, not just SEO bots
- **Bilingual out of the box**: an i18n build system that renders `.zh-cn`/`.en` sidecars into a fully localized site, plus a built-in translator plugin that fills the gaps with a local LLM
- **Unified plugin system** ([ADR 0003](docs/adr/0003-unified-plugin-system.md)): capability-based extensions — `Deployer` (Cloudflare) and `Translator` (Qwen3) ship built-in and share one registry
- **Self-contained release & deploy**: `huan release` for cross-platform packaging, `huan deploy` for direct-API Cloudflare publishing, plus GitHub Actions auto-release on tag push
- **`hugo serve`-equivalent dev experience**: HTTP server + fsnotify watcher + LiveReload WebSocket, sub-second browser refresh
- **Verifiable against Hugo**: a diff pipeline byte-compares huan's output against Hugo and gates regressions on three dimensions (visual / SEO / AI)

`huan` is **not** a drop-in Hugo replacement. Templates are migrated once; afterwards huan owns the build pipeline.

---

## Why huan?

Hugo is excellent, but for a single site's needs it carries a lot of surface area that goes unused. `huan` exists to:

1. **Strip the SSG down to the subset that actually ships.** No theme marketplace, no output-format matrix beyond HTML/RSS/sitemap/search — just the parts that reach production.
2. **Treat CJK content as a first-class citizen.** `hasCJKLanguage`, word counting, summary length, and heading-ID generation all account for Chinese text by default.
3. **Be AI-friendly by default.** `llms.txt`, a JSON content API, and per-page Markdown mirrors are generated automatically, so AI agents and LLM crawlers get clean, structured access to your content.
4. **Make bilingual sites a build-time concern, not a manual chore.** Author in one language, drop in `.en.md` sidecars (hand-written or LLM-generated), and huan produces a localized site with parity auditing baked in.
5. **Stay verifiable against Hugo.** A diff pipeline (`scripts/diff-build.sh`) byte-compares huan's output against Hugo's; the 2026/2032 (99.7%) byte-identical baseline is tracked as a regression gate.
6. **Keep the dev loop fast.** `huan serve` rebuilds atomically (no 404s mid-rebuild) and refreshes the browser within ~1 second of saving a Markdown file.

---

## Features

### Commands

| Command | Purpose |
|---|---|
| `huan build` | Build the site into `publishDir` |
| `huan serve` | Start dev server with file watching + LiveReload |
| `huan new <kind>/<path>` | Scaffold content from `archetypes/<kind>.md` (multi-archetype) |
| `huan sync gallery` | Scaffold `content/gallery/<name>.md` for new images |
| `huan toc` | Generate TOC markdown for books / practices / products |
| `huan export` | Export content to a CSV archive (zh_CN sort via i18n collator) |
| `huan translate qwen3` | Translate source markdown into `.en.md` sidecars via a local Qwen3 LLM |
| `huan translate status` | Report translation state (cached / stale / missing) for all sources |
| `huan translate audit` | Audit zh/en parity & per-page language correctness against a running `serve` |
| `huan deploy cloudflare {pages,r2,worker}` | Deploy to Cloudflare Pages / R2 / Workers |
| `huan plugin {list,info}` | Inspect registered plugins |
| `huan release` | Cross-compile + archive + checksums to `release/<version>/` |
| `huan version` / `env` / `config` / `list` | Introspection |

`huan serve` flags:

| Flag | Default | Description |
|---|---|---|
| `--port` | `1313` | Listen port |
| `--bind` | `127.0.0.1` | Bind address (supports `0.0.0.0`, `::`) |
| `-D` / `--buildDrafts` | `false` | Include draft content |
| `--disableLiveReload` | `false` | Disable browser auto-refresh |
| `--disableWatch` | `false` | Do not watch files for changes |
| `--debounce` | `400ms` | File-change debounce delay |

### Rendering pipeline

- **Markdown**: goldmark with `unsafe: true` and configurable typographer; heading IDs aligned with Hugo's algorithm (CJK + Chinese punctuation + HTML entities handled)
- **Shortcodes**: built-in `audio`, `img`; extensible registry
- **Templates**: Go `html/template` with ~40 Hugo-compatible functions (`urlize`, `safeHTML`, `markdownify`, `Scratch`, `partial`, `where`, `sort`, `index`, `len`, math/string/path helpers, …)
- **Taxonomy**: tags and categories with list pages and per-term pages
- **Pagination**: `/page/N/` with `/page/1/` redirecting to `/`
- **Outputs**: HTML, RSS (per-section / per-taxonomy / per-term), `sitemap.xml`, `search.json`
- **AI outputs**: `llms.txt`, `/api/{section}.json`, per-page `index.md` mirrors
- **Minify**: HTML / CSS / JS / JSON / SVG / XML via `tdewolff/minify`
- **canonifyURLs**: root-relative URLs post-processed into absolute URLs
- **i18n**: YAML message bundles + a full bilingual build system ([ADR 0007](docs/adr/0007-i18n-build-system.md))

### Dev server internals

- HTTP static file server with custom 404
- Recursive fsnotify watcher with configurable debounce
- LiveReload WebSocket hub with per-client broadcast channels (slow clients don't block)
- Atomic rebuild: writes to a sibling staging directory, then `rename(2)` swaps it in — old content stays served during multi-second rebuilds, no 404s
- Serialized rebuilds (`atomic.Bool` busy + pending flags) — bursts of edits coalesce into one rebuild
- Rebuild errors broadcast as LiveReload `alert` messages; dev server keeps running
- Always serves from a temp dir; never touches the real `publishDir`

---

## Quick Start

### Install

**From source (recommended for now):**

```bash
git clone https://github.com/iannil/huan.git
cd huan
go build -o huan ./cmd/huan
```

**Via `go install`:**

```bash
go install github.com/iannil/huan/cmd/huan@latest
```

**From a release tarball** (no Go toolchain required):

```bash
# 1. Download huan_<version>_<os>_<arch>.tar.gz from /release/<version>/ or GitHub Releases
# 2. Verify checksum (optional but recommended):
shasum -a 256 -c huan_<version>_checksums.txt
# 3. Extract:
tar xzf huan_<version>_darwin_arm64.tar.gz   # produces ./huan, ./LICENSE, ./README*.md
# 4. Move into PATH:
sudo mv huan /usr/local/bin/
huan version                                  # confirm: "huan <version> (<git sha>)"
```

Windows users: download `huan_<version>_windows_amd64.zip` and extract `huan.exe`.

Requires Go 1.26+ for the `go install` / `go build` paths; pre-built tarballs have no Go dependency.

### Minimal `huan.yaml`

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

### Content layout

```
my-site/
├── huan.yaml
├── content/
│   ├── posts/
│   │   └── 2026/
│   │       └── 06/
│   │           └── hello.md       # → /posts/2026/06/hello/
│   │           └── hello.en.md    # → /en/posts/2026/06/hello/ (optional sidecar)
│   └── _index.md                  # home page
├── layouts/                       # Go html/templates
│   ├── _default/
│   │   ├── single.html
│   │   └── list.html
│   └── partials/
└── static/                        # copied verbatim
```

### Build & serve

```bash
# Build to publishDir (default: ./public)
./huan build

# Start dev server (default: http://localhost:1313)
./huan serve

# Common serve variations
./huan serve --port 8080 --bind 0.0.0.0 -D
./huan serve --disableLiveReload    # no WS, just static files
./huan serve --disableWatch         # no rebuild on file change
```

### Verify against Hugo (regression gate)

```bash
./scripts/diff-build.sh             # full rebuild + byte diff vs Hugo
./scripts/diff-summary.sh           # structured report only
./scripts/diff-patterns.sh          # categorize diffs by pattern
```

---

## Translation & i18n

huan turns a bilingual site into a build-time concern ([ADR 0007](docs/adr/0007-i18n-build-system.md), [ADR 0008](docs/adr/0008-translator-capability-qwen3-plugin.md)):

1. **Author once.** Write your content in the source language (e.g. `hello.md`).
2. **Add sidecars.** Place a `hello.en.md` next to it — hand-written, or generated by a translator plugin. huan renders it under a localized URL prefix (e.g. `/en/...`).
3. **Fill the gaps with an LLM.** `huan translate qwen3` walks every source file, calls a local **Qwen3** model over the Ollama HTTP API, and writes `.en.md` sidecars. It is incremental (source-hash cached), structure-aware (verifies markdown chunk structure round-trips), and observable (`--progress-every` prints throughput + ETA on long runs).
4. **Audit parity.** `huan translate audit` crawls a running `huan serve`, enumerates the zh and en sitemaps, and reports missing/orphan sidecars plus per-page language correctness (English pages with untranslated Chinese, or vice-versa). Read-only; never touches content.

```bash
# Translate everything that's new or changed (incremental)
./huan translate qwen3

# Preview what would be translated, without calling the LLM
./huan translate qwen3 --dry-run

# Translate a single file, forcing a re-run
./huan translate qwen3 --file posts/2026/06/hello.md --force

# Report cached / stale / missing translation state
./huan translate status

# Audit zh/en parity against a running dev server
./huan serve &
./huan translate audit --fail      # exit non-zero if any parity/language issue
```

The `Translator` capability is part of the unified plugin system, so additional providers (cloud APIs, other local models) can be added under `internal/translate/<provider>/` without touching the build pipeline.

---

## Deploy & Release

- **Unified plugin system** ([ADR 0003](docs/adr/0003-unified-plugin-system.md)): capability-based extensions sharing one registry; current capabilities are `Deployer` and `Translator`.
- **Cloudflare deploy** ([ADR 0002](docs/adr/0002-cloudflare-deploy-plugin.md)): pure-Go direct API (no wrangler shell-out). Pages uses blake3 hash + direct upload, R2 uses minio-go (S3-compatible, MD5 etag), Worker uses the multipart modules API. Deploys are self-contained ([ADR 0009](docs/adr/0009-self-contained-downstream-deploys.md)).
- **Local packaging** ([ADR 0004](docs/adr/0004-release-command.md)): `huan release` cross-compiles the standard platforms with `CGO_ENABLED=0 -trimpath -ldflags=-s -w`, producing tarball/zip + sha256 checksums + a JSON manifest. Deterministic: same commit + Go version → identical sha256.
- **CI auto-release** ([ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)): a GitHub Actions workflow runs on `v*` tag push, builds artifacts via `go run ./cmd/huan release`, and creates a GitHub Release with all tarballs attached.

---

## Project Status

**Current version: v0.3.3.**

**Stage 1 (Hugo parity): complete.** On the reference site ([zhurongshuo.com](https://zhurongshuo.com)), the three-dimension equivalence gate passes:

| Dimension | Result | Status |
|---|---|---|
| **SEO** | 0 differing | ✅ PASS |
| **AI** | 0 differing | ✅ PASS |
| Byte-identical files | **2026 / 2032** | 99.7% |
| Files only in Hugo / only in huan | 0 / 0 | ✅ |

The remaining handful of normalized/byte diffs are all either chroma syntax-highlighter version differences (huan ships chroma v2.26.1 vs Hugo's bundled version) or non-visible artifacts (RSS description indentation, sitemap URL ordering). **Visual, SEO, and AI dimensions are all difference-free.** See [`docs/standards/equivalence.md`](docs/standards/equivalence.md) and [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) for the live status.

**Stage 2 (AI-friendly outputs + i18n + translation): shipped.** `llms.txt`, content API, Markdown mirrors, the bilingual build system, and the Qwen3 translator plugin all landed across the v0.2.x–v0.3.x line.

---

## Project Structure

```
huan/
├── cmd/
│   ├── huan/              # CLI entrypoint (main.go + per-command files)
│   └── equiv-check/       # equivalence-check helper binary
├── internal/
│   ├── build/             # BuildSite core + atomic swap
│   ├── config/            # huan.yaml parser
│   ├── content/           # content loader + tree + frontmatter
│   ├── markdown/          # goldmark pipeline
│   ├── shortcode/         # audio / img
│   ├── template/          # html/template loader + funcmap
│   ├── taxonomy/          # tags / categories
│   ├── pagination/
│   ├── output/            # writer + canonify + minify + AI outputs
│   ├── i18n/              # message bundles + collator + audit + langdetect
│   ├── translate/         # Translator capability + qwen3 provider
│   ├── plugin/            # unified plugin registry
│   ├── deploy/            # Deployer capability + cloudflare provider
│   ├── release/           # cross-compile + packaging
│   ├── equiv/             # Hugo equivalence checks
│   ├── observability/     # structured logging / tracing
│   ├── version/           # build version info
│   └── serve/             # HTTP server + watcher + LiveReload
├── scripts/               # diff-build.sh + diff-summary.sh + diff-patterns.*
├── docs/                  # see docs/INDEX.md
├── memory/                # project memory (MEMORY.md + daily notes)
├── huan.yaml              # example config
├── go.mod / go.sum
└── CLAUDE.md              # contributor guide (Chinese)
```

---

## Documentation

- [`docs/INDEX.md`](docs/INDEX.md) — documentation index (start here)
- [`docs/technical-plan.md`](docs/technical-plan.md) — full architecture blueprint
- [`docs/standards/equivalence.md`](docs/standards/equivalence.md) — the three-dimension equivalence definition & registry
- [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) — live progress and remaining diffs
- [`docs/adr/`](docs/adr/) — architecture decision records (0001–0009)
- [`CLAUDE.md`](CLAUDE.md) — contributor guide (coding conventions, observability, memory system; Chinese)

---

## Roadmap

**Stage 4 — Admin Panel（一体化内容引擎管理后台）**

以 huan 新定位「一体化内容引擎」为轴心，构建内置管理后台，实现基于文件系统的内容管理。

| PR | 范围 | 内容 |
|----|------|------|
| 1 | `web/admin/` + `internal/admin/` | 前后端一起：Vite+React+TS+Tailwind+Shadcn UI 骨架 + Go JSON API（内容CRUD）+ `//go:embed` 挂载到 `huan serve` 的 `/admin` 路由 |

**技术栈**：Go JSON API + 嵌入式 React SPA · React 19 + Shadcn UI · Vite + TypeScript + Tailwind CSS · 无认证（localhost only）

**Beyond:**

- Admin 认证系统（session/token）
- 媒体库管理（相册）
- 用户与权限管理
- Dashboard 站点统计
- Admin 内集成 i18n / deploy 配置
- 从其他 CMS（WordPress / Ghost）的迁移工具

---

## Contributing

Pull requests welcome against the `master` branch.

**Hard rule:** every change must keep `./scripts/diff-build.sh` at zero new diffs on the SEO/AI dimensions vs Hugo (or explicitly document expected diffs in the PR description and [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md)).

Workflow:

```bash
# 1. Make your change
# 2. Verify
go build -o huan ./cmd/huan
go test ./...
./scripts/diff-build.sh

# 3. Commit (small, focused commits preferred)
```

For coding conventions, observability requirements, and the memory system, read [`CLAUDE.md`](CLAUDE.md) before contributing.

---

## License

[MIT](./LICENSE) © 2026 iannil
