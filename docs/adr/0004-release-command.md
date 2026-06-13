# ADR 0004：本地打包发布命令 `huan release`

- **状态**：Accepted
- **日期**：2026-06-13
- **决策者**：用户（owner）+ Claude（grill-me 15 轮收敛）
- **替代方案**：见下方各分叉点的备选项
- **被引用**：[ADR 0002](0002-cloudflare-deploy-plugin.md)（远程发布边界），[ADR 0003](0003-unified-plugin-system.md)（插件系统，release 不进该机制）

## 背景

[ADR 0002](0002-cloudflare-deploy-plugin.md) 的 Cloudflare deploy 插件 + 审计修复完成后，huan 已能远程发布站点产物，但**没有任何机制发布 huan 二进制本身**。VERSION = `0.1.0` 但无 goreleaser config、无 `/release/` 目录、无 git tag、无 LICENSE 文件。

当前让其他人（或未来的自己）用上 huan 的唯一路径是 `git clone && go build`——没有可分发的二进制产物。

需要一个**本地打包发布命令**：跨平台编译 huan、生成 tarball/zip + checksums + manifest、输出到 `/release/{version}/`。

## 决策

15 个分叉点的收敛结果（详见 [`docs/progress/release-command.md`](../progress/release-command.md)）。下面是关键决策定型。

### 1. 打包对象：huan 二进制本身（非站点产物）

- **选定**：`huan release` 跨编译 huan 二进制
- **否决**：站点产物打包（已被 deploy 插件覆盖）；二合一（YAGNI）

### 2. 机制：Go 子命令（非脚本 / goreleaser / Makefile）

- **选定**：`huan release` cobra 子命令，复用 `internal/observability/` 架构
- **否决**：
  - `scripts/release.sh`——发现性差，Windows bash 不友好
  - goreleaser——引入重型外部依赖，违反 CLAUDE.md "无 Makefile / CI" 轻量原则
  - Makefile——CLAUDE.md 项目规则明文禁止

### 3. 边界：纯本地产物，零 git 操作

- **选定**：只产出到 `/release/{version}/`，不 tag、不 push、不 bump VERSION
- **否决**：+ git tag / + GitHub Releases 上传（远程发布应走 deploy-style 插件机制，未来 `huan release publish-github`）

### 4. 平台矩阵：5 标准平台 + `--targets` 覆盖

- **默认**：darwin/amd64 + darwin/arm64 + linux/amd64 + linux/arm64 + windows/amd64（覆盖 Go CLI 工具 ~99% 用户）
- **flag 覆盖**：`--targets=all|current|darwin/arm64,linux/amd64,...`
- **否决**：全平台（4 个边缘平台，99% 用户不需要）；只当前平台（不满足"发布"语义）；完全可配置（违反"零配置可用"）

### 5. 目录布局：扁平 + manifest + shasum 兼容 checksums

```
/release/
  {version}/
    huan_{version}_{os}_{arch}.tar.gz   × 4 (unix)
    huan_{version}_windows_amd64.zip    × 1
    huan_{version}_checksums.txt        # shasum -a 256 -c 兼容
    huan_{version}_manifest.json        # 机器可读 provenance
```

- **manifest**：version / go_version / git_sha / git_dirty / build_time / targets / artifacts（含 sha256+size+binary name）
- **否决**：无 manifest（失去 provenance）；按 os-arch 分层（冗余）；顶层扁平（违反 CLAUDE.md "全量+增量发布" 多版本共存）

### 6. tarball 内容：binary + LICENSE + 2 READMEs，扁平，fail-fast on missing LICENSE

- **必装**：binary / LICENSE / README.md / README.zh-CN.md
- **LICENSE 缺失**：第一阶段 fail-fast 报错退出（forcing function，强迫用户在第一版补 LICENSE）
- **否决**：纯二进制（失去 LICENSE 法律前提 + README 使用文档）；全文档（CHANGELOG/VERSION 冗余）；warn 不 fail（隐藏真实问题）

### 7. 构建参数：`CGO_ENABLED=0 -trimpath -ldflags=-s -w` + debug.ReadBuildInfo

- **铁三角**：
  - `CGO_ENABLED=0`：全静态链接，跨平台编译零障碍（huan 无 cgo 依赖）
  - `-trimpath`：剥本地路径，保证可重现 + 不泄露 home dir
  - `-ldflags=-s -w`：剥符号表 + DWARF，二进制 ~27MB → ~19MB
- **git SHA**：走 `debug.ReadBuildInfo()` 自动嵌入的 VCS 信息（Go 1.18+），不走 ldflags 注入
- **否决**：ldflags 注入 git SHA（Go 1.18+ 后被 ReadBuildInfo 取代，多一层 plumbing）；不嵌 SHA（失去 provenance）；不 strip（debug build 才该有 debug info）

### 8. 版本号：VERSION 文件权威，semver fail-fast，无 override

- **选定**：永远读 `internal/version/VERSION`，bump = 编辑文件 + commit
- **校验**：空 / 非 semver / 带 v 前缀 → 报错退出
- **否决**：`--version` override（与 `//go:embed VERSION` 编译时嵌入打架，要么状态污染要么 ldflags 覆盖 embed）；不校验（反 fail-fast）；git tag 交叉校验（Q3 已划清"release 不碰 git"）

### 9. 可观测性：提取 `internal/observability/` 共享包

