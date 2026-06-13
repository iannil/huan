# huan 文档导航

> **LLM 入口**：从本文件开始阅读 huan 项目。任何新会话先读 `CLAUDE.md` 与本文件即可掌握全貌。

## 一句话定位

**huan** 是用 Go 编写的静态站点生成器。阶段一目标：替代 Hugo 构建 [zhurongshuo.com](https://zhurongshuo.com)，输出与 Hugo 在「肉眼 / SEO / AI 三维度」无差异（甚至更好），详见 [`standards/equivalence.md`](standards/equivalence.md)。

关联内容项目：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`），当前仍由 Hugo 构建。

---

## 文档树

| 文档 | 说明 | 状态 |
|------|------|------|
| [`../CLAUDE.md`](../CLAUDE.md) | 项目根指南（语言/发布/记忆/可观测性约定） | 永久 |
| [`technical-plan.md`](technical-plan.md) | **项目蓝图总图** — 架构决策、模块设计、实施里程碑、Hugo 兼容现状 | 已落地 |
| [`progress/CURRENT_STATE.md`](progress/CURRENT_STATE.md) | **当前实际进展** — 阶段一进度、剩余差异、stage 2 待办 | 持续更新 |
| [`adr/0001-redefine-equivalence.md`](adr/0001-redefine-equivalence.md) | **ADR 0001：重新界定「100% 还原」为三维度等价** | Accepted |
| [`adr/0002-cloudflare-deploy-plugin.md`](adr/0002-cloudflare-deploy-plugin.md) | **ADR 0002：Cloudflare 发布插件**（首 PR = Pages only） | Accepted |
| [`adr/0003-unified-plugin-system.md`](adr/0003-unified-plugin-system.md) | **ADR 0003：统一插件系统**（plugin host / capability / 配置 / 注册） | Accepted |
| [`standards/equivalence.md`](standards/equivalence.md) | **三维度等价标准** — 肉眼 / SEO / AI 三维度无差异 | 永久 |
| [`superpowers/plans/2026-06-12-redefine-equivalence.md`](superpowers/plans/2026-06-12-redefine-equivalence.md) | 三维度等价标准实施 plan（2026-06-12） | 进行中 |
| [`standards/documentation.md`](standards/documentation.md) | 文档规范 — 目录用途、命名、归档时机 | 永久 |
| [`templates/progress-template.md`](templates/progress-template.md) | 进行中工作模板 | 引用 |
| [`templates/report-template.md`](templates/report-template.md) | 完成报告模板 | 引用 |

### reports/completed/（已完成报告，按时间倒序）

| 日期 | 文档 | 主题 |
|------|------|------|
| 2026-06-12 | [`serve-implementation-report.md`](reports/completed/2026-06-12-serve-implementation-report.md) | huan serve 实现完成报告（HTTP+fsnotify+LiveReload） |
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

# 测试
go test ./...

# Hugo 兼容回归（必须零回归才允许合并）
./scripts/diff-build.sh        # 完整 diff（含 Hugo/huan 重建）
./scripts/diff-summary.sh      # 仅生成结构化报告
./scripts/diff-patterns.sh     # 按差异模式归类
```

---

## 代码骨架（速览）

```
huan/
├── cmd/huan/                # CLI 入口（main.go=build/serve 分发，serve.go=serve 实现）
├── internal/
│   ├── build/               # BuildSite 主流程（含 LiveReload 注入、原子 swap）
│   ├── config/              # huan.yaml 解析
│   ├── content/             # 内容加载、frontmatter、内容树
│   ├── markdown/            # goldmark 渲染
│   ├── shortcode/           # 内联短代码（redact/audio/img）
│   ├── encrypt/             # 整页加密/涂黑
│   ├── template/            # html/template 加载与函数注册
│   ├── taxonomy/            # 标签/分类
│   ├── pagination/          # 分页器
│   ├── output/              # 写入、canonify、minify
│   ├── i18n/                # i18n bundle
│   └── serve/               # HTTP server + watcher + LiveReload
├── scripts/                 # diff-build / diff-summary / diff-patterns
├── docs/                    # 本目录
└── memory/                  # 项目双层记忆系统（MEMORY.md + daily/）
```

`internal/pipeline/`、`internal/plugin/`、`internal/search/`、`pkg/` 当前**不存在**——属于 stage 2 范围，详见 [`progress/CURRENT_STATE.md`](progress/CURRENT_STATE.md)。

---

## 记忆系统

按 CLAUDE.md 双层架构：
- 沉积层（长期）：[`../memory/MEMORY.md`](../memory/MEMORY.md)
- 流层（每日）：[`../memory/daily/`](../memory/daily/)
