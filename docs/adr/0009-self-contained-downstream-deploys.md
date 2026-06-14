# ADR 0009：Self-Contained Downstream Deploys（"只依赖 huan"）

- **状态**：Accepted
- **日期**：2026-06-14
- **决策者**：用户（owner）+ Claude（plan mode 收敛）
- **依赖**：[ADR 0003](0003-unified-plugin-system.md)（统一插件系统，huan deploy cloudflare worker 多 Worker 支持）/ [ADR 0004](0004-release-command.md)（release 命令产出 binary）

## 背景

zhurongshuo 项目（huan 的首个生产用户）的部署链路经过 v0.3.0 i18n 系统落地后，仍然依赖多种非 huan 工具：

1. **CI 安装 huan**：GitHub Actions 用 `curl + jq + wget + tar + mktemp` 五个 shell 工具下载 huan GitHub Release 二进制
2. **本地 deploy.sh**：用 `source .env` 读 CF 凭证 + `grep` 检测 qwen3_translate plugin 配置
3. **运行时浏览器**：6 个 cdn.jsdelivr.net 引用（jQuery / Fancybox / html2canvas / qrcodejs2 / MathJax）从未 self-host
4. **wrangler 残留**：PR8 阶段引入 wrangler.toml 用于 i18n-router Worker 部署（PR9 已用 huan 多 Worker 替代，但 wrangler 仍是 zhurongshuo 的可选依赖）

用户诉求："zhurongshuo 项目要求只能依赖 huan"——意味着构建/部署链路纯 huan 单一工具，且发布产物不外引第三方 CDN。

## 决策

### 1. huan 发布官方 Docker image 到 GHCR

- 新增 `Dockerfile`（仓库根目录）：基于 `alpine:3.19` + huan binary + git + ca-certificates + tzdata
- 镜像大小 ~78MB（alpine 5MB + git 15MB + huan binary 28MB + tzdata 5MB + 其他 ~25MB）
- 修改 `.github/workflows/release.yml`：在 `go run ./cmd/huan release` 之后增加 4 个 step：
  - `docker/setup-buildx-action@v3`
  - `docker/login-action@v3`（用 `GITHUB_TOKEN` 鉴权 GHCR）
  - Prepare build context（把 `release/<version>/huan_linux_amd64/huan` 复制到 context root）
  - `docker/build-push-action@v6`（推 3 个 tag：`v$VERSION` / `$VERSION` / `latest`）
- 加 `permissions: packages: write`（GHCR 推送必需）
- 触发：与现有 release.yml 一致（`v*` tag push 或 workflow_dispatch）

### 2. 下游项目 CI 用 `container:` 引用镜像

- 删除 "Install huan" step（curl + jq + wget + tar + mktemp，12+ 行）
- 在 `jobs.build` 加 `container: ghcr.io/iannil/huan:v0.3.0`
- 保留 `actions/checkout@v4` + `actions/configure-pages@v5` + `actions/upload-pages-artifact@v3`（GitHub 官方 actions，不可替代）

### 3. huan 内置 `.env` 加载

- `internal/config/load.go::Load()` 在读 huan.yaml 前自动调 `loadDotEnv(sourceDir)`
- `.env` 文件格式：标准 `KEY=VALUE`，忽略 `#` 注释和空行
- **不覆盖已存在的 env var**——CI 注入的凭证优先于本地 .env
- 引号剥离（`"value"` 和 `'value'` 都识别）
- key 格式校验 `[A-Z_][A-Z0-9_]*`（与 `${VAR}` 插值 regex 一致）

### 4. translate graceful skip

- `cmd/huan/translate_cmd.go::runTranslateQwen3` 在 plugin 未注册时输出友好提示 + 返回 nil（exit 0）
- 让 deploy.sh 无需 grep 检测 plugin 配置即可直接调用

### 5. 运行时 CDN 全 self-host

- zhurongshuo 仓库 `static/js/` 增加本地副本：`html2canvas.min.js` / `qrcode.min.js` / `MathJax/`（含主 JS + TeX-AMS-MML_HTMLorMML combined config）
- 模板 `layouts/partials/{js,mathjax}.html` 把所有 `https://cdn.jsdelivr.net/...` 改为 `/js/...` 本地路径
- `layouts/partials/head.html` 删除 jsdelivr DNS prefetch / preconnect
- jQuery + Fancybox JS + Fancybox CSS 改用已存在的本地文件（template 之前引用 CDN + SRI hash）

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| Phase 1 实现方式 | zhurongshuo CI 用 `go install github.com/iannil/huan/cmd/huan@v0.3.0` 从源码构建 | 需要 Go 工具链（~30s build time）；多平台支持复杂；与"只依赖 huan"语义冲突（依赖 Go） |
| Phase 1 实现方式 | zhurongshuo CI vendor huan binary（commit 进仓库） | 仓库增大 ~10MB；huan 升级需手动 swap binary；多平台需多 binary |
| Phase 1 实现方式 | 用 huan release CLI 内置"下载并解压 tarball"能力 | 与 huan 单二进制定位冲突；Docker image 已替代 |
| Phase 3 实现方式 | huan yaml schema 加 `env: .env` 显式字段 | 增加配置复杂度；自动加载 .env 是约定俗成的 dotfile 行为 |
| Phase 3 实现方式 | 用 `direnv` 或类似工具 | 引入新工具依赖；与"只依赖 huan"冲突 |
| Phase 4 实现 | 升级 MathJax 2.7.8 → 3.x 单文件 | MathJax 3 API breaking change；现有 `MathJax.Hub.Config` 配置需重写为 `MathJax = { tex: ... }` |
| Phase 4 实现 | 用 KaTeX 替代 MathJax | 渲染能力差异；用户已有 MathJax 渲染样式 |

