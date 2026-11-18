# GitHub Release 桌面 DMG 发布（release.yml 桌面 job）设计

- 日期：2026-11-18
- 状态：已获用户批准（对话裁决「可以」）
- 范围：`.github/workflows/release.yml` 单文件改造；不改任何 Go/SPA 代码

## 1. 背景与目标

现有 `release.yml`（v* tag / 手动 dispatch 触发）在 ubuntu 上发布 CLI 产物（`huan release` + 插件 + SHA256 清单）并推 GHCR Docker。桌面端 DMG 一直只在本地 `release/desktop/vX.Y.Z/` 人工发布，从未上 GitHub Release。

目标：v* tag 推送时，同一 GitHub Release 同时获得桌面 DMG（当前阶段仅 DMG，不含 app.zip 与 SHA256SUMS 全表）。

## 2. 结构（三 job，并行、无竞态）

```
tag v* push / workflow_dispatch(tag)
        │
   [create-release]  ubuntu-latest
     ├─ tag 解析与格式校验（vX.Y.Z，防注入，逻辑自现 release job 迁出）
     ├─ 单轨一致性门禁：tag 必须同时等于 internal/version/VERSION 与
     │  mygo.json 的 version（去 v 前缀比较），不等则 fail-fast
     └─ gh release create（标题=tag，notes 沿用现有文案）
        ├──────────────────────┬─────────────────────┐
[release]（现有 job 瘦身）            [desktop]（新，macos-latest）
  CLI 产物 + Docker/GHCR               构建 DMG 并上传
  gh release upload --clobber          gh release upload --clobber
```

- 把「Create GitHub Release」步骤从现 release job 迁至 create-release job，两个产物 job 只做 upload——并行上传同一 Release 无创建竞态。
- tag 解析收敛在 create-release job，并以 job output `outputs.tag` 暴露；release job 与 desktop job 通过 `needs.create-release.outputs.tag` 消费（release job 的 `gh release upload` 与 Docker 推送、desktop job 的 checkout 与上传均用同一 tag），单一事实来源，无重复解析。

## 3. desktop job 步骤

1. 解析 tag（同款 `^v[0-9]+\.[0-9]+\.[0-9]+$` 校验）。
2. `actions/checkout@v4`（at tag = needs.create-release.outputs.tag）。
3. `actions/setup-go@v5`（go-version-file: go.mod，cache: false，与现 job 一致）。
4. `actions/setup-node@v4`（node 20，npm 缓存按 web/admin/package-lock.json）→ `npm ci && npm run build`——admin SPA 是 go:embed 输入（internal/admin/dist 不入库），必须先建。
5. `go install github.com/egoist/mygo/cmd/mygo@v0.9.2`（精确钉版，与本机 mygo 二进制 provenance 一致）。
6. `mygo build -platform darwin/arm64` → build/desktop/darwin-arm64/huan-{ver}-arm64.dmg。
7. 改名/复制为 `huan-v{ver}-darwin-arm64.dmg`（对齐本地发布命名），`shasum -a 256` 生成 `huan-v{ver}-darwin-arm64.dmg.sha256` 边车。
8. `gh release upload "$TAG" dmg sha256 --clobber`（幂等，支持重跑/补发）。

运行器：`macos-latest`（arm64）。mygo 官方无 cgo 构建 .app/DMG，纯 Go 工具链即可；WebKit 系统框架由 darwin 运行器天然满足。

## 4. 一致性门禁细则（create-release job）

```
TAG=v0.13.4
VERSION_FILE=internal/version/VERSION 内容（去空白）
MYGO_JSON=jq -r .version mygo.json
失败条件：三者任一不相等
```

报错用 `::error::` 注解并 exit 1，指明三者实际值。这是版本单轨制（2026-11-18 裁决）的 CI 执行面。

## 5. 风险与验证

- **mygo 在 CI 的首次实战**：本机与 CI 同为 darwin/arm64 且 mygo 声明无需 cgo，理论零障碍；仍需一次真实运行验证。验证路径：workflow 保留 workflow_dispatch（输入已存在的 tag），合并后可对 v0.13.4（或临时 tag）dispatch 补发验证，无需新切 tag。
- **npm/go 缓存**：沿用现有 cache: false（Go）与 npm cache（lockfile 键）。
- **权限**：沿用现有 `contents: write`；GHCR 相关权限仅 release job 需要，维持 workflow 级声明不变。

## 6. 不做（YAGNI）

- 签名/公证（Phase 1 adhoc；README 已说明 xattr -cr 首启方式）
- app.zip / 完整 SHA256SUMS / CLI darwin 产物（用户明确「只发 dmg 即可」）
- Intel / universal 交叉构建
- 发布前测试门禁（tag 从已验证树切出，维持现状）
- matrix 多平台

## 7. 验收标准

1. `act` 不可用时以真实 dispatch 验证：对已有 tag dispatch，GitHub Release 新增 dmg + sha256 附件，命名与本地约定一致。
2. 一致性门禁：人为构造不一致（如本地将 VERSION 改错后 dispatch）→ job 红，错误信息列出三者值。
3. 现有 CLI 发布路径行为不变（job 结构迁移后产物、Docker 推送不变）。
