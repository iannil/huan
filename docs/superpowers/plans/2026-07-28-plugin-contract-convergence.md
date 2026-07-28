# 插件契约收敛 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把最后一个散落在 `internal/` 的 `.so` 能力契约（`ImageProcessor`）收敛到 `pkg/`，并加两道护栏（能力解析诊断 + 契约位置测试），杜绝「跨 `.so` 契约类型不一致」这类 bug 再以反应式方式复发。

**Architecture:** 镜像已验证的 `pkg/deploy` / `pkg/translate` 模式——契约声明在 `pkg/`，`internal/` 用类型别名回填以保调用点零改动；`.so` 插件显式 import 共享契约包获得编译期锁步。护栏一：`Find[T]` 落空且已有插件加载时，产出枚举 + 指向根因的诊断，替代裸的 `no XXX available`。护栏二：AST 单元测试断言所有「embed Plugin + 加方法」的能力契约都在 `pkg/`，`EventSubscriber` / `GRPCPlugin` 以带理由的白名单登记留痕。

**Tech Stack:** Go 1.26.2；`go/parser` + `go/ast`（契约位置测试）；Go plugin（`-buildmode=plugin`）；模块 `github.com/iannil/huan`。

## Global Constraints

- Go 版本：`go 1.26.2`（host 与所有 `.so` 插件必须同版本、同依赖，否则 `plugin.Open` 报版本错）。
- 模块路径：`github.com/iannil/huan`；`.so` 插件为独立 module，各自 `replace github.com/iannil/huan => ../../`。
- 共享契约包（`pkg/deploy` / `pkg/translate` / 新增 `pkg/image`）为 host+plugin **共享**，二者须同源重建：`go install ./cmd/huan` + `scripts/build-plugins.sh` + `cp release/plugins/*.so "$HUAN_HOME"`。
- 文档中文、代码英文。
- 批量程序性改动前先备份改动文件到 `/backup` 相对路径；错误数异常上升立即回滚。
- 每个任务结束：`go build ./...` + 相关 `.so` `go build -buildmode=plugin` + `go test ./...` 三绿方可提交。
- 提交信息结尾附：`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`。

---

### Task 1: `ImageProcessor` 契约迁移到 `pkg/image`（host 侧）

**Files:**
- Create: `pkg/image/types.go`
- Create: `pkg/image/types_test.go`
- Modify: `internal/image/processor.go`（整文件替换为别名）

**Interfaces:**
- Produces: `pkg/image.ImageProcessor`（interface：embed `pkg/plugin.Plugin` + `Process(outputDir, sourceDir string) error`）。
- Produces: `internal/image.ImageProcessor = pkgimage.ImageProcessor`（类型别名，供既有调用点 `internal/daemon/daemon.go:186`、`cmd/huan/plugins.go:119`、`cmd/huan/build_image.go:20`、`internal/image/processor_test.go` 继续引用，零改动）。

- [ ] **Step 1: 备份将被改写的文件**

Run:
```bash
mkdir -p backup/2026-07-28-contract-convergence/internal/image
cp internal/image/processor.go backup/2026-07-28-contract-convergence/internal/image/processor.go
```

- [ ] **Step 2: 写 `pkg/image` 的失败测试（新包尚不存在 → 编译失败即红）**

Create `pkg/image/types_test.go`:
```go
package image_test

import (
	"testing"

	pkgimage "github.com/iannil/huan/pkg/image"
	pkgplugin "github.com/iannil/huan/pkg/plugin"
)

// mockProc satisfies both the base Plugin and the ImageProcessor capability.
type mockProc struct{}

func (mockProc) Name() string                             { return "mock" }
func (mockProc) Process(outputDir, sourceDir string) error { return nil }

func TestImageProcessorSatisfiedByMock(t *testing.T) {
	var _ pkgplugin.Plugin = mockProc{}
	var _ pkgimage.ImageProcessor = mockProc{}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./pkg/image/ -run TestImageProcessorSatisfiedByMock -v`
Expected: FAIL — `package github.com/iannil/huan/pkg/image is not in std` / 无法编译（包不存在）。

