# GitHub Desktop DMG 发布：release.yml 三 job 改造与实弹验证交接

> 状态：代码与本地验证完成，**实弹验证 PENDING** · 起始：2026-11-18 · 计划：[docs/superpowers/plans/2026-11-18-github-desktop-dmg-release.md](../superpowers/plans/2026-11-18-github-desktop-dmg-release.md) · 规格：[docs/superpowers/specs/2026-11-18-github-desktop-dmg-release-design.md](../superpowers/specs/2026-11-18-github-desktop-dmg-release-design.md)

## 背景

huan desktop（MyGo 壳）自 v0.13.0 起已有本地构建与验收记录（见 `docs/reports/2026-11-18-desktop-v0.13.2-release.md` 等），但 DMG 一直靠手工 mygo 构建，未进 CI。原 `.github/workflows/release.yml` 只有单 job（CLI 产物 + GHCR 镜像），桌面产物无法随 v* tag 自动发布。本改造把发布流水线拆为三 job，让 CLI 与 desktop DMG 在同一个 GitHub Release 中并行上传，并加版本一致性门禁防止 tag 与引擎/桌面 bundle 版本漂移。

## 实施摘要（Task 1，commit `e211d05`）

- **变更范围**：仅 `.github/workflows/release.yml`（105 insertions / 18 deletions，整文件重写）；任务评审结论 Spec ✅ / Quality Approved（评审 diff 存档：`.superpowers/sdd/2026-11-18-github-desktop-dmg-release/review-6a4ce73..e211d05.diff`）。
- **触发**：`push: tags: v*` + `workflow_dispatch`（`tag` 输入，用于对已存在 tag 补发/重跑）。

### 三 job 结构

| Job | Runner | 职责 |
| --- | --- | --- |
| `create-release` | ubuntu-latest | 解析 tag（dispatch 输入优先，回退 `GITHUB_REF_NAME`；`^v[0-9]+\.[0-9]+\.[0-9]+$` 格式校验，防注入）→ 输出 `outputs.tag` 作为唯一 tag 源 → **单版本线门禁** → 预创建空 Release（幂等：已存在则跳过，供两 artifact job 并行上传） |
| `release` | ubuntu-latest | 原 CLI 流程原样保留：admin SPA 构建 → `go run ./cmd/huan release` → 七插件 `build-plugins.sh` → `gh release upload --clobber` 注入预创建 Release → GHCR 镜像（`ghcr.io/iannil/huan:v{version}` + `latest`） |
| `desktop` | macos-latest | admin SPA 构建 → `go install mygo@v0.9.2`（版本固定）→ `mygo build -platform darwin/arm64` → 改名 + sha256 → `gh release upload --clobber` |

两 artifact job 均 `needs: create-release` 并以 `needs.create-release.outputs.tag` 取 tag，tag 只解析一次。

### 门禁（单版本线，2026-11-18 裁决）

tag 必须同时等于 `internal/version/VERSION`（去空白后）与 `mygo.json` 的 `version`（均补 `v` 前缀比较）；任一不等即 `::error::Version drift` 并 fail `create-release`，两个 artifact job 不会启动。门禁在 `create-release` 内 checkout 到 tag ref 后执行，校验的是被发布代码里的真实版本。

### DMG 命名

- mygo 原始输出：`build/desktop/darwin-arm64/huan-${TAG#v}-arm64.dmg`（如 `huan-0.13.4-arm64.dmg`，实测一致）。
- 发布附件名：从 `TAG` 派生重命名为 `huan-${TAG}-darwin-arm64.dmg`（如 `huan-v0.13.4-darwin-arm64.dmg`，带 `v`），并生成同名 `.sha256`；名字源自 tag，附件名不可能与 Release 漂移。

### 已知遗留（Task 1 评审 deferred minor，不阻塞）

`desktop` job 的 `echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"` 只对后续 step 生效，同 step 内不生效；当前可用是因为 setup-go v5 默认已把 GOPATH/bin 加进 PATH。稳健性建议（后续可选）：在该 step 内显式 `export PATH="$PATH:$(go env GOPATH)/bin"`。

## 本地验证证据（已通过，Task 1）

1. **YAML 语法**：`python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('yaml-ok')"` → 输出 `yaml-ok`。
2. **门禁脚本本地干跑**（`mktemp -d` 临时目录，jq 以 python3 json 替代，两用例）：

   ```
   v0.13.4 用例（一致）
   consistent-case-ok
   v0.13.5 用例（漂移）
   DRIFT-DETECTED: internal/version/VERSION=0.13.4 vs v0.13.5
   DRIFT-DETECTED: mygo.json=0.13.4 vs v0.13.5
   drift-case-ok
   ```

   阴性用例（tag == VERSION == mygo.json）无 DRIFT 行；阳性用例（tag 漂移至 v0.13.5）两处均报 `DRIFT-DETECTED`。逐字证据见 `.superpowers/sdd/2026-11-18-github-desktop-dmg-release/task-1-report.md`。

## PENDING：实弹验证（待 master 推送后执行）

本地只能证明语法与门禁逻辑；`actions` 上游行为、GitHub Release 并行上传、GHCR 不回归必须在真实 run 中确认。以下为交接给下一次会话的完整待办。

### 前置状态（2026-11-18 21:49 +0800 实测）

