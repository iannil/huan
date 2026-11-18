# Admin 鉴权开关（admin.requireToken）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** huan.yaml 新增 `admin.requireToken` 开关，受信回环环境可关闭 admin 面板 token 鉴权；非回环 bind 关闭时拒绝启动。

**Architecture:** Config 新增 `Admin AdminConfig`（`*bool` + 访问器，照搬 CleanPublishDir 先例）；L1 `CheckBindSafety` 加第三参维护 fail-fast 矩阵；L2 `HandlerOptions.RequireToken` 显式旗标决定是否挂 TokenMiddleware；三个 token 流调用方（dev/daemon/desktop）从 cfg 透传；桌面菜单按旗标裁剪「复制管理 Token」项。

**Tech Stack:** Go 1.27（stdlib `net/http`、`crypto/subtle`、gopkg.in/yaml.v3），测试 stdlib testing + httptest。

**Spec:** `docs/superpowers/specs/2026-11-18-admin-auth-switch-design.md`

## Global Constraints

- 旧报错文案逐字节不变：`huan: admin panel requires HUAN_ADMIN_TOKEN env when binding to non-loopback address (%s);\nset %s to a random string, e.g. \`export %s=$(openssl rand -hex 32)\`,\nor bind to 127.0.0.1 / localhost`（auth.go:30-34 现文）。
- 绝不以「token 为空」推断鉴权关闭——`RequireToken` 是显式旗标（空 token + 空 provided 会通过 `ConstantTimeCompare`，属退化放行路径）。
- SPA（`web/admin/`）一行不改。
- ADR 0011 状态保持 Accepted，只追加带日期修订小节。
- 代码注释英文；conventional commits；`go build ./... && go vet ./...` 每任务全绿；admin/config 包测试 `-race`。
- 缺省行为回归底线：不写 `admin:` 段时，一切输出（含 stderr）与今天逐字节一致。
- **每个提交点 `go build ./...` 必须全绿**（可 bisect 历史）：T2/T3 在改 admin 包签名/字段的同时，同提交内给三个调用方补**字面量保绿值**（`true`），T4/T5 再把字面量替换为 cfg 驱动——字面量注释注明「literal keep-green; config wiring lands in Task 4/5」。

---

### Task 1: Config — `Admin AdminConfig` + `RequireTokenEnabled()`

**Files:**
- Modify: `internal/config/config.go:46-56`（Config struct 字段区）、`:74-77` 附近（AuthorConfig 旁新增 AdminConfig）、`:218-222` 附近（ShouldCleanPublishDir 旁新增访问器）
- Test: `internal/config/admin_config_test.go`（新建）

**Interfaces:**
- Produces: `Config.Admin AdminConfig`；`AdminConfig.RequireToken *bool`；`(*AdminConfig).RequireTokenEnabled() bool`（nil → true）。后续所有任务经 `cfg.Admin.RequireTokenEnabled()` 消费。

- [ ] **Step 1: 写失败测试**

```go
package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRequireTokenEnabled_DefaultIsTrue(t *testing.T) {
	var c Config
	if err := yaml.Unmarshal([]byte("title: t\n"), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Admin.RequireTokenEnabled() {
		t.Fatal("absent admin section must default to requireToken=true")
	}
}

func TestRequireTokenEnabled_ExplicitValues(t *testing.T) {
	var off Config
	if err := yaml.Unmarshal([]byte("admin:\n  requireToken: false\n"), &off); err != nil {
		t.Fatal(err)
	}
	if off.Admin.RequireTokenEnabled() {
		t.Fatal("requireToken: false must disable")
	}
	var on Config
	if err := yaml.Unmarshal([]byte("admin:\n  requireToken: true\n"), &on); err != nil {
		t.Fatal(err)
	}
	if !on.Admin.RequireTokenEnabled() {
		t.Fatal("requireToken: true must stay enabled")
	}
}

func TestRequireTokenEnabled_LoadFromSiteYaml(t *testing.T) {
	dir := t.TempDir()
	yml := "baseURL: http://x/\ntitle: t\nadmin:\n  requireToken: false\n"
	if err := os.WriteFile(filepath.Join(dir, "huan.yaml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Admin.RequireTokenEnabled() {
		t.Fatal("site yaml admin.requireToken: false must load through config.Load")
	}
	_ = bytes.MinRead // keep bytes import if unused elsewhere
}
```

