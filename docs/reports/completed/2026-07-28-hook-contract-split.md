# Hook 契约拆分（修复 `.so` 钩子失效）完成报告

> 完成日期：2026-07-29（工作起于 2026-07-28）  ·  对标：修复 [ADR 0013](../../adr/0013-plugin-contract-convergence.md) 登记的「Hook 运行期接线缺口」（同类 `.so`/host 契约不一致 bug 在 build pipeline 轴上的实例）
> 计划与设计原文：[设计 spec](../../superpowers/specs/2026-07-28-hook-contract-split-design.md) · [ADR 0014](../../adr/0014-hook-contract-split.md)

## 1. 概述

huan 的三个随站点发布的 `.so` 插件（seo-injector / sitemap-enhancer / html-injector）实现 `pkg/plugin.Hook`（3 方法、`interface{}` 页面参数），而 `internal/build/pipeline.go` 对钩子做 `h.(internal/build.Hook)`（3 方法、`*content.Page`）断言。二者是**结构不同的接口**，宿主侧**无桥接 adapter** → 断言恒为 `false` → 这些插件的钩子**从未被构建管线调用**：**SEO meta 注入、sitemap 增强、自定义 HTML 注入在真实站点上全是死代码，静默无效**。

本次采用**契约拆分**（设计 spec Design 3）而非双向 adapter：三个插件的实际逻辑**全部**在 `OnOutputWritten(ctx, outputDir string)`（纯 `string` 参数、磁盘后处理），两个页面方法跨 `.so` 本质无用（`.so` 拿到装箱的不透明 `interface{}`，无 `content.Page` 类型可检视）。故把 `.so` 面向的契约精简为只暴露真正可跨界的 `OnOutputWritten`，语义诚实：页面级钩子=编译期专属，输出级钩子=`.so` 可用。

## 2. 新增依赖

| 依赖 | 版本 | 用途 |
|---|---|---|
| 无 | — | 纯重构 + 桥接 + 标准库测试 |

## 3. 新增 / 修改的文件

| 路径 | 职责 | 变更 |
|---|---|---|
| `pkg/plugin/hook.go` | `.so` 面向输出钩子契约 | `Hook`（3 方法）→ `PostBuildHook`（embed `Plugin` + 仅 `OnOutputWritten`）+ doc 注释 |
| `pkg/plugin/plugin.go` | 包/`Find` doc 注释 | 3 处 `pkg/plugin.Hook` 引用改 `PostBuildHook`（均注释，无消费者） |
| `internal/plugin/plugin.go` | 类型别名 | 新增 `type PostBuildHook = pkgplugin.PostBuildHook` |
| `internal/plugin/contract_location_test.go` | 护栏白名单注释 | `"Hook"` 项注释澄清拆分后边界（`.so` 对应物为 `PostBuildHook`） |
| `internal/build/pipeline.go` | pipeline 桥接 | `runOnOutputWritten` 在 `build.Hook` 之外 `else if` 桥接 `PostBuildHook`；`runOnContentLoaded`/`runOnPageRendered` 不变 |
| `internal/build/hook_test.go`（新） | 回归测试 | `TestRunOnOutputWritten_InvokesPostBuildHook`（红→绿）+ `TestRunOnOutputWritten_InvokesBuildHook`（不回归） |
| `cmd/huan/plugins.go` | 能力标签 | `capabilityLabels` 加 `else if PostBuildHook` 分支，仍标 `"hook"` |
| `plugins/seo-injector/plugin.go` | 插件瘦身 | 删 no-op `OnContentLoaded`/`OnPageRendered`，仅留 `OnOutputWritten` |
| `plugins/sitemap-enhancer/plugin.go` | 插件瘦身 | 同上 |
| `plugins/html-injector/plugin.go` | 插件瘦身 | 同上 |

## 4. 关键设计决策

