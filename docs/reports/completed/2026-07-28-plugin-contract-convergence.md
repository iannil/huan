# 插件能力契约主动收敛（方案 B）完成报告

> 完成日期：2026-07-28  ·  对标：主动收敛「跨 `.so` 契约类型不一致」bug 类，防回归
> 计划与设计原文：[设计 spec](../../superpowers/specs/2026-07-28-plugin-contract-convergence-design.md) · [实现计划](../../superpowers/plans/2026-07-28-plugin-contract-convergence.md) · [ADR 0013](../../adr/0013-plugin-contract-convergence.md)

## 1. 概述

延续 2026-07-28 一轮响应式修复（ThemePlugin / Deployer / Translator 各修一次，均因能力契约声明在 `internal/` → `.so` 插件自带类型副本 → 静默不满足接口）。本次**主动**收敛最后一个残留契约 `ImageProcessor`（迁 `pkg/image`），并加两道护栏（运行期能力解析诊断 + 设计期契约位置 AST 测试），杜绝该 bug 类再以「等它爆 → 再修」的方式复发。对项目意义：把「静默 no-match」升级为「带根因诊断 + 编译期锁步 + CI 拦截」。

## 2. 新增依赖

| 依赖 | 版本 | 用途 |
|---|---|---|
| 无 | — | 仅用标准库 `go/ast` / `go/parser` / `go/token`（契约位置测试） |

## 3. 新增 / 修改的包

| 路径 | 职责 | 关键文件 |
|---|---|---|
| `pkg/image`（新） | `ImageProcessor` 能力契约（host+plugin 共享） | `types.go`、`types_test.go` |
| `internal/image`（改） | 类型别名回填，既有调用点零改动 | `processor.go` |
| `plugins/image-pipeline`（改） | 显式导入 `pkg/image` + 编译期断言 | `plugin.go` |
| `cmd/huan`（改） | `diagnoseCapabilityGap` 诊断 + 接入 deploy/translate | `plugins.go`、`deploy.go`、`translate_cmd.go`、`plugins_test.go` |
| `internal/plugin`（改） | 契约位置 AST 护栏 + 白名单 | `contract_location_test.go` |

## 4. CLI / API 变更

| Flag / 接口 | 默认值 | 说明 |
|---|---|---|
| `huan deploy`（错误信息） | — | `Find[Deployer]` 落空且已加载插件时，由裸 `no deployer plugin available` 变为枚举已加载插件 + 指向 contract mismatch 的诊断 |
| `huan translate`（skip 提示） | — | `Find[Translator]` 落空且已加载插件时，同上（graceful skip 语义不变） |

无对外 API/schema 变更；`ImageProcessor` 类型别名保证所有调用点签名不变。

## 5. 关键设计决策

1. **契约迁 `pkg/` + `internal/` 类型别名回填** —— 镜像已验证的 `pkg/deploy` / `pkg/translate` 模式。别名让所有既有调用点零改动，同时 `.so` 插件导入到与 host **完全同一**的类型。
2. **插件侧编译期断言 `var _ pkgimage.ImageProcessor = (*ImagePipelinePlugin)(nil)`** —— 把「碰巧结构匹配（全 `string` 参数）」升级为「显式共享契约依赖」；未来签名演进时 host 与插件强制同源锁步，在**插件构建期**报错而非运行期静默。
3. **诊断挂在能力查找点，而非配置声明** —— `config.PluginConfig` 无「能力角色」字段；命令自己知道需要哪个能力。故护栏挂 `Find[T]` 落空处：registry 空 → 维持「未配置」原提示（不误导）；registry 非空 → 枚举 + 指向根因。
4. **image 不挂运行期诊断，靠编译期断言兜底** —— 避免 build 时对无关插件误报；断言在更早的插件构建期已锁步。
5. **契约位置测试用白名单显式登记例外** —— 推迟/不适用决策**留痕在代码里**，评审可见、不会遗忘；注释「Keep this list shrinking」表达收敛意图。

## 6. 验收记录