（若 `bytes` 无其他用途，删掉该 import 与末行占位——不要留死代码。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/config/ -run TestRequireTokenEnabled -count=1`
Expected: FAIL —— `c.Admin undefined`（字段尚不存在）。

- [ ] **Step 3: 最小实现**

`config.go` struct 字段区（`Theme string` 之前、`Plugins` 之后）加：

```go
	// Admin gates the local admin panel; see docs/adr/0011-admin-security-boundary.md.
	Admin AdminConfig `yaml:"admin"`
```

`AuthorConfig` 定义之后加：

```go
// AdminConfig configures the admin panel (ADR 0011). RequireToken follows
// the CleanPublishDir *bool convention: nil means "use the safe default".
type AdminConfig struct {
	// RequireToken gates every /admin/api/* request behind a bearer token.
	// Default true. false is honored on loopback binds only — CheckBindSafety
	// refuses a non-loopback bind with auth disabled.
	RequireToken *bool `yaml:"requireToken"`
}

// RequireTokenEnabled reports the effective requireToken setting: only an
// explicit false disables auth; absent/nil keeps the token requirement.
func (a *AdminConfig) RequireTokenEnabled() bool {
	return a.RequireToken == nil || *a.RequireToken
}
```

`Defaults()` 不动（nil → true 即缺省）。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/config/ -race -count=1`
Expected: 全 PASS（既有测试无回归）。

- [ ] **Step 5: 提交**

```bash
git add internal/config/config.go internal/config/admin_config_test.go
git commit -m "feat(config): admin.requireToken switch (default true)"
```

---

### Task 2: L1 — CheckBindSafety 第三参 + PrintAuthDisabledNotice

**Files:**
- Modify: `internal/admin/auth.go:21-37`（CheckBindSafety）、文件尾部（TokenMiddleware 之后新增 PrintAuthDisabledNotice）
- Modify: `cmd/huan/dev.go:73`、`internal/daemon/daemon.go:358`、`internal/desktop/server.go:175`（Step 3b 保绿字面量）
- Test: `internal/admin/auth_test.go:13-55`（既有三测试加参）、文件尾部新增

**Interfaces:**
- Consumes: 无（纯 admin 包内）。
- Produces: `CheckBindSafety(bindAddr, token string, requireToken bool) error`（三参）；`PrintAuthDisabledNotice()`。Task 4/5 的调用方依赖这两个签名。

- [ ] **Step 1: 更新既有测试为三参 + 写新失败测试**

既有三个 CheckBindSafety 测试（auth_test.go:13/28/50）全部改为传 `true`（缺省语义不变，编译即验证）：

```go
admin.CheckBindSafety(bind, token, true)
```

新测试追加到 auth_test.go 尾部：

```go
func TestCheckBindSafety_Loopback_DisabledAuth_Allow(t *testing.T) {
	for _, bind := range []string{"", "localhost", "127.0.0.1", "::1"} {
		if err := CheckBindSafety(bind, "", false); err != nil {
			t.Fatalf("loopback bind %q with auth disabled must allow: %v", bind, err)
		}
	}
}

func TestCheckBindSafety_NonLoopback_DisabledAuth_FailFast(t *testing.T) {
	err := CheckBindSafety("0.0.0.0", "sometoken", false)
	if err == nil {
		t.Fatal("non-loopback bind with auth disabled must refuse")
	}
	const want = "admin.requireToken: false"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error must name the config switch %q, got: %v", want, err)
	}
	if strings.Contains(err.Error(), "HUAN_ADMIN_TOKEN env when binding") {
		t.Fatal("disabled-auth error must be the dedicated message, not the legacy token message")
	}
}

func TestPrintAuthDisabledNotice_ExactLine(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	PrintAuthDisabledNotice()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	got := buf.String()
	want := "huan: admin auth disabled (admin.requireToken: false) — loopback only\n"
	if got != want {
		t.Fatalf("stderr line mismatch:\n got: %q\nwant: %q", got, want)
	}
}
```

