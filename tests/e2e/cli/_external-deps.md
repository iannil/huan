# CLI 套件排除清单（_external-deps.md）

四个 CLI 套件（build/new/config/version）**有意不覆盖**以下命令——每项一句排除原因与「何时补测」条件。新增套件时先对照本表，避免重复决策。

| 命令 | 排除原因 | 何时补测 |
|---|---|---|
| `translate`（含 `translate qwen3`） | 需 qwen3 翻译插件 + 本地 Ollama/qwen3 HTTP 端点，E2E 环境无此 AI 服务且调用有成本与时延 | CI 或本机提供稳定 qwen3 端点（`translate status` 可先行——纯本地状态盘点，见下注） |
| `deploy` | 需 Cloudflare API 凭据与真实目标（Pages/R2），执行即产生远端副作用，不可在测试环境重复触发 | 提供 sandbox 级 CF 凭据 + 专用 preview 分支/测试 bucket 后补 `deploy --dry-run` 类只读路径 |
| `sync`（含 `sync gallery`） | 对 `$site/static/`、`content/gallery/` 做批量脚手架生成，副作用面大且与 fixture 已知状态强耦合（会破坏 FIXTURES.md 常量） | fixture 体系支持「每用例独立 site 拷贝 + 用后即弃」的隔离约定后（现有 runtime 已可，待有真实 gallery 素材需求再立套件） |
| `release` | 交叉编译全平台 + 打包归档到 `/release`，产物体积大、耗时长，且发布产物属发布流程验证（另有发布约定管辖） | 发布流程自动化任务（非 E2E 体系）接管；E2E 仅在 release 产物断言成为独立需求时补 |
| `daemon` 的 `--tls-cert/--tls-key` flags | TLS 握手需证书/私钥对与 systemd 环境外的 TLS 探活改造（https curl 探测 + 自签证书注入），当前 daemon 套件（api/sse-events）只覆盖明文 HTTP | 自签证书 fixture 就绪（mkcert 或 openssl 现场生成）+ daemon 套件增加 https 探活模式时 |
| `daemon` 的 `--systemd` flag | systemd notify 集成需 Linux + systemd 环境，macOS E2E 环境无法触发 `Type=notify` 语义 | E2E 迁移到 Linux CI runner（systemd 容器）后，作为 daemon 启动路径专项 |

注：`translate status` 本身是纯本地命令，理论上可测；但 translate 命令族共享 fixture 前提（已翻译内容对），单测 status 会引入翻译缓存文件到 fixture——与 translate 一起等端点条件成熟再补，避免半族覆盖。

关联：排除清单内的 daemon 明文路径已由 [api 套件](../api/sse-events.yaml)（public-api/sse-events）覆盖；`serve` 是 `dev` 的 deprecated 别名，dev 路径由 api/browser 套件覆盖。