- **master 未推送**：本地 master 领先 `origin/master` **70 个提交**（计划时估算 ~41，期间 desktop/admin 文档与功能提交继续累积），`origin/master` tip 为 `d93f7ab`（2026-11-13）。**禁止以 origin 为基准 reset**（SDD ledger 规则：origin/master 过期，只以本地 master 为准）。
- **Task 1 提交在 worktree 分支上**：`worktree-gh-dmg-release` = master + `e211d05`（本文档提交后为 +2），需先 fast-forward 并回 master 再推送，否则线上没有新 workflow。
- **v0.13.4 tag 尚不存在**：本地与远端最高 tag 均为 `v0.12.7`；`internal/version/VERSION` = `0.13.4`、`mygo.json` = `0.13.4`，与门禁期待的三方一致就绪。v0.13.0–v0.13.3 桌面版本均为本地构建，未打 tag。

### 验证步骤

```bash
# 1. 并回 master 并推送（workflow_dispatch 要求 workflow 文件位于默认分支）
git checkout master
git merge --ff-only worktree-gh-dmg-release
git push origin master

# 2. 打 tag 并推送 —— push v* tag 本身即触发 workflow（push 路径）
git tag v0.13.4
git push origin v0.13.4

# 3. 重跑/补发入口（tag 已存在于远端时）：dispatch 已有 tag
gh workflow run release.yml -f tag=v0.13.4
#    （或对任意已推送的 v* tag 同理；dispatch 前提是 tag 已在远端——
#     workflow 首步 checkout at tag 会因 tag 不存在而先失败）

# 4. 观测
gh run watch   # 或 gh run list --workflow=release.yml
gh release view v0.13.4
```

### 验收三点（全部通过才算实弹验证完成）

1. **Release 出现 DMG + sha256 附件**：`gh release view v0.13.4` 可见 `huan-v0.13.4-darwin-arm64.dmg` 与 `huan-v0.13.4-darwin-arm64.dmg.sha256`；desktop job 日志中 `cat "$out.sha256"` 输出校验值，本地 `shasum -a 256 -c` 可复验。
2. **CLI 产物不受影响**：同一 Release 中 CLI 产物照常上传——工作流上传的只有 `release/<version>/` 产物（对照 v0.12.7 记录：5 平台归档 + `huan_0.12.7_checksums.txt` + manifest，共 7 个附件）；七插件 `.so` 只进 `docker-plugins/` 供 GHCR 镜像使用，**不作为 Release 附件**。GHCR `ghcr.io/iannil/huan:v0.13.4` 与 `latest` 照常推送（`docker manifest inspect` 或 `ghcr` 页面确认）。对比改造前 v0.12.7 的 Release 附件清单：恰好 9 个附件 = 原 7 件 + 新增 DMG 两件（`huan-v0.13.4-darwin-arm64.dmg` + `.sha256`），除新增两件外无缺失、无意外附件。
3. **人为漂移时门禁红**：真实漂移用例（tag 与 VERSION/mygo.json=0.13.4 不一致，且 tag 必须匹配格式门禁 `^v[0-9]+\.[0-9]+\.[0-9]+$`——不能用 `v0.13.5-drift` 这类后缀名，否则先挂在格式校验而到不了版本门禁）：
   1. `git tag v0.13.5 && git push origin v0.13.5`（漂移 tag 指向当前提交，VERSION/mygo.json 仍为 0.13.4）；push v* tag 即自动触发 workflow，无需 dispatch（dispatch 路径同理，但要求该 tag 已在远端，否则 checkout at tag 先失败、到不了门禁）。
   2. 观测 run：`create-release` 失败，日志含 `::error::Version drift` 并列出实际值（tag=v0.13.5 vs internal/version/VERSION=0.13.4 与 mygo.json=0.13.4）；gate 先于 `gh release create` 执行，因此无 Release、无任何附件产生；两 artifact job 因 `needs` 未满足而 skip。
   3. 清理：`gh release delete v0.13.5 --yes 2>/dev/null; git push origin :refs/tags/v0.13.5; git tag -d v0.13.5`（正常情况无 Release 可删，命令防御性保留）。

## 完成标准

- 实弹三点全部通过 → 本文档移入 `docs/reports/completed/`，并在 `docs/reports/` 补验收记录（含 run URL、Release 附件清单、sha256 复验、漂移用例日志）。
- 任一点失败 → 以评审 diff 为基准定位（`review-6a4ce73..e211d05.diff`），必要时 `git revert e211d05` 回滚 workflow 后重推；失败证据追加到本文档进展日志。

## 进展日志

（倒序追加；格式：YYYY-MM-DD HH:MM — 动作 + 结果。）

- 2026-11-18 21:49 — Task 2：创建本文档；核实前置状态（master 领先 origin/master 70 提交未推送、e211d05 在 worktree 分支待并回、v0.13.4 tag 不存在、VERSION/mygo.json 均为 0.13.4）。实弹验证交接就绪。
- 2026-11-18（晚间）— Task 1：`e211d05` 完成 release.yml 三 job 重写；yaml-ok + 门禁干跑两用例通过；评审 Spec ✅ / Quality Approved（1 个 deferred minor：GITHUB_PATH 同 step 限制，见上文已知遗留）。
