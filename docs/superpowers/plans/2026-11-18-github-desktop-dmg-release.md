# GitHub Release 桌面 DMG 发布 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** v* tag 推送时 GitHub Release 自动附带桌面 DMG（含 sha256 边车），三 job 并行无竞态，单轨版本一致性门禁。

**Architecture:** 重构 `.github/workflows/release.yml` 为三个 job——`create-release`（tag 解析/版本门禁/建空 Release，输出 tag）、`release`（现有 CLI+GHCR job 瘦身为纯上传）、`desktop`（macos-latest 构建并上传 DMG）。

**Tech Stack:** GitHub Actions、actions/checkout@v4、actions/setup-go@v5、actions/setup-node@v4、gh CLI、mygo v0.9.2（go install 钉版）、jq、shasum。

## Global Constraints

- 仅 DMG：桌面产物只有 `huan-v{ver}-darwin-arm64.dmg` + `.sha256` 边车（spec §6，用户裁决）
- 单轨版本：tag == `internal/version/VERSION` == `mygo.json` `.version`（spec §4），三者不一致必须 fail-fast
- mygo 钉版 `github.com/egoist/mygo/cmd/mygo@v0.9.2`（与本地二进制 provenance 一致）
- 权限维持 workflow 级 `contents: write` + `packages: write` 不变
- 现有 CLI 发布行为不变（产物、notes 文案、Docker 推送）
- SPA 是 go:embed 输入：desktop job 必须 `npm ci && npm run build` 先于 `mygo build`

---

### Task 1: release.yml 三 job 重构

**Files:**
- Modify: `.github/workflows/release.yml`（整文件重写，见 Step 1 全文）

**Interfaces:**
- Consumes: 现有 release job 的全部步骤（原样保留，仅上传步骤改写与 tag 来源改写）
- Produces: `create-release` job output `tag`（vX.Y.Z 字符串）；GitHub Release 在产物 job 运行前已存在

- [ ] **Step 1: 整文件重写 release.yml**

用以下完整内容覆盖 `.github/workflows/release.yml`：

