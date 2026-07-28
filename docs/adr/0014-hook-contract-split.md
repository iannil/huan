# ADR 0014: Hook 契约拆分（`.so` 输出钩子桥接）

- **状态**: Accepted
- **日期**: 2026-07-28（完成于 2026-07-29）
- **决策者**: 用户 + Claude
- **依赖**: [ADR 0003](0003-unified-plugin-system.md)（统一插件系统）、[ADR 0013](0013-plugin-contract-convergence.md)（插件契约主动收敛）
- **被引用**: [Hook 契约拆分设计](../superpowers/specs/2026-07-28-hook-contract-split-design.md)

## 背景

[ADR 0013](0013-plugin-contract-convergence.md) 在「后续 / 待办」中登记了一个**同类 bug（`.so`/host 契约类型不一致）在 build pipeline 轴上的实例**，本 ADR 处理它。

huan 的 `.so` 插件与主程序之间存在**两个同名但不同的 Hook 契约**：

- `internal/build.Hook`：3 方法，页面参数用 `*content.Page`（富类型，内部专属）。build pipeline 就是对它做类型断言来发现并调用钩子。
- `pkg/plugin.Hook`：3 方法，页面参数用 `interface{}`（`.so` 可导入）。三个随站点发布的 `.so` 插件（seo-injector / sitemap-enhancer / html-injector）实现它。

`internal/build/pipeline.go` 的三处钩子调用（`runOnContentLoaded` / `runOnPageRendered` / `runOnOutputWritten`）都做 `h.(build.Hook)` 断言。而三个 `.so` 插件实现的是 `pkg/plugin.Hook`——不同接口，断言恒为 `false`，宿主侧**无桥接 adapter**。

### 确诊影响

- zhurongshuo `huan.yaml` 声明这三个插件为 `category: static`，但 `category: static` **不是**「编译进二进制」——`newPluginRegistry`（`cmd/huan/plugins.go`）仍通过 `loader.ScanAndLoadByCategory` **从 `.so` 加载**，只是构建启动时急加载。它们被成功注册进 registry，但因契约不匹配**钩子从不被调用**。
- 结果：**SEO meta 注入、sitemap 增强、自定义 HTML 注入在真实站点上全是死代码**，静默无效。
- 这与 ADR 0013 修复的是同一 bug 类（跨 `.so` 契约类型不一致），只是发生在 build pipeline 这条轴上，方向为**运行期接线**而非契约位置。

### 关键设计洞察

1. 三个插件的实际逻辑**全部**在 `OnOutputWritten(ctx, outputDir string)`（扫描输出目录做后处理），`OnContentLoaded` / `OnPageRendered` 都是 no-op。
2. `pkg/plugin.Hook` 的两个页面方法**跨 `.so` 本质无用**：`.so` 插件拿到 `interface{}` 装箱的 `*content.Page`，既读不了也改不了字段（没有 `content.Page` 类型）。真正能跨 `.so` 的只有 `OnOutputWritten`（纯 `string` 参数，在磁盘上处理文件）。
3. `pkg/plugin.Hook` 在 host 侧**没有任何消费者**（pipeline 用的是 `internal/build.Hook`）——这正是 bug 成因，也意味着精简它对 host 零风险。

## 决策

采用**契约拆分**（设计 spec 的 Design 3）：让 `.so` 面向的契约只暴露真正可跨界的 `OnOutputWritten`，语义诚实——页面级钩子=编译期专属，输出级钩子=`.so` 可用。

### 1. 契约拆分（`pkg/plugin`）

把 `pkg/plugin/hook.go` 的 `Hook`（3 个 `interface{}` 方法）**替换为** `PostBuildHook`：embed `Plugin` + 仅 `OnOutputWritten(ctx context.Context, outputDir string) error`，doc 注释说明只有 `OnOutputWritten` 能有意义地跨 `.so` 边界（页面钩子会收到插件无法检视的不透明 `interface{}`，`content.Page` 无法从 `pkg/` 或 `.so` 模块导入；富页面钩子留在 `internal/build.Hook`，仅编译进二进制）。

`internal/build.Hook`（3 方法、`*content.Page`）**保持不变**——编译期/host 内部钩子仍可做富页面操作。`internal/plugin` 加类型别名 `type PostBuildHook = pkgplugin.PostBuildHook`。

### 2. pipeline 桥接（`internal/build/pipeline.go`）

- `runOnOutputWritten`：在既有 `h.(build.Hook)` 之外，**同时**判定 `h.(pkgplugin.PostBuildHook)` 并调用其 `OnOutputWritten`。逐插件用 `if bh, ok := h.(build.Hook); ok { ... } else if pbh, ok := h.(pkgplugin.PostBuildHook); ok { ... }` 保证不重复调用（`build.Hook` 优先）。保持 collection-not-interruption 语义（错误只 warn，不 abort build）。
- `runOnContentLoaded` / `runOnPageRendered`：保持 `build.Hook`-only，**不变**（页面钩子编译期专属）。

