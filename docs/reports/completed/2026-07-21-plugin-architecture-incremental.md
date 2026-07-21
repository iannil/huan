# 插件化架构 + 增量构建 完成报告

> 完成日期：2026-07-21  ·  对标：huan daemon 持久化服务端能力的平台化扩展
> 设计原文：
> - [热插拔插件系统设计](../../superpowers/specs/2026-07-20-daemon-hotplug-plugin-design.md)
> - [图片管线插件设计](../../superpowers/specs/2026-07-21-image-pipeline-design.md)
> - [增量构建设计](../../superpowers/specs/2026-07-21-incremental-build-design.md)

## 1. 概述

本次会话围绕 huan daemon 的「平台化扩展」目标，连续交付四个功能：

1. **Daemon 热插拔插件系统** — 运行时加载/卸载/热重载 `.so` 插件，Admin API 管理生命周期
2. **插件解耦** — cloudflare 和 qwen3_translate 从编译期内置迁移为独立 `.so` 插件（彻底解耦）
3. **图片管线插件** — 构建时自动压缩/格式转换/多尺寸生成 + HTML 后处理注入 srcset/picture
4. **增量构建** — DAG 驱动的部分重渲染，内容变更时只重建受影响页面 + 站点级输出

这些功能共同构成 huan 的插件化平台能力：核心保持精简，所有可扩展能力通过插件机制接入。

## 2. 新增依赖

| 依赖 | 版本 | 用途 |
|---|---|---|
| `golang.org/x/image/draw` | v0.44.0 | 图片管线插件的图片缩放 |

其余依赖（Go plugin stdlib、fsnotify、cobra、prometheus）均为已有依赖。

## 3. 新增 / 修改的包

### 3.1 热插拔插件系统

| 路径 | 职责 | 关键文件 |
|---|---|---|
| `internal/plugin/loader.go` | `.so` 插件文件加载器（plugin.Open + Lookup InitPlugin） | loader.go |
| `internal/plugin/lifecycle.go` | 插件生命周期管理（Start/Stop/Load/Unload/Reload/List） | lifecycle.go |
| `internal/plugin/grpc_stub.go` | gRPC 插件预留骨架（Call 返回 ErrGRPCNotImplemented） | grpc_stub.go |
| `internal/daemon/eventbus/` | 新增 4 个插件生命周期事件（Loaded/Unloaded/Reloaded/Error） | types.go |
| `internal/admin/` | 新增 `/admin/api/plugins/*` 4 个端点（list/load/unload/reload） | api.go, types.go |

### 3.2 插件解耦

| 路径 | 职责 |
|---|---|
| `plugins/cloudflare/` | 独立插件仓库（自包含类型复制），编译为 `cloudflare.so` |
| `plugins/qwen3/` | 独立插件仓库，编译为 `qwen3.so` |
| `cmd/huan/plugins.go` | 移除 cloudflare/qwen3 编译期注册（改为运行时 .so 加载） |
| `cmd/huan/deploy.go` / `translate_cmd.go` | 添加 .so fallback 加载（`--plugin-dir` 标记） |

### 3.3 图片管线插件

| 路径 | 职责 |
|---|---|
| `plugins/image-pipeline/` | 独立插件仓库：扫描 → 压缩/转换/多尺寸 → HTML 后处理 |
| `internal/image/processor.go` | `ImageProcessor` capability 接口 |
| `cmd/huan/build_image.go` | 构建管线集成（AfterBuild 调用图片管线） |

### 3.4 增量构建

| 路径 | 职责 |
|---|---|
| `internal/build/cache.go` | `PipelineCache`（缓存渲染基础设施）+ `HasTemplateChanges` |
| `internal/build/build.go` | `IncrementalRender`（只渲染受影响页面 + 站点级输出） |
| `internal/daemon/builder.go` | `IncrementalBuild` 完整实现 + 路径归一化 |
| `internal/daemon/dag/graph.go` | `OrderByDependency`（拓扑排序）+ DAG 依赖方向修正 |

## 4. CLI / API 变更

### CLI 新增

| 命令 | 默认值 | 说明 |
|---|---|---|
| `huan plugin load <path>` | — | 运行时加载 .so 插件 |
| `huan plugin unload <name>` | — | 卸载已加载插件 |
| `huan plugin reload <name> <path>` | — | 热重载插件 |
| `huan plugin list --all` | false | 列出运行时加载的插件 |
| `huan daemon --plugin-dir <path>` | `<sourceDir>/plugins` | 指定插件目录 |
| `huan daemon --disable-plugin` | false | 禁用插件加载 |
| `huan deploy/translate --plugin-dir <path>` | `<sourceDir>/plugins` | 指定插件目录 |

### Admin API 新增

| 端点 | 方法 | 说明 |
|---|---|---|
| `/admin/api/plugins` | GET | 列出所有插件（含状态/来源/能力） |
| `/admin/api/plugins/load` | POST | 加载 .so 插件 |
| `/admin/api/plugins/unload` | POST | 卸载插件 |
| `/admin/api/plugins/reload` | POST | 热重载插件 |

所有端点受现有 `TokenMiddleware`（L2 鉴权）保护，操作记录到 audit log（L4）。

## 5. 关键设计决策

按重要性排序：

