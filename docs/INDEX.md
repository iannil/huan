# huan 文档导航

> **LLM 入口**：从本文件开始阅读 huan 项目。任何新会话先读 `CLAUDE.md` 与本文件即可掌握全貌。
> **当前版本**：v0.6.0+（详见 [版本时间线](#版本时间线)）
> **当前定位**：local-first single-user content engine with built-in admin — daemon 持久化服务端内容引擎

## 一句话定位

**huan** 是用 Go 编写的 **local-first single-user content engine with built-in admin**（本地优先、单用户、内置 admin 的内容引擎）。基于文件管理内容，当前核心能力是 **daemon 持久化服务端模式**（`huan daemon`），提供静态服务 + JIT 按需渲染 + 内容查询 REST API + SSE 实时推送 + Admin 管理后台 + 插件热插拔。传统 SSG 模式（`huan build` / `huan serve`）完整保留。

关联内容项目：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`），当前已运行在 huan daemon 生产环境。

---

## 文档树

| 文档 | 说明 | 状态 |
|------|------|------|
| [`../CLAUDE.md`](../CLAUDE.md) | 项目根指南（语言/发布/记忆/可观测性约定） | 永久 |
| [`progress/CURRENT_STATE.md`](progress/CURRENT_STATE.md) | **当前实际进展** — daemon 时代完整状态、功能列表、版本演进 | 持续更新 |
| [`adr/0001-redefine-equivalence.md`](adr/0001-redefine-equivalence.md) | **ADR 0001：重新界定「100% 还原」为三维度等价** | Accepted |
| [`adr/0002-cloudflare-deploy-plugin.md`](adr/0002-cloudflare-deploy-plugin.md) | **ADR 0002：Cloudflare 发布插件** | Accepted |
| [`adr/0003-unified-plugin-system.md`](adr/0003-unified-plugin-system.md) | **ADR 0003：统一插件系统**（plugin host / capability / 配置 / 注册） | Accepted |
| [`adr/0004-release-command.md`](adr/0004-release-command.md) | **ADR 0004：本地打包发布命令 `huan release`** | Accepted |
| [`adr/0005-remove-encrypt-and-v02-feature-batch.md`](adr/0005-remove-encrypt-and-v02-feature-batch.md) | **ADR 0005：v0.2 系列决策** | Accepted |
| [`adr/0006-remove-encryptgroups-dead-config.md`](adr/0006-remove-encryptgroups-dead-config.md) | **ADR 0006：移除 `encryptGroups` dead config** | Accepted |
| [`adr/0007-i18n-build-system.md`](adr/0007-i18n-build-system.md) | **ADR 0007：i18n 多语言构建系统** | Accepted |
| [`adr/0008-translator-capability-qwen3-plugin.md`](adr/0008-translator-capability-qwen3-plugin.md) | **ADR 0008：Translator capability + Qwen3 插件** | Superseded by [ADR 0015](adr/0015-deprecate-qwen3-translate.md) |
| [`adr/0009-self-contained-downstream-deploys.md`](adr/0009-self-contained-downstream-deploys.md) | **ADR 0009：Self-contained downstream deploys** | Accepted |
| [`adr/0010-v1-0-scope-and-positioning-split.md`](adr/0010-v1-0-scope-and-positioning-split.md) | **ADR 0010：v1.0 scope + 定位拆段** | Accepted |
| [`adr/0011-admin-security-boundary.md`](adr/0011-admin-security-boundary.md) | **ADR 0011：Admin 安全边界** | Accepted |
| [`adr/0012-theme-plugin-system.md`](adr/0012-theme-plugin-system.md) | **ADR 0012：主题插件系统** | Accepted |
| [`adr/0013-plugin-contract-convergence.md`](adr/0013-plugin-contract-convergence.md) | **ADR 0013：插件契约收敛** | Accepted |
| [`adr/0014-hook-contract-split.md`](adr/0014-hook-contract-split.md) | **ADR 0014：Hook 契约拆分** | Accepted |
| [`adr/0015-deprecate-qwen3-translate.md`](adr/0015-deprecate-qwen3-translate.md) | **ADR 0015：废弃 qwen3-translate 插件与 Translator 翻译基建** | Accepted |
| [`standards/equivalence.md`](standards/equivalence.md) | **三维度等价标准**（Hugo 等价历史参考） | 永久（历史参考） |
| [`standards/documentation.md`](standards/documentation.md) | 文档规范 | 永久 |
| [`templates/progress-template.md`](templates/progress-template.md) | 进行中工作模板 | 引用 |
| [`templates/report-template.md`](templates/report-template.md) | 完成报告模板 | 引用 |
| [`archived/technical-plan.md`](archived/technical-plan.md) | **已归档**历史技术方案（Hugo 等价时代） | 已归档 |

### reports/completed/（已完成报告，按时间倒序）

| 日期 | 文档 | 主题 |
|------|------|------|
| 2026-07-22 | [`sse-push.md`](reports/completed/2026-07-22-sse-push.md) | SSE 实时推送（daemon 实时性优势） |
| 2026-07-22 | [`jit-content-api.md`](reports/completed/2026-07-22-jit-content-api.md) | JIT 渲染 + 内容查询 REST API |
| 2026-07-21 | [`plugin-architecture-incremental.md`](reports/completed/2026-07-21-plugin-architecture-incremental.md) | 插件化架构 + 增量构建 |
| 2026-06-30 | [`v0.5.0-v1-0-gates.md`](reports/completed/2026-06-30-v0.5.0-v1-0-gates.md) | v0.5.0 发版，6 hard gate 全交付 |
| 2026-06-26 | [`admin-content-redesign.md`](reports/completed/2026-06-26-admin-content-redesign.md) | /admin/content 页面全面重设计 |
| 2026-06-26 | [`positioning-redefine.md`](reports/completed/2026-06-26-positioning-redefine.md) | 定位从 SSG 升级为一体化内容引擎 |
| 2026-06-26 | [`admin-settings-phase1.md`](reports/completed/2026-06-26-admin-settings-phase1.md) | Admin Settings 页面 Phase 1 |
| 2026-06-14 | [`i18n-plugin-implementation.md`](reports/completed/2026-06-14-i18n-plugin-implementation.md) | i18n 多语言系统 v1 完整实施 |
| 2026-06-14 | [`chunked-translation.md`](reports/completed/2026-06-14-chunked-translation.md) | 翻译插件 section 级切块 + sliding window |
| 2026-06-14 | [`translate-format-purity-fix.md`](reports/completed/2026-06-14-translate-format-purity-fix.md) | 翻译插件质量门修复 |
| 2026-06-13 | [`release-command.md`](reports/completed/2026-06-13-release-command.md) | `huan release` 本地打包发布命令 |
| 2026-06-12 | [`serve-implementation-report.md`](reports/completed/2026-06-12-serve-implementation-report.md) | huan serve 实现完成报告 |
| 2026-06-12 | [`redefine-equivalence-report.md`](reports/completed/2026-06-12-redefine-equivalence-report.md) | 三维度等价标准重定义完成报告 |
| 2026-06-12 | [`redefine-equivalence-plan.md`](reports/completed/2026-06-12-redefine-equivalence-plan.md) | 三维度等价标准实施计划 |
| 2026-06-12 | [`cjk-sort-report.md`](reports/completed/2026-06-12-cjk-sort-report.md) | CJK 拼音排序 Port 完成报告 |
| 2026-06-12 | [`cjk-sort-plan.md`](reports/completed/2026-06-12-cjk-sort-plan.md) | CJK 拼音排序实施计划 |
| 2026-06-12 | [`meta-plainify-report.md`](reports/completed/2026-06-12-meta-plainify-report.md) | meta description plainify 完成报告 |
| 2026-06-12 | [`meta-plainify-plan.md`](reports/completed/2026-06-12-meta-plainify-plan.md) | meta description plainify 实施计划 |
| 2026-06-11 | [`serve-implementation-plan.md`](reports/completed/2026-06-11-serve-implementation-plan.md) | huan serve 实现计划 |
| 2026-06-11 | [`serve-design-spec.md`](reports/completed/2026-06-11-serve-design-spec.md) | huan serve 设计规范 |

### superpowers/specs/（设计文档，功能完成前存放，完成后保留为设计参考）

所有 superpowers 设计文档随对应功能完成而生成。已完成功能的 design spec 保留在 `docs/superpowers/specs/` 作为详细设计参考，对应完成报告在 `reports/completed/`。

### archived/（已归档内容）

历史开发记录、旧版技术方案等，移入 `docs/archived/` 保留供参考。

---

## 常用命令速查

```bash
# 构建
go build -o huan ./cmd/huan

# Daemon 持久化服务（v0.6+ 核心）
./huan daemon -s /Users/rong.zhu/Code/zhurongshuo

# 开发服务器
./huan serve -s /Users/rong.zhu/Code/zhurongshuo
./huan dev -s /Users/rong.zhu/Code/zhurongshuo --port 8080 --bind 0.0.0.0 -D

# 插件管理
./huan plugin list -s .
./huan plugin load plugins/cloudflare.so -s .
./huan plugin unload cloudflare -s .

# 部署
./huan deploy cloudflare pages -s .
./huan deploy cloudflare r2 -s .

# 发布
./huan release

# 内容运维
./huan new post/my-post
./huan sync gallery -s .
./huan toc -s .
./huan export -s .

# 翻译
./huan translate qwen3 -s .

# 测试
go test ./...
go test -race ./...
```

---

## 代码骨架（速览）

```
huan/
├── cmd/
│   ├── huan/                # CLI 入口（16 子命令）
│   └── equiv-check/         # 三维度等价检查工具
├── internal/
│   ├── build/               # 构建管线（BuildSite / PipelineCache / IncrementalRender / JIT）
│   ├── config/              # huan.yaml 解析
│   ├── content/             # 内容加载 + frontmatter + 内容树
│   ├── markdown/            # goldmark + chroma 语法高亮
│   ├── shortcode/           # shortcode（audio/img）
│   ├── template/            # 模板加载 + 函数注册
│   ├── taxonomy/            # 标签/分类
│   ├── pagination/          # 分页器
│   ├── output/              # 写入 / minify / canonify / contentapi / llmstxt
│   ├── i18n/                # i18n bundle + collator + audit + langdetect
│   ├── admin/               # Admin API（Go API + 嵌入式 React SPA）
│   ├── daemon/              # Daemon 核心（builder / serving / watcher / lifecycle / metrics）
│   │   ├── eventbus/        # 事件总线（build / content / plugin 事件）
│   │   ├── dag/             # DAG 依赖图 + 拓扑排序
│   │   ├── cache/           # JITCache（LRU + TTL）
│   │   ├── contentindex/    # 内容查询索引
│   │   └── sse/             # SSE 实时推送
│   ├── deploy/              # Deployer 接口 + Report
│   ├── translate/           # Translator 接口 + types
│   ├── plugin/              # 统一插件宿主（Loader / LifecycleManager / GRPCStub / MetadataProvider / EventSubscriber）
│   ├── image/               # ImageProcessor 接口
│   ├── seo/                 # SEO 编译期插件（injector / sitemap / htmlinjector）
│   ├── observability/       # JSON Logger
│   ├── release/             # 跨平台打包
│   ├── version/             # VCS info
│   └── equiv/               # 三维度等价算法
├── plugins/                 # 独立 .so 插件仓库
│   ├── cloudflare/          # Cloudflare deploy 插件
│   ├── qwen3/               # Qwen3 翻译插件
│   └── image-pipeline/      # 图片管线插件
├── web/admin/               # Admin React SPA
├── scripts/                 # diff-build / diff-summary / diff-patterns
├── docs/                    # 文档
├── memory/                  # 双层记忆（MEMORY.md + daily/）
├── release/                 # 发布产物（.gitignore）
├── .github/workflows/       # CI（release.yml）
├── go.mod / go.sum / huan.yaml
├── README.md / README.zh-CN.md / LICENSE
└── CLAUDE.md
```

---

## 版本时间线

| 版本 | 日期 | 关键变更 |
|------|------|---------|
| **v0.6.0+** | 2026-07-20~ | **Daemon 时代**：持久化服务端内容引擎。四大运行时能力全部就绪（静态服务 + JIT 按需渲染 + 内容查询 REST API + SSE 实时推送 + 插件热插拔 + 增量构建 + SEO 编译期插件） |
| **v0.5.0** | 2026-06-30 | **v1.0 hard gate 1-5 全交付**：定位拆段 / no-op funcs 三档 / I/O 包测试 / Admin 安全边界 / BuildSite 6 文件重构 |
| **v0.4.x** | 2026-06-14~27 | i18n 翻译系统 + Stage 4 Admin Panel v0.4.0~v0.4.2 增量演进 |
| **v0.3.0** | 2026-06-14~26 | i18n 多语言系统（Translator 插件 + MultiSite + 双语上线）+ Admin Panel（Go API + React SPA CRUD） |
| **v0.2.3** | 2026-06-14 | 移除 `encryptGroups` dead config |
| **v0.2.2** | 2026-06-13 | CI 自动建 GitHub Release |
| **v0.2.1** | 2026-06-13 | `huan toc/export/sync` + multi-archetype `huan new` |
| **v0.2.0** | 2026-06-13 | 移除 encrypt/redact（-593 行） |
| **v0.1.0** | 2026-06-13 | `huan release` 首发版本 |

---

## 记忆系统

按 CLAUDE.md 双层架构（项目根 `memory/` 是唯一真相源）：
- 沉积层（长期）：[`../memory/MEMORY.md`](../memory/MEMORY.md)
- 流层（每日）：[`../memory/daily/`](../memory/daily/)
  - `2026-07-20.md` ~ `2026-07-22.md`：daemon 时代核心功能开发记录
  - `2026-06-12.md` ~ `2026-06-30.md`：Hugo 等价时代开发记录