```yaml
name: Release

# Trigger on v* tag push (e.g., v0.13.4) — creates one GitHub Release that
# two artifact jobs upload into in parallel: the CLI release + GHCR image
# (ubuntu) and the desktop DMG (macOS). workflow_dispatch allows publishing
# an already-pushed tag (back-filling or re-runs).
on:
  push:
    tags:
      - "v*"
  workflow_dispatch:
    inputs:
      tag:
        description: "Tag to publish (must already exist, format vX.Y.Z)"
        required: true
        type: string

permissions:
  contents: write  # required by gh release create/upload
  packages: write  # required by docker push to GHCR

jobs:
  # Single source of truth for the tag, the single-version-line gate, and a
  # race-free empty Release that both artifact jobs upload into.
  create-release:
    runs-on: ubuntu-latest
    outputs:
      tag: ${{ steps.tag.outputs.tag }}
    steps:
      - name: Resolve tag (with format validation)
        id: tag
        env:
          INPUT_TAG: ${{ inputs.tag }}
        run: |
          # Prefer explicit workflow_dispatch input; fall back to the tag ref
          # that triggered the run. Validate format to guard against injection
          # in downstream ref: and run: usages.
          tag="${INPUT_TAG:-${GITHUB_REF_NAME}}"
          if ! echo "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
            echo "::error::Invalid tag format: $tag (expected vX.Y.Z)"
            exit 1
          fi
          echo "tag=$tag" >> "$GITHUB_OUTPUT"

      - name: Checkout (at tag)
        uses: actions/checkout@v4
        with:
          ref: ${{ steps.tag.outputs.tag }}

      # Single version line (2026-11-18 ruling): the tag must equal the
      # engine VERSION file and the desktop bundle version in mygo.json.
      - name: Verify single version line
        run: |
          tag="${{ steps.tag.outputs.tag }}"
          version_file="$(tr -d '[:space:]' < internal/version/VERSION)"
          mygo_version="$(jq -r .version mygo.json)"
          fail=0
          for pair in "internal/version/VERSION:$version_file" "mygo.json:$mygo_version"; do
            name="${pair%%:*}"
            value="${pair#*:}"
            if [ "v$value" != "$tag" ]; then
              echo "::error::Version drift: tag=$tag but $name=$value — single version line requires vX.Y.Z in all three"
              fail=1
            fi
          done
          exit $fail

      - name: Create GitHub Release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG: ${{ steps.tag.outputs.tag }}
        run: |
          # If a release already exists for this tag (e.g., workflow re-run),
          # the artifact jobs upload into it instead of failing.
          if gh release view "$TAG" >/dev/null 2>&1; then
            echo "Release $TAG already exists; artifact jobs will upload into it"
          else
            gh release create "$TAG" \
              --title "$TAG" \
              --notes "Automated release for $TAG. See huan_${TAG#v}_checksums.txt for CLI SHA256 verification and huan-v${TAG#v}-darwin-arm64.dmg.sha256 for the desktop build."
          fi

  release:
    needs: create-release
    runs-on: ubuntu-latest
    steps:
      - name: Checkout (at tag)
        uses: actions/checkout@v4
        with:
          ref: ${{ needs.create-release.outputs.tag }}

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "npm"
          cache-dependency-path: web/admin/package-lock.json

      - name: Build admin UI
        run: |
          cd web/admin
          npm ci
          npm run build

      - name: Build release artifacts
        run: |
          # huan release writes to release/<version>/ (no 'v' prefix).
          # go run ensures we use the version pinned at the tag, not a stale local binary.
          go run ./cmd/huan release
          # Build all seven plugins, including root-module diagram-renderer
          # (zhurongshuo.so removed with the theme migration — ADR 0017).
          # Use the same toolchain and normal flags as the Docker host.
          export CGO_ENABLED=1
          scripts/build-plugins.sh "$PWD/docker-plugins"
          ls -lh docker-plugins
          # VERSION file content tells us the path; strip trailing newline.
          version="$(tr -d '\n' < internal/version/VERSION)"
          if ! echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
            echo "::error::Invalid VERSION content: $version"
            exit 1
          fi
          echo "version=$version" >> "$GITHUB_ENV"
          echo "Artifacts built at release/$version/:"
          ls -la "release/$version/"

      - name: Upload CLI artifacts
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG: ${{ needs.create-release.outputs.tag }}
          VERSION: ${{ env.version }}
        run: |
          gh release upload "$TAG" "release/$VERSION/"* --clobber

      # Build & publish Docker image to GHCR so downstream projects
      # (e.g. zhurongshuo) can run their CI jobs in a container with huan
      # pre-installed — no curl/wget/jq/tar dance to download release artifacts.
      #
      # Tags pushed:
      #   ghcr.io/iannil/huan:v0.3.0   (pinned to this release)
      #   ghcr.io/iannil/huan:latest   (rolling)
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push Docker image
        uses: docker/build-push-action@v6
        with:
          context: .
          file: ./Dockerfile
          # Multi-stage Dockerfile builds the CGO-enabled host binary AND all
          # plugins/* .so from the same source in the builder stage, so no
          # host/plugin toolchain drift (plugin.Open would fail).
          push: true
          tags: |
            ghcr.io/iannil/huan:v${{ env.version }}
            ghcr.io/iannil/huan:latest
          labels: |
            org.opencontainers.image.title=huan
            org.opencontainers.image.description=Static site generator written in Go
            org.opencontainers.image.source=https://github.com/iannil/huan
            org.opencontainers.image.version=${{ env.version }}
            org.opencontainers.image.revision=${{ github.sha }}

  desktop:
    needs: create-release
    runs-on: macos-latest
    steps:
      - name: Checkout (at tag)
        uses: actions/checkout@v4
        with:
          ref: ${{ needs.create-release.outputs.tag }}

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "npm"
          cache-dependency-path: web/admin/package-lock.json

      - name: Build admin UI
        run: |
          cd web/admin
          npm ci
          npm run build

      - name: Build desktop DMG
        env:
          TAG: ${{ needs.create-release.outputs.tag }}
        run: |
          # mygo is not on PATH after go install; expose GOPATH/bin explicitly.
          echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
          go install github.com/egoist/mygo/cmd/mygo@v0.9.2
          mygo build -platform darwin/arm64
          # The tag gate guarantees mygo.json version == TAG; derive the name
          # from TAG so the artifact name cannot drift from the release.
          out="huan-${TAG}-darwin-arm64.dmg"
          cp "build/desktop/darwin-arm64/huan-${TAG#v}-arm64.dmg" "$out"
          shasum -a 256 "$out" > "$out.sha256"
          cat "$out.sha256"

      - name: Upload DMG to Release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG: ${{ needs.create-release.outputs.tag }}
        run: |
          gh release upload "$TAG" "huan-${TAG}-darwin-arm64.dmg" "huan-${TAG}-darwin-arm64.dmg.sha256" --clobber
```

- [ ] **Step 2: YAML 语法校验**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('yaml-ok')"`
Expected: `yaml-ok`（无异常栈）