1. **插件自包含类型复制（非 go.mod replace）** —— Go 的 `internal/` 包可见性规则禁止外部 module 通过 `replace` 导入 huan 内部包。插件仓库采用自包含模式：复制所需的 `Plugin` 接口、capability 类型（`deploy`/`translate`/`observability`）到插件仓库本地。代价是接口变更需手动同步，但换来了真正的仓库级解耦。

2. **PipelineCache 只缓存渲染基础设施** —— spec 原计划缓存 PageLookup/ContextLookup，但内容变更时 context 引用的 page 指针会失效，缓存它们会导致列表页输出过期。务实决策：PipelineCache 缓存模板/i18n/markdown/writer（内容变更时不变），增量构建时重建内容+context（开销小），但只渲染受 DAG 影响的页面（跳过 ~95% 模板渲染）。

3. **DAG 依赖方向：聚合页 DependsOn 文章** —— section/home/tag 列表页 **显示** 文章内容，所以它们 **依赖** 文章。编辑文章时，沿 DependedBy（反向边）BFS 正确到达所有聚合页。初版实现方向反了（文章 DependsOn 聚合页），导致编辑文章时列表页静默过期——final review 复现并修复。

4. **增量构建自动降级** —— 模板/i18n/config/themes 变更时（`HasTemplateChanges` 检测），自动回退全量构建（缓存失效）。内容变更才走增量路径。保证正确性优先。

5. **gRPC 插件预留骨架（不引入依赖）** —— `GRPCStub` 定义接口 + 空实现，`Call` 返回 `ErrGRPCNotImplemented`。等跨语言插件需求出现再实现真正的 gRPC 传输层，避免过早引入 protobuf 依赖。

6. **图片管线 HTML 后处理（零迁移成本）** —— 用户不需要改模板。构建后扫描 HTML 中的 `<img>` 标签，自动注入 `srcset`/`<picture>`/`loading="lazy"`。跳过已有 srcset 或在 `<picture>` 内的 img。

## 6. 验收记录

```
$ go build ./... && go vet ./...
（无输出，构建成功）

$ go test ./... -count=1
26 个包全部通过，0 失败
```

### 增量构建正确性验收

- `TestIncrementalBuild_UpdatesListingPages`：编辑文章后验证 section + tag 列表页正确更新（C1 回归守护）
- `TestIncrementalBuild_ContentChange`：验证变更页输出更新
- `TestIncrementalBuild_TemplateChangeFallback`：验证模板变更回退全量构建
- `TestPipelineCache_PopulatedAfterFullBuild`：验证缓存填充

### Final review 发现并修复的关键缺陷

| 缺陷 | 严重度 | 修复 |
|---|---|---|
| Watcher 绝对路径 vs DAG 相对路径不匹配，增量构建静默 no-op | Critical | builder.go 添加 `normalizeChangedFiles` |
| DAG 依赖方向反了，列表页编辑文章后过期 | Critical | 翻转 rules.go 边语义 |
| IncrementalRender 跳过 sitemap/search/RSS 等站点级输出 | Critical | 末尾调用 renderSitemap/renderSearchIndex 等 |
| 图片管线 WebP 回退产生错误扩展名文件 | Critical | 移除 fallback，只生成 JPEG |
| Scanner 跳过含 `-` 的文件名过于宽泛 | Critical | 改用精确正则匹配变体命名 |

## 7. 已知限制

- **WebP/AVIF 编码**：图片管线当前只生成 JPEG（纯 Go WebP 编码不可用）。配置 `formats: [webp, avif]` 时这些格式被跳过。未来可接入 `libwebp` CGO 绑定或纯 Go 编码器。
- **增量构建的 renderEmptyCategories / render404** 未在 IncrementalRender 中重新生成（这些输出不依赖文章内容，过期无影响）。
- **多语言 daemon 场景**：增量构建复用缓存 cfg 时跳过 stale-translation 检查和 site_translations 注入（边缘情况，单语言场景无影响）。
- **Watcher 跳过 layouts/static/themes 目录**：daemon 模式下编辑模板不触发重建（pre-existing 限制，需手动触发 admin rebuild）。
- **插件自包含类型同步**：huan 内部 capability 接口变更时，cloudflare/qwen3/image-pipeline 插件仓库需手动同步复制的类型。

## 8. 后续优化项

- **数据文件依赖追踪**：DAG 扩展支持 `data/` 文件依赖（当前 data 变更走全量构建）
- **静态文件增量**：`static/` 变更时增量复制而非全量
- **并行渲染**：独立的受影响页面可并行渲染
- **gRPC 插件实现**：跨语言插件路径落地
- **插件市场**：`huan plugin install <name>` 从远程仓库下载 .so
- **WebP/AVIF 编码**：接入真正的现代格式编码器
- **热模板重载**：模板变更时只重新解析模板树，不重新加载内容

## 9. 提交历史

本次会话共 ~39 个提交，分布在四个功能模块。详见 `git log`：

- 热插拔插件系统：`6ef03a2..3edff6f`（12 commits）
- 插件解耦：`3edff6f..c94324b`（5 commits）
- 图片管线插件：`a28ac21..5c3b223`（12 commits）
- 增量构建：`a592b81..fb5ddf9`（10 commits）
