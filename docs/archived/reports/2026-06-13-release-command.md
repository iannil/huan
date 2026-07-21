# `huan release` 本地打包发布命令

> 状态：已完成 · 完成日期：2026-06-13 · 起始：2026-06-13 · 负责人：iannil · 对标：v0.1.0 发版（后延续到 v0.2.0/v0.2.1/v0.2.2）

## 背景

huan stage 1 + stage 2/3 + Cloudflare deploy 插件（PR1/2/3 + 审计修复）全部落地后，VERSION = `0.1.0` 但**没有任何打包发布机制**：无 goreleaser config、无 `/release/` 目录、无 git tag、无 LICENSE 文件。开发者要让其他人（或未来的自己）用上 huan，目前只能 `git clone && go build`，没有可分发的二进制产物。

本任务实现 `huan release` 子命令，把 huan 跨平台编译为 5 个标准平台的 tarball/zip + checksums + manifest，输出到 `/release/{version}/`。与 [`huan deploy cloudflare`](../adr/0002-cloudflare-deploy-plugin.md)（远程发布站点产物）形成"本地打包 vs 远程推送"边界。

## 目标

- `go run ./cmd/huan release` 一键产出 5 平台二进制 + tarball/zip + sha256 checksums + JSON manifest 到 `/release/{version}/`
- VERSION = `0.1.0` 时产物：5 个归档（darwin amd64+arm64 / linux amd64+arm64 / windows amd64）+ `huan_0.1.0_checksums.txt` + `huan_0.1.0_manifest.json`
- 同 commit + 同 Go 版本两次 build 产出的二进制 sha256 字节一致（可重现构建）
- `huan version` 输出 shortSHA（如 `huan 0.1.0 (87b2836)` 或 `87b2836-dirty`）
- 全程 JSON 结构化日志（trace_id / span_id / event_type / payload），与 deploy 同形

## 范围

**做**：
- `huan release` 子命令（`--targets` / `--dry-run` / `--out-dir` 三个 flag）
- `internal/release/` 新包（types / semver / naming / manifest / checksums / archive / build / release orchestrator）
- `internal/observability/` 新包（从 `internal/deploy/logging.go` 提取，deploy + release 共用）
- `internal/version/` 加 `VCS()` 函数（基于 `debug.ReadBuildInfo()`）
- 单元测试 + `//go:build integration` 集成测试 + 确定性测试
- ADR 0004 + MEMORY.md + daily note + README installation 章节

**不做**（明确排除）：
- 不做 git tag / push / VERSION bump（手动跑 `git tag v0.1.0 && git push --tags`）
- 不做 GitHub Releases 上传（未来若需要，做成 deploy-style 插件 `huan release publish-github`）
- 不做 brew formula / scoop / snapcraft / docker image（YAGNI）
- 不做 changelog 自动生成（手动维护或未来 feature）
- 不做二进制签名（GPG / cosign）（YAGNI）
- 不做 `--jobs` 并发（5 target 串行 ~25s 可接受，等真嫌慢再加）
- 不做 `--no-checksums` / `--no-manifest` / `--version` override（违反契约）

## 决策定型（grill-me 15 轮收敛）

| # | 决策 | 选定 | 否决 |
|---|---|---|---|
| Q1 | 打包对象 | huan 二进制本身 | 站点产物 / 二合一 |
| Q2 | 机制 | `huan release` Go 子命令 | shell 脚本 / goreleaser / Makefile（项目禁止） |
| Q3 | 边界 | 纯本地产物，零 git 操作 | + git tag / + GitHub Releases |
| Q4 | 平台矩阵 | 5 标准 + `--targets` 覆盖 | 全平台 / 当前平台 / 完全可配置 |
| Q5 | 目录布局 | 扁平 + manifest.json + shasum 兼容 checksums.txt | 分层 / 顶层扁平 / 无 manifest |
| Q6 | tarball 内容 | binary + LICENSE + 2 READMEs + LICENSE 缺失 fail-fast | 纯二进制 / 全文档 / warn 不 fail |
| Q7 | build flags | `CGO_ENABLED=0 -trimpath -ldflags=-s -w` + `debug.ReadBuildInfo()` | ldflags 注入 git SHA / 不 strip |
| Q8 | 版本号 | VERSION 文件权威，semver fail-fast | `--version` override / 不校验 / git tag 交叉校验 |
| Q9 | 可观测性 | 提取 `internal/observability/` 共享包 | 复用 deploy / 复制 130 行 / 无 JSON 日志 |
| Q10 | 测试 | 三层（单元 / integration / 确定性）+ Builder interface | 无 interface / 全集成 / 无集成 |
| Q11 | CLI flags | 极简 3 个（`--targets` / `--dry-run` / `--out-dir`） | + log-level / trace-id / jobs / no-manifest |
| Q12 | dry-run | 完整管道到 /tmp、删 /tmp、不碰 /release | 纯计划 / smoke test / 不删 /tmp |
| Q13 | 文档 + commits | ADR + progress + MEMORY + daily + README + 6 commits | 合并 commit / 跳过 ADR |
| Q14 | bootstrap | `go run ./cmd/huan release` 规范，operator 透明 | 预构建 operator / 改脚本 / 改 goreleaser |
| Q15 | 运行时边缘 | A1 继续聚合 + B1 覆盖不破坏额外文件 + C1 signal.NotifyContext | 立即中止 / fail-fast 重发 / 不处理 signal |

