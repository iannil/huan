# 插件契约收敛设计（Plugin Contract Convergence）

> **日期**：2026-07-28 · **版本目标**：v0.7.x · **方案**：B（收真凶 + 强护栏，YAGNI）
> **状态**：设计已确认，待 review → writing-plans

## 一、背景与动机

v0.7.0 之前的数天是一轮**响应式 bug 修复**，全部由真实运行 zhurongshuo 站点触发。其中一类 bug 有**共同根因**：`.so` 插件与主程序之间的**能力契约类型不一致**。

Go 的 plugin（`.so`）机制下，接口满足要求「具名类型同包同一」。当能力契约（如 `Deployer`）声明在 `internal/`，`.so` 插件因 Go internal 可见性规则**无法 import**，只能**自带类型副本** → 字段一样但包不同 = 不同类型 → 静默不满足接口，`Find[T]` 返回空。

这类 bug 被**一个一个反应式地修**：

- `ThemePlugin`（d6437b7 改 map-based → 后续迁 `pkg/plugin`）
- `Deployer`（f071661 迁 `pkg/deploy`）
- `Translator`（迁 `pkg/translate`）

本设计的目标是**主动收敛剩余契约 + 加防回归护栏**，杜绝这一类 bug 再次以「等它爆→再修」的方式发生。

## 二、契约现状盘点（2026-07-28）

| 能力契约 | 定义位置 | 跨 `.so` 安全 | 实现方导入 |
|---|---|---|---|
| `Plugin` / `MetadataProvider` / `SchemaProvider` / `Schema…` | `pkg/plugin`（`internal/plugin` 为别名） | ✅ 共享 | 全部 |
| `Deployer` | `pkg/deploy` ✅ | ✅ | cloudflare → `pkg/deploy` |
| `Translator` | `pkg/translate` ✅ | ✅ | qwen3 → `pkg/translate` |
| `ThemePlugin` / `Hook` / `ThemeHooks` / `ShortcodeProvider` | `pkg/plugin` ✅ | ✅ | seo/sitemap/html/zhurongshuo → `pkg/plugin` |
| **`ImageProcessor`** | **`internal/image`** ❌ | ⚠️ 靠运气 | image-pipeline **不导入任何契约包**，纯结构匹配 |
| **`EventSubscriber`** | **`internal/plugin`**（引用 `internal/daemon/eventbus`） | ❌ 结构不可能 | 仅 internal 实现，**无 `.so` 使用者** |

### 两个残留隐患（性质不同）

1. **`ImageProcessor`（真凶，需处理）**
   image-pipeline 的 `Process(outputDir, sourceDir string) error` 之所以能跨 `.so` 匹配，**纯因参数全是 `string` 内建类型**（与 `Name() string` 同理），并未导入任何 huan 契约包。一旦 `Process` 未来加 config 结构体参数或返回 `Report`，将**与 Deployer 一模一样地静默失效**。这是「下一个待爆 bug」。

2. **`EventSubscriber`（潜伏，显式推迟）**
   引用 `internal/daemon/eventbus.EventType/Event`，`.so` 插件无法 import internal 包，结构上无法满足。但**当前无任何 `.so` 插件使用它**（仅 `internal/plugin/lifecycle.go`、`plugin.go`、测试 helper 实现）。为一个无人使用的能力做 eventbus 大重构违背 YAGNI，故**显式推迟**，但由护栏登记留痕。

## 三、方案（B）

迁 1 个真凶契约（`ImageProcessor`）+ 加 2 道护栏 + 显式推迟 `EventSubscriber`。

### 第 1 块：`ImageProcessor` → `pkg/image`

镜像 `pkg/deploy` / `pkg/translate` 的已验证模式：

1. 新建 `pkg/image/types.go`：

   ```go
   package image

   import pkgplugin "github.com/iannil/huan/pkg/plugin"

   // ImageProcessor is the capability interface for build-time image plugins.
   type ImageProcessor interface {
       pkgplugin.Plugin
       Process(outputDir, sourceDir string) error
   }
   ```

2. `internal/image/processor.go` 改为**类型别名**：

   ```go
   type ImageProcessor = pkgimage.ImageProcessor
   ```

   → 所有调用点零改动：`internal/daemon/daemon.go:186`、`cmd/huan/plugins.go:119`、`internal/image/processor_test.go`、`cmd/huan/plugins_test.go`。

3. **关键**：`plugins/image-pipeline/plugin.go` 加显式契约依赖 + 编译期断言：

   ```go
   import pkgimage "github.com/iannil/huan/pkg/image"
   var _ pkgimage.ImageProcessor = (*ImagePipelinePlugin)(nil)
   ```

   把「碰巧结构匹配」升级为「**显式共享契约依赖**」。即便今天 `string` 参数可工作，未来签名演进时 host 与插件**强制同源锁步**——`plugin.Open` 会显式报版本错，而非静默 `isProcessor=false`。

### 第 2 块：能力解析诊断（主护栏，防运行期分叉）

**要抓的失败**：命令需要某能力（`huan deploy` → `Find[deploy.Deployer]`），yaml 声明的插件已加载，但因契约不一致**不满足**该能力 → `Find[T]` 返回空 → 现状是报一句离根因很远的 `no deployer plugin available`（无法区分「根本没配插件」和「配了但契约分叉」）。

