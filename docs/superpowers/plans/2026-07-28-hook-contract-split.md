# Hook 契约拆分 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复三个 `.so` 插件（seo-injector / sitemap-enhancer / html-injector）在 `huan build` 时钩子从不被调用的 bug——把 `pkg/plugin.Hook` 拆成诚实的 `.so` 契约 `PostBuildHook`（仅 `OnOutputWritten`），在 pipeline 桥接它，并补上当初缺失的回归测试。

**Architecture:** 保留 `internal/build.Hook`（编译期富页面钩子，用 `*content.Page`）不变；将 host 侧无消费者的 `pkg/plugin.Hook`（3 个 `any` 方法）替换为 `PostBuildHook`（embed Plugin + 仅 `OnOutputWritten(ctx, outputDir string) error`）——这是唯一能跨 `.so` 且有用的钩子。pipeline 的 `runOnOutputWritten` 额外识别并调用 `PostBuildHook`。三个插件删掉 no-op 页面方法。

**Tech Stack:** Go 1.26.2；Go plugin（`-buildmode=plugin`）；模块 `github.com/iannil/huan`。

## Global Constraints

- Go 版本 `go 1.26.2`；host 与所有 `.so` 插件必须同版本、同依赖（否则 `plugin.Open` 报版本错）。
- 模块路径 `github.com/iannil/huan`；`.so` 插件为独立 module，各自 `replace github.com/iannil/huan => ../../`。
- `pkg/plugin` 为 host+plugin 共享包：改动后二者须同源重建（`go install ./cmd/huan` + `scripts/build-plugins.sh` + `cp release/plugins/*.so ~/.huan/`）。
- **备份位置**：批量改动前备份到 `.superpowers/sdd/2026-07-28-hook-contract-split/backup/`（Go 忽略 `.`-前缀目录）——**绝不用**模块根的 `backup/`（会让 `go build ./...` 尝试编译其中的 `package main` 文件而失败）。
- 文档中文、代码英文。
- 每个任务结束：`go build ./...` +（涉及插件时）`go build -buildmode=plugin` + `go test ./...` 三绿方可提交。
- 提交信息结尾附：`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`。

---

### Task 1: 契约拆分 `pkg/plugin.Hook` → `PostBuildHook`（host 侧）

**Files:**
- Modify: `pkg/plugin/hook.go`（整文件替换）
- Modify: `pkg/plugin/plugin.go`（更新 3 处引用旧 `Hook` 名的文档注释）
- Modify: `internal/plugin/plugin.go`（新增类型别名 `PostBuildHook`）
- Modify: `internal/plugin/contract_location_test.go`（更新 `internal/build.Hook` 白名单注释）

**Interfaces:**
- Produces: `pkg/plugin.PostBuildHook`（interface：embed `Plugin` + `OnOutputWritten(ctx context.Context, outputDir string) error`）。
- Produces: `internal/plugin.PostBuildHook = pkgplugin.PostBuildHook`（类型别名，供 `cmd/huan` 用内部 `plugin` 包引用）。
- Removes: `pkg/plugin.Hook`（旧的 3 方法接口，host 侧无消费者）。

- [ ] **Step 1: 备份将改写的文件**

Run:
```bash
mkdir -p .superpowers/sdd/2026-07-28-hook-contract-split/backup/pkg/plugin .superpowers/sdd/2026-07-28-hook-contract-split/backup/internal/plugin
cp pkg/plugin/hook.go pkg/plugin/plugin.go .superpowers/sdd/2026-07-28-hook-contract-split/backup/pkg/plugin/
cp internal/plugin/plugin.go internal/plugin/contract_location_test.go .superpowers/sdd/2026-07-28-hook-contract-split/backup/internal/plugin/
```

- [ ] **Step 2: 替换 `pkg/plugin/hook.go`**

Replace the entire contents of `pkg/plugin/hook.go` with:
```go
// Package plugin provides canonical type definitions for the huan plugin system.
package plugin

import "context"

// PostBuildHook is the .so-facing build hook. Only OnOutputWritten crosses a
// .so boundary usefully: page-mutating hooks would receive an opaque
// interface{} the plugin cannot inspect (content.Page is not importable from
// pkg/ or .so plugin modules). Rich page-level hooks stay in internal/build.Hook,
// which is compiled-in only.
type PostBuildHook interface {
	Plugin

	// OnOutputWritten is called after all output files are written, before the
	// build result is finalized. Receives the output directory for
	// post-processing (e.g. inject SEO meta, enhance sitemap, inject HTML).
	// Collection-not-interruption: a returned error logs a warning and does
	// not abort the build.
	OnOutputWritten(ctx context.Context, outputDir string) error
}
```