- **选定**：从 `internal/deploy/logging.go` 整体迁入 `internal/observability/`，deploy + release 共用 Logger
- **理由**：Logger 当前已 domain-agnostic（纯 JSON 输出，无 deploy 专属字段），住 deploy 包纯属历史偶然。CLAUDE.md 把"全链路可观测性"列为项目级 mandate，observability 是 cross-cutting concern，不是某 domain 的私产
- **Report 仍各自留**：deploy.Report（per-file upload）与 release.Report（per-target artifact）shape 不同，强行合并会塞进两套无关字段
- **否决**：复用 deploy 包（反向依赖，release 依赖 deploy 语义错位）；复制 130 行（drift 风险）；无 JSON 日志（违反 CLAUDE.md）

### 10. 测试：三层 + Builder interface 注入

- **单元**（默认跑）：纯函数 semver/naming/checksums/archive/manifest + orchestrator 失败聚合（mockBuilder 注入）
- **集成**（`//go:build integration`）：真 `go build` 单 target / 5 平台
- **确定性**（`//go:build integration`）：两次 build sha256 字节一致
- **Builder interface**：`Build(ctx, target, outPath) error`，生产 GoBuildBuilder / 测试 MockBuilder，在 I/O 边界抽象
- **否决**：直接 os.Exec（slow + flaky）；全集成（定位困难）；无集成（永远不验证 flag 拼对没拼对）

### 11. CLI flags：极简 3 个

```
huan release [--targets=all|current|os/arch,...] [--dry-run] [--out-dir=...] [--source=...]
```

- **砍掉**：`--log-level` / `--trace-id` / `--keep-workdir` / `--no-checksums` / `--no-manifest` / `--version` / `--jobs`
- **理由**：前置决策已砍掉 8 成候选；剩余 3 个覆盖全部真实场景

### 12. `--dry-run` 语义：完整管道到 /tmp、删 /tmp、不碰 /release

- **选定**：跑完整流程到 `/tmp/huan-release-{traceID}/stage/`，验证 checksums + manifest + 真编译，最后删 /tmp，**完全不碰** `/release/`
- **否决**：纯计划（不验证编译）；smoke test 单平台（覆盖不全）；不删 /tmp（留垃圾）

### 13. 文档 + commits

- ADR 0004（本文档）+ progress doc + MEMORY.md + daily note + README install 章节
- 6+1 commits：LICENSE / observability 提取 / VCS info / release 主功能（含单元测试）/ 集成测试 / docs

### 14. Bootstrap：`go run ./cmd/huan release` 为规范调用

- **operator huan**（跑 `huan release` 的那个）vs **artifact huan**（tarball 里的）完全解耦
- 维护者用 `go run ./cmd/huan release` 现编现跑 operator，无预构建步骤
- **否决**：预构建 operator（多余一步）；改 `scripts/release.sh`（推翻 9 个决策）；改 goreleaser（同上）
- **修正 Q2 论证错误**：原"dogfooding，与 Hugo/Caddy 同模式"是错的——Hugo 用 GitHub Actions 脚本，Caddy 用 goreleaser，无自己发自己的先例。但 subcommand 路线的**其他**理由（observability 复用 / 类型安全 / LLM friendly / 跨平台无 bash）仍成立

### 15. 运行时边缘行为

- **A1 部分失败**：per-target 失败继续聚合到 `Report.Failures`，与 deploy 同形（attempted/succeeded/failed）
- **B1 重发覆盖**：re-running release 覆盖预期 archive/checksums/manifest 文件，**不动**目录里其他文件（operator 可能手动放了 .sig / RELEASE_NOTES.md）
- **C1 signal handling**：`signal.NotifyContext` 捕获 SIGINT/SIGTERM，propagate 到 `go build` 子进程，defer 删 /tmp

## 与 ADR 0002 / 0003 的关系

| 维度 | ADR 0002 (deploy) | ADR 0003 (plugins) | ADR 0004 (release) |
|---|---|---|---|
| **目的** | 远程发布站点产物 | 一等扩展机制 | 本地打包 huan 二进制 |
| **是否 plugin** | 是（首个 Deployer capability） | host + capability 接口 | **否**（不进 plugin 系统） |
| **输入** | `docs/` 站点产物 | yaml `plugins:<name>.*` | huan 源码（cmd/huan + internal/） |
| **输出** | 远端（CF Pages / R2 / Worker） | n/a | `/release/{version}/` 本地文件 |
| **observability** | observability.Logger | n/a | observability.Logger（首个非 deploy 消费者，触发提取） |

**release 不进 ADR 0003 plugin 系统**：plugin 系统是"用户在 yaml 配置 + huan 加载注册"的扩展机制，release 是"维护者本地编译自己"的发布工具，**关注点完全不同**。强行把 release 做成 plugin 会让 plugin 系统承担它不该承担的角色。

## 资源限制 / 不变量

- **CGO_ENABLED=0**：跨平台编译铁律
- **`-trimpath` + 不注入 wall clock**：可重现构建（determinism_test.go 守护）
- **零外部依赖**：stdlib only（archive/tar / compress/gzip / archive/zip / crypto/sha256 / encoding/json / os/exec / context / signal）
- **fail-fast 三处**：VERSION 非 semver / LICENSE 缺失 / targets 非法
- **不碰 git**：no tag / no push / no VERSION bump

## 未来扩展（YAGNI，目前不做）

- `huan release publish-github`——若真要 GitHub Releases 上传，做成独立 plugin（deploy-style）
- `huan upgrade`——读 manifest 自动升级；manifest 已是零成本预埋的钩子
- 二进制签名（cosign / GPG）——若供应链安全需求出现
- brew / scoop / snapcraft formula——若分发渠道扩张
- changelog 自动生成——若发版节奏密集到人工写 changelog 不划算