**约束**：`config.PluginConfig` 只有 `Category`（static/dynamic/mixed）+ inline `Config`，**没有**「能力角色」字段。命令**自己知道**需要哪个能力（deploy 需 Deployer、translate 需 Translator、build/daemon 需 ImageProcessor），yaml 只决定装载哪些插件。因此护栏挂在**能力查找点**，而非配置声明。

**做法**：新增一个共享诊断 helper，在各能力消费点 `Find[T]` 返回空时调用——

- deploy：`cmd/huan/deploy.go:73/83/170` 的 `Find[deploy.Deployer]`
- translate：`cmd/huan/translate_cmd.go` 的 `Find[translate.Translator]`
- image：`internal/daemon/daemon.go:186` / `cmd/huan/plugins.go:119` 的 `ImageProcessor` 断言点

helper 逻辑：`Find[T]` 为空**且** registry 已加载 ≥1 个插件时，枚举已加载插件名，产出**指向根因 + 修复动作**的错误，替代裸的 `no XXX available`：

> `no deploy.Deployer among 1 loaded plugin(s): [cloudflare]. A loaded plugin failed to satisfy the capability — likely a .so/host contract mismatch. Rebuild the plugin against the current pkg/deploy: scripts/build-plugins.sh && cp release/plugins/*.so $HUAN_HOME`

（registry 为空时维持"未配置任何插件"的原有提示，不误导。）

**价值**：把「静默/含糊 no-match」变「响亮、指向根因」。此护栏若早存在，上周 deploy/translate 系列 bug 会在**第一时间被定位**；未来 image 若因签名演进破裂也会在此被抓。

**可选增强**（YAGNI，实施时评估）：诊断里对每个已加载插件跑一遍 `detectCapabilityFn`，打印它**实际**满足的能力标签，让"配了 cloudflare 但它只满足 base Plugin"一目了然。

### 第 3 块：契约位置测试（廉价保险，防设计期漂移）

一个 host 侧单元测试，AST 扫描源码：**凡是 embed `plugin.Plugin`（或 `pkg/plugin.Plugin`）且新增了方法的能力接口，必须声明在 `pkg/` 目录下**。谁将来在 `internal/` 新增能力契约 → 测试红。

- `EventSubscriber` 作为**已知例外**登记进白名单常量，注释写明推迟理由：

  > `EventSubscriber: references internal/daemon/eventbus; no .so implementer today; deferred (YAGNI). Migrate to pkg/ when the first .so plugin needs event subscription.`

  → 推迟决策**留痕在代码里**，评审可见，不会被遗忘。

## 四、落地方式（TDD）

测试先行，红→绿：

1. **`plugins/image-pipeline`**：`var _ pkgimage.ImageProcessor = (*ImagePipelinePlugin)(nil)` 编译期断言（迁移后即绿）。
2. **能力解析诊断**：注册一个只满足 base `Plugin`、不满足 `deploy.Deployer` 的假插件，断言诊断 helper 在 `Find[Deployer]` 为空时产出**枚举已加载插件 + 指向 contract mismatch** 的错误；registry 为空时维持原"未配置"提示。
3. **契约位置扫描**：AST 测试断言所有能力契约在 `pkg/`；`EventSubscriber` 在白名单内；若临时把某契约移回 `internal/` 则测试红。

其他约定：

- 批量改动前先**备份改动文件到 `/backup`** 相对路径；错误数异常上升立即回滚。
- 全程 `go build ./...`、`go build -buildmode=plugin`（image-pipeline）、`go test ./...` 三绿。
- 生效前提（Go plugin lockstep）：`pkg/image` 为 host+plugin 共享包，二者须同源重建（`go install ./cmd/huan` + `scripts/build-plugins.sh` + 部署 `~/.huan`）。

## 五、交付物

- 代码：`pkg/image/types.go`（新）、`internal/image/processor.go`（改别名）、`plugins/image-pipeline/plugin.go`（加断言）、`cmd/huan` 加载期自检、契约位置测试。
- 文档：本设计 → `docs/superpowers/specs/`；完成后报告 → `docs/reports/completed/`；ADR 记录方案 B + `EventSubscriber` 推迟决策；更新 `memory/`（daily + MEMORY.md 关键决策）。

## 六、非目标（Out of Scope / YAGNI）

- **不迁** `EventSubscriber` / eventbus 类型到 `pkg/`（无 `.so` 使用者；由护栏登记推迟）。
- **不引入** gRPC 跨语言插件路径（另有 backlog）。
- **不改动** 现有 deploy/translate/theme 契约（已在 `pkg/`）。
- **不做** 全文搜索、图片 WebP/AVIF、插件市场 install 等其他 backlog 项。

## 七、验收标准

1. 所有能力契约（除白名单 `EventSubscriber`）均声明于 `pkg/`；契约位置测试绿。
2. image-pipeline **显式导入** `pkg/image` 并通过编译期断言；`huan build` 图片管线正常。
3. 已加载但契约不满足的插件，在**能力查找点**（`Find[T]` 为空且 registry 非空）报枚举 + 指向根因的诊断错误，替代裸的 `no XXX available`。
4. `go build ./...` + `go build -buildmode=plugin`（image-pipeline）+ `go test ./...` 全绿。
5. `EventSubscriber` 推迟决策在代码（白名单注释）与 ADR 中均有留痕。
