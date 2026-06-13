# ADR 0002：Cloudflare 发布插件设计

- **状态**：Accepted
- **日期**：2026-06-13
- **决策者**：用户（owner）+ Claude（grill-me 收敛）
- **替代方案**：见下方各分叉点的备选项
- **依赖**：[ADR 0003](0003-unified-plugin-system.md)（统一插件系统）

## 修订记录

- **2026-06-13（同日二次修订）**：grill-me 二轮收敛后，把「插件系统」抽象层拆到 [ADR 0003](0003-unified-plugin-system.md)。本文档聚焦 Cloudflare 实施细节：
  - §2 形态：去掉 `internal/deploy/types.go` 拥有 `Registry` 的描述（迁到 `internal/plugin/`），引用 0003。
  - §4 CLI：增加 `huan plugin list/info` 管理命令（来自 0003）。
  - §5 配置：yaml 形状从 `deploy.cloudflare.*` 改为 `plugins.cloudflare.*`（来自 0003）。
  - §5 凭证：明确 `${VAR}` strict 插值规则（来自 0003）。
  - 新增 §13 首次实施范围（Pages only；R2/Worker 后续 PR）。
  - 新增 §14 Pages UX 决策（branch / commit metadata / concurrency 默认值）。
- **2026-06-13（同日三次修订）**：首 PR 实施前 Plan agent 反向工程 wrangler 源码（`cloudflare/workers-sdk`）验证实际协议，修正 4 处：
  - §7 Pages 协议：从「4 步 + SHA256」改为「**5 步 + blake3**」（新增 upsert-hashes 步骤；hash 算法是 `blake3(base64(content)+ext).hex()[:32]`）。
  - §7 新增 §7.1（blake3 算法详述）+ 双 token 鉴权模型（`apiToken` 长期 + `jwt` 300s 短期）。
  - §9 retry：补充「遇 gateway 5xx 自动降 HTTP 并发到 1」语义。
  - §14.3 concurrency：拆分为「文件 hash 并行（默认 `min(GOMAXPROCS,8)`，`--concurrency` 覆盖）」与「HTTP POST 并行（硬上限 3，不可调）」两层。
  - 新增 §14.4 资源硬上限（25 MiB/file、40 MiB/bucket、2000/bucket、20000 total，manifest 阶段即检查）。

## 背景

zhurongshuo 当前发布链路依赖一束 shell 脚本（`deploy.sh` + `scripts/r2-sync.sh` + `scripts/deploy-worker.sh`）协调 Hugo + Cloudflare，存在以下问题：

1. **强依赖 Node.js + wrangler CLI**：与 huan 「Go 单二进制」卖点冲突。
2. **静态站本身未走 Cloudflare**：HTML 仍走 GitHub Pages，R2 只装图片，发布目标分散。
3. **technical-plan.md §4.11 的插件骨架只覆盖 build 时**（Content/Template/Output Processor），未预留给 deploy 时插件。
4. **脚本难调试**：缺少结构化日志、重试、增量、并发控制等生产特性。

stage 2 起步需要把发布能力沉淀为 huan 一等公民的「独立插件」，并完成 zhurongshuo 的全量 Cloudflare 化（HTML → Pages、图片 → R2、image-resizer → Worker）。

## 决策

把 Cloudflare 发布做成 huan 的**内置插件**，作为 [ADR 0003](0003-unified-plugin-system.md) 统一插件系统的**首个 deploy 插件实例**。本文档聚焦 Cloudflare 实施细节；插件系统抽象层（Plugin 基接口、Registry、capability 接口模型、yaml `plugins:` 命名空间、`${VAR}` 插值、CLI 管理命令、注册机制）见 0003。

### 1. 范围：全量 Cloudflare 化

| 目标 | Cloudflare 产品 | 实现路径 |
|---|---|---|
| HTML/RSS/sitemap 等静态产物 | Cloudflare Pages | direct-upload API |
| 图片等大对象 | R2 bucket | S3 兼容 API（minio-go） |
| image-resizer 等 edge 逻辑 | Workers | modules API |

