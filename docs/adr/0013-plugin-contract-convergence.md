# ADR 0013: 插件能力契约主动收敛（方案 B）

- **状态**: Accepted
- **日期**: 2026-07-28
- **决策者**: 用户 + Claude
- **依赖**: [ADR 0003](0003-unified-plugin-system.md)（统一插件系统）、[ADR 0008](0008-translator-capability-qwen3-plugin.md)（Translator capability）、[ADR 0012](0012-theme-plugin-system.md)（主题插件系统）
- **被引用**: [插件契约收敛设计](../superpowers/specs/2026-07-28-plugin-contract-convergence-design.md)、[实现计划](../superpowers/plans/2026-07-28-plugin-contract-convergence.md)

## 背景

v0.7.0 之前的数天是一轮**响应式 bug 修复**，全部由真实运行 zhurongshuo 站点触发。其中一类 bug 有**共同根因**：`.so` 插件与主程序之间的**能力契约类型不一致**。

Go 的 plugin（`.so`）机制下，接口满足要求「具名类型同包同一」。当能力契约（如 `Deployer`）声明在 `internal/`，`.so` 插件因 Go internal 可见性规则**无法 import**，只能**自带类型副本** → 字段一样但包不同 = 不同类型 = 静默不满足接口 → `plugin.Find[T]` 返回空。命令层随后报一句离根因很远的 `no X available`，无法区分「根本没配插件」与「配了但契约分叉」。

这类 bug 被**一个一个反应式地修**：

- `ThemePlugin`（d6437b7：map-based → 后续迁 `pkg/plugin`）
- `Deployer`（f071661：迁 `pkg/deploy`）
- `Translator`（f071661 收尾：迁 `pkg/translate`）

修完前三个后，仅剩一个残留契约 `ImageProcessor` 仍在 `internal/image`。它当前之所以能跨 `.so` 匹配，**纯因参数全是 `string` 内建类型**（`Process(outputDir, sourceDir string) error`），并未导入任何 huan 契约包——这是「下一个待爆 bug」：一旦 `Process` 未来加 config 结构体参数或返回 `Report`，将**与 Deployer 一模一样地静默失效**。

本 ADR 的目标是**主动收敛剩余契约 + 加防回归护栏**，杜绝这一类 bug 再以「等它爆 → 再修」的方式发生。

## 决策

采用**方案 B**（收真凶 + 强护栏，YAGNI）：迁 1 个真凶契约 + 加 2 道护栏 + 显式推迟无用者。

### 1. `ImageProcessor` → `pkg/image`（别名回填）

镜像已验证的 `pkg/deploy` / `pkg/translate` 模式：

- 新建 `pkg/image/types.go`：`ImageProcessor` 接口，嵌入 `pkg/plugin.Plugin` + `Process(outputDir, sourceDir string) error`。契约声明在 `pkg/`，`.so` 插件可导入**与 host 完全同一**的类型。
- `internal/image/processor.go` 改为**类型别名** `type ImageProcessor = pkgimage.ImageProcessor` → 所有既有调用点（`internal/daemon/daemon.go`、`cmd/huan/plugins.go`、`cmd/huan/build_image.go`、`internal/image/processor_test.go`）**零改动**。
- `plugins/image-pipeline/plugin.go` 加显式契约依赖 + 编译期断言 `var _ pkgimage.ImageProcessor = (*ImagePipelinePlugin)(nil)`。把「碰巧结构匹配」升级为「**显式共享契约依赖**」——未来签名演进时 host 与插件强制同源锁步，`plugin.Open` 显式报版本错，而非静默 `isProcessor=false`。

### 2. 能力解析诊断 `diagnoseCapabilityGap`（主护栏，防运行期分叉）

新增共享 helper `diagnoseCapabilityGap(registry *plugin.Registry, capability string) string`（`cmd/huan/plugins.go`），在各能力消费点 `Find[T]` 返回空时调用：

- registry **为空** → 返回 `""`，维持「未配置任何插件」的原有提示，不误导。
- registry **非空但无插件满足能力** → 枚举每个已加载插件及其**实际满足的能力标签**（复用 `capabilityLabels`），产出指向根因 + 修复动作的诊断，替代裸的 `no X available`：

  > `N plugin(s) loaded, none satisfy deploy.Deployer: cloudflare[none]; a plugin that should provide it but shows [none] indicates a .so/host contract mismatch — rebuild plugins against current pkg/ contracts (scripts/build-plugins.sh && cp release/plugins/*.so "$HUAN_HOME")`

接入点：`deploy`（`Find[deploy.Deployer]` 落空 → 硬错）、`translate`（`Find[translate.Translator]` 落空 → 打印提示后 graceful skip）。`image` 由第 1 块的**编译期断言**在更早的插件构建期兜底，故运行期不再挂诊断，避免 build 时对无关插件误报。

**价值**：把「静默/含糊 no-match」变「响亮、指向根因」。此护栏若早存在，上周 deploy/translate 系列 bug 会在第一时间被定位。

### 3. 契约位置测试 `TestCapabilityContractsLiveInPkg`（廉价保险，防设计期漂移）

host 侧单元测试（`internal/plugin/contract_location_test.go`），用 `go/ast` 扫描 `../../internal`、`../../pkg`、`../../cmd`：**凡是 embed `plugin.Plugin`（或 `pkg/plugin.Plugin`）且新增了方法的能力接口，必须声明在 `pkg/` 目录下**。谁将来在 `internal/` 新增能力契约 → 测试红。已通过临时移除白名单项验证扫描器真实生效（`GRPCPlugin` 会被如期标记）。

