# huan-plugin-cloudflare

Cloudflare 部署插件，用于将 huan 构建输出部署到 Cloudflare Pages、R2 和 Workers。

## 前提

- Go 1.26.2（必须与 huan 主二进制使用相同的 Go 版本）
- Cloudflare 账号（Pages、R2 或 Workers 按需配置）

## 编译

```bash
cd plugins/cloudflare
go build -buildmode=plugin -o ../cloudflare.so .
```

产物：`plugins/cloudflare.so`

## 配置

在 `huan.yaml` 的 `plugins.cloudflare` 下配置：

```yaml
plugins:
  cloudflare:
    accountId: ${CLOUDFLARE_ACCOUNT_ID}
    apiToken: ${CLOUDFLARE_API_TOKEN}
    pages:
      project: my-site
      branch: main
    # r2:  # 可选
    #   accessKeyId: ${CLOUDFLARE_R2_ACCESS_KEY_ID}
    #   secretAccessKey: ${CLOUDFLARE_R2_SECRET_ACCESS_KEY}
    #   bucket: my-bucket
    #   sync:
    #     - from: static/images
    #       to: images
    # worker:  # 可选
    #   name: my-worker
    #   script: workers/my-worker.js
```

插件配置通过 `${VAR}` 环境变量插值，由 huan 的 config 层处理。

## 使用

```bash
# 部署到 Cloudflare Pages
huan deploy cloudflare pages

# 部署到 R2
huan deploy cloudflare r2

# 部署到 Workers
huan deploy cloudflare worker

# 不执行实际部署，只计算变更
huan deploy cloudflare pages --dry-run

# 指定自定义插件目录
huan deploy cloudflare pages --plugin-dir ./plugins
```

## 能力

该插件实现 `deploy.Deployer` 接口，支持三个目标：

| 目标 | 描述 | 必需配置 |
|------|------|---------|
| `pages` | Cloudflare Pages 直接上传 | `accountId`, `apiToken`, `pages.project`, `pages.branch` |
| `r2` | R2 存储桶同步 | `r2.accessKeyId`, `r2.secretAccessKey`, `r2.bucket` |
| `worker` | Cloudflare Workers 部署 | `worker.name`, `worker.script` |

## 本地开发

插件仓库是自包含的，复制了 huan 的 `deploy`、`plugin`、`observability` 接口类型。

当这些接口变更时，需要手动同步 `plugins/cloudflare/deploy/`、`plugins/cloudflare/plugin/`、`plugins/cloudflare/observability/`、`plugins/cloudflare/version/` 下的文件。

## 依赖

- `gopkg.in/yaml.v3` — 配置解析
- `github.com/zeebo/blake3` — 文件哈希
- `github.com/minio/minio-go/v7` — R2 S3 兼容客户端