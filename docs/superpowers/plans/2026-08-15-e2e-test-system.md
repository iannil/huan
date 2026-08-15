# huan E2E 测试用例体系 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立面向 agent 驱动执行的 E2E 测试用例体系：3 个 fixture 站点 + runbooks + 10 个 API 套件 + 5 个浏览器文件 + 4 个 CLI 文件，全部为结构化 YAML 验收规范。

**Architecture:** 分层双 schema——api/cli 用严格结构化 schema（curl 可复现），browser 用旅程式 schema（意图 + 里程碑）。fixture 是共享数据层，runbook 是 agent 执行手册。不写测试代码、不写 runner。

**Tech Stack:** YAML 用例文件 + Markdown runbook/schema 文档 + bash/curl/agent-browser 执行。设计文档：`docs/superpowers/specs/2026-08-15-e2e-test-system-design.md`。

## Global Constraints

- 所有文档（YAML 内 name/description、MD 文档）用**中文**；YAML 字段名、代码、路径用英文
- 用例放 `tests/e2e/`，按 spec §2 目录结构，不引入其他位置
- 硬规则 1：每个 `http` step 的 `expect` 必须显式断言 `status` + 至少一个 body 字段（或显式 `body_absent: true`）
- 硬规则 2：每个写操作（POST/PUT/DELETE）用例必须在响应断言后跟至少一个 `fs.assert`
- 错误响应体断言 `{"error": "..."}` 结构（`internal/admin/types.go` APIError 契约）
- **运行时选择（经实测修正）**：所有需要 admin API 的套件用 **dev 模式**（`huan dev`，token 自动生成打印 stderr，流转正确）。daemon 模式当前 admin token 鉴权失效（Task 1 修复前），修复后 daemon 仅用于 SSE 公开端点用例
- token 捕获正则（runbook 契约）：从启动 stderr 匹配 `admin panel token` 后的 32 位 hex，即 `grep -oE '[0-9a-f]{32}'` 取第一处
- dev 模式默认端口 1313、bind 127.0.0.1；daemon 默认 8080、bind 0.0.0.0（测试一律显式 `--port` + `--bind 127.0.0.1`）
- fixture 的 `huan.yaml` 必须显式写 `ai: { llmsTxt: true, contentAPI: true }`？——**不需要**：`internal/config/config.go:190` 默认即 true，fixture 不写 ai 块（YAGNI）
- 每个 fixture 必须含 `layouts/` 最小模板（实测：无 layouts 时 build 渲染 0 页、Errors: 1；最小 single.html + list.html 即可全绿）
- 模板内分页列表用 `.RelPermalink`（实测 `.URL` 在 list 上下文报 `can't evaluate field URL`）
- YAML 中含 `---` frontmatter 的多行内容，用块标量 `|` 并避免 YAML 解析歧义；文件内出现 `---` 的行必须整体用单引号或块标量包裹
- 提交信息用英文 conventional commits

## 探针实测事实（用例断言依据，全部经 2026-08-15 实测）