## 白名单例外（带理由留痕，共 3 项）

`capabilityContractWhitelist` 常量登记**允许在 `pkg/` 之外**的能力接口，每项写明理由。设计原意为 2 项（`EventSubscriber` / `GRPCPlugin`），实施时 AST 扫描**新发现第 3 项** `internal/build.Hook`，按同类先例登记：

| 契约 | 位置 | 理由 |
|---|---|---|
| `EventSubscriber` | `internal/plugin`（引用 `internal/daemon/eventbus`） | 引用 internal eventbus 类型，且**当前无任何 `.so` 使用者**；迁 `pkg/`（连同 eventbus 类型）违背 YAGNI，**显式推迟**。注：它**不** embed `Plugin`，结构扫描本就不会命中——该白名单项纯作推迟决策的文档留痕。首个需要事件订阅的 `.so` 插件出现时再迁。 |
| `GRPCPlugin` | `internal/plugin/grpc_stub.go` | 走 **gRPC 跨进程**传输，接口满足从不跨越 Go `.so` 边界，类型同一性隐患**天然不适用**。保留/未实现。 |
| `Hook` | `internal/build/hook.go` | embed `Plugin` 但方法签名用 `[]*content.Page`（内部类型，**无法搬进 `pkg/`**）；其 **.so 安全对应物已存在**为 `pkg/plugin.Hook`（用 `[]any` 传页引用），`.so` 插件实现的是后者。`internal/build.Hook` 仅供**宿主内**使用，不跨 `.so` 边界，故不受类型同一性约束——与 `EventSubscriber` 属同类例外。 |

## 后果

- 所有 embed-`Plugin` 且跨 `.so` 边界的能力契约现均在 `pkg/`（Deployer / Translator / ImageProcessor / ThemePlugin）；`internal/` 仅用类型别名回填。
- 新增同类契约若落 `internal/` → `go test` 红（CI 拦截），杜绝设计期漂移。
- 能力缺失从「命令期含糊报错」提前到「带根因的诊断」；签名演进破裂从「运行期静默」提前到「插件构建期编译错」。
- 生效前提（Go plugin lockstep）：`pkg/image` 为 host+plugin **共享**包，二者须同源重建（`go install ./cmd/huan` + `scripts/build-plugins.sh` + `cp release/plugins/*.so ~/.huan`）。

## 后续 / 待办（本 epic 未处理，留痕）

- **`Hook` 运行期接线缺口（同类 bug，本 epic 范围外）**：`internal/build/pipeline.go:75` 把 hooks **类型断言**为 `internal/build.Hook`（`[]*content.Page`），而已发布的 `.so` 插件（seo-injector / sitemap-enhancer / html-injector）实现的是 `pkg/plugin.Hook`（`[]any`）。二者是**结构不同的接口**，宿主侧**无适配器桥接**，故这些 `.so` 插件的 hook **很可能从未被构建管线调用**。这是与本 epic 收敛的 `ImageProcessor` **同一 bug 类**（契约类型不一致），但方向是**运行期接线**而非契约位置，故不在本 epic 范围（本 epic 只收敛 ImageProcessor + 加护栏）。建议作为**独立后续计划**处理：要么加宿主侧桥接 adapter（`internal/build.Hook` ↔ `pkg/plugin.Hook`），要么统一两个 Hook 接口。届时可从白名单移除 `Hook`（白名单注释「Keep this list shrinking」已隐含此意图）。

## 非目标（Out of Scope / YAGNI）

- **不迁** `EventSubscriber` / eventbus 类型到 `pkg/`（无 `.so` 使用者；由护栏登记推迟）。
- **不引入** gRPC 跨语言插件路径（另有 backlog）。
- **不改动** 现有 deploy / translate / theme 契约（已在 `pkg/`）。
- **不修** 上述 `Hook` 运行期接线缺口（留痕为独立后续计划）。

## 交付物

- 代码：`pkg/image/types.go`（新）、`pkg/image/types_test.go`（新）、`internal/image/processor.go`（改别名）、`plugins/image-pipeline/plugin.go`（加断言）、`cmd/huan/{plugins,deploy,translate_cmd}.go`（诊断接入）、`cmd/huan/plugins_test.go`（诊断测试）、`internal/plugin/contract_location_test.go`（契约位置护栏）。
- 文档：本 ADR（0013）+ [完成报告](../reports/completed/2026-07-28-plugin-contract-convergence.md)；更新 `memory/`（daily + MEMORY.md 关键决策）。

## 验收标准（全部满足）

1. 所有能力契约（除白名单 3 项）均声明于 `pkg/`；契约位置测试绿。
2. image-pipeline **显式导入** `pkg/image` 并通过编译期断言。
3. 已加载但契约不满足的插件在**能力查找点**报枚举 + 指向根因的诊断错误。
4. `go build ./...` + image-pipeline `go build -buildmode=plugin` + `go test ./...` 全绿。
5. `EventSubscriber` / `GRPCPlugin` / `Hook` 推迟决策在代码（白名单注释）与本 ADR 中均有留痕；`Hook` 运行期接线缺口在「后续 / 待办」中登记。
