# Hook 契约拆分设计（Hook Contract Split）

> **日期**：2026-07-28 · **版本目标**：v0.7.x · **方案**：契约拆分（Design 3）
> **状态**：设计已确认，待 review → writing-plans
> **前置**：承接 [ADR 0013 插件契约收敛] 记录的 Hook 运行期后续/待办

## 一、背景与动机

huan 的 `.so` 插件与主程序之间存在**两个同名但不同的 Hook 契约**：

- `internal/build.Hook`：3 方法，页面参数用 `*content.Page`（富类型，内部专属）。build pipeline 就是对它做类型断言来发现并调用钩子。
- `pkg/plugin.Hook`：3 方法，页面参数用 `interface{}`（`.so` 可导入）。三个随站点发布的 `.so` 插件实现它。

pipeline 的三处钩子调用（`internal/build/pipeline.go` 的 `runOnContentLoaded` / `runOnPageRendered` / `runOnOutputWritten`）都做 `h.(build.Hook)` 断言。而 `seo_injector` / `html_injector` / `sitemap_enhancer` 三个 `.so` 插件实现的是 `pkg/plugin.Hook`——不同接口，断言恒为 `false`。

### 确诊影响

- zhurongshuo `huan.yaml` 声明这三个插件为 `category: static`。`category: static` **不是**"编译进二进制"——`newPluginRegistry`（`cmd/huan/plugins.go`）仍通过 `loader.ScanAndLoadByCategory` **从 `.so` 加载**，只是构建启动时急加载。它们被成功注册进 registry，但因契约不匹配**钩子从不被调用**。
- 结果：**SEO meta 注入、sitemap 增强、自定义 HTML 注入在真实站点上全是死代码**，静默无效。
- 这与 ADR 0013 修复的是同一 bug 类（跨 `.so` 契约类型不一致），只是发生在 build pipeline 这条轴上。

### 关键设计洞察

1. 三个插件的实际逻辑**全部**在 `OnOutputWritten(ctx, outputDir string)`（扫描输出目录做后处理），`OnContentLoaded` / `OnPageRendered` 都是 no-op。
2. `pkg/plugin.Hook` 的两个页面方法**跨 `.so` 本质无用**：`.so` 插件拿到 `interface{}` 装箱的 `*content.Page`，既读不了也改不了字段（没有 `content.Page` 类型）。真正能跨 `.so` 的只有 `OnOutputWritten`（纯 `string` 参数，在磁盘上处理文件）。
3. `pkg/plugin.Hook` 目前在 host 侧**没有任何消费者**（pipeline 用的是 `internal/build.Hook`）——这正是 bug 成因，也意味着精简 `pkg/plugin.Hook` 对 host 零风险，只有三个插件实现它。

因此选择**契约拆分**：让 `.so` 面向的契约只暴露真正可跨界的 `OnOutputWritten`，语义诚实——页面级钩子=编译期专属，输出级钩子=`.so` 可用。

## 二、方案（Design 3：契约拆分 + 桥接）

### 第 1 块：契约拆分（`pkg/plugin`）

把 `pkg/plugin/hook.go` 的 `Hook`（3 个 `interface{}` 方法）**替换为** `PostBuildHook`：

```go
// PostBuildHook is the .so-facing build hook. Only OnOutputWritten crosses a
// .so boundary usefully: page-mutating hooks would receive an opaque
// interface{} the plugin cannot inspect (content.Page is not importable from
// pkg/ or .so modules). Rich page hooks stay in internal/build.Hook, which is
// compiled-in only.
type PostBuildHook interface {
	Plugin

	// OnOutputWritten is called after all output files are written, before the
	// build result is finalized. Receives the output directory for
	// post-processing (e.g. inject SEO meta, enhance sitemap, inject HTML).
	// Collection-not-interruption: a returned error logs a warning, does not
	// abort the build.
	OnOutputWritten(ctx context.Context, outputDir string) error
}
```

`internal/build.Hook`（3 方法、`*content.Page`）**保持不变**——编译期/host 内部钩子仍可做富页面操作。

### 第 2 块：pipeline 桥接（`internal/build/pipeline.go`）

- `runOnOutputWritten`：在既有 `h.(build.Hook)` 之外，**同时**判定 `h.(pkgplugin.PostBuildHook)` 并调用其 `OnOutputWritten`。两接口在实践中互斥（host 编译期钩子满足 `build.Hook`；`.so` 插件满足 `PostBuildHook`），逐插件用 `if bh, ok := h.(build.Hook); ok { ... } else if pbh, ok := h.(pkgplugin.PostBuildHook); ok { ... }` 保证不重复调用。保持 collection-not-interruption 语义（错误只 warn）。
- `runOnContentLoaded` / `runOnPageRendered`：保持 `build.Hook`-only，**不变**（页面钩子编译期专属）。

### 第 3 块：插件瘦身 + 能力标签

- `plugins/seo-injector/plugin.go`、`plugins/sitemap-enhancer/plugin.go`、`plugins/html-injector/plugin.go`：**删除** no-op 的 `OnContentLoaded` / `OnPageRendered` 方法，只保留 `OnOutputWritten`（现满足 `PostBuildHook`）。已核实三个插件均**无** `var _ plugin.Hook = ...` 显式断言（结构满足），故只需删方法；重编三个 `.so`。
- `cmd/huan/plugins.go` 的 `capabilityLabels`：把 `if _, ok := p.(build.Hook); ok` 补上 `|| p.(pkgplugin.PostBuildHook)` 分支，仍标 `"hook"`，否则这些 `.so` 插件在 `huan plugin list` 里会丢失 hook 标签。

