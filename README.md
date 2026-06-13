# huan

[中文](./README.zh-CN.md) | **English**

> A Go-based static site generator, designed as a Hugo replacement for [zhurongshuo.com](https://zhurongshuo.com).

`huan` builds a static website from Markdown + YAML config + Go templates, producing output that is byte-for-byte comparable with Hugo. It ships as a single binary with zero runtime dependencies, uses the same goldmark Markdown engine as Hugo, and adds first-class support for CJK content, deploy/release tooling, and a `hugo serve`-style dev server with LiveReload.

---

## Table of Contents

- [What is huan?](#what-is-huan)
- [Why huan?](#why-huan)
- [Features](#features)
- [Quick Start](#quick-start)
- [Project Status](#project-status)
- [Project Structure](#project-structure)
- [Documentation](#documentation)
- [Roadmap](#roadmap)
- [Contributing](#contributing)

---

## What is huan?

`huan` is a static site generator written in Go. Stage 1's goal is to fully replace Hugo for building [zhurongshuo.com](https://zhurongshuo.com) — every HTML, RSS, sitemap, and search-index byte must match Hugo's output.

Key characteristics:

- **Single binary**, no runtime dependencies, fast cold start
- **goldmark** for Markdown rendering — the same library Hugo uses
- **`huan.yaml`** for configuration (YAML, not TOML)
- **CJK-aware**: word counting, heading IDs, summary truncation all handle Chinese, Japanese, Korean correctly
- **Unified plugin system** ([ADR 0003](docs/adr/0003-unified-plugin-system.md)): `huan deploy cloudflare {pages,r2,worker}` ships built-in; deploy is the first capability, future plugins follow the same pattern
- **`huan release`** for self-hosted cross-platform packaging ([ADR 0004](docs/adr/0004-release-command.md)) + GitHub Actions auto-release on tag push ([ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md))
- **Content ops commands**: `huan new` (multi-archetype), `huan sync gallery`, `huan toc`, `huan export` — replaces zhurongshuo's Node.js/bash orchestration scripts
- **`hugo serve`-equivalent dev experience**: HTTP server + fsnotify file watcher + LiveReload WebSocket, sub-second browser refresh

`huan` is **not** a drop-in Hugo replacement. Templates are migrated once; afterwards huan owns the build pipeline.

---

## Why huan?

Hugo is excellent, but for [zhurongshuo.com](https://zhurongshuo.com)'s needs it carries a lot of surface area that goes unused. `huan` exists to:

1. **Strip Hugo down to the subset zhurongshuo actually uses.** No theme system, no taxonomies-of-taxonomies, no multiple output formats beyond HTML/RSS/sitemap/search — just the parts that ship to production.
2. **Treat CJK content as a first-class citizen.** `hasCJKLanguage`, word counting, summary length, and heading ID generation all account for Chinese text without configuration.
3. **Bake encryption into the core.** zhurongshuo uses page-level access control (`access: protected`), random-ratio content redaction, and per-group encryption — these are built into huan rather than bolted on.
4. **Stay verifiable against Hugo.** A diff pipeline (`scripts/diff-build.sh`) byte-compares huan's output against Hugo's. The 905/2028 (44.5%) byte-identical baseline is tracked as a regression gate.
5. **Keep the dev loop fast.** `huan serve` rebuilds atomically (no 404s during rebuild) and broadcasts a LiveReload signal that refreshes the browser within ~1 second of saving a Markdown file.

---

## Features

### Commands

| Command | Purpose |
|---|---|
| `huan build` | Build the site into `publishDir` |
| `huan serve` | Start dev server with file watching + LiveReload |
| `huan new <kind>/<path>` | Scaffold content from `archetypes/<kind>.md` (multi-archetype) |
| `huan sync gallery` | Scaffold `content/gallery/<name>.md` for new images |
| `huan toc` | Generate TOC markdown (byte-identical to zhurongshuo's `generate-toc.js`) |
| `huan export` | Export CSV (md5-identical to zhurongshuo's `export.sh`, zh_CN sort via i18n collator) |
| `huan deploy cloudflare {pages,r2,worker}` | Deploy to Cloudflare Pages / R2 / Workers |
| `huan plugin {list,info}` | Inspect registered plugins |
| `huan release` | Cross-compile + archive + checksums to `release/<version>/` |
| `huan version` / `config` / `env` / `list` | Introspection |

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
- **Shortcodes**: built-in `audio`, `img`; extensible registry (the legacy `redact` shortcode was removed in v0.2.0 — see [ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md))
- **Templates**: Go `html/template` with ~40 Hugo-compatible functions (`urlize`, `safeHTML`, `markdownify`, `Scratch`, `partial`, `where`, `sort`, `index`, `len`, math/string/path helpers, …)
- **Taxonomy**: tags and categories with list pages and per-term pages
- **Pagination**: `/page/N/` with `/page/1/` redirecting to `/`
- **Outputs**: HTML, RSS (per-section / per-taxonomy / per-term), `sitemap.xml`, `search.json`
- **Minify**: HTML / CSS / JS / JSON / SVG / XML via `tdewolff/minify`
- **canonifyURLs**: root-relative URLs post-processed into absolute URLs
- **i18n**: YAML-based message bundles (e.g. `zh-cn.yaml`)

### Encryption & redaction (removed in v0.2.0)

The page-level encryption and `redact` shortcode were removed in v0.2.0 because zhurongshuo never enabled them in practice. See [ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md) for rationale. `huan.yaml`'s `params.encryptGroups` is now a dead config (kept for backward compat, not consumed).

### Deploy & release

- **Unified plugin system** ([ADR 0003](docs/adr/0003-unified-plugin-system.md)): capability-based extensions, first capability = `Deployer`
- **Cloudflare deploy** ([ADR 0002](docs/adr/0002-cloudflare-deploy-plugin.md)): pure-Go direct API (no wrangler shell-out); Pages uses blake3 hash + 5-endpoint direct upload, R2 uses minio-go (S3-compatible, MD5 etag), Worker uses multipart modules API
- **Local packaging** ([ADR 0004](docs/adr/0004-release-command.md)): `huan release` cross-compiles 5 standard platforms with `CGO_ENABLED=0 -trimpath -ldflags=-s -w`, produces tarball/zip + sha256 checksums + JSON manifest; deterministic builds (same commit + Go version → identical sha256)
- **CI auto-release** ([ADR 0005](docs/adr/0005-remove-encrypt-and-v02-feature-batch.md)): GitHub Actions workflow runs on `v*` tag push, builds artifacts via `go run ./cmd/huan release`, creates a GitHub Release with all tarballs attached

### Dev server internals

- HTTP static file server with custom 404
- Recursive fsnotify watcher with configurable debounce
- LiveReload WebSocket hub with per-client broadcast channels (slow clients don't block)
- Atomic rebuild: writes to a sibling staging directory, then `rename(2)` swaps it in — old content stays served during multi-second rebuilds, no 404s
- Serialized rebuilds (`atomic.Bool` busy + pending flags) — bursts of edits coalesce into one rebuild
- Rebuild errors broadcast as LiveReload `alert` messages; dev server keeps running
- Port-conflict detection with friendly error message
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
# 1. Download huan_0.1.0_<os>_<arch>.tar.gz from /release/0.1.0/ or GitHub Releases
# 2. Verify checksum (optional but recommended):
shasum -a 256 -c huan_0.2.2_checksums.txt   # reports OK for the archive you downloaded
# 3. Extract:
tar xzf huan_0.2.2_darwin_arm64.tar.gz      # produces ./huan, ./LICENSE, ./README*.md
# 4. Move into PATH:
sudo mv huan /usr/local/bin/
huan version                                  # confirm: "huan 0.2.2 (<git sha>)"
```

Windows users: download the `huan_0.2.2_windows_amd64.zip` instead and extract `huan.exe`.

Requires Go 1.26+ for `go install` / `go build` paths; pre-built tarballs have no Go dependency.

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

## Project Status

**Stage 1 (Hugo parity): essentially complete.**

- Milestones 1–9 all landed: CLI / content loading / templates / shortcodes / lists+taxonomy+pagination / auxiliary outputs (RSS, sitemap, search) / minify / verification / dev server
- Hugo output parity: **905 of 2028 shared files byte-identical (44.5%)**, 0 files missing, 8 extra files (intentional)
- 5 known edge-case diffs remain (CJK word segmentation precision, RSS item ordering / description truncation, summary block-level whitespace handling) — see [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) for the live status

**Stage 2 (plugin architecture): planned, not started.**

`internal/{pipeline,plugin,search}/` and `pkg/` are intentionally absent — they belong to stage 2 and will be created from scratch when that work begins (see [`docs/technical-plan.md`](docs/technical-plan.md) §4.11 for the proposed interface).

---

## Project Structure

```
huan/
├── cmd/huan/              # CLI entrypoint (main.go, serve.go)
├── internal/
│   ├── build/             # BuildSite core + atomic swap
│   ├── config/            # huan.yaml parser
│   ├── content/           # content loader + tree + frontmatter
│   ├── markdown/          # goldmark pipeline
│   ├── shortcode/         # redact / audio / img
│   ├── encrypt/           # page-level encryption + redaction
│   ├── template/          # html/template loader + funcmap
│   ├── taxonomy/          # tags / categories
│   ├── pagination/
│   ├── output/            # writer + canonify + minify
│   ├── i18n/              # message bundles
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
- [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md) — live progress and remaining diffs
- [`docs/standards/documentation.md`](docs/standards/documentation.md) — documentation conventions
- [`docs/reports/completed/`](docs/reports/completed/) — implementation reports (serve design + plan + report)
- [`CLAUDE.md`](CLAUDE.md) — contributor guide (coding conventions, observability, memory system; Chinese)

---

## Roadmap

**Stage 1 polish:**

- Close the remaining 5 Hugo-parity edge-case diffs
- Expand test coverage in `internal/{config,content,markdown,output,template,i18n}`

**Stage 2 — plugin architecture (proposed):**

- `AuthPlugin` — JWT authentication for protected content
- `PaymentPlugin` — paid-content verification
- `MemberPlugin` — membership tiers and entitlements
- `DynamicRenderPlugin` — HTTP server for dynamic protected-content rendering
- `SearchPlugin` — server-side full-text search
- `ContentRelationPlugin` — content relationship graph and cross-references
- `CustomTemplatePlugin` — pluggable template engine

Plugin interfaces are sketched in [`docs/technical-plan.md`](docs/technical-plan.md) §4.11.

---

## Contributing

Pull requests welcome against the `master` branch.

**Hard rule:** every change must keep `./scripts/diff-build.sh` at zero new diffs vs Hugo (or explicitly document expected diffs in the PR description and [`docs/progress/CURRENT_STATE.md`](docs/progress/CURRENT_STATE.md)).

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
