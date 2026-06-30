# huan 文档导航

> **LLM 入口**：从本文件开始阅读 huan 项目。任何新会话先读 `CLAUDE.md` 与本文件即可掌握全貌。
> **当前版本**：v0.3.0（见 [版本时间线](#版本时间线)）

## 一句话定位

**huan** 是用 Go 编写的 **local-first single-user content engine with built-in admin**（本地优先、单用户、内置 admin 的内容引擎）——基于文件管理内容，通过 `huan serve` 的 `/admin` 路由提供管理后台。Hugo 三维度等价（肉眼 / SEO / AI）已基本达成（99.7% 字节一致）。"Path toward all-CMS replacement"（替代所有 CMS 的路径）是 v1.x+ roadmap——详见 [ADR 0010](adr/0010-v1-0-scope-and-positioning-split.md)。

关联内容项目：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`），当前处于 Hugo→huan 迁移 Phase 1。

---

## 文档树

| 文档 | 说明 | 状态 |
|------|------|------|
| [`../CLAUDE.md`](../CLAUDE.md) | 项目根指南（语言/发布/记忆/可观测性约定） | 永久 |
| [`technical-plan.md`](technical-plan.md) | **项目蓝图总图** — 架构决策、模块设计、实施里程碑、Hugo 兼容现状 | 已落地 |
| [`progress/CURRENT_STATE.md`](progress/CURRENT_STATE.md) | **当前实际进展** — 阶段一进度、stage 2/3 完成、v0.2.x 系列、剩余差异 | 持续更新 |
| [`adr/0001-redefine-equivalence.md`](adr/0001-redefine-equivalence.md) | **ADR 0001：重新界定「100% 还原」为三维度等价** | Accepted |
| [`adr/0002-cloudflare-deploy-plugin.md`](adr/0002-cloudflare-deploy-plugin.md) | **ADR 0002：Cloudflare 发布插件**（Pages/R2/Worker 三 target 全部实施完毕） | Accepted |
| [`adr/0003-unified-plugin-system.md`](adr/0003-unified-plugin-system.md) | **ADR 0003：统一插件系统**（plugin host / capability / 配置 / 注册） | Accepted |
| [`adr/0004-release-command.md`](adr/0004-release-command.md) | **ADR 0004：本地打包发布命令 `huan release`** | Accepted |
| [`adr/0005-remove-encrypt-and-v02-feature-batch.md`](adr/0005-remove-encrypt-and-v02-feature-batch.md) | **ADR 0005：v0.2 系列决策**（remove encrypt + toc/export/sync + CI release） | Accepted |
| [`adr/0006-remove-encryptgroups-dead-config.md`](adr/0006-remove-encryptgroups-dead-config.md) | **ADR 0006：v0.2.3 移除 `encryptGroups` dead config**（反转 ADR 0005 §1.2） | Accepted |
| [`adr/0007-i18n-build-system.md`](adr/0007-i18n-build-system.md) | **ADR 0007：i18n 多语言构建系统**（MultiSite + hreflang + sitemap） | Accepted |
| [`adr/0008-translator-capability-qwen3-plugin.md`](adr/0008-translator-capability-qwen3-plugin.md) | **ADR 0008：Translator capability + Qwen3 插件**（本地 MoE 翻译） | Accepted |
| [`adr/0009-self-contained-downstream-deploys.md`](adr/0009-self-contained-downstream-deploys.md) | **ADR 0009：Self-contained downstream deploys**（"只依赖 huan" + Docker image + .env） | Accepted |
| [`adr/0010-v1-0-scope-and-positioning-split.md`](adr/0010-v1-0-scope-and-positioning-split.md) | **ADR 0010：v1.0 scope + 定位拆段**（local-first single-user 为 v0.x；6 hard gate） | Accepted |
| [`adr/0011-admin-security-boundary.md`](adr/0011-admin-security-boundary.md) | **ADR 0011：Admin 安全边界**（L1 fail-fast + L2 token + L4 audit log） | Accepted |
| [`standards/equivalence.md`](standards/equivalence.md) | **三维度等价标准** — 肉眼 / SEO / AI 三维度无差异 | 永久 |
| [`standards/documentation.md`](standards/documentation.md) | 文档规范 — 目录用途、命名、归档时机 | 永久 |
| [`templates/progress-template.md`](templates/progress-template.md) | 进行中工作模板 | 引用 |
| [`templates/report-template.md`](templates/report-template.md) | 完成报告模板 | 引用 |

### reports/completed/（已完成报告，按时间倒序）

| 日期 | 文档 | 主题 |
|------|------|------|
| 2026-06-26 | [`admin-content-redesign.md`](reports/completed/2026-06-26-admin-content-redesign.md) | /admin/content 页面全面重设计（中英文合并+语言徽章+3列布局） |
| 2026-06-26 | [`positioning-redefine.md`](reports/completed/2026-06-26-positioning-redefine.md) | 定位从 SSG 升级为一体化内容引擎，替代所有 CMS |
| 2026-06-26 | [`admin-settings-phase1.md`](reports/completed/2026-06-26-admin-settings-phase1.md) | Admin Settings 页面 Phase 1（表单+YAML 双轨编辑） |
| 2026-06-14 | [`i18n-plugin-implementation.md`](reports/completed/2026-06-14-i18n-plugin-implementation.md) | i18n 多语言系统 v1 完整实施（7 PR + Worker 部署） |
| 2026-06-14 | [`chunked-translation.md`](reports/completed/2026-06-14-chunked-translation.md) | 翻译插件 section 级切块 + sliding window |
| 2026-06-14 | [`translate-format-purity-fix.md`](reports/completed/2026-06-14-translate-format-purity-fix.md) | 翻译插件质量门修复（format_purity + length_ratio + heading 非对称） |
| 2026-06-13 | [`release-command.md`](reports/completed/2026-06-13-release-command.md) | `huan release` 本地打包发布命令（v0.1.0 首发） |
| 2026-06-12 | [`serve-implementation-report.md`](reports/completed/2026-06-12-serve-implementation-report.md) | huan serve 实现完成报告（HTTP+fsnotify+LiveReload） |
| 2026-06-12 | [`redefine-equivalence-report.md`](reports/completed/2026-06-12-redefine-equivalence-report.md) | 三维度等价标准重定义完成报告 |
| 2026-06-12 | [`redefine-equivalence-plan.md`](reports/completed/2026-06-12-redefine-equivalence-plan.md) | 三维度等价标准实施计划 |
| 2026-06-12 | [`cjk-sort-report.md`](reports/completed/2026-06-12-cjk-sort-report.md) | CJK 拼音排序 Port 完成报告 |
| 2026-06-12 | [`cjk-sort-plan.md`](reports/completed/2026-06-12-cjk-sort-plan.md) | CJK 拼音排序实施计划 |
| 2026-06-12 | [`meta-plainify-report.md`](reports/completed/2026-06-12-meta-plainify-report.md) | meta description plainify 完成报告 |
| 2026-06-12 | [`meta-plainify-plan.md`](reports/completed/2026-06-12-meta-plainify-plan.md) | meta description plainify 实施计划 |
| 2026-06-11 | [`serve-implementation-plan.md`](reports/completed/2026-06-11-serve-implementation-plan.md) | huan serve 实现计划（Phase A–K，17 commits 已落地） |
| 2026-06-11 | [`serve-design-spec.md`](reports/completed/2026-06-11-serve-design-spec.md) | huan serve 设计规范 |

---

## 常用命令速查

```bash
# 构建（在源项目根目录运行，源由 -s 指定）
go build -o huan ./cmd/huan
./huan build -s /Users/rong.zhu/Code/zhurongshuo

# 开发服务器（HTTP + LiveReload，默认 127.0.0.1:1313）
./huan serve -s /Users/rong.zhu/Code/zhurongshuo
./huan serve -s . --port 8080 --bind 0.0.0.0 -D

# 内容运维（v0.2.1 起原生支持，对标 zhurongshuo 的 Node.js/bash 脚本）
./huan new post/my-post            # multi-archetype：顶部 path segment 选 archetypes/<kind>.md
./huan sync gallery -s .           # 为 static/images/gallery/ 新图 scaffold content/gallery/<name>.md
./huan toc -s .                    # 生成 developer/toc/{books,practices,products}-toc.md（byte-identical generate-toc.js）
./huan export -s .                 # 生成 CSV（md5-identical export.sh，i18n zh_CN 排序）

# 发布
./huan release                     # 本地打包：5 平台 tarball/zip + checksums + manifest 到 /release/{version}/
./huan deploy cloudflare pages -s .  # 部署站点产物到 Cloudflare Pages
./huan deploy cloudflare r2 -s .     # 同步图片到 R2（--prune 删孤儿）
./huan deploy cloudflare worker -s . # 部署 Worker（modules API）
./huan plugin list -s .            # 列出已注册插件
./huan plugin info cloudflare -s . # 查看插件配置摘要

# 版本与配置
./huan version                     # 输出 huan {version} ({shortSHA}{-dirty})
./huan config -s .                 # 打印解析后的配置
./huan env -s .                    # 打印环境信息

# 测试
go test ./...
go test -tags=integration ./internal/release/...   # release 集成测试
go test -race ./...                                 # 全量 + race 检测

# Hugo 兼容回归（必须零回归才允许合并）
./scripts/diff-build.sh            # 完整 diff（含 Hugo/huan 重建）
./scripts/diff-summary.sh          # 仅生成结构化报告
./scripts/diff-patterns.sh         # 按差异模式归类
```

---

## 代码骨架（速览）

```
huan/
├── cmd/
│   ├── huan/                # CLI 入口（14 子命令，见下表）
│   └── equiv-check/         # 三维度等价检查独立工具（byte/normalized/seo/ai 四模式）
├── internal/
│   ├── build/               # BuildSite 主流程（含 LiveReload 注入、原子 swap）
│   ├── config/              # huan.yaml 解析 + ${VAR} strict 插值
│   ├── content/             # 内容加载、frontmatter、content tree、cascade inheritance
│   ├── markdown/            # goldmark + chroma 语法高亮（stage 3 port）
│   ├── shortcode/           # 内联短代码（audio/img；redact 已在 v0.2.0 移除）
│   ├── template/            # html/template 加载、函数注册、Scratch、SortDefault
│   ├── taxonomy/            # 标签/分类（含 BuildWithOriginalCase）
│   ├── pagination/          # 分页器
│   ├── output/              # 写入、minify、canonify（跳过 code/pre）、contentapi、llmstxt
│   ├── i18n/                # i18n bundle + collator（zh-cn 拼音序）
│   ├── serve/               # HTTP server + fsnotify watcher + LiveReload
│   ├── plugin/              # 统一插件宿主（Plugin/Registry/Find[T]，ADR 0003）
│   ├── deploy/              # Deployer capability 接口 + Report + JSON Logger
│   ├── deploy/cloudflare/   # Cloudflare 部署实现：Pages（blake3）/ R2（minio-go+MD5）/ Worker（multipart modules API）
│   ├── observability/       # 跨包 JSON Logger（从 deploy 提取，deploy+release 共用）
│   ├── release/             # 跨平台打包（types/semver/naming/checksums/archive/manifest/build）
│   ├── version/             # VCS info（git SHA via shell out）
│   └── equiv/               # 三维度等价算法（SEO/AI extractor、allowlist 支持）
├── scripts/                 # diff-build / diff-summary / diff-patterns / allowed-diffs.txt
├── docs/                    # 本目录
├── memory/                  # 项目双层记忆系统（MEMORY.md + daily/）
├── release/                 # 发布产物（.gitignore，本地生成）
├── .github/workflows/       # CI：release.yml（v* tag push 自动建 GitHub Release）
├── go.mod / go.sum
├── huan.yaml                # huan 项目自身的示例配置
├── README.md / README.zh-CN.md
├── LICENSE                  # MIT
└── CLAUDE.md                # 项目根指南
```

### CLI 子命令一览（cmd/huan/main.go 注册）

| 子命令 | 用途 | 实现文件 |
|---|---|---|
| `build` | 构建静态站点（支持 draft/future/expired 过滤） | main.go |
| `serve` | 开发服务器（HTTP + LiveReload + /admin 路由） | serve.go |
| `deploy` | 部署到 Cloudflare（pages/r2/worker） | deploy.go |
| `plugin` | 插件管理（list/info） | plugin_cmd.go |
| `release` | 本地打包发布（5 平台 tarball+checksums+manifest） | release.go |
| `version` | 输出版本 + git SHA | version.go |
| `env` | 打印环境信息 | main.go（newEnvCmd） |
| `config` | 打印解析后的配置 | config.go |
| `list` | 列出内容 | list.go |
| `new` | 新建内容（multi-archetype，v0.2.1） | newcmd.go |
| `sync` | 同步资源（gallery scaffold，v0.2.1） | sync.go |
| `toc` | 生成 TOC（byte-identical generate-toc.js，v0.2.1） | toc.go |
| `export` | 导出 CSV（md5-identical export.sh，v0.2.1） | export.go |
| `translate` | 翻译内容（qwen3 插件 + audit 审计，v0.3.0） | translate_cmd.go |

---

## 版本时间线

| 版本 | 日期 | commit | 关键变更 |
|---|---|---|---|
| **v0.3.0** | 2026-06-14~26 | `6364086f`~`f877b2a` | i18n 多语言系统（Translator 插件+MultiSite+双语上线）+ Stage 4 Admin Panel（Go API+React SPA CRUD）+ Settings+Dashboard+v0.2.3 encryptGroups cleanup |
| **v0.2.3** | 2026-06-14 | — | 移除 `huan.yaml` 的 `encryptGroups` dead config + 全文档同步（反转 ADR 0005 §1.2），详见 [ADR 0006](adr/0006-remove-encryptgroups-dead-config.md) |
| **v0.2.2** | 2026-06-13 | `393ba19` | CI 自动建 GitHub Release（`.github/workflows/release.yml`），详见 [ADR 0005](adr/0005-remove-encrypt-and-v02-feature-batch.md) |
| **v0.2.1** | 2026-06-13 | `afe89a9` | `huan toc/export/sync` 子命令 + multi-archetype `huan new`（zhurongshuo Phase 1） |
| **v0.2.0** | 2026-06-13 | `5c220e2` | 移除未启用的 `internal/encrypt/` + `shortcode/redact.go`（-593 行） |
| **v0.1.0** | 2026-06-13 | `424d3b7`~`d4c83bb` | `huan release` 命令落地（6 commits），首发版本，详见 [ADR 0004](adr/0004-release-command.md) |

历史 stage 1/2/3 进度（Hugo 三维度等价达成）详见 [`progress/CURRENT_STATE.md`](progress/CURRENT_STATE.md)。

---

## 记忆系统

按 CLAUDE.md 双层架构（项目根 `memory/` 是唯一真相源）：
- 沉积层（长期）：[`../memory/MEMORY.md`](../memory/MEMORY.md)
- 流层（每日）：[`../memory/daily/`](../memory/daily/)
  - `2026-06-12.md`：stage 1 收尾 + stage 2 phase 1（meta plainify）
  - `2026-06-13.md`：stage 2 phase 2~5 + stage 3 + Cloudflare 插件 + v0.1.0 发版
  - `2026-06-13-v02.md`：v0.2.0/v0.2.1/v0.2.2 三个发版续记
  - `2026-06-14.md`：i18n 翻译系统 grill-me + 实施 + v0.3.0 上线
  - `2026-06-15.md`：英文站三态栏目 + translate audit + serve 多语言修复
  - `2026-06-16.md`：盘古之白 CSS
  - `2026-06-26.md`：定位升级 + Stage 4 Admin Panel 实施 + ContentList 重设计 + 多语言管理