（auth_test.go 现有 import 若缺 `bytes`/`io`/`strings` 按需补；stderr 捕获模式与 cmd/huan/plugins_test.go 既有先例一致。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/admin/ -run "CheckBindSafety|PrintAuthDisabled" -count=1`
Expected: FAIL —— 编译错误（CheckBindSafety 两参调用不匹配三参定义尚不存在 / PrintAuthDisabledNotice undefined）。

- [ ] **Step 3: 最小实现**

`auth.go` 的 CheckBindSafety 替换为：

```go
// CheckBindSafety enforces ADR 0011 L1: a non-loopback bind requires token
// auth. With requireToken=true the bind needs a token (env or generated
// upstream); with requireToken=false (admin.requireToken: false, the 2026-11-18
// revision of ADR 0011) the bind must still be loopback — admin API grants
// arbitrary content/ writes, so a LAN-exposed bind may never run unauthenticated.
// Loopback binds are safe by default (CSRF + local privilege already implied
// by local access).
func CheckBindSafety(bindAddr, token string, requireToken bool) error {
	if isLoopback(bindAddr) {
		return nil
	}
	if !requireToken {
		return fmt.Errorf(
			"huan: admin.requireToken: false cannot be combined with a non-loopback bind (%s);\n"+
				"disabled auth is loopback-only — bind to 127.0.0.1 / localhost, or remove admin.requireToken from huan.yaml",
			bindAddr)
	}
	if token == "" {
		return fmt.Errorf(
			"huan: admin panel requires %s env when binding to non-loopback address (%s);\n"+
				"set %s to a random string, e.g. `export %s=$(openssl rand -hex 32)`,\n"+
				"or bind to 127.0.0.1 / localhost",
			AdminTokenEnvVar, bindAddr, AdminTokenEnvVar, AdminTokenEnvVar)
	}
	return nil
}
```

`TokenMiddleware` 之后新增：

```go
// PrintAuthDisabledNotice tells the operator token auth is off. Called once
// at startup by each CLI serve path (dev/daemon) in place of the auto-generated
// token print; the desktop runtime stays silent (no stderr surface in a GUI).
func PrintAuthDisabledNotice() {
	fmt.Fprintln(os.Stderr, "huan: admin auth disabled (admin.requireToken: false) — loopback only")
}
```

（`os` 已在 auth.go import 列表中。）

- [ ] **Step 3b: 保绿调用点更新（同提交）**

签名变为三参后，三个调用方必须同提交更新，否则 `go build ./...` 破绿。各处字面量传 `true`（= 缺省语义，行为零变化）：

```go
// cmd/huan/dev.go:73
if err := admin.CheckBindSafety(bind, token, true); err != nil { // literal keep-green; config wiring lands in Task 4
	return err
}
```

```go
// internal/daemon/daemon.go:358（resolveAdminToken 内）
if err := admin.CheckBindSafety(bind, token, true); err != nil { // literal keep-green; config wiring lands in Task 4
	return "", false, err
}
```

```go
// internal/desktop/server.go:175
if err := admin.CheckBindSafety("127.0.0.1", token, true); err != nil { // literal keep-green; config wiring lands in Task 5
	return "", err
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go build ./... && go vet ./... && go test ./internal/admin/ -race -count=1`
Expected: 全绿（调用方已同提交保绿，全仓可构建）。

- [ ] **Step 5: 提交**

```bash
git add internal/admin/auth.go internal/admin/auth_test.go cmd/huan/dev.go internal/daemon/daemon.go internal/desktop/server.go
git commit -m "feat(admin): CheckBindSafety requireToken param + auth-disabled notice"
```

---

### Task 3: L2 — HandlerOptions.RequireToken 条件挂载中间件

**Files:**
- Modify: `internal/admin/handler.go:18-35`（HandlerOptions struct + 注释）、`:62`（apiWithAuth 挂载点）
- Modify: `cmd/huan/dev.go`、`internal/daemon/daemon.go`、`internal/desktop/server.go` 各自 HandlerOptions（Step 3b 保绿字面量）
- Test: `internal/admin/handler_test.go`（新建）

**Interfaces:**
- Consumes: Task 2 的旗标语义（本任务不 import 新东西，仅布尔字段）。
- Produces: `HandlerOptions.RequireToken bool`。Task 4/5 的三处 `NewHandler` 调用点传 `RequireToken: cfg.Admin.RequireTokenEnabled()`。

- [ ] **Step 1: 写失败测试**

```go
package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iannil/huan/internal/config"
)