- [ ] **Step 4: 创建 `pkg/image/types.go`**

Create `pkg/image/types.go`:
```go
// Package image defines the ImageProcessor capability contract for build-time
// image plugins. It lives in pkg/ (not internal/) so .so plugins can import
// the exact host type — a contract defined in internal/ causes silent
// cross-.so interface mismatch (see the deploy/translate bugs of 2026-07-28).
package image

import pkgplugin "github.com/iannil/huan/pkg/plugin"

// ImageProcessor is the capability interface for plugins that process images
// during the build pipeline (compress, resize, format conversion). It embeds
// the base Plugin and adds Process.
type ImageProcessor interface {
	pkgplugin.Plugin

	// Process compresses, converts, and resizes images in the output directory.
	// outputDir is the build output directory (publishDir).
	// sourceDir is the project root (for config resolution).
	Process(outputDir, sourceDir string) error
}
```

- [ ] **Step 5: 把 `internal/image/processor.go` 改为别名**

Replace the entire contents of `internal/image/processor.go` with:
```go
// Package image re-exports the ImageProcessor capability contract from
// pkg/image via a type alias, so existing internal call sites keep importing
// internal/image while the real type lives in pkg/ (shared with .so plugins).
package image

import pkgimage "github.com/iannil/huan/pkg/image"

// ImageProcessor is an alias of pkg/image.ImageProcessor. Both names denote the
// exact same type, so .so plugins importing pkg/image and internal code
// importing internal/image satisfy one identical interface across the .so
// boundary.
type ImageProcessor = pkgimage.ImageProcessor
```

- [ ] **Step 6: 运行 pkg 测试 + 全量构建/测试确认绿**

Run:
```bash
go test ./pkg/image/ -run TestImageProcessorSatisfiedByMock -v
go build ./...
go test ./internal/image/ ./cmd/huan/ ./internal/daemon/
```
Expected: PASS（pkg/image 测试通过；既有 `internal/image/processor_test.go` 的 `Find[image.ImageProcessor]` 用例经别名依旧通过；全量构建绿）。

- [ ] **Step 7: 提交**

```bash
git add pkg/image/ internal/image/processor.go
git commit -m "refactor(image): move ImageProcessor contract to pkg/image (alias internal)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: image-pipeline 插件显式依赖 `pkg/image`（plugin 侧编译期锁步）

**Files:**
- Modify: `plugins/image-pipeline/plugin.go`（加 import + 编译期断言）

**Interfaces:**
- Consumes: `pkg/image.ImageProcessor`（Task 1 产出）。
- Produces: 无新导出；把「碰巧结构匹配」升级为「显式共享契约依赖」——`ImagePipelinePlugin` 与 host 从此同源锁步。

- [ ] **Step 1: 备份**

Run:
```bash
mkdir -p backup/2026-07-28-contract-convergence/plugins/image-pipeline
cp plugins/image-pipeline/plugin.go backup/2026-07-28-contract-convergence/plugins/image-pipeline/plugin.go
```

- [ ] **Step 2: 加 import 与编译期断言**

In `plugins/image-pipeline/plugin.go`, 修改 import 块并在 `ImagePipelinePlugin` 类型声明后加断言：
```go
import (
	"fmt"

	pkgimage "github.com/iannil/huan/pkg/image"
)

// Compile-time proof that ImagePipelinePlugin satisfies the shared host
// contract. If Process's signature ever diverges from pkg/image (e.g. a config
// struct is added), this fails at plugin build time — catching the mismatch
// before it becomes a silent runtime "no image processor" like the 2026-07-28
// deploy/translate bugs.
var _ pkgimage.ImageProcessor = (*ImagePipelinePlugin)(nil)
```

- [ ] **Step 3: 构建插件确认断言通过（绿）**

Run:
```bash
cd plugins/image-pipeline && go build -buildmode=plugin -o /tmp/image-pipeline.so . && cd -
```
Expected: 构建成功（`Process(string,string) error` + `Name() string` 已满足 `pkg/image.ImageProcessor`）。若报 `undefined: pkgimage` 或断言失败，则 import/签名有误。

- [ ] **Step 4: 回归 host 构建**

Run: `go build ./...`
Expected: PASS（host 不受插件模块影响）。

- [ ] **Step 5: 提交**

```bash
git add plugins/image-pipeline/plugin.go
git commit -m "feat(image-pipeline): import pkg/image + compile-time contract assertion

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 能力解析诊断 helper（主护栏）+ 接入 deploy / translate