不走 R2 静态托管（默认页/Content-Type/404 需手工处理，Pages 更原生）；不走 wrangler（破坏单二进制定位）；不引 cloudflare-go SDK（管理面导向、发布 API 仍需补齐）。

### 2. 形态：内置插件 + 接口隔离

Cloudflare 插件实现 [ADR 0003](0003-unified-plugin-system.md) 定义的 `Deployer` capability 接口：

- `internal/plugin/` 拥有 `Plugin` 基接口（`Name() string`）与 `Registry`（详见 0003）。
- `internal/deploy/types.go` 拥有 `Deployer` capability 接口（依赖 `plugin.Plugin`）。
- Cloudflare 实现位于 `internal/deploy/cloudflare/`，编译期内置，**同一二进制**。
- 未来可平行加 `netlify` / `s3` / `github-pages` 等 deployer（在 `internal/deploy/<name>/` 下，注册到同一 Registry）。

```go
// internal/deploy/types.go
type Deployer interface {
    plugin.Plugin  // 嵌入 Name()
    Deploy(ctx context.Context, opts Options) (*Report, error)
}
```

Registry 与 Plugin 基接口定义见 [ADR 0003 §2-§3](0003-unified-plugin-system.md)。

### 3. 通信：纯 Go 直连 Cloudflare API

不 shell out wrangler，不引 Node.js。所有 HTTP 调用走 `net/http` + 标准库；R2 走 minio-go（S3 兼容）；结构化日志满足 CLAUDE.md 的可观测性要求（trace_id/span_id/event_type）。

### 4. CLI：三层子命令 + 管理命令

**动作命令**（per-capability verb，详见 [ADR 0003 §6](0003-unified-plugin-system.md)）：

```
huan deploy cloudflare              # Pages + R2 + Worker 全推
huan deploy cloudflare pages        # 仅 HTML
huan deploy cloudflare r2           # 仅图片
huan deploy cloudflare worker       # 仅 Worker
```

- 默认不跑 build；`--build` flag 一键 build + deploy。
- `--dry-run`：计算 diff 但不真传，输出 JSON 总结报告。
- `--prune`（R2）：删除远端孤儿对象，默认不删。
- `--branch=<name>`（Pages）：覆盖 yaml 里的 `pages.branch`，可填 `preview` 触发 CF Pages preview deployment。
- `--commit-sha=<sha>` / `--commit-message="<msg>"`（Pages）：显式 commit 元数据；缺失则从 git 推断；再缺失则空（详见 §14）。
- `--concurrency=N`：默认 `min(GOMAXPROCS, 8)`（详见 §14）。

**管理命令**（统一 plugin 系统，详见 [ADR 0003 §6](0003-unified-plugin-system.md)）：

```
huan plugin list                    # 列出所有编译期内置 plugin + capability + 配置状态
huan plugin info cloudflare         # 元数据 + effective config + health + last action
huan plugin info cloudflare --show-secrets   # 显示敏感字段（默认 mask 为 ***）
```

### 5. 配置：huan.yaml `plugins:` 命名空间

遵循 [ADR 0003 §4-§5](0003-unified-plugin-system.md)：所有 plugin 配置在 yaml 顶层 `plugins.<name>.*`；凭证通过 `${VAR}` strict 插值（unset 报错）注入。

```yaml
plugins:
  cloudflare:
    accountId: ${CLOUDFLARE_ACCOUNT_ID}
    apiToken: ${CLOUDFLARE_API_TOKEN}
    pages:
      project: zhurongshuo
      branch: main                  # 必填；CLI --branch 覆盖
    r2:
      accessKeyId: ${CLOUDFLARE_R2_ACCESS_KEY_ID}
      secretAccessKey: ${CLOUDFLARE_R2_SECRET_ACCESS_KEY}
      bucket: zhurongshuo
      sync:
        - { from: static/images/gallery, to: images/gallery/ }
    worker:
      name: image-resizer
      script: workers/image-resizer.js
      compatibilityDate: "2024-01-01"   # 默认值
      bindings:
        - { type: r2_bucket, name: R2_BUCKET, bucket: zhurongshuo }
      routes:
        - { pattern: r2.zhurongshuo.com/*, zone: zhurongshuo.com }
```

