# ADR 0005：v0.2 系列决策（remove encrypt + toc/export/sync + CI release）

- **状态**：Accepted
- **日期**：2026-06-13
- **决策者**：用户（owner）+ Claude
- **替代方案**：见下方各分叉点的备选项
- **被引用**：[ADR 0001](0001-redefine-equivalence.md)（三维度等价，encrypt 移除不破坏），[ADR 0002](0002-cloudflare-deploy-plugin.md)（deploy 边界，sync gallery 不上传），[ADR 0004](0004-release-command.md)（release 命令，CI 自动化其产物）

## 背景

[ADR 0004](0004-release-command.md) 的 `huan release` 命令落地并产出 v0.1.0 后，连续三个发版（v0.2.0 / v0.2.1 / v0.2.2）各自承担一类决策：

| 版本 | commit | 决策主题 |
|---|---|---|
| v0.2.0 | `5c220e2` | 移除未启用的 encrypt 功能 |
| v0.2.1 | `afe89a9` | 补齐 SSG 运维子命令（toc/export/sync + multi-archetype new） |
| v0.2.2 | `e3dd9f3` | GitHub Actions 自动建 Release |

本 ADR 合并记录这三个发版的决策上下文，便于未来回溯「为什么 encrypt 没了」「为什么 huan 有 toc/export」「为什么 CI 自动发版」。

## 决策

### 1. v0.2.0 — remove encrypt

#### 1.1 移除 `internal/encrypt/` 整包 + `internal/shortcode/redact.go`

- **选定**：彻底删除代码（-593 行），不保留占位
- **否决**：
  - 保留 dead code 等未来需求 —— 维护负担持续存在，违反 CLAUDE.md「面向 LLM 的可改写性 · 明确边界与单一职责」
  - 标 `// Deprecated` 注释保留 —— Go 没有真正的 deprecated enforcement，注释会腐烂
  - 移到 `internal/legacy/` —— 制造「垃圾抽屉」反模式

#### 1.2 `huan.yaml` 的 `params.encryptGroups` 保留为 dead config

> **⚠️ Superseded by [ADR 0006](0006-remove-encryptgroups-dead-config.md)，2026-06-14**：v0.2.3 反转本决策，彻底删除 `huan.yaml` 的 `encryptGroups` 11 行 + 同步全套文档。本节内容作为 v0.2.0–v0.2.2 期间的历史决策保留，不再反映当前状态。

- **选定**（v0.2.0 时）：不删 yaml 配置块，让 huan 不再消费它（`internal/config` 不再解析 EncryptGroups 字段）
- **否决**：
  - 同时删 yaml 配置 —— 用户（zhurongshuo）的 huan.yaml 还在用，删了会 git diff 噪声；用户可自行决定何时清理
  - huan.yaml 启动时报错 "encryptGroups no longer supported" —— 违反向后兼容；`params.*` 本来就是开放 map，多余字段无害

#### 1.3 加密密文生成器（Node.js `scripts/encrypt-content.js`）不动

- **选定**：encrypt-content.js 留在 zhurongshuo 项目，huan 不接手
- **否决**：port 到 Go —— 没有用户需求，且加密算法变了会破坏历史密文

### 2. v0.2.1 — toc/export/sync + multi-archetype new

#### 2.1 子命令实现位置：`cmd/huan/`（CLI 层），不进 internal/

- **选定**：toc/export/sync 都是薄 CLI 包装，业务逻辑直接写在 cmd/huan/{toc,export,sync}.go
- **否决**：
  - 建 `internal/toc/` / `internal/export/` —— 这些是「一次性产物生成」不是「可复用领域逻辑」，过度分层
  - 做成插件（`huan plugin run toc`）—— YAGNI，toc/export/sync 是 SSG 核心运维命令，不是可选扩展

#### 2.2 byte-identical / md5-identical 作为硬契约

- **选定**：`huan toc` 输出 byte-identical `scripts/generate-toc.js`；`huan export` 输出 md5-identical `scripts/export.sh`
- **否决**：
  - 「差不多就行」—— 用户已经在用原脚本输出做 git diff / 下游分析，任何字节差都破坏 review 流程
  - 重新设计输出格式 —— 强迫用户改下游脚本，违反"独立项目，非 drop-in"原则的反面（应该是"运维命令 drop-in"）

#### 2.3 复用 `i18n.BuildCollator` 实现 zh_CN.UTF-8 sort

- **选定**：`huan export` 直接调 stage 2 phase 2 引入的 `i18n.BuildCollator("zh-cn")`
- **否决**：
  - shell out `LC_ALL=zh_CN.UTF-8 sort` —— 引入环境依赖，Windows 不可用
  - 重新实现拼音排序 —— stage 2 phase 2 已经踩过坑（Port 上游算法必须查 collator），不重蹈覆辙

#### 2.4 `huan new` kind-specific archetype