**Files:**
- Modify: `cmd/huan/plugins.go`（新增 `diagnoseCapabilityGap`；`strings` import）
- Modify: `cmd/huan/deploy.go`（Find[Deployer] 落空处接入诊断）
- Modify: `cmd/huan/translate_cmd.go`（Find[Translator] 落空处接入诊断）
- Test: `cmd/huan/plugins_test.go`（新增 `TestDiagnoseCapabilityGap`）

**Interfaces:**
- Consumes: `plugin.Registry`（`.All() []plugin.Plugin`）、`capabilityLabels(plugin.Plugin) []string`（`cmd/huan/plugins.go` 既有）。
- Produces: `diagnoseCapabilityGap(registry *plugin.Registry, capability string) string` —— registry 为空返回 `""`（真「未配置」）；否则返回枚举各已加载插件及其能力标签、指向 contract mismatch + 修复动作的诊断串。

- [ ] **Step 1: 备份**

Run:
```bash
mkdir -p backup/2026-07-28-contract-convergence/cmd/huan
cp cmd/huan/plugins.go cmd/huan/deploy.go cmd/huan/translate_cmd.go cmd/huan/plugins_test.go backup/2026-07-28-contract-convergence/cmd/huan/
```

- [ ] **Step 2: 写失败测试**