func newTestHandler(t *testing.T, requireToken bool) http.Handler {
	t.Helper()
	return NewHandler(HandlerOptions{
		Cfg:          config.Defaults(),
		SourceDir:    t.TempDir(),
		Rebuild:      func() {},
		ServeURL:     "http://127.0.0.1:1313/",
		BindAddr:     "127.0.0.1",
		Token:        "test-secret",
		RequireToken: requireToken,
	})
}

func TestNewHandler_DisabledAuth_NoHeader_PassesMiddleware(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(t, false))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/admin/api/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("RequireToken=false must not gate /admin/api/* (401 means middleware still mounted)")
	}
}

func TestNewHandler_DefaultEnabled_MissingToken_Returns401(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(t, true))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/admin/api/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("RequireToken=true without header must 401, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/admin/ -run TestNewHandler -count=1`
Expected: FAIL —— `HandlerOptions.RequireToken` 字段不存在（编译错误）。

- [ ] **Step 3: 最小实现**

`HandlerOptions` 的 `Token` 字段之后加：

```go
	// RequireToken mounts TokenMiddleware over /admin/api/* when true
	// (the default via cfg.Admin.RequireTokenEnabled()). When false, the API
	// mounts bare — auth is disabled by config and the bind is guaranteed
	// loopback by CheckBindSafety (ADR 0011 L1). This is an explicit flag:
	// never infer it from Token=="" (an empty expected token would let an
	// empty provided token pass ConstantTimeCompare).
	RequireToken bool
```

`apiWithAuth := TokenMiddleware(api, opts.Token)` 替换为：

```go
	// L2 token gate is config-conditional (admin.requireToken). Disabled auth
	// mounts the API bare; CheckBindSafety already confined that to loopback.
	apiWithAuth := api
	if opts.RequireToken {
		apiWithAuth = TokenMiddleware(api, opts.Token)
	}
```

- [ ] **Step 3b: 保绿调用点更新（同提交，防静默关鉴权）**

`RequireToken` 零值为 `false`——三处 `NewHandler` 调用点若不同提交显式置 true，中间提交会**静默关闭全部 admin 鉴权**（破坏缺省回归底线）。各处加字面量 true：

```go
// cmd/huan/dev.go HandlerOptions、internal/daemon/daemon.go HandlerOptions、
// internal/desktop/server.go HandlerOptions —— Token: 字段之后各加：
		RequireToken:  true, // literal keep-green; config wiring lands in Task 4/5
```

- [ ] **Step 4: 运行确认通过**

Run: `go build ./... && go vet ./... && go test ./internal/admin/ -race -count=1`
Expected: 全 PASS（含 Task 2 既有测试；三调用方已保绿且行为不变）。

- [ ] **Step 5: 提交**

```bash
git add internal/admin/handler.go internal/admin/handler_test.go cmd/huan/dev.go internal/daemon/daemon.go internal/desktop/server.go
git commit -m "feat(admin): conditional token middleware via HandlerOptions.RequireToken"
```

---

### Task 4: CLI 接线 — dev.go 与 daemon.go

**Files:**
- Modify: `cmd/huan/dev.go:72-82`（token 流）、`:261-272`（HandlerOptions）
- Modify: `internal/daemon/daemon.go:223-224`（调用点）、`:228-240`（HandlerOptions）、`:353-372`（resolveAdminToken）
- Test: 既有 `go build ./...` + `go vet ./...` 全绿即验收（两处无独立单测面——resolveAdminToken 矩阵已由 Task 2 的 CheckBindSafety 测试承载；CLI stderr 顺序由 Task 2 的 PrintAuthDisabledNotice 逐字节测试承载）

**Interfaces:**
- Consumes: `cfg.Admin.RequireTokenEnabled()`（Task 1）、`CheckBindSafety(bind, token, requireToken)` 与 `PrintAuthDisabledNotice()`（Task 2）、`HandlerOptions.RequireToken`（Task 3）。
- Produces: 无新接口（纯接线）。

- [ ] **Step 1: dev.go token 流替换**

`runDev` 中 `:72-82` 现文（Task 2/3 已插入保绿字面量）：

```go
	requireToken := true // 已被 Task 2/3 字面量化，此处以实际文件为准
	token, tokenFromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety(bind, token, true); err != nil { // literal keep-green; config wiring lands in Task 4
		return err
	}
	if !tokenFromEnv {
		var err error
		if token, err = admin.GenerateToken(); err != nil {
			return fmt.Errorf("generate admin token: %w", err)
		}
		admin.MustPrintAutoGeneratedToken(token, true)
	}
```

替换为（字面量 → cfg 驱动）：

```go
	requireToken := cfg.Admin.RequireTokenEnabled()
	token, tokenFromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety(bind, token, requireToken); err != nil {
		return err
	}
	if !requireToken {
		// admin.requireToken: false (loopback-only, enforced above) — no token
		// to generate or print; the API mounts unauthenticated downstream.
		admin.PrintAuthDisabledNotice()
	} else if !tokenFromEnv {
		var err error
		if token, err = admin.GenerateToken(); err != nil {
			return fmt.Errorf("generate admin token: %w", err)
		}
		admin.MustPrintAutoGeneratedToken(token, true)
	}
```

`:261-272` 的 `HandlerOptions{...}` 中 `RequireToken: true, // literal keep-green...` 一行替换为：

```go
		RequireToken:  requireToken,
```

- [ ] **Step 2: daemon.go resolveAdminToken 签名与实现**

`:353-372` 现文函数头与校验行（Task 2 已插入保绿字面量）：

```go
func resolveAdminToken(bind string) (string, bool, error) {
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety(bind, token, true); err != nil { // literal keep-green; config wiring lands in Task 4
		return "", false, err
	}
	if !fromEnv {
```

替换为：

```go
func resolveAdminToken(bind string, requireToken bool) (string, bool, error) {
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety(bind, token, requireToken); err != nil {
		return "", false, err
	}
	if !requireToken {
		admin.PrintAuthDisabledNotice()
		return token, fromEnv, nil
	}
	if !fromEnv {
```

（函数头注释补一句：`// requireToken=false (admin.requireToken: false) skips generation and prints the disabled notice; loopback-only is enforced by CheckBindSafety.`）

`:223-224` 调用点改为：

```go
	adminToken, _, err := resolveAdminToken(opts.Bind, cfg.Admin.RequireTokenEnabled())
```

`:228-240` 的 `HandlerOptions{...}` 中 `RequireToken: true, // literal keep-green...` 一行替换为：

```go
		RequireToken:  cfg.Admin.RequireTokenEnabled(),
```

- [ ] **Step 3: 全仓构建 + 全量验证**

Run: `go build ./... && go vet ./... && go test ./internal/admin/... ./internal/daemon/... ./cmd/huan/... ./internal/config/... -race -count=1`
Expected: 全绿（保绿字面量已全部替换为 cfg 驱动；daemon/cmd 既有测试无回归）。

- [ ] **Step 4: 运行时冒烟（CLI 行为三态）**

```bash
go build -o /tmp/huan-authswitch ./cmd/huan
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
# 态 1 缺省：token 行仍在
unset HUAN_ADMIN_TOKEN; /tmp/huan-authswitch dev --port 1399 2>&1 | head -5   # 期望含 "admin panel token"
# 态 2 关闭：无 token 行、有 disabled 行（先在 huan.yaml 加 admin: {requireToken: false}，验后还原）
/tmp/huan-authswitch dev --port 1399 2>&1 | head -5   # 期望含 "admin auth disabled"，不含 token 行
# 态 3 非回环 + 关闭：拒绝启动（保持 admin.requireToken: false）
/tmp/huan-authswitch dev --bind 0.0.0.0 --port 1399 2>&1 | head -5   # 期望 "cannot be combined with a non-loopback bind" + exit 非零
```

验后**必须还原** zhurongshuo 的 huan.yaml（该文件属站点仓，不属于本仓改动）。三态截图/日志行贴进任务报告。

- [ ] **Step 5: 提交**

```bash
git add cmd/huan/dev.go internal/daemon/daemon.go
git commit -m "feat(cli): wire admin.requireToken through dev and daemon serve paths"
```

---

### Task 5: 桌面端接线 — server.go + 菜单裁剪

**Files:**
- Modify: `internal/desktop/server.go:172-181`（token 流）、`:320-331`（HandlerOptions）、ServeRuntime struct（`:55` 附近）+ 新方法（`:124` Token() 旁）
- Modify: `internal/desktop/menu.go:35`（BuildTrayMenu 签名+条目）、`:64`（BuildAppMenu 签名+条目）、`:91`（BuildTray 签名透传）、`:146-151`（RefreshRecents 签名透传）
- Modify: `internal/desktop/app.go:123`（BuildTray 调用）、`:326`（RefreshRecents 调用）+ App struct 加字段 + startAndServe 内同步旗标
- Test: `internal/desktop/menu_test.go`（追加）

**Interfaces:**
- Consumes: `cfg.Admin.RequireTokenEnabled()`（Task 1）、`CheckBindSafety` 三参（Task 2）、`HandlerOptions.RequireToken`（Task 3）、`ServeRuntime.Token()`（既有）。
- Produces: `ServeRuntime.AdminAuthRequired() bool`（Start 前/鉴权开启恒 true）；`BuildTrayMenu(c Controller, recents []string, adminAuthRequired bool) *mygo.Menu`；`BuildAppMenu(c Controller, adminAuthRequired bool) *mygo.Menu`；`BuildTray(c Controller, recents []string, adminAuthRequired bool)`；`(*MenuSet).RefreshRecents(c Controller, recents []string, adminAuthRequired bool) error`。

- [ ] **Step 1: 写失败测试（菜单裁剪）**

menu_test.go 追加（stub Controller 补齐全部 11 个方法，空实现）：

```go
type stubController struct{}

func (stubController) ToggleServe()       {}
func (stubController) Rebuild()           {}
func (stubController) CopyAdminToken()    {}
func (stubController) OpenSiteFolder()    {}
func (stubController) OpenSiteInBrowser() {}
func (stubController) SwitchSite(string)  {}
func (stubController) ChooseSite()        {}
func (stubController) OpenLogs()          {}
func (stubController) ShowWindow()        {}
func (stubController) Quit()              {}
func (stubController) ShowDiagnostics()   {}

func TestBuildTrayMenu_OmitsTokenItem_WhenAuthDisabled(t *testing.T) {
	menu := BuildTrayMenu(stubController{}, nil, false)
	if item := menu.ItemByID(itemCopyToken); item != nil {
		t.Fatal("tray menu must omit 复制管理 Token when admin auth is disabled")
	}
}

func TestBuildTrayMenu_IncludesTokenItem_WhenAuthEnabled(t *testing.T) {
	menu := BuildTrayMenu(stubController{}, nil, true)
	if item := menu.ItemByID(itemCopyToken); item == nil {
		t.Fatal("tray menu must keep 复制管理 Token when admin auth is enabled")
	}
}

func TestBuildAppMenu_OmitsTokenItem_WhenAuthDisabled(t *testing.T) {
	menu := BuildAppMenu(stubController{}, false)
	if item := menu.ItemByID(itemCopyToken); item != nil {
		t.Fatal("app menu must omit 复制管理 Token when admin auth is disabled")
	}
}
```

（`mygo.NewMenu`/`ItemByID` 为纯内存构造，无需 GUI；Task 4 计划（启动诊断）已验证 `diagMenuLabel` 可测，同一可行面。若 `ItemByID` 在未创建窗口的 Menu 上不可用，退化为遍历 items 断言——以实现者实测为准，报告记录。）

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/desktop/ -run "TokenItem" -count=1`
Expected: FAIL —— 编译错误（三参签名尚不存在）。

- [ ] **Step 3: menu.go 签名与条件条目**

`BuildTrayMenu`：签名加第三参；`items` 初始切片把 token 项改为条件追加（在 diag 项之前）：

```go
// BuildTrayMenu assembles the tray menu; recents are the recent site paths.
// adminAuthRequired=false (admin.requireToken: false) omits the copy-token
// item — there is no token to copy.
func BuildTrayMenu(c Controller, recents []string, adminAuthRequired bool) *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "打开预览窗口", ID: "show-window", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
		{Label: "启动/停止预览服务", ID: itemToggleServe, Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
		{Label: "一键构建站点", ID: itemRebuild, Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
	}
	if adminAuthRequired {
		items = append(items, &mygo.MenuItem{Label: "复制管理 Token", ID: itemCopyToken, Click: func(*mygo.MenuItem, *mygo.Window) { c.CopyAdminToken() }})
	}
	items = append(items,
		&mygo.MenuItem{Label: diagMenuLabel(0), ID: itemDiag, Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowDiagnostics() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "打开站点目录", ID: "open-folder", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
		&mygo.MenuItem{Label: "在浏览器打开站点", ID: "open-browser", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
		&mygo.MenuItem{Label: "打开日志目录", ID: "open-logs", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenLogs() }},
		mygo.Separator(),
	)
```

（后续 recents/choose-site/quit 追加段与原文件 :48-60 逐行一致，仅需注意 append 实参用 `&mygo.MenuItem{...}` 形式。`BuildAppMenu`：签名 `BuildAppMenu(c Controller, adminAuthRequired bool)`，内部改为与 BuildTrayMenu 相同的条件追加结构——先建不含 token 项的站点子菜单切片，`if adminAuthRequired` 时在「一键构建站点」项之后 `append` token 项，再接 diag/Separator/其余项；切片字面量上下文 `{...}` 省略写法合法，append 实参位置用 `&mygo.MenuItem{...}`。`BuildTray` 与 `RefreshRecents` 签名加同名参数并透传给两个 Build 函数。）

- [ ] **Step 4: server.go 旗标 + HandlerOptions**

`:172-181` token 流替换（桌面无 stderr 打印面——既不生成也不提示，保持静默）。现文（Task 2 已插入保绿字面量）：

```go
	// Admin token: generate silently (no stdout printing in a GUI app).
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety("127.0.0.1", token, true); err != nil { // literal keep-green; config wiring lands in Task 5
		return "", err
	}
	if !fromEnv {
		if token, err = admin.GenerateToken(); err != nil {
			return "", fmt.Errorf("generate admin token: %w", err)
		}
	}
```

替换为：

```go
	// Admin auth: admin.requireToken=false (loopback-only, enforced below)
	// skips token generation entirely — the desktop has no stderr surface.
	requireToken := cfg.Admin.RequireTokenEnabled()
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety("127.0.0.1", token, requireToken); err != nil {
		return "", err
	}
	if requireToken && !fromEnv {
		if token, err = admin.GenerateToken(); err != nil {
			return "", fmt.Errorf("generate admin token: %w", err)
		}
	}
```

`ServeRuntime` struct 加字段 `adminAuthRequired bool`（初值 true 由 Start 内赋值覆盖）；Start 内 cfg 加载后赋值 `r.adminAuthRequired = requireToken`（按 struct 既有锁约定，参照 `Token()` 的实现方式加锁/免锁）；`Token()` 旁新增：

```go
// AdminAuthRequired reports whether the admin API of the current serve
// session requires a token (admin.requireToken). True before the first
// Start — the safe default — and updated by Start after config load.
func (r *ServeRuntime) AdminAuthRequired() bool {
	return r.adminAuthRequired
}
```

`:320-331` `HandlerOptions{...}` 中 `RequireToken: true, // literal keep-green...` 一行替换为 `RequireToken: requireToken,`。

- [ ] **Step 5: app.go 旗标同步 + 调用点**

App struct 加字段 `adminAuthRequired bool`（零值 false，但 onReady 首建菜单发生在首次 Start 之前——见下）。`app.go:123` `BuildTray(a, a.cfg.RecentSites)` 改为 `BuildTray(a, a.cfg.RecentSites, a.adminAuthRequired)`；App 构造路径（New* 函数）把该字段初始化为 `true`（缺省语义，首建菜单含 token 项）。`startAndServe` 内 `rt.Start()` 成功后、`RefreshRecents` 之前加：

```go
	a.adminAuthRequired = rt.AdminAuthRequired()
```

`:326` `a.menus.RefreshRecents(a, a.cfg.RecentSites)` 改为 `a.menus.RefreshRecents(a, a.cfg.RecentSites, a.adminAuthRequired)`。（时序注：首建菜单用缺省 true；auth off 时 startAndServe 的 RefreshRecents 紧随 Start 重建菜单，token 项随即消失——一次启动内的瞬态可接受。）

- [ ] **Step 6: 验证 + 提交**

Run: `go build ./... && go vet ./... && go test ./internal/desktop/... ./internal/admin/... ./internal/config/... -race -count=1 && go test ./... -count=1`
Expected: 全绿。

```bash
git add internal/desktop/server.go internal/desktop/menu.go internal/desktop/app.go internal/desktop/menu_test.go
git commit -m "feat(desktop): honor admin.requireToken — skip token, trim copy-token menu item"
```

---

### Task 6: 文档收尾 + 全量门禁

**Files:**
- Modify: `docs/adr/0011-admin-security-boundary.md`（追加修订小节）
- Modify: `README.md` / `README.zh-CN.md`（admin 安全段各一行）
- Modify: `memory/daily/2026-11-18.md`（实现条目）

**Interfaces:**
- Consumes: 全部前序任务落地的行为。
- Produces: 无代码接口。

- [ ] **Step 1: ADR 0011 追加修订小节**

在「不在范围（YAGNI）」之前插入：

```markdown
## 2026-11-18 修订：`admin.requireToken` 开关

新增配置 `admin.requireToken`（缺省 true，行为与本 ADR 原决策逐字节一致）：

- `false` 时 L2 token 门禁不挂载、自动生成 token 与 stderr 打印跳过（改为单行 disabled 提示）；仅限回环 bind——L1 扩展为：非回环 + `requireToken: false` **拒绝启动**（专属报错），LAN 暴露面不因该开关扩大。
- `false` 与 `HUAN_ADMIN_TOKEN` 并存时开关赢（env 忽略）。
- 原「否决 warn-and-continue」结论不变：本修订不引入任何「警告但继续暴露」路径。
- 设计与验收：`docs/superpowers/specs/2026-11-18-admin-auth-switch-design.md`。
```

- [ ] **Step 2: README 两语言各加一行**

README.md / README.zh-CN.md 的 admin 安全说明段（各文件 grep `HUAN_ADMIN_TOKEN` 定位）旁加：

- 英文：`On a trusted loopback setup you can disable the admin token prompt with admin.requireToken: false in huan.yaml (loopback binds only; LAN-exposed binds still refuse to start).`
- 中文：`受信回环环境可在 huan.yaml 配置 admin.requireToken: false 关闭管理面板 token 提示（仅限回环 bind；非回环 bind 仍拒绝启动）。`

- [ ] **Step 3: 日志条目 + 全量门禁**

`memory/daily/2026-11-18.md` 追加实现完成条目（时间戳、关键决策、验证结果）。

Run: `gofmt -l internal/ cmd/ | grep -E "admin|config|desktop|daemon|dev" ; go vet ./... && go test ./... -count=1`
Expected: gofmt 交集为空（既有漂移文件不在本次触碰集合）；vet/test 全绿。

- [ ] **Step 4: 提交**

```bash
git add docs/adr/0011-admin-security-boundary.md README.md README.zh-CN.md memory/daily/2026-11-18.md
git commit -m "docs: admin auth switch — ADR 0011 revision + README notes"
```

---

## Self-Review 记录

1. **Spec 覆盖**：配置语义→T1；L1 矩阵+新报错+disabled 提示→T2；L2 条件挂载+空 token 退化防护→T3；三调用方（serve=dev 别名共用）→T4/T5；桌面菜单裁剪+瞬态时序→T5；文档→T6；验收四场景→T4 Step 4（CLI 三态）+ T5（桌面）+ T3（handler 级 401 回归底线）。无缺口。
2. **占位符扫描**：T5 Step 3 的 `...` 为「保持原文件其余条目不动」的显式省略标记，非待填内容；T1 的 bytes 占位行已注明按需删除。其余步骤均含完整代码。
3. **类型一致性**：`RequireTokenEnabled()`（T1 定义，T4/T5 消费）、`CheckBindSafety(bindAddr, token string, requireToken bool)`（T2 定义，T4/T5 消费）、`HandlerOptions.RequireToken bool`（T3 定义，T4/T5 消费）、`PrintAuthDisabledNotice()`（T2 定义，T4 消费）、`AdminAuthRequired()`（T5 内自洽）逐一核对一致。