四次提交（TDD 红→绿，各任务独立提交）：

| 任务 | Commit | 内容 |
|---|---|---|
| Task 1 | `e80e1a3` | `refactor(image)`: `ImageProcessor` 迁 `pkg/image` + internal 别名 |
| Task 2 | `a241243` | `feat(image-pipeline)`: 导入 `pkg/image` + 编译期断言 |
| Task 3 | `4162566` | `feat(cmd)`: `diagnoseCapabilityGap` 替代裸 `no X available` |
| Task 4 | `e8137a0` | `test(plugin)`: 断言能力契约在 `pkg/`（护栏） |

三绿验证（全部 PASS）：

```
$ go build ./...
（exit 0，无输出）

$ cd plugins/image-pipeline && go build -buildmode=plugin -o /tmp/image-pipeline.so .
（exit 0，产出 ~6.86 MB .so；编译期断言首次即通过）

$ go test ./...
ok  github.com/iannil/huan/pkg/image         （TestImageProcessorSatisfiedByMock PASS）
ok  github.com/iannil/huan/cmd/huan          （TestDiagnoseCapabilityGap PASS）
ok  github.com/iannil/huan/internal/plugin   （TestCapabilityContractsLiveInPkg PASS）
ok  github.com/iannil/huan/internal/image
ok  github.com/iannil/huan/internal/build
ok  github.com/iannil/huan/internal/daemon
（无 FAIL；pkg/deploy、pkg/plugin、pkg/translate、internal/deploy 等为 no-test-files）
```

扫描器有效性已用「临时移除白名单项 → FAIL」验证：注释掉 `GRPCPlugin` 白名单项后测试如期报 `capability interface "GRPCPlugin" ... must live under pkg/`，恢复后 PASS。

## 7. 白名单例外（3 项，带理由）

设计原定 2 项；Task 4 的 AST 扫描新发现第 3 项 `internal/build.Hook`，按同类先例登记：

- **`EventSubscriber`**（`internal/plugin`）：引用 `internal/daemon/eventbus` 类型，且当前**无任何 `.so` 使用者**；迁 `pkg/` 违背 YAGNI，**显式推迟**。它不 embed `Plugin`，结构扫描本就不命中——该白名单项纯作推迟决策的文档留痕。
- **`GRPCPlugin`**（`internal/plugin/grpc_stub.go`）：走 gRPC 跨进程传输，接口满足不跨 `.so` 边界，类型同一性隐患天然不适用。保留/未实现。
- **`Hook`**（`internal/build/hook.go`）：embed `Plugin` 但方法签名用 `[]*content.Page`（内部类型无法搬进 `pkg/`）；其 .so 安全对应物 `pkg/plugin.Hook`（`[]any`）已存在，`.so` 插件实现的是后者。`internal/build.Hook` 仅供宿主内使用，不跨 `.so` 边界。

## 8. 已知限制

- 生效需 host 与插件同源重建（Go plugin lockstep）：`go install ./cmd/huan` + `scripts/build-plugins.sh` + `cp release/plugins/*.so ~/.huan`。
- 端到端插件安装/部署验证为可选项（需装插件），本次以三绿 + 单测覆盖为准。

## 9. 后续优化项（不在当前阶段范围）

- **`Hook` 运行期接线缺口（同类 bug）**：`internal/build/pipeline.go:75` 把 hooks 断言为 `internal/build.Hook`（`[]*content.Page`），而已发布 `.so` 插件（seo-injector / sitemap-enhancer / html-injector）实现 `pkg/plugin.Hook`（`[]any`）；二者结构不同接口、宿主无桥接 adapter，故这些插件的 hook **很可能从未被构建管线调用**。与本次收敛的 `ImageProcessor` 属**同一 bug 类**，但方向为运行期接线，**本 epic 范围外**。建议作为独立后续计划：加宿主侧桥接 adapter，或统一两个 Hook 接口；届时可从白名单移除 `Hook`。
- 清理 `backup/2026-07-28-contract-convergence/`（确认无回滚需要后）。