- [ ] **Step 3: 更新 `pkg/plugin/plugin.go` 文档注释**

In `pkg/plugin/plugin.go`, replace the two doc-comment mentions of `pkg/plugin.Hook` with `pkg/plugin.PostBuildHook`:
- The package comment line (currently `// interfaces (e.g. pkg/plugin.Hook, pkg/plugin.ThemePlugin) embed Plugin and`) → `// interfaces (e.g. pkg/plugin.PostBuildHook, pkg/plugin.ThemePlugin) embed Plugin and`.
- The `Find` doc comment: change `// pkg/plugin.Hook or pkg/plugin.ThemePlugin.` → `// pkg/plugin.PostBuildHook or pkg/plugin.ThemePlugin.` and the example line `//	hooks := plugin.Find[pkgplugin.Hook](registry)` → `//	hooks := plugin.Find[pkgplugin.PostBuildHook](registry)`.

- [ ] **Step 4: 在 `internal/plugin/plugin.go` 加类型别名**

In `internal/plugin/plugin.go`, alongside the other aliases (after `type SchemaProvider = pkgplugin.SchemaProvider`), add:
```go
type PostBuildHook = pkgplugin.PostBuildHook
```

- [ ] **Step 5: 更新契约位置护栏白名单注释**

In `internal/plugin/contract_location_test.go`, replace the `internal/build.Hook`-related comment content inside `capabilityContractWhitelist` (the `"Hook"` entry, added in the previous epic) so it reads:
```go
	// internal/build.Hook uses *content.Page (an internal type not importable
	// from pkg/ or .so modules), so it stays compiled-in only. Its .so-facing
	// counterpart is pkg/plugin.PostBuildHook (OnOutputWritten only), which the
	// build pipeline bridges — so .so plugins reach the build via PostBuildHook,
	// not this interface.
	"Hook": "internal/build.Hook uses *content.Page (compiled-in only); .so plugins use pkg/plugin.PostBuildHook",
```
(Keep the map key `"Hook"` — it matches the interface name `internal/build.Hook`. Only the reason string + comment change.)

- [ ] **Step 6: 构建 + 全量测试**

Run:
```bash
go build ./...
go test ./...
```
Expected: PASS. Rationale: `pkg/plugin.Hook` had no host consumer (pipeline uses `internal/build.Hook`), so removing it and adding `PostBuildHook` compiles cleanly; the contract-location test still passes (PostBuildHook is correctly in `pkg/`; the `Hook` whitelist entry still matches `internal/build.Hook`).

- [ ] **Step 7: 提交**

```bash
git add pkg/plugin/hook.go pkg/plugin/plugin.go internal/plugin/plugin.go internal/plugin/contract_location_test.go
git commit -m "refactor(plugin): split pkg/plugin.Hook into .so-facing PostBuildHook

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: pipeline 桥接 `PostBuildHook` + 回归测试 + 能力标签

**Files:**
- Create: `internal/build/hook_test.go`
- Modify: `internal/build/pipeline.go`（`runOnOutputWritten`）
- Modify: `cmd/huan/plugins.go`（`capabilityLabels`）

**Interfaces:**
- Consumes: `pkg/plugin.PostBuildHook` (Task 1); `internal/plugin.PostBuildHook` alias (Task 1); existing `newPipeline(opts Options) *pipeline`, `Options{OutputDir, PluginRegistry, Logf}`, `(*pipeline).runOnOutputWritten()`, `internal/build.Hook`.
- Produces: `runOnOutputWritten` now also invokes `PostBuildHook.OnOutputWritten`; `capabilityLabels` labels `PostBuildHook` plugins as `"hook"`.

- [ ] **Step 1: 写回归测试（红）**

Create `internal/build/hook_test.go`:
```go
package build