## 实施步骤

按可独立提交的粒度拆分；每步对应一个 commit。

- [x] **Step 1**：`chore(license): add MIT LICENSE` — 添加 LICENSE 文件（release fail-fast 前提）
- [x] **Step 2**：`refactor(observability): extract Logger from deploy` — `internal/deploy/logging.go` 整体迁入 `internal/observability/logging.go`，纯机械移动
- [x] **Step 3**：`refactor(deploy): use observability.Logger` — deploy 包改 import，类型重命名 `deploy.Logger` → `observability.Logger`
- [x] **Step 4**：`feat(version): expose VCS info via debug.ReadBuildInfo` — `internal/version/version.go` 加 `VCS() (sha, dirty, commitTime)`，`huan version` 输出 shortSHA
- [x] **Step 5**：`feat(release): cross-compile + archive + checksums + manifest`（含单元测试）
  - `internal/release/types.go`（Report / Artifact / Failure / Options / Target）
  - `internal/release/semver.go` + 测试（`0.1.0` ✓ / `v0.1.0` ✗ / `` ✗ / `0.1.0-rc1` ✓）
  - `internal/release/naming.go` + 测试（`ArtifactName(target, version)` → `huan_0.1.0_darwin_arm64.tar.gz`）
  - `internal/release/checksums.go` + 测试（sha256 + shasum 兼容格式）
  - `internal/release/archive.go` + 测试（tar.gz / zip stdlib 实现，扁平结构）
  - `internal/release/manifest.go` + 测试 + golden（JSON schema 守门）
  - `internal/release/build.go`（Builder interface + goBuildBuilder 用 os/exec）
  - `internal/release/release.go`（orchestrator：A1 继续聚合 + B1 覆盖 + C1 signal handling）
  - `cmd/huan/release.go`（CLI wiring，3 flag）
- [x] **Step 6**：`test(release): integration + determinism` — `//go:build integration`
  - `release_integration_test.go`：真 `go build` 单 target → 验证 tarball/checksums/manifest
  - `determinism_test.go`：两次 build sha256 一致
- [x] **Step 7**（可选）：`docs(adr+memory+readme): release command`
  - `docs/adr/0004-release-command.md`（决策摘要 + 备选否决理由 + 与 ADR 0002/0003 关系）
  - `MEMORY.md`（关键决策 + 项目上下文）
  - `memory/daily/2026-06-13.md`（grill-me 15 轮收敛过程）
  - `README.md` / `README.zh-CN.md` 加 `## Installation` 章节

## 关键不变量

- **可重现构建**：`-trimpath + CGO_ENABLED=0` + 不用 wall clock ldflags → 同 commit 字节一致二进制（`determinism_test.go` 守护）
- **零外部依赖**：stdlib only（archive/tar / compress/gzip / archive/zip / crypto/sha256 / encoding/json / os/exec / context / signal）
- **fail-fast 三处**：VERSION 非 semver / LICENSE 缺失 / target 非法（不在 5 标准 + `current` 范围内）
- **不碰 git**：no tag / no push / no VERSION bump（Q3 边界，git 操作由用户手动）
- **dogfooding**：`go run ./cmd/huan release` 是规范调用，无 operator 预构建步骤
- **与 deploy 同形**：subcommand + Report + 结构化日志 + `--dry-run`，学习曲线零

## 风险与回滚