环境变量（通过 `${VAR}` 引用，未设置则启动报错）：

| 变量 | 用途 |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Pages + Workers API 鉴权 |
| `CLOUDFLARE_ACCOUNT_ID` | 账户路由 |
| `CLOUDFLARE_R2_ACCESS_KEY_ID` | R2 S3 兼容鉴权 |
| `CLOUDFLARE_R2_SECRET_ACCESS_KEY` | R2 S3 兼容鉴权 |

### 6. R2 同步策略

- **增量判定**：先 `LIST` bucket 拿 `{key → etag}` 映射，walk 本地目录算 SHA256，比对一致则跳过。
- **孤儿处理**：默认保留（避免误删老页面引用的图片）；`--prune` 显式开启。
- **多路径映射**：`sync: [{from, to}]` 数组，每项 from 是本地路径，to 是 R2 key 前缀。
- **上传协议**：minio-go 客户端，path-style URL，region `auto`，account-id 隔离 endpoint。

### 7. Pages 部署策略

走 Cloudflare direct-upload API（**5 步协议**，2026-06-13 grill-me 二轮后通过反向工程 wrangler 源码验证）：

1. **本地 manifest 构建**：走 publishDir，对每个文件算 **blake3** hash（详见 §7.1）+ size + content-type。
2. **GET upload-token**：`GET /accounts/{id}/pages/projects/{project}/upload-token` → 返回短期 **JWT**（300s 过期），用 `apiToken` 鉴权。
3. **POST check-missing**：`POST /pages/assets/check-missing`，body `{hashes: ["...", ...]}`，用 JWT 鉴权 → 返回**缺失**的 hash 数组（CF 已有的不需要重传）。
4. **POST upload**（分批 ≤ 2000/bucket）：`POST /pages/assets/upload`，body `[{key: "<hash>", value: "<base64-content>", metadata: {contentType: "<mime>"}, base64: true}, ...]`，用 JWT 鉴权。
5. **POST upsert-hashes**：`POST /pages/assets/upsert-hashes`，body `{hashes: ["...", ...]}`（**本次部署完整 hash 列表**，不只是新上传的），用 JWT 鉴权 —— 告诉 CF 「这次 deployment 由这些 asset 组成」。
6. **POST deployment**：`POST /accounts/{id}/pages/projects/{project}/deployments`，`multipart/form-data`：`manifest` 字段（JSON `{"/<path>": "<hash>"}`，**key 必须前导 `/`**）+ 可选 `branch` / `commit_message` / `commit_hash` / `commit_dirty` 字段，用 `apiToken` 鉴权。