### 第 4 块：回归测试（当初缺失的那条）

- `internal/build`（package `build`）新增测试：构造一个满足 `pkgplugin.PostBuildHook` 的 stub 插件（`OnOutputWritten` 里翻 flag 或写标记文件），放进 `plugin.Registry`，构造最小 pipeline 调用 `runOnOutputWritten`，断言 stub 的 `OnOutputWritten` **被调用**。
  - 这条测试直击 bug 现场：原实现下 stub 满足 `PostBuildHook` 但不满足 `build.Hook`，`runOnOutputWritten` 会跳过它 → 测试失败（红）；桥接后通过（绿）。TDD 红→绿成立。
- 同时补一个 host stub 满足 `build.Hook` 的用例，断言编译期钩子路径**不回归**。
- 交付端到端验证：`scripts/build-plugins.sh` 重编 → 部署 `~/.huan` → `huan build` → 确认输出 HTML 含注入的 SEO meta / sitemap 增强 / 自定义 HTML（输出文件 diff）。

### 第 5 块：文档 + 护栏留痕

- 新增 `docs/adr/0014-hook-contract-split.md`，记录拆分决策，并**关闭 ADR 0013 的 Hook 后续/待办**（回填一句"已由 ADR 0014 解决"）。
- 更新 `internal/plugin/contract_location_test.go` 中 `internal/build.Hook` 的白名单注释，澄清拆分后的清晰边界：`build.Hook`=编译期富页面钩子（用 `*content.Page`，不可入 `pkg/`）；`pkg/plugin.PostBuildHook`=`.so` 输出钩子。消除上轮 review 指出的"半个故事"。
- 更新 `pkg/plugin/plugin.go` 中引用旧 `pkg/plugin.Hook` 的文档注释/示例（`:7` / `:103` / `:107`，如 `Find[pkgplugin.Hook]`）为 `PostBuildHook`——已核实这些是仅有的其余 host 侧引用，且均为注释，无实际消费者。
- 完成报告 `docs/reports/completed/` + 记忆更新（daily 追加 + MEMORY.md 关键决策合并）。

## 三、落地方式（TDD）

- 测试先行：先写第 4 块的 `PostBuildHook` 调用断言（红：`PostBuildHook` 类型尚不存在 / 桥接未接），再拆契约 + 接桥接（绿）。
- 批量改动前先备份改动文件到 `.superpowers/` 下的 Go-ignored 目录（不要用模块根的 `backup/`，会污染 `go build ./...`）。
- 生效前提（Go plugin lockstep）：`pkg/plugin` 为 host+plugin 共享包，二者须同源重建（`go install ./cmd/huan` + `scripts/build-plugins.sh` + `cp release/plugins/*.so ~/.huan/`）。
- 全程 `go build ./...` + 三个受影响 `.so` 的 `go build -buildmode=plugin` + `go test ./...` 三绿。

## 四、交付物

- 代码：`pkg/plugin/hook.go`（`Hook`→`PostBuildHook`）、`internal/build/pipeline.go`（桥接 `runOnOutputWritten`）、三个插件 `plugin.go`（删 no-op 方法）、`cmd/huan/plugins.go`（`capabilityLabels`）、`internal/build` 回归测试。
- 文档：本设计 spec；ADR 0014；ADR 0013 回填；完成报告；记忆更新；护栏白名单注释更新。

## 五、非目标（Out of Scope / YAGNI）

- **不改** `internal/build.Hook`（编译期富页面钩子保持 `*content.Page`）。
- **不**为 `.so` 页面级钩子造 box/unbox 适配器（跨 `.so` 无用，YAGNI）。
- **不**给 `PostBuildHook` 增加 `OnOutputWritten` 以外的方法（未来有需要再加）。
- **不**动 daemon/JIT 的钩子路径（当前无 `.so` 钩子在 daemon 侧的需求）。

## 六、验收标准

1. `pkg/plugin.PostBuildHook`（embed Plugin + `OnOutputWritten`）取代旧 `pkg/plugin.Hook`；host 侧无残留对旧 3 方法接口的引用。
2. `runOnOutputWritten` 对满足 `PostBuildHook` 的插件调用其 `OnOutputWritten`；`runOnContentLoaded`/`runOnPageRendered` 仍仅走 `build.Hook`。
3. 三个 `.so` 插件仅实现 `OnOutputWritten`，`go build -buildmode=plugin` 通过；`capabilityLabels` 仍将其标为 `"hook"`。
4. 新回归测试：`PostBuildHook` stub 在原实现下不被调用（红）、桥接后被调用（绿）；`build.Hook` 编译期路径不回归。
5. 端到端：重编部署后 `huan build` 实际注入 SEO meta / sitemap 增强 / 自定义 HTML。
6. `go build ./...` + 三 `.so` `-buildmode=plugin` + `go test ./...` 全绿；契约位置护栏仍绿（白名单注释已更新）。
7. ADR 0014 记录决策，ADR 0013 后续/待办已关闭。
