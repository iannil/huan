# skipProjectStatic 缺陷：移除该开关并加资产完整性门禁

- 时间：2026-11-13 22:35 起，2026-11-13 22:52 完成实现与验证
- 状态：**实现完成、未提交、未发布**（工作区改动待确认后提交）
- 触发来源：下游站点 zhurongshuo 第三轮审计（`drafts/reports/2026-11-13-general-papers-audit/audit-round3.md` §7.1）
- 关联版本：v0.12.6（`be4d41f` 引入 `skipProjectStatic`）

## 1. 缺陷

`languages.<code>.skipProjectStatic: true` 会跳过把共享 `static/` 拷进该语言输出，但 **huan 自己的 canonify 会把该语言页面的根相对资源改写成语言前缀**，于是被跳过的资源必然 404，构建全程不报错。

实测链路（zhurongshuo，HEAD `2b22eb2ebe`）：

| 环节 | 事实 | 依据 |
| --- | --- | --- |
| 模板 | `<link rel="stylesheet" href="/css/normalize.css">` | `layouts/partials/head.html:118` |
| canonify | 写成 `https://zhurongshuo.com/en/css/normalize.css` | `docs/en/index.html` grep 命中 |
| 配置注释 | 「页面用根绝对路径 /css /js 时省掉重复 I/O」 | `huan.example.yaml` |
| 实际 | EN 页面用 `/en/css/...`，必须存在 `docs/en/css/` | 同上 + `static/css/` 存在 |
| 0.12.6 实测 | en 产物 5047 → 3545 文件，`en/css` 消失 | 审计 §7.1 表格（`huan build -d /tmp` 双向实测） |

结论：配置注释的前提（页面引用根绝对资源）与 huan 自身 canonify 行为相反。只要模板用 Hugo 惯用的根相对路径，开启该开关就等于发布无样式英文站，且 **构建期零提示**。

## 2. 设计取舍

用户的硬约束有两条：**英文版不允许静默损失样式**；**中文和英文是对照的，要对应**。

因此放弃「让 EN 页面引用主机根资源、借用默认语言目录」的方案：那会让 EN 不自足，两个语言版本不再一一对应。改为：

1. **取消跨语言借用** —— 删除 `skipProjectStatic`，共享 `static/` 永远拷进每个语言输出；
2. **构建期硬门禁** —— 页面引用的资产必须存在于本语言输出；每个语言输出的共享静态文件集合必须完整。缺失即构建失败，不再有静默路径。

## 3. 实现

| 层 | 改动 | 文件 |
| --- | --- | --- |
| output | `CanonifyOptions.OnRewrite`：canonify 改写根相对 URL 时回调原始值；nil 时零成本，输出字节不变 | [canonify.go](../../internal/output/canonify.go) |
| build | 新增资产门禁：`checkReferencedAssets`（引用存在性）、`checkStaticParity`（语言间共享静态对照）、`expectedStaticFiles`（镜像拷贝规则）、`HUAN_ASSET_CHECK=off` 降级 | [asset_gate.go](../../internal/build/asset_gate.go) |
| build | 静态阶段改为可返回错误；门禁在拷完后执行（`static + finalize` → 失败即中止构建） | [pipeline_write.go](../../internal/build/pipeline_write.go)、[build.go](../../internal/build/build.go) |
| build | 每个语言武装门禁；去掉 `SkipProjectStatic` 注入 | [multisite.go](../../internal/build/multisite.go)、[pipeline.go](../../internal/build/pipeline.go)、[pipeline_setup.go](../../internal/build/pipeline_setup.go) |
| config | 删除 `SkipProjectStatic` 字段；`load.go` 显式拒绝已移除键并给迁移提示（typed 解码会静默忽略未知键） | [languages.go](../../internal/config/languages.go)、[load.go](../../internal/config/load.go)、[removed_keys.go](../../internal/config/removed_keys.go) |
| 文档 | `huan.example.yaml` 语言级 static 注释改写：共享 static 必拷 + 门禁说明 | [huan.example.yaml](../../huan.example.yaml) |

门禁只判「像静态文件」的引用（有 `assetFileExts` 里的扩展名），页面路由（`/papers/x/`）不判——它们可能由 `_redirects` 提供，避免误报。

## 4. 验证（原始输出）

| 项 | 命令 | 结果 |
| --- | --- | --- |
| 单元/集成 | `go test ./...` | 全绿 |
| 静态检查 | `go vet ./...` | 无输出 |
| 竞态 | `go test -race ./internal/build ./internal/output` | ok（2.1s / 1.5s） |
| canonify 性能 | `-bench CanonifyCompared` | attributes 85.5µs/6 allocs（与改动前一致量级），observer 为 nil 时零分支外开销 |
| 真实站点回归 | `huan build -s ../zhurongshuo -d /tmp/... --noCache` | exit 0，`built 2 languages: zh-cn=1396 pages en=1343 pages (1.85s)`，产物 5215 + 5047 文件，**门禁零误报** |
| 门禁开销 | 同上 `--timings` | `asset gates` 17.2ms（zh-cn）/ 16.8ms（en），占 1.71s 总耗时约 1% |
| 回归用例（历史形状） | `TestMultiSiteFailsWhenLanguageDropsReferencedAsset` | en 输出缺 `css/` 且页面仍引用 `/css/site.css` → 构建失败并点名该引用 |
| 迁移守卫 | `TestRejectRemovedKeys` | `languages.en.skipProjectStatic` → `huan.yaml: languages.en.skipProjectStatic was removed: ...` |
| 对照用例 | `TestMultiSiteEveryLanguageCarriesStatic` | `docs/css/site.css` 与 `docs/en/css/site.css` 同时存在且逐字节相同 |

## 5. 影响与边界

- **行为变化**：`skipProjectStatic` 从「静默生效」变为「配置报错」。保留该行的站点会在 `config.Load` 阶段失败并看到迁移提示；删掉该行即恢复全量拷贝（正确行为）。
- **性能代价**：放弃 0.12.6 的 -7.5%（那 7.5% 的收益来自跳过拷贝，而跳过本身是错的）。构建期新增约 17ms/语言门禁开销，约 1%。
- **门禁范围**：只覆盖 HTML 的 `href`/`src`。`.so` 插件或 CSS 内部 `url(...)` 引用的资源不在判定内（仍是既有边界）。
- **逃生口**：`HUAN_ASSET_CHECK=off` 把两个门禁降级为 WARN，同时仍把计数写入 `Result.MissingAssets` / `Result.MissingStatic`。
- 下游 zhurongshuo 已在 `2b22eb2ebe` 删除该开关，本次改动不要求它再动配置。

## 6. 未做

- 未提交、未打标签、未发布；版本号未改（行为变更+配置项移除，建议下一个补丁版本 v0.12.7）。
- 未对 0.12.6 的 release 报告做正式勘误，只在本文件与示例配置里写清了成立条件（如需对外，另起一条勘误）。
- 未做「一次渲染、多语言共享静态」的更大改造——那是把 7.5% 拿回来的唯一正确路径，超出本次范围。