- **选定**：顶部 path segment 选 `archetypes/<kind>.md`，回退 `archetypes/default.md`，最后回退内置模板
- **否决**：
  - Hugo 默认 single archetype —— zhurongshuo 实际需要按内容类型（post/book/practice/product）分模板
  - `--kind` flag —— 路径推断（`post/foo` → kind=post）更符合直觉，少一个 flag

#### 2.5 `huan sync gallery` 只 scaffold，不上传

- **选定**：sync 只生成 `content/gallery/<name>.md`；图片上传独立走 `huan deploy cloudflare r2`
- **否决**：
  - sync 同时上传 R2 —— 违反 ADR 0002 单一职责（sync=scaffold，deploy=upload）
  - sync 自动调 deploy —— 隐式副作用，用户失去「先 review scaffold 再上传」的控制点

### 3. v0.2.2 — CI auto-create GitHub Release

#### 3.1 触发：`v*` tag push + workflow_dispatch（back-fill）

- **选定**：tag push 自动建 Release；workflow_dispatch + tag input 给历史 tag 补 Release
- **否决**：
  - 仅 workflow_dispatch 手动触发 —— 失去「tag 即发版」的简洁语义
  - 仅 tag push —— 历史 tag（v0.1.0/v0.2.0/v0.2.1）无法 back-fill
  - PR merge 触发 —— 混淆"代码合并"与"版本发布"

#### 3.2 dogfooding：CI 用 `go run ./cmd/huan release`

- **选定**：CI 在 ubuntu-latest 上 `go run ./cmd/huan release`，与本地规范调用一致（ADR 0004 §Q14）
- **否决**：
  - CI 预编译 operator binary —— 引入"operator 怎么来的"问题，违反透明原则
  - 用 goreleaser —— 项目禁外部重型依赖（ADR 0004 §Q2 已否决）
  - 直接 `go build` 5 平台 + 手拼 tarball —— 重复 release 命令逻辑，维护两套

#### 3.3 安全：tag 格式严格校验 + cache:false + ref pin

- **选定**：
  - tag name grep `^v[0-9]+\.[0-9]+\.[0-9]+$` 才用于 `ref:` / `run:`
  - `actions/setup-go` 设 `cache: false`（避免 cache poisoning）
  - `actions/checkout` `ref: ${{ steps.tag.outputs.tag }}`（pin 到 tag）
- **否决**：
  - 信任 GitHub 自动 escape —— GitHub Actions injection 是已知攻击面
  - 默认 checkout HEAD —— tag push 后 HEAD 可能已移动

#### 3.4 幂等：release 已存在时改 upload --clobber

- **选定**：`gh release view` 检查存在性，存在则 `gh release upload --clobber`，不存在则 `gh release create`
- **否决**：
  - 失败时人工干预 —— CI flake 不应阻塞发版
  - 强制 `gh release create` —— re-run 时报错 "release already exists"

## 影响

### 文档

- `docs/INDEX.md` 补 ADR 0005 + v0.2.x 版本时间线
- `docs/technical-plan.md` §3 删 `encrypt/` 行，§4.5 encrypt 子节标"v0.2.0 移除"
- `docs/progress/CURRENT_STATE.md` 加"v0.2.x 系列"段
- `memory/MEMORY.md` 删"加密密文仍由外部 Node.js 生成"那条
- `memory/daily/2026-06-13-v02.md` 记录三个发版的实施细节

### 代码

- `internal/encrypt/` 整包删除（v0.2.0）
- `internal/shortcode/redact.go` 删除（v0.2.0）
- `cmd/huan/{toc,export,sync}.go` 新增（v0.2.1）
- `cmd/huan/newcmd.go` 扩展 multi-archetype（v0.2.1）
- `.github/workflows/release.yml` 新增（v0.2.2）

### 配置

- `huan.yaml` 的 `params.encryptGroups` 保留为 dead config（向后兼容）
- `internal/version/VERSION` 0.1.0 → 0.2.0 → 0.2.1 → 0.2.2

### 迁移路径

- **zhurongshuo 项目**：无强制改动；`scripts/generate-toc.js` / `scripts/export.sh` 可保留可逐步替换为 `huan toc` / `huan export`
- **huan.yaml**：encryptGroups 字段可保留可删，huan 都不报错
- **历史 tag**：v0.1.0 / v0.2.0 / v0.2.1 可通过 workflow_dispatch 手动补建 GitHub Release（assets 从对应 tag 的 `go run ./cmd/huan release` 重建）

## 风险与回滚

- **风险 1**：encrypt 移除后用户突然想恢复 —— git revert `5c220e2` 即可，encrypt 包历史完整保留在 git
- **风险 2**：CI 自动发版 push 了 malformed tag —— 格式校验在前，invalid tag 不会触发 release 步骤
- **风险 3**：`huan toc` byte-identical 在 zhurongshuo 内容变化后失效 —— golden file 测试覆盖，CI 跑测试会捕获
- **风险 4**：GitHub Actions injection 漏洞 —— tag 格式 `^v[0-9]+\.[0-9]+\.[0-9]+$` 严格限定，无特殊字符空间