- 风险 1：**跨平台 go build 在 CI 偶发 flaky**（module cache lock / 网络抖动） · 缓解：集成测试用 `--targets=current` 只跑当前平台，确定性测试串行不并发
- 风险 2：**Go 版本升级时 `-ldflags` 语法变化** · 缓解：单元测试覆盖 `goBuildBuilder.Build` 的 flag 拼接（断言 exec.Args 切片）；集成测试守护真实编译
- 风险 3：**`internal/observability/` 提取破坏 deploy**（commit 2/3 重命名漏改） · 缓解：commit 2/3 之间 `go test ./...` 必须全过才进下一步
- 风险 4：**`debug.ReadBuildInfo()` 在某些构建环境返回空 VCS**（如非 git checkout / `GOFLAGS=-buildvcs=false`） · 缓解：`VCS()` 返回零值时不影响 `huan version` 主输出（"0.1.0" 仍正常，无 "(sha)" 后缀）
- 风险 5：**Windows zip 文件权限丢失**（zip 格式原生不支持 unix 权限） · 缓解：zip 内 `huan.exe` 不需要执行权限（Windows 不用）；README 说明 unix 用户应下载 tarball

回滚策略：
- 每 commit 独立可回滚（git revert）
- commit 2/3 是纯重构，回滚不影响 deploy 现有行为
- commit 5/6 失败时 `rm -rf internal/release/ && git checkout cmd/huan/main.go` 回到无 release 命令状态
- `/release/` 目录在 .gitignore 里加（避免误提交大文件）

## 验收

- [x] `go build -o huan ./cmd/huan` 通过
- [x] `go test ./...` 全通过（含 observability 重构后 deploy 测试不退化）
- [x] `go test -tags=integration ./internal/release/...` 通过
- [x] `go run ./cmd/huan release --dry-run --targets=current` 输出合法 manifest JSON 到 stdout，不写 /release
- [x] `go run ./cmd/huan release` 在 `/release/0.1.0/` 产出 5 tarball/zip + checksums.txt + manifest.json
- [x] `shasum -a 256 -c /release/0.1.0/huan_0.1.0_checksums.txt` 验证全过
- [x] `tar tzf /release/0.1.0/huan_0.1.0_darwin_arm64.tar.gz` 列出 `huan` / `LICENSE` / `README.md` / `README.zh-CN.md` 四文件
- [x] 解开 `darwin_arm64` tarball 后 `./huan version` 输出 `huan 0.1.0 (87b2836)` 或类似
- [x] LICENSE 缺失时 `go run ./cmd/huan release` fail-fast 报错且不写任何文件
- [x] VERSION 改为 `v0.1.0` 时 release 报"leading v not allowed"
- [x] 两次连续 `go run ./cmd/huan release` 产出的二进制 sha256 字节一致

## 归档记录

- **完成日期**：2026-06-13
- **首发版本**：v0.1.0（tag 已 push，GitHub Release 由 v0.2.2 的 CI 自动建）
- **后续发版**：均基于本机制
  - v0.2.0（remove encrypt）：未改 release 命令
  - v0.2.1（toc/export/sync）：未改 release 命令
  - v0.2.2（CI auto release）：加 `.github/workflows/release.yml` 自动调用本命令，详见 [ADR 0005](../../adr/0005-remove-encrypt-and-v02-feature-batch.md)
- **归档动作**：从 `docs/progress/` 移到 `docs/reports/completed/`（2026-06-13，文档对账梳理）

## 进展日志

（按时间倒序，每次会话追加。格式：YYYY-MM-DD HH:MM — 动作 + 结果。）

- 2026-06-13 夜 — 全部 7 commits 落地（含合并的 commit 2+3）：
  - `424d3b7` chore(license): add MIT LICENSE
  - `9681615` refactor(observability): extract Logger from deploy
  - `d529420` feat(version): expose VCS info via debug.ReadBuildInfo
  - `1baeac0` feat(release): cross-compile + archive + checksums + manifest
  - `d4c83bb` test(release): integration + determinism
  - （本文档 + ADR 0004 + MEMORY + daily + README commit，pending）
- 2026-06-13 夜 — Step 1-6 全部完成。验收：`go test ./...` 全 PASS（~175 测试）；`go test -tags=integration ./internal/release/...` PASS（host platform / 5 标准 / 确定性）；`huan release --dry-run --targets=current` 端到端跑通
- 2026-06-13 — grill-me 15 轮收敛完成，决策落盘到本文档