### 3. 插件瘦身 + 能力标签

- `plugins/{seo-injector,sitemap-enhancer,html-injector}/plugin.go`：**删除** no-op 的 `OnContentLoaded` / `OnPageRendered` 方法，只保留 `OnOutputWritten`（现满足 `PostBuildHook`）。重编三个 `.so`。
- `cmd/huan/plugins.go` 的 `capabilityLabels`：把 `if _, ok := p.(build.Hook); ok` 补上 `else if p.(pkgplugin.PostBuildHook)` 分支，仍标 `"hook"`，否则这些 `.so` 插件在 `huan plugin list` 里会丢失 hook 标签。

### 4. 回归测试（当初缺失的那条）

`internal/build/hook_test.go` 新增两条测试：

- `TestRunOnOutputWritten_InvokesPostBuildHook`：构造满足 `pkgplugin.PostBuildHook` 的 stub 插件，放进 registry，调用 `runOnOutputWritten`，断言其 `OnOutputWritten` **被调用**。原实现下 stub 满足 `PostBuildHook` 但不满足 `build.Hook`，会被跳过 → 测试**红**；桥接后**绿**。TDD 红→绿成立，直击 bug 现场。
- `TestRunOnOutputWritten_InvokesBuildHook`：host stub 满足 `build.Hook`，断言编译期钩子路径**不回归**。

## 后果

- **`.so` 钩子插件真实生效**：SEO meta 注入、sitemap 增强、自定义 HTML 注入不再是死代码。
- **契约语义诚实**：页面级钩子=编译期专属（`internal/build.Hook`，`*content.Page`）；输出级钩子=`.so` 可用（`pkg/plugin.PostBuildHook`，纯 `string`）。不再存在「跨 `.so` 无用的页面方法」。
- **护栏白名单注释已澄清**：`internal/plugin/contract_location_test.go` 的 `"Hook"` 白名单项注释更新，说明拆分后的清晰边界（`build.Hook`=编译期富页面钩子，其 `.so` 对应物为 `pkg/plugin.PostBuildHook`），消除上轮 review 指出的「半个故事」。
- **生效前提（Go plugin lockstep）**：`pkg/plugin` 为 host+plugin **共享**包，二者须同源重建（`go install ./cmd/huan` + `scripts/build-plugins.sh` + `cp release/plugins/*.so ~/.huan/`）。

## 非目标（Out of Scope / YAGNI）

- **不改** `internal/build.Hook`（编译期富页面钩子保持 `*content.Page`）。
- **不**为 `.so` 页面级钩子造 box/unbox 适配器（跨 `.so` 无用，YAGNI）。
- **不**给 `PostBuildHook` 增加 `OnOutputWritten` 以外的方法（未来有需要再加）。
- **不**动 daemon/JIT 的钩子路径（当前无 `.so` 钩子在 daemon 侧的需求）。

## 引用（提交）

| 任务 | Commit | 内容 |
|---|---|---|
| Task 1 | `9f08581` | `refactor(plugin)`: 拆 `pkg/plugin.Hook` → `PostBuildHook`（+ internal 别名 + doc 注释 + 护栏白名单注释） |
| Task 2 | `c9f7c60` | `fix(build)`: `runOnOutputWritten` 桥接 `PostBuildHook` + 回归测试 + `capabilityLabels` |
| Task 3 | `f715b14` | `refactor(plugins)`: 三插件删 no-op 页面钩子、仅实现 `PostBuildHook`，重编 `.so` |

## 验收标准（全部满足）

1. `pkg/plugin.PostBuildHook`（embed `Plugin` + `OnOutputWritten`）取代旧 `pkg/plugin.Hook`；host 侧无残留对旧 3 方法接口的引用。
2. `runOnOutputWritten` 对满足 `PostBuildHook` 的插件调用其 `OnOutputWritten`；`runOnContentLoaded` / `runOnPageRendered` 仍仅走 `build.Hook`。
3. 三个 `.so` 插件仅实现 `OnOutputWritten`，`go build -buildmode=plugin` 通过；`capabilityLabels` 仍将其标为 `"hook"`。
4. 回归测试：`PostBuildHook` stub 在原实现下不被调用（红）、桥接后被调用（绿）；`build.Hook` 编译期路径不回归。
5. `go build ./...` + 三 `.so` `-buildmode=plugin` + `go test ./...` 全绿；契约位置护栏仍绿（白名单注释已更新）。
6. 本 ADR 记录决策，ADR 0013 后续/待办已关闭。
