# huan 定位重新定义：从 SSG 到一体化内容引擎

> 状态：立即生效 · 日期：2026-06-26 · 决策者：用户（owner）+ Claude（grill-me 收敛） · 关联：本决策取代 CLAUDE.md/INDEX.md/technical-plan.md/README.md/README.zh-CN.md 中所有旧定位描述

## 1. 背景

huan 最初定位为「用 Go 编写的静态站点生成器，用于替代 Hugo 构建 zhurongshuo.com 网站」。经过 stage 1→2→3 的开发（2026-06-12~13），Hugo 三维度等价目标已基本达成（99.7% 字节一致，SEO/AI 维度 0 差异），zhurongshuo 生产已切换至 huan。

在这个节点上，huan 面临一个根本性问题：**huan 的本质是什么？**

- 它不仅能替代 Hugo，还能替代内容管理的全部环节
- 它的 Markdown 文件即内容源模式比传统 CMS 更简洁、可 Git 追踪
- 它的 CLI 已涵盖 build/serve/deploy/release/translate 等全链路能力
- 它缺少的只是一个管理界面，让非技术用户也能使用

因此，huan 的定位需要从「SSG」跃升到「替代所有 CMS」。

## 2. 决策过程（grill-me）

通过 `/grill-me` 多轮面试，逐项收敛以下决策树：

| # | 问题 | 选项 | 选定 | 理由 |
|---|------|------|------|------|
| 1 | 替代范畴 | A=全环节替代 / B=渲染+发布 / C=传统CMS静态场景 / D=其他 | **A. 全环节替代 CMS** | 内容管理+渲染+发布一站式 |
| 2 | 核心架构 | A=SSG+文件系统 / B=Hybrid动态 / C=全栈框架 / D=需要分析 | **A. SSG + 文件系统内容源** | 保留现有架构，Admin UI 直接读写 Markdown 文件 |
| 3 | 目标用户 | A=所有CMS用户 / B=技术用户 / C=自己场景→通用 / D=小团队 | **A. 所有 CMS 用户** | 包括非技术用户 |
| 4 | 后台水准 | A=可视化后台 / B=简洁Markdown后台 / C=暂不考虑 / D=接近WordPress | **D. 接近 WordPress 水准** | 对标 WordPress 体验但更简洁现代 |
| 5 | 时间线 | A=立即生效 / B=长期愿景 / C=下一阶段 | **A. 立即生效的当前定位** | 现在就说清楚 huan 是什么 |
| 6 | Admin 形态 | A=huan serve 内置 / B=独立子命令 / C=本地开发用 | **A. huan serve 内置 /admin 路由** | 零额外部署成本 |
| 7 | 数据模型 | A=基于文件系统 / B=SQLite / C=混合同步 | **A. 基于文件系统（Markdown 即内容源）** | Git 原生追踪，无 DB 运维负担 |
| 8 | 一句话定位 | A=基于文件的CMS / B=SSG+内建后台 / C=一体化内容引擎 / D=文件即内容CMS | **C. 一体化内容引擎** | 概括「CMS + SSG + 管理后台」三位一体 |

## 3. 新定位陈述

**huan 是一个用 Go 编写的一体化内容引擎（All-in-One Content Engine），基于文件管理内容，内置管理后台（huan serve 的 /admin 路由），替代所有 CMS。**

### 与旧定位的对比

| 维度 | 旧定位 | 新定位 |
|------|--------|--------|
| 品类 | 静态站点生成器 (SSG) | 一体化内容引擎 |
| 对标 | Hugo | 所有 CMS (WordPress/Drupal/Ghost 等) |
| 用户 | 开发者/技术用户 | 所有 CMS 用户（含非技术用户） |
| 界面 | CLI 命令行 | CLI + Web 管理后台 |
| 数据 | Markdown 文件 | 仍是 Markdown 文件（不变） |
| 核心竞争力 | Hugo 三维度等价 | 文件即内容 + 内建后台 + SSG + AI 友好 |

### 核心特征

- **Single binary**, zero runtime deps, fast cold start
- **文件即内容**：Markdown 文件是唯一内容源，Git 天然追踪
- **goldmark** 渲染（与 Hugo 同源）
- **内建管理后台**：`huan serve` 的 `/admin` 路由，接近 WordPress 水准
- **CJK 友好**：字数、标题 ID、摘要默认支持中文
- **AI 友好**：`llms.txt`、内容 API、Markdown 镜像
- **开箱双语**：i18n 构建 + 翻译插件
- **统一插件系统**：Deployer/Translator 等
- **支持从其他 CMS 迁移**（待实现）

## 4. 已更新的文档

| 文件 | 修改内容 |
|------|---------|
| `CLAUDE.md` | 项目概述改为「一体化内容引擎，基于文件管理内容，内置管理后台，替代所有CMS」 |
| `docs/INDEX.md` | 一句话定位改为「一体化内容引擎，定位转向替代所有CMS」 |
| `docs/technical-plan.md` | §1 项目定位更新，保留 Hugo 等价历史 |
| `README.md` | tagline → "all-in-one content engine"；What is huan → "full CMS replacement" |
| `README.zh-CN.md` | tagline → "一体化内容引擎"；huan 是什么 → "全功能 CMS 替代品" |
| `memory/MEMORY.md` | 项目上下文与最近更新行同步 |
| `memory/daily/2026-06-26.md` | 记录完整 grill-me 共识 |

## 5. 不变的部分

以下保持不变，新旧定位兼容：

- **代码架构**：不因定位变化而立刻改写
- **CLI 子命令**：13 个现有子命令不变
- **Hugo 等价验证**：`scripts/diff-build.sh` 继续守护三维度门禁
- **插件系统**：ADR 0003 继续有效
- **发布管线**：`huan release` + GitHub Actions 不变
- **zhurongshuo 站点**：继续由 huan build 构建

## 6. 后续方向

### 短期（PR 级）

1. **在 `huan serve` 中实现 `/admin` 路由**——管理后台入口
2. **管理后台 v1**：先围绕已有内容类型（文章、页面、媒体、分类）做 CRUD 界面
3. **撰写 ADR 000？** 记录本次定位变更（本报告即定位变更的正式记录）

### 中期

- 用户认证系统
- 从其他 CMS（WordPress / Ghost / Strapi）的迁移工具
- 多用户协作支持

## 7. 已知限制 / 风险

- **管理后台尚未实现**：定位已变但能力还在建设中，用户期望需管理
- **「替代所有 CMS」是宏大目标**：需分阶段落地，避免承诺过度
- **非技术用户的 onboarding**：需要有友好的安装/启动/上手体验