Append to `cmd/huan/plugins_test.go`:
```go
// baseOnlyPlugin satisfies only the base Plugin interface — it deliberately
// does NOT satisfy any capability, simulating a .so plugin whose contract
// diverged from the host (so Find[Deployer] returns nothing).
type baseOnlyPlugin struct{}

func (baseOnlyPlugin) Name() string { return "faux" }

func TestDiagnoseCapabilityGap(t *testing.T) {
	// Empty registry => genuine "nothing configured", no root-cause hint.
	if got := diagnoseCapabilityGap(plugin.NewRegistry(), "deploy.Deployer"); got != "" {
		t.Fatalf("empty registry: want \"\", got %q", got)
	}

	// A plugin is loaded but satisfies no capability => root-cause diagnostic.
	reg := plugin.NewRegistry()
	if err := reg.Register(baseOnlyPlugin{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	got := diagnoseCapabilityGap(reg, "deploy.Deployer")
	if !strings.Contains(got, "faux") {
		t.Errorf("diagnostic should name the loaded plugin; got %q", got)
	}
	if !strings.Contains(got, "contract mismatch") {
		t.Errorf("diagnostic should point at contract mismatch; got %q", got)
	}
	if !strings.Contains(got, "deploy.Deployer") {
		t.Errorf("diagnostic should name the wanted capability; got %q", got)
	}
}
```
（`cmd/huan/plugins_test.go` 已 import `plugin` 与 `strings`；若缺 `strings` 则补上。）

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./cmd/huan/ -run TestDiagnoseCapabilityGap -v`
Expected: FAIL — `undefined: diagnoseCapabilityGap`。

- [ ] **Step 4: 实现 `diagnoseCapabilityGap`**

In `cmd/huan/plugins.go`：确保 import 含 `"fmt"` 与 `"strings"`，并新增：
```go
// diagnoseCapabilityGap explains why a capability lookup came up empty. When no
// plugin is loaded it returns "" (a genuine "not configured" case the caller
// should report plainly). When plugins ARE loaded but none satisfy the wanted
// capability, it lists each plugin with the capabilities it actually satisfies,
// so an operator can spot the plugin that should provide `capability` but shows
// [none] — the signature of a .so/host contract mismatch (see 2026-07-28).
func diagnoseCapabilityGap(registry *plugin.Registry, capability string) string {
	plugins := registry.All()
	if len(plugins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(plugins))
	for _, p := range plugins {
		labels := capabilityLabels(p)
		if len(labels) == 0 {
			labels = []string{"none"}
		}
		parts = append(parts, fmt.Sprintf("%s[%s]", p.Name(), strings.Join(labels, ",")))
	}
	return fmt.Sprintf(
		"%d plugin(s) loaded, none satisfy %s: %s; a plugin that should provide it but shows [none] indicates a .so/host contract mismatch — rebuild plugins against current pkg/ contracts (scripts/build-plugins.sh && cp release/plugins/*.so \"$HUAN_HOME\")",
		len(plugins), capability, strings.Join(parts, ", "),
	)
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/huan/ -run TestDiagnoseCapabilityGap -v`
Expected: PASS。

- [ ] **Step 6: 接入 deploy 落空处**

In `cmd/huan/deploy.go`，把：
```go
	if len(deployers) == 0 {
		return fmt.Errorf("no deployer plugin available (declare a deployer plugin under huan.yaml plugins)")
	}
```
替换为：
```go
	if len(deployers) == 0 {
		if hint := diagnoseCapabilityGap(registry, "deploy.Deployer"); hint != "" {
			return fmt.Errorf("no deployer plugin available: %s", hint)
		}
		return fmt.Errorf("no deployer plugin available (declare a deployer plugin under huan.yaml plugins)")
	}
```

- [ ] **Step 7: 接入 translate 落空处**

In `cmd/huan/translate_cmd.go`，把：
```go
		fmt.Fprintln(os.Stderr, "translate: no translator plugin configured; skipping (declare a translator plugin under huan.yaml plugins to enable)")
		return nil
```
替换为：
```go
		if hint := diagnoseCapabilityGap(registry, "translate.Translator"); hint != "" {
			fmt.Fprintf(os.Stderr, "translate: %s\n", hint)
		} else {
			fmt.Fprintln(os.Stderr, "translate: no translator plugin configured; skipping (declare a translator plugin under huan.yaml plugins to enable)")
		}
		return nil
```

- [ ] **Step 8: 全量构建 + 测试**

Run:
```bash
go build ./...
go test ./cmd/huan/
```
Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add cmd/huan/plugins.go cmd/huan/deploy.go cmd/huan/translate_cmd.go cmd/huan/plugins_test.go
git commit -m "feat(cmd): capability-gap diagnostic replaces bare 'no X available'

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 契约位置测试（廉价保险，防设计期漂移）

**Files:**
- Create: `internal/plugin/contract_location_test.go`

**Interfaces:**
- Consumes: 源码树（AST 扫描 `../../internal`、`../../pkg`、`../../cmd`）。
- Produces: `TestCapabilityContractsLiveInPkg`（断言 embed `Plugin` + 加方法的能力接口都在 `pkg/`）；`capabilityContractWhitelist`（带理由的例外：`EventSubscriber`、`GRPCPlugin`）。

- [ ] **Step 1: 写测试（先红——`GRPCPlugin` 在 internal 且未白名单时会 FAIL，用于验证扫描器真的生效）**

Create `internal/plugin/contract_location_test.go`:
```go
package plugin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// capabilityContractWhitelist lists capability interfaces intentionally allowed
// OUTSIDE pkg/, each with the reason. Keep this list shrinking.
var capabilityContractWhitelist = map[string]string{
	// EventSubscriber references internal/daemon/eventbus types and has no .so
	// implementer today; migrating it (and the eventbus types it needs) to pkg/
	// is deferred (YAGNI) until the first .so plugin needs event subscription.
	"EventSubscriber": "references internal/daemon/eventbus; no .so implementer; deferred (YAGNI)",
	// GRPCPlugin is served over gRPC (a cross-process transport), so interface
	// satisfaction never crosses a Go .so boundary — the type-identity hazard
	// does not apply. Reserved/unimplemented.
	"GRPCPlugin": "gRPC cross-process transport; not subject to the in-process .so type-identity hazard",
}

// TestCapabilityContractsLiveInPkg asserts every capability interface — one
// that embeds plugin.Plugin and adds at least one method — is declared under
// pkg/, so .so plugins can import the exact host type. A contract defined in
// internal/ causes silent cross-.so interface mismatch (see the deploy /
// translate bugs of 2026-07-28). Intentional exceptions go in
// capabilityContractWhitelist with a reason.
func TestCapabilityContractsLiveInPkg(t *testing.T) {
	roots := []string{"../../internal", "../../pkg", "../../cmd"}
	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					return true // type aliases / non-interface specs are skipped
				}
				if !embedsPlugin(it) || !addsMethod(it) {
					return true
				}
				if strings.Contains(filepath.ToSlash(path), "/pkg/") {
					return true // correctly placed
				}
				if _, ok := capabilityContractWhitelist[ts.Name.Name]; ok {
					return true // known, documented exception
				}
				t.Errorf("capability interface %q in %s must live under pkg/ "+
					"(it embeds plugin.Plugin and adds methods; .so plugins cannot "+
					"import internal/). Move it to pkg/, or add it to "+
					"capabilityContractWhitelist with a reason.", ts.Name.Name, path)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

// embedsPlugin reports whether the interface embeds a type named Plugin
// (either bare `Plugin` or a selector like `pkgplugin.Plugin`).
func embedsPlugin(it *ast.InterfaceType) bool {
	for _, field := range it.Methods.List {
		if len(field.Names) != 0 {
			continue // a method, not an embed
		}
		switch e := field.Type.(type) {
		case *ast.Ident:
			if e.Name == "Plugin" {
				return true
			}
		case *ast.SelectorExpr:
			if e.Sel != nil && e.Sel.Name == "Plugin" {
				return true
			}
		}
	}
	return false
}

// addsMethod reports whether the interface declares at least one method
// (beyond embedded interfaces).
func addsMethod(it *ast.InterfaceType) bool {
	for _, field := range it.Methods.List {
		if len(field.Names) != 0 {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 临时移除 `GRPCPlugin` 白名单项，运行确认扫描器会 FAIL（证明测试有效）**

临时把 `capabilityContractWhitelist` 里的 `"GRPCPlugin"` 行注释掉，然后：
Run: `go test ./internal/plugin/ -run TestCapabilityContractsLiveInPkg -v`
Expected: FAIL — 报 `capability interface "GRPCPlugin" in ../../internal/plugin/grpc_stub.go must live under pkg/ …`。这证明扫描器真的在检测 embed-Plugin 契约。

- [ ] **Step 3: 恢复 `GRPCPlugin` 白名单项，运行确认通过**

恢复刚注释的 `"GRPCPlugin"` 行。
Run: `go test ./internal/plugin/ -run TestCapabilityContractsLiveInPkg -v`
Expected: PASS —— `Deployer`/`Translator`/`ImageProcessor`/`ThemePlugin` 等均在 `pkg/`；`GRPCPlugin` 被白名单豁免；`EventSubscriber` 不 embed Plugin 故本就不触发（其白名单项作为推迟决策的显式留痕保留）。

- [ ] **Step 4: 全量测试**

Run: `go test ./...`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/plugin/contract_location_test.go
git commit -m "test(plugin): assert capability contracts live in pkg/ (guardrail)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 文档 —— ADR + 完成报告 + 记忆

**Files:**
- Create: `docs/adr/0013-plugin-contract-convergence.md`
- Create: `docs/reports/completed/2026-07-28-plugin-contract-convergence.md`
- Modify: `memory/daily/2026-07-28.md`（追加）
- Modify: `memory/MEMORY.md`（合并关键决策）

**Interfaces:**
- Consumes: Task 1-4 的实际改动与验证结果。
- Produces: 决策留痕（ADR）、验收报告、记忆更新。

- [ ] **Step 1: 写 ADR 0013**

Create `docs/adr/0013-plugin-contract-convergence.md`，覆盖：
  - **背景**：2026-07-28 一轮反应式修复暴露「`.so` 能力契约声明在 `internal/` → 插件自带类型副本 → 静默不满足接口」根因（ThemePlugin/Deployer/Translator 各修一次）。
  - **决策**：(1) 把最后一个残留契约 `ImageProcessor` 迁 `pkg/image`（别名回填）；(2) 加载空能力时用 `diagnoseCapabilityGap` 产出指向根因的诊断；(3) 加 `TestCapabilityContractsLiveInPkg` AST 护栏。
  - **推迟（YAGNI）**：`EventSubscriber` 引用 `internal/daemon/eventbus` 且无 `.so` 使用者，暂不迁 `pkg/`；`GRPCPlugin` 走 gRPC 跨进程、不受 `.so` 类型同一性约束——二者以带理由白名单登记。
  - **后果**：所有 embed-Plugin 能力契约现均在 `pkg/`；新增同类契约若落 `internal/` 会 CI 红；能力缺失从"命令期含糊报错"提前到"带根因的诊断"。

- [ ] **Step 2: 写完成报告**

Create `docs/reports/completed/2026-07-28-plugin-contract-convergence.md`：按 `docs/standards` / `docs/templates` 模板，记录实际改动文件、验证命令与结果（`go build ./...` / image-pipeline `-buildmode=plugin` / `go test ./...` 三绿）、白名单两项及理由。

- [ ] **Step 3: 更新每日笔记（追加，不覆盖）**

在 `memory/daily/2026-07-28.md` 末尾追加一节「插件契约主动收敛（方案 B）」，简述：ImageProcessor→pkg/image、诊断护栏、契约位置测试、EventSubscriber/GRPCPlugin 推迟。

- [ ] **Step 4: 合并长期记忆**

在 `memory/MEMORY.md` 的「关键决策」补一条：
  > **能力契约一律置于 `pkg/`（2026-07-28）**：所有 embed `plugin.Plugin` 的 `.so` 能力契约必须在 `pkg/`（Deployer/Translator/ImageProcessor/ThemePlugin）；`internal/` 仅用类型别名回填。`TestCapabilityContractsLiveInPkg` 护栏 + `diagnoseCapabilityGap` 诊断。例外：`EventSubscriber`（引用 internal eventbus、无 .so 用者，YAGNI 推迟）、`GRPCPlugin`（gRPC 跨进程，不受 .so 类型同一性约束）。

- [ ] **Step 5: 提交**

```bash
git add docs/adr/0013-plugin-contract-convergence.md docs/reports/completed/2026-07-28-plugin-contract-convergence.md memory/
git commit -m "docs(adr,report,memory): record plugin contract convergence (ADR 0013)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## 收尾验证（全部任务后）

- [ ] `go build ./...` 绿。
- [ ] `cd plugins/image-pipeline && go build -buildmode=plugin -o /tmp/ip.so .` 绿。
- [ ] `go test ./...` 全绿（含 `pkg/image`、`cmd/huan` 诊断、`internal/plugin` 契约位置）。
- [ ] （可选端到端，需装插件）重建并部署插件后 `huan build` 图片管线正常、`huan deploy cloudflare --dry-run` 若契约错则报**带根因**的诊断。
- [ ] 清理 `/backup/2026-07-28-contract-convergence/`（确认无回滚需要后）。

## 自检结论（对照 spec）

- Spec 第 1 块（ImageProcessor→pkg/image + 插件断言）→ Task 1、Task 2 ✅
- Spec 第 2 块（能力解析诊断）→ Task 3 ✅（挂 deploy 硬错 + translate 跳过提示；image 由 Task 2 编译期断言在**更早**的插件构建期兜底，故运行期不再挂载，避免 build 时对无关插件误报）
- Spec 第 3 块（契约位置测试 + 白名单留痕）→ Task 4 ✅（白名单含 EventSubscriber 推迟 + GRPCPlugin N/A）
- Spec 第 4 块（文档/ADR/记忆/备份）→ Task 5 + 各任务 Step 1 备份 ✅
- 类型一致性：`diagnoseCapabilityGap`、`capabilityLabels`、`embedsPlugin`/`addsMethod`、`ImageProcessor` 别名 在各处签名一致 ✅