| 事实 | 出处 |
|---|---|
| dev 启动后 stderr 打印 token，格式 `huan: admin panel token (save this; will not be shown again):\n    <32hex>` | `internal/admin/handler.go:101` |
| 无 token → 401 `{"error":"missing admin token"}` + `WWW-Authenticate: Bearer realm="huan-admin"` | `internal/admin/auth.go:94-97` |
| 错 token → 401 `{"error":"invalid admin token"}` | `internal/admin/auth.go:99-102` |
| `X-Huan-Admin-Token` 头与 Bearer 等价 | `internal/admin/auth.go` extractToken |
| GET /admin/api/status 200 返回 `{"title","baseURL","serveURL","total","published","drafts","sections","languages","mediaCount","sectionBreakdown","recentContent"}` | `internal/admin/types.go:37-49` |
| POST /admin/api/content 成功 → **201** + ContentDetail（含 `relPath` 如 `posts/x.md`，filename 自动补 .md） | `internal/admin/api.go:181` + `content.go:110` |
| 缺 title/filename → 400 `{"error":"title and filename are required"}` | `api.go:165` |
| PUT 成功 → 200 + ContentDetail；DELETE 成功 → 200 `{"status":"deleted"}` | `api.go:203,222` |
| POST /admin/api/build → **202** `{"status":"rebuild triggered"}` | `api.go:306` |
| media 上传 multipart 字段名 `file`，可选 `dir`；成功 → **201** mediaItem `{name,path,size,ext}` | `internal/admin/media.go:105-160` |
| 不支持扩展名 → 400 `{"error":"unsupported file type: <ext>"}`；合法扩展名清单见 media.go:41（.jpg/.png/.svg/.mp4/.pdf 等 18 种） | `media.go:41-46,137` |
| media 删除路径穿越 → **403** `{"error":"path traversal denied"}` | `media.go:166-171` |
| settings PUT → 200 `{"status":"saved"}` 并异步触发 rebuild；settings/yaml PUT body 为 `{"content": "..."}`，非法 YAML → 400 | `api.go:320-381` |
| settings/yaml GET 返回 **text/plain** 原文 | `api.go:345-352` |
| plugins load 成功 → 200 `{"status":"loaded","plugin":{...}}`；path 为空 → 400 `{"error":"path is required"}`；名字冲突 → **409**；缺 Init 符号 → 400 | `api.go:396-427` |
| GET /admin/api/plugins → 200 `{"plugins":[...],"total":N}` | `api.go:383-394` |
| **daemon 传 `Token: ""` 且无 env 回退 → 带 HUAN_ADMIN_TOKEN 的正确请求也 401**（缺陷，Task 1 修） | `internal/daemon/daemon.go:215` + `auth.go:91` |
| dev 模式**没有**挂载 /health、/api/v1/*、SSE（404）；这些只在 daemon | `cmd/huan/dev.go` 探针实测 |
| daemon /health → 200 `{"status":"ok","uptime":"...","version":"0.6.0"}` | `serving.go:59-63` |
| LiveReload 协议：WS 连接后服务端先发 `{"command":"hello","protocols":["http://livereload.com/protocols/official-7"]}`；变更广播 `{"command":"reload"}`；错误 `{"command":"alert","message":...}` | `internal/dev/livereload.go:49-102` |
| SSE 心跳间隔 15s（注释行形式） | `internal/daemon/sse/hub.go:23` |
| `huan new posts/x.md`：存在即报 `file already exists`；archetype 查找 `archetypes/<kind>.md` → `archetypes/default.md` → 内置默认 | `cmd/huan/newcmd.go:39-41` |
| `huan config`：合法时输出 YAML 全量（yaml.Marshal）；非法时非零退出 | `cmd/huan/config.go:16-24` |
| `huan version`：输出 `huan <semver>`（当前 0.7.1，`-v` 时更多） | `cmd/huan/version.go:17` |
| 无 layouts 时 build：Rendered 0 pages、Errors: 1、仍产 sitemap.xml/api/llms.txt/page/1 | 探针实测 |
| 最小 layouts（_default/single.html + list.html）后 build 全绿，产物含 `posts/<slug>/index.html` | 探针实测 |

---

### Task 1: 修复 daemon admin token 缺陷（前置）

**Files:**
- Modify: `internal/daemon/daemon.go:209-218`（admin.NewHandler 调用处）
- Test: `internal/daemon/daemon_test.go`（追加测试）

**Interfaces:**
- Consumes: `admin.ResolveToken() (token string, fromEnv bool)`、`admin.CheckBindSafety(bindAddr, token string) error`、`admin.GenerateToken() (string, error)`、`admin.MustPrintAutoGeneratedToken(token string, autoGenerated bool)`（均已在 `internal/admin/auth.go` 存在）
- Produces: daemon 模式下 admin API 鉴权与 dev 一致——loopback 自动生成 token 打印 stderr；`HUAN_ADMIN_TOKEN` 环境变量优先；非 loopback bind 无 env → 拒绝启动。后续 Task 14/15 的 SSE/daemon 用例依赖此行为。

- [ ] **Step 1: 写失败测试**

在 `internal/daemon/daemon_test.go` 追加（该文件已有测试，遵循其模式；若没有合适 harness 则建 `internal/daemon/token_test.go`）：

```go
// TestDaemonAdminTokenFromEnv verifies that daemon.Run wires the admin token
// from HUAN_ADMIN_TOKEN into the admin handler. Regression for the bug where
// daemon.go hardcoded Token: "" with a comment claiming env fallback that
// never existed — making every admin API request 401 even with the correct
// env token.
func TestDaemonAdminTokenFromEnv(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "huan.yaml"), "baseURL: http://127.0.0.1:1/\ntitle: t\nlanguageCode: zh-cn\npublishDir: public\n")
	writeFile(t, filepath.Join(src, "content", "posts", "a.md"), "---\ntitle: A\ndate: 2026-08-15\n---\nbody\n")

	t.Setenv("HUAN_ADMIN_TOKEN", "e2e-fixed-token")
	// Run daemon in a goroutine on an ephemeral port, then probe /admin/api/status.
	go func() {
		_ = Run(Options{SourceDir: src, Port: "0", Bind: "127.0.0.1"})
	}()
	// Poll until the listener is up (Run exposes no addr; poll a known-good
	// side effect is not possible with Port "0" — use a fixed high port instead).
}
```

注意：`Run` 阻塞且不回传实际端口。务实做法：不测 `Run` 全流程，改测提取出的纯函数（见 Step 2 的 `resolveAdminToken`），直接单元测试：

```go
func TestResolveAdminToken(t *testing.T) {
	t.Run("env set wins", func(t *testing.T) {
		t.Setenv("HUAN_ADMIN_TOKEN", "from-env")
		tok, fromEnv := resolveAdminToken("127.0.0.1")
		if !fromEnv || tok != "from-env" {
			t.Fatalf("token=%q fromEnv=%v, want from-env", tok, fromEnv)
		}
	})
	t.Run("loopback no env generates", func(t *testing.T) {
		t.Setenv("HUAN_ADMIN_TOKEN", "")
		tok, fromEnv := resolveAdminToken("127.0.0.1")
		if fromEnv || len(tok) != 32 {
			t.Fatalf("token=%q fromEnv=%v, want generated 32-hex", tok, fromEnv)
		}
	})
	t.Run("non-loopback no env errors", func(t *testing.T) {
		t.Setenv("HUAN_ADMIN_TOKEN", "")
		if _, err := resolveAdminToken("0.0.0.0"); err == nil {
			t.Fatal("want error for non-loopback without env token")
		}
	})
}
```

（`writeFile` 若文件内无此 helper，加：`func writeFile(t *testing.T, path, content string) { t.Helper(); os.MkdirAll(filepath.Dir(path), 0o755); os.WriteFile(path, []byte(content), 0o644) }`）

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/daemon/ -run TestResolveAdminToken -v`
Expected: FAIL（`undefined: resolveAdminToken`）

- [ ] **Step 3: 实现**

在 `internal/daemon/daemon.go` 加函数并在 NewHandler 调用处使用：

```go
// resolveAdminToken mirrors dev.go's token flow for the daemon: env token
// wins; loopback binds auto-generate (printed to stderr by the caller);
// non-loopback binds without env fail fast (ADR 0011 L1).
func resolveAdminToken(bind string) (string, bool, error) {
	token, fromEnv := admin.ResolveToken()
	if err := admin.CheckBindSafety(bind, token); err != nil {
		return "", false, err
	}
	if !fromEnv {
		var err error
		if token, err = admin.GenerateToken(); err != nil {
			return "", false, fmt.Errorf("generate admin token: %w", err)
		}
		admin.MustPrintAutoGeneratedToken(token, true)
	}
	return token, fromEnv, nil
}
```

`Run` 内（daemon.go:209 之前）：

```go
adminToken, _, err := resolveAdminToken(opts.Bind)
if err != nil {
	return err
}
```

并把 `Token: ""` 行改为 `Token: adminToken,`（删除误导注释）。

- [ ] **Step 4: 运行测试确认通过 + 手工冒烟**

Run: `go test ./internal/daemon/ -v && go build ./...`
手工冒烟（验证 401 缺陷消除）：
```bash
go build -o /tmp/huan-t1 ./cmd/huan
mkdir -p /tmp/t1site/content/posts && cd /tmp/t1site
printf 'baseURL: http://127.0.0.1:1/\ntitle: t\nlanguageCode: zh-cn\npublishDir: public\n' > huan.yaml
printf -- '---\ntitle: A\n---\nbody\n' > content/posts/a.md
(HUAN_ADMIN_TOKEN=fixedtok /tmp/huan-t1 daemon -s . --port 13199 --bind 127.0.0.1 &>/tmp/t1.log &); sleep 4
curl -s -H "Authorization: Bearer fixedtok" http://127.0.0.1:13199/admin/api/status | head -c 80
# 期望不再是 {"error":"invalid admin token"} 而是 status JSON
pkill -f "huan-t1 daemon"; rm -rf /tmp/t1site /tmp/huan-t1 /tmp/t1.log
```

- [ ] **Step 5: 提交**

```bash
git add internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "fix(daemon): wire admin token from env or auto-generate (was hardcoded empty)"
```

---

### Task 2: Fixture 站点 minimal + FIXTURES.md 已知状态表

**Files:**
- Create: `tests/e2e/fixtures/minimal/huan.yaml`
- Create: `tests/e2e/fixtures/minimal/layouts/_default/single.html`
- Create: `tests/e2e/fixtures/minimal/layouts/_default/list.html`
- Create: `tests/e2e/fixtures/minimal/layouts/index.html`
- Create: `tests/e2e/fixtures/minimal/content/posts/hello-huan.md`
- Create: `tests/e2e/fixtures/minimal/content/posts/draft-post.md`
- Create: `tests/e2e/fixtures/minimal/content/about.md`
- Create: `tests/e2e/fixtures/minimal/static/logo.svg`
- Create: `tests/e2e/fixtures/minimal/archetypes/default.md`
- Create: `tests/e2e/fixtures/FIXTURES.md`

**Interfaces:**
- Consumes: 无
- Produces: fixture 名 `minimal`；已知状态常量（后续所有用例断言引用）：total=3、drafts=1、published=2、sections=2（posts+about 所在根 section）、公开 API posts 条数=1（draft 排除）。文件锚点：`posts/hello-huan.md`（title=你好 huan，tags=[go, e2e]）、`posts/draft-post.md`（draft=true）、`about.md`（title=关于）。

- [ ] **Step 1: 写 huan.yaml**

```yaml
baseURL: http://127.0.0.1:1313/
title: huan E2E 最小站
languageCode: zh-cn
publishDir: public
paginate: 10
hasCJKLanguage: true
```

- [ ] **Step 2: 写 3 篇内容 + archetype + static**

`content/posts/hello-huan.md`：
```markdown
---
title: 你好 huan
date: 2026-08-10
tags: [go, e2e]
description: 第一篇正式文章
---
# 你好 huan

这是 **huan** E2E 测试固定数据的第一篇文章。
```

`content/posts/draft-post.md`：
```markdown
---
title: 草稿文章
date: 2026-08-11
draft: true
tags: [go]
---
还是草稿，不应出现在公开输出。
```

`content/about.md`：
```markdown
---
title: 关于
date: 2026-08-01
---
关于本测试站点。
```

`archetypes/default.md`：
```markdown
---
title: {{ .Name }}
date: {{ .Date }}
draft: true
---
```

`static/logo.svg`：
```xml
<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="#36c"/></svg>
```

- [ ] **Step 3: 写最小 layouts（探针验证过的形态）**

`layouts/_default/single.html`：
```html
<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>{{ .Title }} - {{ .Site.Title }}</title></head>
<body><h1 id="page-title">{{ .Title }}</h1><article>{{ .Content }}</article></body></html>
```

`layouts/_default/list.html`：
```html
<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>{{ .Title }} - {{ .Site.Title }}</title></head>
<body><h1>{{ .Title }}</h1>{{ range .Pages }}<a class="post-link" href="{{ .RelPermalink }}">{{ .Title }}</a>{{ end }}</body></html>
```

`layouts/index.html`：
```html
<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>{{ .Site.Title }}</title></head>
<body><h1>{{ .Site.Title }}</h1>{{ range .Site.RegularPages }}<a class="post-link" href="{{ .RelPermalink }}">{{ .Title }}</a>{{ end }}</body></html>
```

- [ ] **Step 4: 实测校准 FIXTURES.md 常量（关键步骤——先跑再写表）**

```bash
go build -o /tmp/huan-t2 ./cmd/huan
rm -rf /tmp/t2 && cp -r tests/e2e/fixtures/minimal /tmp/t2
/tmp/huan-t2 build -s /tmp/t2 2>&1 | tail -3   # 期望 Rendered ≥3、Errors: 0
ls /tmp/t2/public/posts/hello-huan/index.html /tmp/t2/public/about/index.html /tmp/t2/public/index.html
/tmp/huan-t2 dev -s /tmp/t2 --port 13202 &>/tmp/t2.log & sleep 3
TOK=$(grep -oE '[0-9a-f]{32}' /tmp/t2.log | head -1)
curl -s -H "Authorization: Bearer $TOK" http://127.0.0.1:13202/admin/api/status
pkill -f "huan-t2 dev"
```
按实际 JSON 校准 total/drafts/sections/languages/mediaCount/sectionBreakdown 的精确值。

- [ ] **Step 5: 写 FIXTURES.md（用校准后的真实数字）**

结构：
```markdown
# Fixture 已知状态表

用例断言直接引用本表常量；fixture 内容变更必须同步本表并重跑相关套件。

## minimal
- 文件清单：…（列全部内容文件与模板）
- GET /admin/api/status 期望：total=3, published=2, drafts=1, sections=2, languages=[], mediaCount=1, sectionBreakdown={posts:2, …}
- 公开 API（daemon）：/api/v1/pages total=2（draft 排除后）…（按实测校准）
- build 产物锚点：public/posts/hello-huan/index.html 存在且 h1=「你好 huan」；public/posts/draft-post/ 不存在；public/logo.svg 存在
```

（后续 Task 3/4 往同一文件追加 multilang / with-plugins 小节。）

- [ ] **Step 6: 提交**

```bash
git add tests/e2e/fixtures/
git commit -m "test(e2e): add minimal fixture site with known-state table"
```

---

### Task 3: Fixture multilang + with-plugins

**Files:**
- Create: `tests/e2e/fixtures/multilang/huan.yaml`
- Create: `tests/e2e/fixtures/multilang/layouts/_default/single.html`（内容同 minimal，复制）
- Create: `tests/e2e/fixtures/multilang/layouts/_default/list.html`（同上）
- Create: `tests/e2e/fixtures/multilang/layouts/index.html`（同上）
- Create: `tests/e2e/fixtures/multilang/content/posts/hello.zh-cn.md`
- Create: `tests/e2e/fixtures/multilang/content/posts/hello.en.md`
- Create: `tests/e2e/fixtures/multilang/content/posts/zh-only.zh-cn.md`
- Create: `tests/e2e/fixtures/with-plugins/huan.yaml`
- Create: `tests/e2e/fixtures/with-plugins/layouts/`（复制 minimal 三模板）
- Create: `tests/e2e/fixtures/with-plugins/content/posts/seo-post.md`
- Modify: `tests/e2e/fixtures/FIXTURES.md`（追加两节）

**Interfaces:**
- Consumes: Task 2 的 FIXTURES.md 结构与 minimal 模板。
- Produces: fixture 名 `multilang`（已知状态：zh-cn 2 篇 + en 1 篇；`GET /admin/api/content/posts/hello.zh-cn.md/languages` 返回 siblings 含 `hello.en.md`）与 `with-plugins`（huan.yaml 声明 seo-injector + sitemap-enhancer；build 产物 head 含 seo 注入的 meta）。

- [ ] **Step 1: multilang huan.yaml + 内容**

```yaml
baseURL: http://127.0.0.1:1313/
title: huan E2E 多语言站
languageCode: zh-cn
publishDir: public
languages:
  zh-cn:
    weight: 1
    languageName: 中文
  en:
    weight: 2
    languageName: English
    baseURL: /en/
```

`hello.zh-cn.md`（frontmatter title=你好（中文版），tags=[i18n]）、`hello.en.md`（title=Hello (EN)）、`zh-only.zh-cn.md`（title=仅中文，无 en 版）。正文各 1 行。

- [ ] **Step 2: with-plugins huan.yaml + 内容**

```yaml
baseURL: http://127.0.0.1:1313/
title: huan E2E 插件站
languageCode: zh-cn
publishDir: public
plugins:
  - name: seo-injector
    category: seo
  - name: sitemap-enhancer
    category: seo
```

`content/posts/seo-post.md`（title=SEO 验证文章，date=2026-08-12，tags=[seo]，正文 1 段）。

- [ ] **Step 3: 实测校准两个 fixture**

```bash
go build -o /tmp/huan-t3 ./cmd/huan
for f in multilang with-plugins; do
  rm -rf /tmp/t3-$f && cp -r tests/e2e/fixtures/$f /tmp/t3-$f
  /tmp/huan-t3 build -s /tmp/t3-$f 2>&1 | tail -2
done
ls /tmp/t3-multilang/public/en/posts/hello/index.html   # 多语言产物锚点
grep -l "og:" /tmp/t3-with-plugins/public/posts/seo-post/index.html 2>/dev/null || echo "注：seo 注入形态按实际产物校准 FIXTURES.md 措辞"
```
注意：seo-injector 是编译期内置插件（plugins/ 下有源码），无需 .so。若注入产物与预期字段不符，按实测写锚点，不虚构。

- [ ] **Step 4: FIXTURES.md 追加两节**（含 languages 端点期望、en/ 产物锚点、seo meta 锚点——全部用 Step 3 实测值）

- [ ] **Step 5: 提交**

```bash
git add tests/e2e/fixtures/
git commit -m "test(e2e): add multilang and with-plugins fixtures"
```

---

### Task 4: Runbook（RUNBOOK.md + patterns.md）

**Files:**
- Create: `tests/e2e/runbooks/RUNBOOK.md`
- Create: `tests/e2e/runbooks/patterns.md`
- Create: `tests/e2e/README.md`

**Interfaces:**
- Consumes: Task 2/3 的 fixture 名与 FIXTURES.md。
- Produces: 保留变量契约 `${bin}`、`${site}`、`${port}`、`${base}`、`${token}`、`${run_id}`、`${artifacts}`；环境生命周期命令（setup/start/stop/teardown）；判定规则；报告模板路径 `docs/reports/`。后续所有 YAML 用例隐式依赖本任务的变量契约与 severity 语义（P0=套件判负停止、P1=记录继续、P2=仅记录）。

- [ ] **Step 1: 写 RUNBOOK.md**

必须包含（全中文，命令可复制执行）：

1. **环境准备**：`go build -o /tmp/huan-e2e/huan ./cmd/huan`；`run_id=$(date +%Y%m%d-%H%M%S)`；`site=/tmp/huan-e2e/sites/$run_id-<fixture>`（`cp -r tests/e2e/fixtures/<fixture> $site`）；`artifacts=/tmp/huan-e2e/artifacts/$run_id`（mkdir -p）
2. **端口分配**：从 13200 起，每套件 +10，避免冲突；写明每个套件的固定端口表
3. **dev 启动与 token 捕获**：`$bin dev -s $site --port $port --bind 127.0.0.1 &> $artifacts/dev.log &`；`sleep 3`；`token=$(grep -oE '[0-9a-f]{32}' $artifacts/dev.log | head -1)`；探活 `curl -s http://127.0.0.1:$port/ | head -c 1`（dev 无 /health）
4. **daemon 启动**（SSE/公开 API 用例用）：同上但命令为 `daemon`，探活 `curl -s http://127.0.0.1:$port/health`；token 同法（Task 1 修复后 stderr 有打印；env 固定 token 时 `HUAN_ADMIN_TOKEN=e2e-fixed` 直接用）
5. **判定规则**：P0 失败→套件判负停止；P1 失败→记录继续；P2→仅记录。verify 失败必须先截图（agent-browser `screenshot`）到 `$artifacts` 再判负
6. **报告**：结果表写入 `docs/reports/e2e/<date>-<module>.md`：每用例一行（id/结果/耗时/证据路径）+ 汇总（通过率/P0 状态/bug 列表）。对接 `docs/templates/` 模板风格
7. **teardown**：`pkill -f "huan-e2e/huan"`；临时目录按 run_id 可整体删除（报告引用的截图先拷出）
8. **api-probe 通则**：curl 示例带 `-s -w '\n%{http_code}'` 同时拿 body 和 status；JSON 断言用 `python3 -c` 或 `jq`（按环境可用性写两种）

- [ ] **Step 2: 写 patterns.md**

至少 6 个模式，每个给完整可复制命令：
- **SSE 监听**（macOS 无 timeout，用 `curl -sN --max-time 5`）：`curl -sN --max-time 5 http://127.0.0.1:$port/api/v1/events | head -8`，说明事件行格式 `event: <type>` / `data: {...}`、心跳为 `: keepalive` 注释行
- **WS 握手**（livereload）：用 agent-browser 打开页面后观察刷新，或 `curl -s -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" http://127.0.0.1:$port/livereload --max-time 3 | head -5` 断言 101
- **并发 PUT**：`for i in 1 2 3 4 5; do curl -s -X PUT ... & done; wait` 然后单 GET 断言内容为其中之一 + 审计行数
- **mtime 对比**（增量构建）：`stat -f %m`（macOS）前后对比未变文件
- **审计断言**：`grep "content.create" $site/memory/daily/$(date +%Y-%m-%d).md`，说明 AuditRecord 行含 action 与 sha256 前 8 位
- **upload multipart**：`curl -s -F "file=@tests/e2e/fixtures/minimal/static/logo.svg" -H "Authorization: Bearer $token" http://127.0.0.1:$port/admin/api/media`

- [ ] **Step 3: 写 README.md（体系入口）**

~40 行：目录结构图、三类用例读法（先读 `_schema.md` 再读 YAML）、执行入口（指向 runbooks/RUNBOOK.md）、与 Go 单测的边界（`go test ./...` 管单元/集成；本体系管 E2E 验收）。

- [ ] **Step 4: 冒烟 runbook 命令**

按 RUNBOOK.md 逐步复制执行 minimal 的 setup/start/token 捕获/status 探活/teardown，确认无一步失效。

- [ ] **Step 5: 提交**

```bash
git add tests/e2e/README.md tests/e2e/runbooks/
git commit -m "test(e2e): add runbook, assertion patterns, and README"
```

---

### Task 5: API schema 文档（api/_schema.md）

**Files:**
- Create: `tests/e2e/api/_schema.md`

**Interfaces:**
- Consumes: spec §3。
- Produces: 七种 step 动作类型（`http` / `fs.assert` / `fs.write` / `fs.delete` / `cli` / `sse.subscribe` / `wait` / `var`——注意 fs.write 与 fs.delete 各算一种，共 8 个动词）的完整字段字典；套件级字段（suite/module/fixture/runtime/startup/tests）；用例级字段（id/name/severity/preconditions/steps/cleanup）；两条硬规则的机器可检查描述。后续 Task 6-13 所有 YAML 按本 schema 写。

- [ ] **Step 1: 写 _schema.md**

内容（结构，全文中文）：
- 套件骨架 YAML 示例（spec §3 原文示例直接复用）
- 每种动作一节：字段表 + 最小示例 + 断言语义（如 `expect.body` 用点路径 `frontmatter.title`；数组断言 `bodyContains`；正则 `matches`）
- `expect.status` 必填；`expect.body` 至少一键或 `body_absent: true`（硬规则 1）
- 写操作后必跟 `fs.assert`（硬规则 2）
- `${token}`/`${port}`/`${base}` 变量替换说明（指向 RUNBOOK）
- severity 判定语义引用 RUNBOOK
- runtime 三值：`daemon` / `dev` / `build-only` 的端口表引用

- [ ] **Step 2: 提交**

```bash
git add tests/e2e/api/_schema.md
git commit -m "test(e2e): add API suite schema reference"
```

---

### Task 6: API 套件 4 个——auth / status / content-crud / media

**Files:**
- Create: `tests/e2e/api/auth.yaml`
- Create: `tests/e2e/api/status.yaml`
- Create: `tests/e2e/api/content-crud.yaml`
- Create: `tests/e2e/api/media.yaml`

**Interfaces:**
- Consumes: Task 5 schema；Task 2 FIXTURES.md 常量；Task 4 变量契约。
- Produces: 用例 id 前缀 `auth-`（6 例）、`stat-`（4 例）、`crud-`（14 例含并发 2 例）、`med-`（8 例）。供 Task 16 验收报告汇总。

- [ ] **Step 1: 写 auth.yaml（6 例）**

按「探针实测事实」表逐条写。示例第 1 例（其余同密度）：

```yaml
suite: auth
module: admin-auth
fixture: minimal
runtime: dev
tests:
  - id: auth-001
    name: 正确 token 通过 Bearer 头访问 status
    severity: P0
    steps:
      - action: http
        method: GET
        path: /admin/api/status
        headers: { Authorization: "Bearer ${token}" }
        expect: { status: 200, body: { total: 3 } }   # FIXTURES.md minimal 常量
  - id: auth-002
    name: 缺失 token 返回 401 与 WWW-Authenticate
    severity: P0
    steps:
      - action: http
        method: GET
        path: /admin/api/status
        expect:
          status: 401
          body: { error: "missing admin token" }
          headers: { WWW-Authenticate: 'Bearer realm="huan-admin"' }
  # auth-003 错误 token → 401 {"error":"invalid admin token"}
  # auth-004 X-Huan-Admin-Token 头等价（同 auth-001 断言）
  # auth-005 loopback 自动 token（启动 stderr 可捕获 32-hex，即 ${token} 非空这一事实本身）
  # auth-006 非 loopback bind 无 HUAN_ADMIN_TOKEN → 进程退出非零、stderr 含 "requires HUAN_ADMIN_TOKEN"
```

（写文件时把注释行展开为完整用例，不留 `# 同上`。）

- [ ] **Step 2: 写 status.yaml（4 例）**

stat-001 初始统计=FIXTURES 常量全字段；stat-002 创建 1 篇后 total+1/drafts 不变（前置 crud 能力内联：POST 后立即断言）；stat-003 recentContent 按 date 降序（创建新文章后它排第一）；stat-004 mediaCount 随上传 +1（内联 POST media）。

- [ ] **Step 3: 写 content-crud.yaml（14 例）**

覆盖：crud-001 创建+落盘+回读（spec §3 示例的完整版，`relPath: posts/crud-001.md` 断言）；002 缺 title→400；003 非法 JSON→400 `invalid JSON`；004 GET 不存在→404；005 PUT 更新 frontmatter+rawContent→落盘 contains 双断言；006 DELETE→200+fs.assert not exists；007 重复创建同 filename（覆盖或错误——**实测后定**，先写探针命令：`curl -X POST ... 同 filename`，按实际行为写断言并回填本文件）；008 draft 文件创建后 status.drafts+1；009 languages 查询（runtime 覆盖 fixture: multilang）；010 审计断言：crud-001 后 `$site/memory/daily/*.md` contains `content.create` 与 sha 片段；011 title 含特殊字符（引号/中文/emoji）；012 filename 不带 .md 自动补全；013 并发 PUT×5 最后写赢+审计多行；014 创建后立即 DELETE 再 GET→404（竞争终态一致）。

- [ ] **Step 4: 写 media.yaml（8 例）**

med-001 上传 logo.svg→201 `{name: logo-e2e.svg, path: logo-e2e.svg}` + fs.assert static/logo-e2e.svg；002 列表含新文件；003 删除→200+fs not exists；004 删不存在→非 200（实测：os.Remove 失败→500，按实际写）；005 不支持扩展名 .exe→400 `unsupported file type`；006 multipart 缺 file 字段→400 `missing file`；007 二进制上传 PNG 魔数（先生成 1x1 PNG：patterns.md 提供 printf 命令）后 fs.assert 前八字节；008 dir 子目录上传→path 含子目录。

- [ ] **Step 5: 校验 YAML 语法 + 实跑抽样**

```bash
python3 -c "import yaml,glob; [yaml.safe_load(open(f)) for f in glob.glob('tests/e2e/api/*.yaml')]; print('yaml ok')"
# 实跑抽样（按 RUNBOOK 起 minimal/dev）：
#   crud-001、med-001、auth-002 逐条 curl 执行，断言与文件一致；不一致改文件（以实测为准）
```

- [ ] **Step 6: 提交**

```bash
git add tests/e2e/api/auth.yaml tests/e2e/api/status.yaml tests/e2e/api/content-crud.yaml tests/e2e/api/media.yaml
git commit -m "test(e2e): add auth/status/content-crud/media API suites"
```

---

### Task 7: API 套件 3 个——settings / build-trigger / public-api

**Files:**
- Create: `tests/e2e/api/settings.yaml`
- Create: `tests/e2e/api/build-trigger.yaml`
- Create: `tests/e2e/api/public-api.yaml`

**Interfaces:**
- Consumes: 同 Task 6。
- Produces: 用例 id 前缀 `set-`（9 例）、`bld-`（7 例）、`pub-`（12 例）。

- [ ] **Step 1: 写 settings.yaml（9 例）**

set-001 GET settings 200 含 title/baseURL；002 PUT 改 title→200 saved + fs.assert huan.yaml contains 新 title + GET 回读一致；003 GET settings/yaml 是 text/plain 且含 `title:`；004 PUT settings/yaml 合法内容→saved+落盘；005 PUT settings/yaml 非法 YAML→400；006 非法 JSON body→400；007 settings 与 settings/yaml 交叉一致（改结构化后 yaml 端点也变）；008 PUT settings 触发 rebuild（wait for file-change 后新 title 出现在构建产物 index.html）；009 审计 settings.update 行存在。

- [ ] **Step 2: 写 build-trigger.yaml（7 例，runtime: dev）**

bld-001 POST build→202 `{"status":"rebuild triggered"}`（注意：POST 无落盘副作用，硬规则 2 例外——但必须在 expect 里显式写 `side_effect: async-rebuild` 并跟 wait step）；002 wait 构建完成后 publishDir 新页出现（先 POST content 再 POST build）；003 增量：改 1 篇 → 该页 index.html mtime 变、另一篇不变（mtime 命令在 patterns.md）；004 draft 不出现在公开产物（创建 draft→build→fs.assert public/posts/<draft>/ not exists）；005 模板变更→全量回退（改 layouts 后所有页 mtime 变）；006 JIT：未构建路径直接 GET 返回渲染 HTML（dev 模式 GET /posts/hello-huan/）；007 构建后 sitemap.xml 含新页 URL。

- [ ] **Step 3: 写 public-api.yaml（12 例，runtime: daemon——公开 API 只挂载在 daemon）**

pub-001 GET /api/v1/pages 200 `{data,total,page,limit}` 结构；002 默认 limit=10；003 limit=51 clamp 到 50（`limit=50` 回读）；004 page=99999 越界返回空 data 不 panic；005 page=9223372036854775807（MaxInt64）→ 200 空 data（历史 DoS 回归，实测若行为不同以实测为准并注明）；006 section=posts 过滤；007 tag 过滤；008 q 过滤命中标题；009 q 201 个中文字符→400 `query too long`（rune 计数回归）；010 GET /api/v1/pages/posts/hello-huan/ 200；011 不存在 URL→404 `{"error":"not found"}`；012 POST /api/v1/pages→405 `method not allowed`。

注意：daemon 公开 API 的数据源是构建产物 `/api/*.json`（`contentindex/index.go:56`），用例前置必须有初始构建——daemon 启动自动全量构建（实测 daemon.err 有 `initial full build`），直接可用。

- [ ] **Step 4: YAML 校验 + 实跑抽样（set-002、bld-004、pub-009、pub-005）**

- [ ] **Step 5: 提交**

```bash
git add tests/e2e/api/settings.yaml tests/e2e/api/build-trigger.yaml tests/e2e/api/public-api.yaml
git commit -m "test(e2e): add settings/build-trigger/public-api suites"
```

---

### Task 8: API 套件 3 个——sse-events / plugins-admin / livereload

**Files:**
- Create: `tests/e2e/api/sse-events.yaml`
- Create: `tests/e2e/api/plugins-admin.yaml`
- Create: `tests/e2e/api/livereload.yaml`

**Interfaces:**
- Consumes: 同 Task 6；Task 1 的 daemon token 修复（sse 套件用 daemon）。
- Produces: 用例 id 前缀 `sse-`（5 例）、`plug-`（9 例）、`lr-`（4 例）。

- [ ] **Step 1: 写 sse-events.yaml（5 例，runtime: daemon）**

sse-001 订阅即收 `Content-Type: text/event-stream`（headers 断言）；002 心跳：--max-time 20 内收到 `:` 开头注释行（写明这是 15s 心跳）；003 触发 build（`$bin` 无法直连 daemon 内部 rebuild——用 POST /admin/api/build，token 走 daemon 流程）→收到 build 类事件；004 内容文件 fs.write→收到 content 事件；005 插件事件（daemon plugins load .so→plugin 事件；若实测 daemon 无 .so 可加载则降级为「记录事件类型清单」并在 cleanup 说明，以实测为准）。

- [ ] **Step 2: 写 plugins-admin.yaml（9 例，fixture: with-plugins，runtime: dev）**

plug-001 GET plugins 200 `{plugins:[...], total: N}`（N 按 FIXTURES.md with-plugins 实测）；002 load 缺 path→400；003 load 不存在 .so→500（实测确认，错误体含 error）；004 load 合法 .so→200 `{status: loaded, plugin: {name: ...}}`——.so 用 `$HUAN_HOME/plugins` 下的现成产物或 `go build -buildmode=plugin` 现编译 `internal/plugin/testdata/simple_plugin/`（patterns.md 加编译命令：`go build -buildmode=plugin -o $site/plugins/simple-plugin.so ./internal/plugin/testdata/simple_plugin`）；005 unload 后 list 不含该名；006 reload→200；007 name 冲突二次 load→409；008 theme GET 列表 200 `{themes, active}`；009 theme activate/deactivate 旅程（若无可用 theme 插件，断言错误响应结构并注明现状，以实测为准）。

- [ ] **Step 3: 写 livereload.yaml（4 例，runtime: dev）**

lr-001 /livereload WS 升级 101（patterns.md 的 curl 握手命令）；002 握手后收到 hello（`command: hello` + `official-7` 协议串；说明：curl 不能完整说 WS 帧，此条注明「由 browser 套件 site-rendering 的 LiveReload 旅程做端到端覆盖，此处仅握手层」或用 python3 websocket 脚本——patterns.md 给 8 行 python3 脚本）；003 fs.write 内容文件→广播 reload（同上脚本接收）；004 GET /livereload.js 200 且是 JS。

- [ ] **Step 4: YAML 校验 + 实跑抽样（sse-003、plug-004、lr-001）**

- [ ] **Step 5: 提交**

```bash
git add tests/e2e/api/sse-events.yaml tests/e2e/api/plugins-admin.yaml tests/e2e/api/livereload.yaml
git commit -m "test(e2e): add sse/plugins/livereload API suites"
```

---

### Task 9: Browser schema 文档（browser/_schema.md）

**Files:**
- Create: `tests/e2e/browser/_schema.md`

**Interfaces:**
- Consumes: spec §5。
- Produces: 七种 browser 动作（`goto` / `enter-token` / `interact` / `verify` / `screenshot` / `wait` / `api-probe`）字段字典；journey/browser_defaults 套件字段；可靠性规则（verify 断言用用户可见内容、DOM 只作 hints、失败先截图）。api-probe 复用 `tests/e2e/api/_schema.md` 的 expect 语法。Task 10-11 依赖。

- [ ] **Step 1: 写 _schema.md**（含 spec §5 的完整示例用例，字段表：每个动作的必填/可选字段、hints 语义「提示非契约」、screenshot 命名 `<case-id>-milestone-<n>`、wait 的三种条件）
- [ ] **Step 2: 提交**

```bash
git add tests/e2e/browser/_schema.md
git commit -m "test(e2e): add browser journey schema reference"
```

---

### Task 10: Browser 套件 3 个——admin-login / admin-content-crud / admin-settings

**Files:**
- Create: `tests/e2e/browser/admin-login.yaml`
- Create: `tests/e2e/browser/admin-content-crud.yaml`
- Create: `tests/e2e/browser/admin-settings.yaml`

**Interfaces:**
- Consumes: Task 9 schema；Task 4 的 `${token}`/`${port}`。
- Produces: 旅程 id 前缀 `lg-`（4）、`bc-`（6）、`bs-`（4）。

- [ ] **Step 1: 写 admin-login.yaml（4 旅程）**

lg-001 首次进入→token 弹层→enter-token→verify 面板可见（expect：「显示站点标题 huan E2E 最小站」「统计卡显示 total=3」；hints：「输入框 input[type=password] 或 placeholder 含 token」）；lg-002 错 token→verify「仍停留 token 界面或出现错误提示」+ screenshot；lg-003 登录后刷新页面→verify 会话保持（sessionStorage，hints: `sessionStorage.huan_admin_token`）；lg-004 直接调 API 制造 401（api-probe）→浏览器回到 token 弹层。

- [ ] **Step 2: 写 admin-content-crud.yaml（6 旅程）**

bc-001 列表浏览（树含 posts/about、统计数字）；bc-002 新建→interact「点击新建/创建按钮」「填写标题 e2e-browser-新建」→verify 列表出现新标题→api-probe GET 该文件 200；bc-003 编辑：改标题+正文→保存→api-probe 回读一致+fs.assert；bc-004 删除→verify 列表消失→api-probe 404；bc-005 draft 开关旅程（切换后 api-probe status.drafts 变化）；bc-006 多语言 siblings（fixture 覆盖 multilang，verify 语言切换 UI 或 languages API 旁证）。

- [ ] **Step 3: 写 admin-settings.yaml（4 旅程）**

bs-001 设置页字段回显（title 输入框值为 fixture 标题）；bs-002 改标题保存→api-probe GET settings 回读新值；bs-003 原始 YAML 编辑保存→fs.assert huan.yaml；bs-004 非法值（YAML 粘贴破坏语法）→verify 前端报错提示+落盘不变（fs.assert contains 旧 title）。

- [ ] **Step 4: 用 agent-browser 实走 lg-001 与 bc-002 校准 expect/hints**（按实际 SPA 文案微调，DOM 仍只进 hints）

- [ ] **Step 5: 提交**

```bash
git add tests/e2e/browser/admin-login.yaml tests/e2e/browser/admin-content-crud.yaml tests/e2e/browser/admin-settings.yaml
git commit -m "test(e2e): add admin login/content/settings browser journeys"
```

---

### Task 11: Browser 套件 2 个——admin-plugins / site-rendering

**Files:**
- Create: `tests/e2e/browser/admin-plugins.yaml`
- Create: `tests/e2e/browser/site-rendering.yaml`

**Interfaces:**
- Consumes: 同 Task 10；site-rendering 用 dev 模式前台（无 token）。
- Produces: 旅程 id 前缀 `bp-`（3）、`sr-`（5）。

- [ ] **Step 1: 写 admin-plugins.yaml（3 旅程，fixture: with-plugins）**

bp-001 插件列表页加载（verify 插件条目数=FIXTURES with-plugins 值）；bp-002 load 旅程（若 SPA 有 load 入口则 interact 上传/输入 .so 路径；无入口则改 verify 列表展示状态字段并注明）；bp-003 unload/reload 旅程（同上原则，以 SPA 实际能力为准，用例注明覆盖范围）。

- [ ] **Step 2: 写 site-rendering.yaml（5 旅程，runtime: dev）**

sr-001 首页渲染（verify `.post-link` 可见文本=「你好 huan」——用户可见文本表述 + hint 选择器）；sr-002 文章页（goto /posts/hello-huan/，verify h1=你好 huan 且正文含加粗「huan」）；sr-003 404（goto /not-exist/，verify 404 页或错误文案）；sr-004 tag 页或分类页（fixture tags go/e2e，按实际路由 verify）；sr-005 LiveReload：开页面→fs.write 改文件→wait reload→verify 新内容出现（两页并开版本：第二 tab 同 URL，双 verify）。

- [ ] **Step 3: 用 agent-browser 实走 sr-001/sr-002/sr-005 校准**

- [ ] **Step 4: 提交**

```bash
git add tests/e2e/browser/admin-plugins.yaml tests/e2e/browser/site-rendering.yaml
git commit -m "test(e2e): add plugins/site-rendering browser journeys"
```

---

### Task 12: CLI schema 说明 + 3 个套件 + 排除清单

**Files:**
- Create: `tests/e2e/cli/_schema.md`
- Create: `tests/e2e/cli/build.yaml`
- Create: `tests/e2e/cli/new.yaml`
- Create: `tests/e2e/cli/config.yaml`
- Create: `tests/e2e/cli/version.yaml`
- Create: `tests/e2e/cli/_external-deps.md`

**Interfaces:**
- Consumes: api/_schema.md 的 `cli` 与 `fs.assert` 动作（_schema.md 只写差异：套件 runtime 固定 `build-only`、无 http 动作）。
- Produces: 用例 id 前缀 `cb-`（8）、`cn-`（3）、`cc-`（3）、`cv-`（1）。

- [ ] **Step 1: 写 _schema.md（差异文档，~30 行）**

- [ ] **Step 2: 写 build.yaml（8 例）**

cb-001 正常构建（断言 stdout 含 `Rendered:` ≥2 与 `Errors: 0` + fs.assert 3 个锚点文件）；002 默认不含 draft（fs.assert public/posts/draft-post/ not exists）；003 `-D` 含 draft（exists）；004 `-F`/`-E` 各一例（fixture 补 1 篇 future + 1 篇 expired 内容，通过套件 startup 的 fs.write 注入，不进 fixture）；005 增量（二次 build，改单文件，断言仅相关产物 mtime 变——同 bld-003 手法）；006 `--minify` 产物单行化（fs.assert matches `<html><head>` 无换行模式）；007 缺 huan.yaml（临时空目录）→退出码非零；008 multilang 构建产物（fs.assert public/en/posts/hello/index.html）。

- [ ] **Step 3: 写 new.yaml + config.yaml + version.yaml（7 例）**

cn-001 `huan new posts/created-by-e2e.md -s $site`→fs.assert 文件存在+contains `title:`；002 重名→非零退出+`file already exists`；003 无 archetype 时内置默认（fs.assert 含 date 行）。cc-001 合法配置输出 YAML 含 title；002 非法 YAML→非零退出；003 `${VAR:-default}` 插值（startup fs.write 带 `baseURL: ${E2E_URL:-http://fallback/}` 的 huan.yaml，断言输出含 fallback）。cv-001 `huan version` stdout matches `^huan \d+\.\d+\.\d+`。

- [ ] **Step 4: 写 _external-deps.md**

排除清单：translate（需 qwen3 端点/AI）、deploy（需 Cloudflare 凭据）、sync、release（打包副作用大）、daemon 的 tls/systemd flags（需 systemd 环境）。每项一句原因 + 「何时补测」条件。

- [ ] **Step 5: 实跑抽样（cb-001、cb-007、cc-003）+ YAML 校验 + 提交**

```bash
git add tests/e2e/cli/
git commit -m "test(e2e): add CLI suites (build/new/config/version) and exclusion list"
```

---

### Task 13: 全量验证与文档收尾

**Files:**
- Create: `docs/reports/e2e/2026-08-15-system-baseline.md`（首次全量基线报告）
- Modify: `docs/progress/CURRENT_STATE.md`（追加 E2E 体系小节）
- Modify: `memory/MEMORY.md`（项目上下文追加）
- Modify: `memory/daily/2026-08-15.md`（追加当日进度）

**Interfaces:**
- Consumes: 全部前序任务产物。
- Produces: 基线报告（每套件汇总行 + 发现的 bug 列表），是后续回归的对照基线。

- [ ] **Step 1: 全量 YAML 语法校验**

```bash
python3 -c "import yaml,glob; [yaml.safe_load(open(f)) for f in glob.glob('tests/e2e/**/*.yaml', recursive=True)]; print('all yaml ok')"
```

- [ ] **Step 2: 按 RUNBOOK 实跑全部 10 个 API 套件抽样用例 + 全部 CLI 套件**（每套件至少 P0 全跑、其余抽 2 例；发现断言与实测不符→改 YAML 并注明「按实测校准」）

- [ ] **Step 3: 用 agent-browser 实跑 browser 套件的全部 P0 旅程**（lg-001、bc-001、bc-002、sr-001、sr-002）

- [ ] **Step 4: 写基线报告**（结构：环境/执行范围/每套件结果表/bug 清单——已知至少 1 项：Task 1 修复前的 daemon token 缺陷，记录为已修复回归项）

- [ ] **Step 5: 更新 CURRENT_STATE.md / MEMORY.md / 当日 daily**（E2E 体系一句话 + 目录指引）

- [ ] **Step 6: 提交**

```bash
git add docs/reports/e2e/ docs/progress/CURRENT_STATE.md memory/
git commit -m "docs: add E2E system baseline report and update project docs"
```

---

## Self-Review 结果

1. **Spec 覆盖**：spec §2 目录（Task 2-12 全部文件）、§3-4 API schema+10 套件（Task 5-8）、§5 browser schema+5 文件（Task 9-11）、§6 CLI（Task 12）、§7 fixture（Task 2-3）、§8 runbook（Task 4）、§9 验收流程（Task 4 RUNBOOK + Task 13 报告落 docs/reports/）——全覆盖。spec 说「~78 API 用例」，本计划 6+14+4+8+9+7+12+5+9+4=78，一致；browser 4+6+4+3+5=22，一致；CLI 8+3+3+1=15，一致。
2. **占位符**：无 TBD/TODO；「以实测为准」处均附带实测命令与按实测回填的明确指令（这是校准程序而非占位）。
3. **类型一致性**：fixture 名、变量名（`${token}` 等）、severity 值、id 前缀在任务间一致；Task 1 修复依赖的 admin 函数签名逐一与 auth.go 核对过。
4. **计划修正说明**：探针实测发现 spec 的「daemon 承载 admin API 用例」假设不成立（token 缺陷），已在 Global Constraints 修正为 dev 承载 + Task 1 修复，spec 不需改（spec 未指定 runtime 细节到端点级）。