import (
	"context"
	"testing"

	"github.com/iannil/huan/internal/content"
	iplugin "github.com/iannil/huan/internal/plugin"
	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// postBuildStub satisfies pkgplugin.PostBuildHook (the .so-facing hook) but NOT
// internal/build.Hook — mirroring the shipped seo/sitemap/html .so plugins.
type postBuildStub struct{ called bool }

func (s *postBuildStub) Name() string { return "post_build_stub" }
func (s *postBuildStub) OnOutputWritten(_ context.Context, _ string) error {
	s.called = true
	return nil
}

var _ pkgplugin.PostBuildHook = (*postBuildStub)(nil)

func TestRunOnOutputWritten_InvokesPostBuildHook(t *testing.T) {
	stub := &postBuildStub{}
	reg := iplugin.NewRegistry()
	if err := reg.Register(stub); err != nil {
		t.Fatalf("register: %v", err)
	}
	p := newPipeline(Options{OutputDir: t.TempDir(), PluginRegistry: reg, Logf: func(string, ...any) {}})
	p.runOnOutputWritten()
	if !stub.called {
		t.Fatal("PostBuildHook.OnOutputWritten was not invoked by the pipeline")
	}
}

// pageHookStub satisfies internal/build.Hook (compiled-in rich hook). Guards
// against the PostBuildHook bridge regressing the existing build.Hook path.
type pageHookStub struct{ outCalled bool }

func (s *pageHookStub) Name() string { return "page_hook_stub" }
func (s *pageHookStub) OnContentLoaded(_ context.Context, _ []*content.Page) ([]*content.Page, error) {
	return nil, nil
}
func (s *pageHookStub) OnPageRendered(_ context.Context, _ *content.Page) error { return nil }
func (s *pageHookStub) OnOutputWritten(_ context.Context, _ string) error {
	s.outCalled = true
	return nil
}

var _ Hook = (*pageHookStub)(nil)

func TestRunOnOutputWritten_InvokesBuildHook(t *testing.T) {
	stub := &pageHookStub{}
	reg := iplugin.NewRegistry()
	if err := reg.Register(stub); err != nil {
		t.Fatalf("register: %v", err)
	}
	p := newPipeline(Options{OutputDir: t.TempDir(), PluginRegistry: reg, Logf: func(string, ...any) {}})
	p.runOnOutputWritten()
	if !stub.outCalled {
		t.Fatal("build.Hook.OnOutputWritten was not invoked")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/build/ -run TestRunOnOutputWritten -v`
Expected: `TestRunOnOutputWritten_InvokesBuildHook` PASS, but `TestRunOnOutputWritten_InvokesPostBuildHook` **FAIL** ("PostBuildHook.OnOutputWritten was not invoked") — current `runOnOutputWritten` only checks `h.(Hook)`, so the PostBuildHook-only stub is skipped.

- [ ] **Step 3: 桥接 `runOnOutputWritten`**

In `internal/build/pipeline.go`, replace the body of `runOnOutputWritten` with:
```go
// runOnOutputWritten invokes registered build.Hook and pkg/plugin.PostBuildHook
// plugins after output is written. build.Hook (compiled-in, rich pages) takes
// precedence; .so plugins reach the pipeline via PostBuildHook. Collection-not-
// interruption: failures log a warning but do not abort.
func (p *pipeline) runOnOutputWritten() {
	if p.opts.PluginRegistry == nil {
		return
	}
	for _, h := range p.opts.PluginRegistry.All() {
		if hook, ok := h.(Hook); ok {
			if err := hook.OnOutputWritten(context.Background(), p.opts.OutputDir); err != nil {
				p.logf("  WARN: hook %s OnOutputWritten: %v\n", hook.Name(), err)
			}
			continue
		}
		if pbh, ok := h.(pkgplugin.PostBuildHook); ok {
			if err := pbh.OnOutputWritten(context.Background(), p.opts.OutputDir); err != nil {
				p.logf("  WARN: hook %s OnOutputWritten: %v\n", pbh.Name(), err)
			}
		}
	}
}
```
(`pkgplugin` is already imported in `pipeline.go` — no import change.)

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/build/ -run TestRunOnOutputWritten -v`
Expected: both tests PASS.

- [ ] **Step 5: 更新 `capabilityLabels`**

In `cmd/huan/plugins.go`, replace the `build.Hook` block in `capabilityLabels`:
```go
	if _, ok := p.(build.Hook); ok {
		labels = append(labels, "hook")
	}
```
with:
```go
	if _, ok := p.(build.Hook); ok {
		labels = append(labels, "hook")
	} else if _, ok := p.(plugin.PostBuildHook); ok {
		labels = append(labels, "hook")
	}
```
(`plugin` is `internal/plugin`, already imported in `cmd/huan/plugins.go`; `plugin.PostBuildHook` is the alias from Task 1 — no new import.)

- [ ] **Step 6: 构建 + 全量测试**

Run:
```bash
go build ./...
go test ./...
```
Expected: PASS.

- [ ] **Step 7: 提交**

```bash
git add internal/build/hook_test.go internal/build/pipeline.go cmd/huan/plugins.go
git commit -m "fix(build): invoke .so PostBuildHook plugins in runOnOutputWritten

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 三个 `.so` 插件瘦身（删 no-op 页面方法）+ 重编

**Files:**
- Modify: `plugins/seo-injector/plugin.go`（删 `OnContentLoaded` / `OnPageRendered`）
- Modify: `plugins/sitemap-enhancer/plugin.go`（同）
- Modify: `plugins/html-injector/plugin.go`（同）

**Interfaces:**
- Consumes: `pkg/plugin.PostBuildHook` (Task 1) — after deletion each plugin satisfies it via its remaining `Name()` + `OnOutputWritten`.
- Produces: 三个精简后的 `.so`，仅实现 `OnOutputWritten` 的 build 钩子。

- [ ] **Step 1: 备份三个插件文件**

Run:
```bash
D=.superpowers/sdd/2026-07-28-hook-contract-split/backup/plugins
mkdir -p "$D/seo-injector" "$D/sitemap-enhancer" "$D/html-injector"
cp plugins/seo-injector/plugin.go "$D/seo-injector/"
cp plugins/sitemap-enhancer/plugin.go "$D/sitemap-enhancer/"
cp plugins/html-injector/plugin.go "$D/html-injector/"
```

- [ ] **Step 2: 删除 seo-injector 的 no-op 方法**

In `plugins/seo-injector/plugin.go`, delete these two methods (including their doc comments):
```go
// OnContentLoaded is a no-op for this plugin.
func (p *SEOInjector) OnContentLoaded(_ context.Context, pages []any) ([]any, error) {
	return nil, nil
}

// OnPageRendered is a no-op for this plugin.
func (p *SEOInjector) OnPageRendered(_ context.Context, page any) error {
	return nil
}
```
Leave `OnOutputWritten` and all other methods intact. `context` stays imported (used by `OnOutputWritten`).

- [ ] **Step 3: 删除 sitemap-enhancer 的 no-op 方法**

In `plugins/sitemap-enhancer/plugin.go`, delete the two methods `OnContentLoaded(_ context.Context, pages []any) ([]any, error)` and `OnPageRendered(_ context.Context, page any) error` (with their doc comments), leaving `OnOutputWritten` intact.

- [ ] **Step 4: 删除 html-injector 的 no-op 方法**

In `plugins/html-injector/plugin.go`, delete the two methods `OnContentLoaded(_ context.Context, pages []any) ([]any, error)` and `OnPageRendered(_ context.Context, page any) error` (with their doc comments), leaving `OnOutputWritten` intact.

- [ ] **Step 5: 各自 `-buildmode=plugin` 构建，确认仍满足契约**

Run:
```bash
( cd plugins/seo-injector && go build -buildmode=plugin -o /tmp/seo-injector.so . )
( cd plugins/sitemap-enhancer && go build -buildmode=plugin -o /tmp/sitemap-enhancer.so . )
( cd plugins/html-injector && go build -buildmode=plugin -o /tmp/html-injector.so . )
```
Expected: all three build with exit 0. (Each still has `Name()` + `OnOutputWritten`, satisfying `pkg/plugin.PostBuildHook`.) If a build reports an unused import (e.g. an import only the deleted methods used), remove that import and rebuild — verify by reading the file's imports; do not remove `context` (used by `OnOutputWritten`).

- [ ] **Step 6: host 回归**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 7: 提交**

```bash
git add plugins/seo-injector/plugin.go plugins/sitemap-enhancer/plugin.go plugins/html-injector/plugin.go
git commit -m "refactor(plugins): drop no-op page hooks; implement PostBuildHook only

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 文档 —— ADR 0014 + 关闭 ADR 0013 后续 + 报告 + 记忆

**Files:**
- Create: `docs/adr/0014-hook-contract-split.md`
- Modify: `docs/adr/0013-plugin-contract-convergence.md`（后续/待办 → 标记已解决）
- Create: `docs/reports/completed/2026-07-28-hook-contract-split.md`
- Modify: `memory/daily/2026-07-28.md`（追加）
- Modify: `memory/MEMORY.md`（合并关键决策）

**Interfaces:**
- Consumes: Task 1-3 的实际改动、提交、验证结果。
- Produces: 决策留痕 + 验收报告 + 记忆更新。

- [ ] **Step 1: 写 ADR 0014**

Create `docs/adr/0014-hook-contract-split.md`，匹配 `docs/adr/` 现有格式（0013 为前一份）。覆盖：
  - **背景**：`pkg/plugin.Hook`（`any` 三方法）与 `internal/build.Hook`（`*content.Page`）分叉；pipeline 断言 `build.Hook`，而三个 `.so` 插件实现 `pkg/plugin.Hook` → 钩子从不被调用（SEO/sitemap/HTML 注入在真实站点上静默失效）。是 ADR 0013 记录的同类 bug 在 build pipeline 轴的实例。
  - **决策**：拆分——`pkg/plugin.Hook` → `PostBuildHook`（仅 `OnOutputWritten`，唯一可跨 `.so`）；`internal/build.Hook` 保留为编译期富页面钩子；pipeline `runOnOutputWritten` 桥接 `PostBuildHook`；三个插件删 no-op 页面方法；补回归测试。
  - **后果**：`.so` 钩子插件真实生效；契约语义诚实（页面钩子=编译期，输出钩子=`.so`）；护栏白名单注释已澄清。
  - **引用**：commits（Task1-3 的实际 SHA）。

- [ ] **Step 2: 关闭 ADR 0013 的 Hook 后续**

In `docs/adr/0013-plugin-contract-convergence.md`, 找到记录 Hook 运行期后续/待办的段落，在其后追加一行：
> ✅ 已由 [ADR 0014](0014-hook-contract-split.md) 解决（2026-07-28）：`pkg/plugin.Hook`→`PostBuildHook`，pipeline 桥接，三插件生效。

（保留原始描述，仅追加解决状态，不删历史。）

- [ ] **Step 3: 写完成报告**

Create `docs/reports/completed/2026-07-28-hook-contract-split.md`，按 `docs/reports/completed/` 现有文件风格：改动文件、Task1-3 提交 SHA、验证命令与结果（`go build ./...` / 三个 `.so` `-buildmode=plugin` / `go test ./...` 三绿 + 回归测试红→绿）、以及端到端说明（见收尾验证）。

- [ ] **Step 4: 追加每日笔记（不覆盖）**

在 `memory/daily/2026-07-28.md` 末尾追加一节「Hook 契约拆分（修复 .so 钩子失效）」，简述 bug、拆分、桥接、回归测试。

- [ ] **Step 5: 合并长期记忆**

在 `memory/MEMORY.md` 「关键决策」补一条：
  > **`.so` 构建钩子用 `PostBuildHook`（2026-07-28）**：`pkg/plugin.Hook`（`any` 三方法）拆为 `PostBuildHook`（仅 `OnOutputWritten`，唯一可跨 `.so`）；`internal/build.Hook`（`*content.Page`）保留编译期富页面钩子。pipeline `runOnOutputWritten` 桥接两者。seo/sitemap/html `.so` 插件据此生效（此前因契约分叉静默失效）。见 ADR 0014。

- [ ] **Step 6: 提交**

```bash
git add docs/adr/0014-hook-contract-split.md docs/adr/0013-plugin-contract-convergence.md docs/reports/completed/2026-07-28-hook-contract-split.md memory/
git commit -m "docs(adr,report,memory): record hook contract split (ADR 0014)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## 收尾验证（全部任务后）

- [ ] `go build ./...` 绿。
- [ ] `go test ./...` 全绿（含 `internal/build` 两条 `TestRunOnOutputWritten`、`internal/plugin` 契约位置护栏）。
- [ ] 三个插件 `go build -buildmode=plugin` 均绿。
- [ ] **端到端（需部署，用户执行）**：`go install ./cmd/huan && scripts/build-plugins.sh && cp release/plugins/*.so ~/.huan/`，然后在 zhurongshuo 站点 `huan build`，确认输出 HTML 实际含注入的 SEO meta / Twitter Card、`sitemap.xml` 已增强、自定义 HTML 已注入（改前这些均无）。
- [ ] 清理 `.superpowers/sdd/2026-07-28-hook-contract-split/backup/`（确认无回滚需要后）。

## 自检结论（对照 spec）

- Spec 第 1 块（契约拆分 + 别名 + 文档注释）→ Task 1 ✅
- Spec 第 2 块（pipeline 桥接）→ Task 2 ✅
- Spec 第 3 块（插件瘦身 + capabilityLabels）→ Task 3（瘦身）+ Task 2 Step 5（labels）✅
- Spec 第 4 块（回归测试 + 端到端）→ Task 2 Step 1-4 + 收尾验证 ✅
- Spec 第 5 块（ADR 0014、关闭 0013、白名单注释、报告、记忆）→ Task 4 + Task 1 Step 5（白名单注释）✅
- 类型一致性：`PostBuildHook`（pkg + internal 别名）、`runOnOutputWritten`、`capabilityLabels`、stub 签名 全对齐 ✅