- [ ] **Step 3: 门禁脚本本地干跑（阳性 + 阴性）**

在临时目录构造三件套，验证门禁对「一致」放行、对「漂移」报错：

```bash
T=$(mktemp -d) && cd "$T"
mkdir -p internal/version
printf 'v0.13.4 用例（一致）\n'
printf '0.13.4\n' > internal/version/VERSION
printf '{"version": "0.13.4"}\n' > mygo.json
tag=v0.13.4
version_file="$(tr -d '[:space:]' < internal/version/VERSION)"
mygo_version="$(python3 -c "import json;print(json.load(open('mygo.json'))['version'])")"
fail=0
for pair in "internal/version/VERSION:$version_file" "mygo.json:$mygo_version"; do
  name="${pair%%:*}"; value="${pair#*:}"
  if [ "v$value" != "$tag" ]; then echo "DRIFT: $name=$value"; fail=1; fi
done
[ $fail -eq 0 ] && echo "consistent-case-ok"
printf 'v0.13.5 用例（漂移）\n'
tag=v0.13.5
fail=0
for pair in "internal/version/VERSION:$version_file" "mygo.json:$mygo_version"; do
  name="${pair%%:*}"; value="${pair#*:}"
  if [ "v$value" != "$tag" ]; then echo "DRIFT-DETECTED: $name=$value vs $tag"; fail=1; fi
done
[ $fail -eq 1 ] && echo "drift-case-ok"
cd / && rm -rf "$T"
```

Expected: 依次输出 `consistent-case-ok` 与 `drift-case-ok`（阴性用例无 DRIFT 行，阳性用例有 DRIFT-DETECTED 行）
注：干跑用 python3 json 而非 jq（本机若无 jq 也能跑；runner 上 workflow 用 jq，ubuntu 镜像自带）。

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci(release): publish desktop DMG on v* tags (three-job split)

create-release resolves the tag, gates the single version line
(tag == VERSION == mygo.json, the 2026-11-18 ruling), and creates the
race-free Release that both artifact jobs upload into. The CLI release
job keeps its steps unchanged and now uploads into the pre-created
release. The new desktop job builds the admin SPA, installs mygo v0.9.2
(pinned), runs mygo build -platform darwin/arm64, and attaches
huan-v{ver}-darwin-arm64.dmg + .sha256 to the release. DMG-only per
scope decision; signing/notarization and zip stay out (Phase 1 adhoc)."
```

### Task 2: 文档收尾与实弹验证交接

**Files:**
- Create: `docs/progress/2026-11-18-github-desktop-dmg-release.md`

**Interfaces:**
- Consumes: Task 1 的提交（workflow 已合入 master 工作树）
- Produces: 实弹验证待办的明确交接（push 后 dispatch）

- [ ] **Step 1: 写进度文档**

创建 `docs/progress/2026-11-18-github-desktop-dmg-release.md`，内容涵盖：实施摘要（三 job 结构、门禁、DMG 命名）、本地验证证据（yaml-ok + 门禁干跑两用例）、**PENDING 实弹验证**（master 尚未推送；推送后对已有 tag 跑 `gh workflow run release.yml -f tag=v0.13.4` 或推送新 tag，验收三点：Release 出现 dmg + sha256 附件、CLI 产物不受影响、人为漂移时门禁红）、完成标准（实弹三点全过后文档移 `docs/reports/completed/`）。

- [ ] **Step 2: Commit**

```bash
git add docs/progress/2026-11-18-github-desktop-dmg-release.md
git commit -m "docs(progress): github desktop dmg release — pending live dispatch verify"
```

## Self-Review

1. **Spec coverage**：§2 三 job 结构（Task 1 全文）✓；§3 desktop 步骤 1-8（Task 1 desktop job）✓；§4 门禁细则（Task 1 Verify 步骤 + Step 3 干跑）✓；§5 风险验证（Task 2 实弹交接 + workflow 保留 dispatch）✓；§6 不做清单（未引入）✓；§7 验收标准（Task 2 覆盖三点）✓。
2. **Placeholder scan**：无 TBD/TODO；所有 shell/ YAML 均为全文 ✓。
3. **Type consistency**：`needs.create-release.outputs.tag` 在 release/desktop 两 job 用法一致；dmg 命名 `huan-${TAG}-darwin-arm64.dmg`（TAG 含 v）与本地 `huan-v0.13.4-darwin-arm64.dmg` 一致 ✓；mygo 输出名 `huan-${TAG#v}-arm64.dmg` 与实测 `huan-0.13.4-arm64.dmg` 一致 ✓。