1. **契约拆分而非双向 adapter** —— 页面级钩子跨 `.so` 无用（YAGNI），只暴露 `OnOutputWritten`。`internal/build.Hook`（`*content.Page`）保持不变，编译期钩子仍可做富页面操作。
2. **`internal/` 类型别名回填** —— `type PostBuildHook = pkgplugin.PostBuildHook`，对齐 ADR 0013 的 Deployer/Translator/ImageProcessor 模式。
3. **桥接用 `else if` 保 `build.Hook` 优先** —— 两接口实践中互斥；同时满足者按 `build.Hook` 处理（单次调用 / 单个 `"hook"` 标签）。
4. **collection-not-interruption 语义保留** —— `OnOutputWritten` 返回错误只 warn，不 abort build。

## 5. 验收记录

三次提交（TDD 红→绿，各任务独立提交）：

| 任务 | Commit | 内容 |
|---|---|---|
| Task 1 | `9f08581` | `refactor(plugin)`: 拆 `pkg/plugin.Hook` → `PostBuildHook`（+ internal 别名 + doc 注释 + 护栏白名单注释） |
| Task 2 | `c9f7c60` | `fix(build)`: `runOnOutputWritten` 桥接 `PostBuildHook` + 回归测试 + `capabilityLabels` |
| Task 3 | `f715b14` | `refactor(plugins)`: 三插件删 no-op 页面钩子、仅实现 `PostBuildHook`，重编 `.so` |

### TDD 红→绿（Task 2 回归测试直击 bug 现场）

桥接前（红）：

```
=== RUN   TestRunOnOutputWritten_InvokesPostBuildHook
    hook_test.go:33: PostBuildHook.OnOutputWritten was not invoked by the pipeline
--- FAIL: TestRunOnOutputWritten_InvokesPostBuildHook
=== RUN   TestRunOnOutputWritten_InvokesBuildHook
--- PASS: TestRunOnOutputWritten_InvokesBuildHook
```

PostBuildHook-only stub 被旧 `h.(Hook)`-only 检查跳过；编译期 `build.Hook` stub 已工作。

桥接后（绿）：

```
=== RUN   TestRunOnOutputWritten_InvokesPostBuildHook
--- PASS: TestRunOnOutputWritten_InvokesPostBuildHook
=== RUN   TestRunOnOutputWritten_InvokesBuildHook
--- PASS: TestRunOnOutputWritten_InvokesBuildHook
PASS
```

### 三绿验证（全部 PASS）

```
$ go build ./...
（exit 0，无输出）

$ (三个插件各自 go build -buildmode=plugin)
plugins/seo-injector     → seo-injector.so     — exit 0
plugins/sitemap-enhancer → sitemap-enhancer.so — exit 0
plugins/html-injector    → html-injector.so    — exit 0

$ go test ./...
ok  internal/build（含 TestRunOnOutputWritten_InvokesPostBuildHook / _InvokesBuildHook）
ok  internal/plugin（TestCapabilityContractsLiveInPkg，白名单注释更新后仍绿）
ok  cmd/huan
（无 FAIL；pkg/plugin、pkg/deploy、pkg/translate 等为 no-test-files）
```

## 6. 端到端说明（需部署，用户执行）

生效需 host 与插件同源重建（Go plugin lockstep）：

```
go install ./cmd/huan && scripts/build-plugins.sh && cp release/plugins/*.so ~/.huan/
```

随后在 zhurongshuo 站点 `huan build`，确认输出 HTML 实际含注入的 SEO meta / Twitter Card、`sitemap.xml` 已增强、自定义 HTML 已注入（改前这些均无）。本次以三绿 + 回归测试覆盖为准，端到端部署验证为可选项。

## 7. 后续 / 清理

- 清理 `.superpowers/sdd/2026-07-28-hook-contract-split/backup/`（确认无回滚需要后）。
- ADR 0013「后续 / 待办」的 Hook 缺口已标记 ✅ 由 ADR 0014 解决。