## 影响

### 文档

- 新增本 ADR（0009）
- 更新 `docs/progress/2026-06-14-i18n-plugin-implementation.md`：加 Phase 1-4 完成记录
- 更新 `memory/MEMORY.md`：加项目上下文 + 关键决策

### 代码（huan）

**新增**：
- `Dockerfile`（仓库根目录）
- `internal/config/dotenv_test.go`（9 个测试）

**改造**：
- `.github/workflows/release.yml`：加 packages: write 权限 + 4 个 docker steps
- `internal/config/load.go`：加 `loadDotEnv()` + `isValidEnvKey()` + `stripEnvValueQuotes()`
- `cmd/huan/translate_cmd.go`：runTranslateQwen3 graceful skip

### 代码（zhurongshuo）

- `.github/workflows/deploy.yml`：删 Install huan step + 加 container
- `deploy.sh`：删 source .env + 删 grep
- `layouts/partials/{js,mathjax,head}.html`：CDN URL → 本地路径
- `static/js/{html2canvas,qrcode}.min.js` + `static/js/MathJax/`：新增（~510KB）

### 风险

1. **GHCR image 可见性**：image 必须 public（zhurongshuo 是 public repo）。huan 项目 GHCR 设置一次即可。缓解：huan 项目本身是 public，GHCR 默认继承
2. **多架构支持 v1 仅 amd64**：linux/arm64（如 Raspberry Pi CI）暂不支持。缓解：GH Actions runner 默认 linux/amd64；arm64 后续用 buildx + QEMU 加（v1.1）
3. **.env 加载副作用**：loadDotEnv 写入 os.Environ 是全局副作用。缓解：严格不覆盖已存在 var；测试覆盖"无 .env"路径
4. **MathJax 字体**：当前只下载 MathJax.js + combined config，未下载字体目录。浏览器渲染数学公式时若需字体可能 404。缓解：HTMLorMML 输出优先用 MathML（浏览器原生渲染，不需要字体）；HTML-CSS fallback 时需观察实际效果
5. **HTML-CSS 字体 404**：实际部署后用 DevTools 监控，必要时下载 `MathJax/fonts/HTML-CSS/TeX/...`

## 验证

### 本地 Docker image 验证（已通过）

```bash
# 1. 构建 linux/amd64 huan binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o huan ./cmd/huan

# 2. 构建 docker image
docker build -t huan:local-test -f Dockerfile .

# 3. 在容器内跑 zhurongshuo build
docker run --rm -v /Users/rong.zhu/Code/zhurongshuo:/workspace -w /workspace \
  -e CLOUDFLARE_ACCOUNT_ID=dummy -e CLOUDFLARE_API_TOKEN=dummy \
  -e CLOUDFLARE_R2_ACCESS_KEY_ID=dummy -e CLOUDFLARE_R2_SECRET_ACCESS_KEY=dummy \
  --entrypoint sh huan:local-test -c "huan build"

# 输出：
#   built 2 languages: zh-cn=1045 pages en=3 pages (5.325s)
#   huan version: huan 0.3.0 (a445949)
#   image size: 78.7 MB
```

### Phase 1 上线条件

push huan `v0.3.x` tag 触发 release.yml → 自动构建 docker image + push 到 ghcr.io/iannil/huan

### Phase 2 上线条件

zhurongshuo push 触发 deploy.yml → CI 在 container 内运行（image 已存在）

### Phase 3 验证

```bash
cd zhurongshuo
unset CLOUDFLARE_ACCOUNT_ID  # 清掉环境
./huan plugin info cloudflare  # 应自动读 .env，accountId 应是 .env 中的真实值
./huan translate qwen3         # 即使 plugin 没配也应退出码 0
```

### Phase 4 验证

```bash
cd zhurongshuo
huan build
grep -r "cdn.jsdelivr.net" docs/   # 应返回空
grep -r "js/html2canvas" docs/     # 应有匹配
```

并在浏览器 DevTools Network 面板访问 zhurongshuo.com 任意文章，确认所有 JS 都从 zhurongshuo.com 加载。

## 不在范围（YAGNI）

- huan 内置"下载并解压 tarball"能力（与单二进制定位冲突）
- huan 成为包管理器（npm/cargo 类）——无需求
- Docker multi-arch build（arm64 / windows）——v1.1
- CI workflow 用 self-written actions 替代 `actions/checkout` / `actions/deploy-pages`——GitHub 官方 actions 无法替代
- 第三方 fonts self-host（MathJax fonts、Google Fonts）——待实际 404 监控后再决定