**双 token 鉴权模型**（关键）：
- `apiToken`（账户级，长期）：用于步骤 2（GET upload-token）和步骤 6（POST deployment）。
- `jwt`（步骤 2 返回的短期 token，300s 过期）：用于步骤 3 / 4 / 5（assets/* 端点，**注意路径不带 `/accounts/{id}` 前缀**，JWT 已 scoped）。
- JWT 过期处理：client 在每次 retry 前检查 `isJwtExpired`，过期自动重新 GET upload-token 一次。

**默认 production branch**；`--branch=preview` 切预览。commit 元数据按 §14.2 三层 fallback。

#### 7.1. blake3 hash 算法（关键）

CF Pages asset hash **不是 SHA256**，是 **blake3** 截断到 32 hex 字符：

```
hash = blake3(base64(content) + ext).hex()[:32]
```

其中 `ext` 是**不含前导 dot** 的扩展名（`"html"` / `"css"` / 无扩展名为空字符串 `""`）。

来源：`cloudflare/workers-sdk` 仓库 `packages/deploy-helpers/src/deploy/helpers/hash.ts`。

**含义**：
- 用 `github.com/zeebo/blake3` 库（纯 Go，~50KB）。
- hash 函数实现必须用 wrangler 已知输出做向量测试，否则算错一个字节整个 dedup 失效。
- R2 sync（PR2）的「SHA256」描述**不**受影响 —— R2 走 S3 兼容 etag，与 Pages asset hash 是两套独立算法。

### 8. Worker 部署策略

- 脚本源：`deploy.cloudflare.worker.script` 指向单文件 `.js`。
- 兼容性日期：默认 `"2024-01-01"`（可覆盖）。
- 绑定/路由/触发器：huan.yaml 全量声明（不读 wrangler.toml）。
- 上传协议：PUT `/accounts/{id}/workers/scripts/{name}`，multipart：`metadata`（JSON）+ `script`（`application/javascript+module`）。

### 9. 失败语义与可观测性

- **重试**：每文件/每 API 调用 3 次指数退避（200ms / 1s / 5s），区分可重试（5xx、429、网络）与不可重试（4xx 鉴权/配置）错误。
- **gateway 5xx 自动降并发**：遇 Cloudflare gateway 5xx（典型的「并发太高」信号），HTTP POST 并发自动降到 1，不再恢复（避免持续撞限流）。
- **收集不中断**：单文件最终失败进入失败列表，继续后续，避免大站点一颗老鼠屎。
- **JSON 总结报告**：部署结束输出 `{trace_id, target, attempted, succeeded, failed, skipped, duration_ms, failures: [...]}`。
- **结构化日志**：每次 API 调用埋点 Function_Start/End + payload 摘要 + 耗时，写入 stderr，便于 grep / 重放。

### 10. 资源存在性假设

插件假设 Pages project / R2 bucket / Worker 已在 Cloudflare 控制台手工创建。不存在则报错退出，附创建指引链接。**不自动创建账户级资源**（避免误扣费、避免抢占用户对账号的控制权）。DNS / SSL 同样需用户预配。

### 11. 测试策略

- **单元测试**：全 `net/http/httptest` mock Cloudflare API（含 Pages、R2 S3、Workers 三套 mock server），覆盖鉴权、重试、增量、孤儿、错误分支。CI 默认跑。
- **集成测试**：仅当 `HUAN_CLOUDFLARE_INTEGRATION=1` 且凭证在场时跑，打真实 Cloudflare（建议独立测试账户 + 小 bucket）。build tag `integration` 隔离。

### 12. 小决定（已按推荐定）

| 项 | 默认 |
|---|---|
| Worker `compatibility_date` | `"2024-01-01"`，可在 huan.yaml 覆盖 |
| `--dry-run` | 支持，输出 diff 报告，不真传 |
| 并发上传 | `min(GOMAXPROCS, 8)`，`--concurrency=N` 可配（详见 §14） |
| Build 耦合 | 默认不 build；`--build` 一键 |

### 13. 首次实施范围（grill-me 二轮收敛）

全量 Cloudflare 化 = Pages（HTML）+ R2（图片）+ Worker（image-resizer）。**首期只做 Pages**，R2 与 Worker 拆为后续 PR。

**首 PR**：
- 插件系统骨架（[ADR 0003](0003-unified-plugin-system.md) 落地）：`internal/plugin/` + `internal/config/interpolate.go` + `cmd/huan/plugins.go` + `cmd/huan/plugin_cmd.go`。
- Deployer capability 接口（`internal/deploy/types.go`）。
- Cloudflare Pages direct-upload 客户端（含 retry / dry-run / JSON report）。
- `huan deploy cloudflare [pages]` 子命令。
- 测试：httptest mock + integration tag。

**后续 PR**：
- PR2：R2 sync（minio-go 集成、增量算法、`--prune`）。
- PR3：Worker modules upload（multipart、bindings、routes）。

**为什么先 Pages**：
1. Pages 是最大价值（HTML 是站点本体）；图片 / Worker 不阻塞迁移主体。
2. 先用一个具体实现验证 plugin 架构，再叠加复杂度（R2 S3 兼容、Worker multipart 都有边缘 case）。
3. Pages direct-upload API 公开稳定；R2 endpoint 拼装 / Worker multipart 边界各自有坑。
4. PR 体量控制在 500-800 行，review 顺。

**过渡期 zhurongshuo 迁移路径**：Pages-only 阶段，HTML 由 `huan deploy cloudflare pages`，图片仍由 `scripts/r2-sync.sh` 跑（不阻塞），Worker 由 `scripts/deploy-worker.sh` 跑（不阻塞）。等 PR2/PR3 落地再逐个移除老脚本。

### 14. Pages UX 决策（grill-me 二轮收敛）

**14.1 branch 默认值**：
- yaml `plugins.cloudflare.pages.branch` **必填**（不写报错）。
- `--branch=<name>` CLI flag 覆盖；填 `preview` 触发 CF Pages preview deployment。
- **不**自动推断 git 当前分支 —— 避免 feature 分支误触发 production deploy 的 footgun。huan 是发布工具，用户应明确指定。

**14.2 commit metadata**（用于 CF Pages dashboard 显示部署对应 commit）：
- 优先级：`--commit-sha` / `--commit-message` CLI flag > git 推断（`git rev-parse HEAD` + `git log -1 --pretty=%s`）> 留空。
- 封装在 `internal/deploy/cloudflare/git.go::InferCommitMetadata()`。
- 非 git 环境：flag 缺失 → git 推断失败 → 留空。CF API 接受空值。

**14.3 并发模型（两层解耦）**：
- **文件 hash 并行**（CPU-bound）：默认 `min(GOMAXPROCS, 8)`；`--concurrency=N` 覆盖。这是 blake3 计算 + base64 编码的并行度。
- **HTTP POST 并行**（I/O-bound）：硬上限 **3**（与 wrangler 默认值一致），**不可**通过 CLI 提速。原因是 CF gateway 对单 project 的高并发上传会返回 5xx。
- 遇 429 / 5xx gateway：按 §9 走 retry 退避 + 自动降 HTTP 并发到 1。

**14.4 资源硬上限**（manifest / Batch 阶段即检查，不进入上传阶段）：

**「bucket」= 一次 `POST /pages/assets/upload` 请求体**（NOT per-deployment total）。20,000 文件的 deployment 会被切成多个 bucket 分多次 POST。

- 单文件大小上限：**25 MiB**（manifest 阶段检查，BuildManifest 报错）。
- 单 bucket 文件数上限：**2000**（Batch 阶段检查）。
- 单 bucket 字节上限：**40 MiB**（Batch 阶段检查，与文件数取先到为准）。
- 总文件数上限：**20,000**（manifest 阶段检查）。
- 命中任一上限 → 对应阶段直接 error，列出超限清单。

来源：`cloudflare/workers-sdk` 仓库 `packages/wrangler/src/pages/constants.ts`：
`MAX_ASSET_COUNT_DEFAULT=20000` / `MAX_ASSET_SIZE=25MiB` / `MAX_BUCKET_SIZE=40MiB` / `MAX_BUCKET_FILE_COUNT=2000`。

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| 范围 | 只移植 R2+Worker，HTML 仍走 GitHub Pages | 用户明确要全量 Cloudflare 化 |
| HTML 托管 | R2 静态托管 | Content-Type / 默认页 / 404 需手工处理，Pages 更原生 |
| 形态 | 外部二进制（Git 风格子命令） | 需发布两个二进制，与项目单二进制定位冲突 |
| 形态 | Build tag 条件编译 | 用户加 tag 重编译门槛高 |
| 通信 | Shell out wrangler | 破坏单二进制定位 |
| 通信 | cloudflare-go SDK | 主要面向管理面，发布 API 仍需手写 |
| R2 上传 | 手写 S3 sig4 | 签名细节多，minio-go 已成熟 |
| R2 上传 | aws-sdk-go-v2 | 依赖重（~10MB+），与 Cloudflare 心智不一致 |
| 配置 | wrangler.toml + huan.yaml 混合 | 双源，心智成本高 |
| 配置 | 纯环境变量 | 本地开发体验差 |
| Worker 配置 | 读 wrangler.toml | 双源，与 Q6 不一致 |
| 增量 | 逐个 HEAD（zhurongshuo 现状） | N+1 API 调用，大目录慢 |
| 增量 | 全量上传 | R2 写入费用累积 |
| 失败 | fail-fast | 大站点中后期失败浪费前期上传 |
| 失败 | 本地 state file（resumable） | 引入 state 文件管理负担 |
| Pages 环境 | 默认 preview | 日常发布多一步、不便 |
| Build 耦合 | 总是先 build | 不能复用预构建产物，CI 不友好 |
| 测试 | 仅 integration | 每次烧配额，离线不能跑 |
| 资源创建 | 自动创建 | 误扣费风险、抢占用户控制权 |

## 影响

### 文档

- 新增本 ADR（Cloudflare 实施细节）。
- 新增 [ADR 0003](0003-unified-plugin-system.md)（统一插件系统抽象层）。
- `docs/technical-plan.md` §4.11 更新：与新统一 plugin 系统对齐（去老 build-time-only 骨架，引用 ADR 0003）。
- `docs/standards/` 增补部署规范（凭证管理、dry-run 流程、失败重试策略）。

### 代码

新增目录结构（粗略）：

```
internal/
├── plugin/                    # 统一插件宿主（详见 ADR 0003）
│   ├── plugin.go              # Plugin 接口 + Registry + Find[T] helper
│   └── plugin_test.go
├── config/
│   └── interpolate.go         # ${VAR} strict 插值
└── deploy/
    ├── types.go               # Deployer capability 接口 + Options + Report
    ├── logging.go             # 结构化日志（trace_id/span_id）
    └── cloudflare/
        ├── plugin.go          # 实现 plugin.Plugin + deploy.Deployer，Name()="cloudflare"
        ├── options.go         # plugins.cloudflare.* 反序列化
        ├── pages.go           # Pages direct-upload 客户端
        ├── git.go             # InferCommitMetadata
        ├── retry.go           # 指数退避 + 错误分类
        ├── pages_test.go      # httptest mock
        └── integration/       # build tag integration，真打 Cloudflare
            └── pages_test.go

cmd/huan/
├── plugins.go                 # cfg → plugin.Registry 接线（composition root）
├── deploy.go                  # huan deploy cloudflare [pages|r2|worker]
└── plugin_cmd.go              # huan plugin list / info
```

注：R2 与 Worker 相关文件（`r2.go` / `worker.go` / `r2_test.go` / `worker_test.go` / 对应 integration）在后续 PR 落地，首 PR 不创建。

CLI 依赖新增：

- `github.com/minio/minio-go/v7` — R2 S3 客户端。

### zhurongshuo 迁移（后续）

- 旧 `deploy.sh` + `scripts/r2-sync.sh` + `scripts/deploy-worker.sh` 在迁移完成后弃用。
- `.github/workflows/hugo.yml` 改为：`huan build && huan deploy cloudflare`，移除 wrangler 安装步骤与 R2 sync step。
- 旧 `wrangler.toml` 仅作历史档案，huan 不读。

### 风险

1. **minio-go 依赖体积**：约 5MB，加进单二进制可接受；若未来要进一步压依赖，可评估手写 S3 sig4。
2. **Cloudflare API 漂移**：直连意味着我们承担 API 兼容性跟踪；建议在 `internal/deploy/cloudflare/` 内集中放 endpoint 常量，便于跟踪变更。
3. **Worker 单文件限制**：当前不支持多模块打包；未来若 zhurongshuo Worker 拆模块，需要扩展。
4. **Pages 文件数上限 20,000**：zhurongshuo 当前规模远低于此；若未来突破，需要拆 deployment 或迁 R2 静态托管。
