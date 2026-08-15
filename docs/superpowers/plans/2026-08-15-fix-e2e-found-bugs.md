# 修复 E2E 发现的引擎遗留 Bug Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 E2E 体系发现的 3 个引擎 bug（draft 聚合泄露 / en 文章 admin API 路径错 / dev 插件面 200-vs-503 + daemon build 缺 PluginRegistry），同步 FIXTURES.md 与 E2E 套件断言并回归。

**Architecture:** 每个 bug 一个 Task，TDD（先写失败测试再修），修完立刻回归对应 E2E 用例并同步文档。Bug4c（$HUAN_HOME 跨树 .so 失配）经复现定性为 Go plugin 固有限制，无引擎修复项——只在 FIXTURES.md 更正表述。

**Tech Stack:** Go（internal/build、internal/admin、internal/daemon、cmd/huan）、React/TS（web/admin Settings 页）、E2E YAML（tests/e2e/）。

## Global Constraints

- 遵循 Hugo 语义优先：draft 过滤的修复必须贴近 Hugo 的 `.Site.Pages` 不含未构建草稿的行为；`-D`（IncludeDrafts=true）时行为不变
- 后端代码英文注释；文档与 E2E YAML 中文
- 每个写操作修复后必须实跑对应 E2E 用例验证（按 `tests/e2e/runbooks/RUNBOOK.md`：`HUAN_HOME=$site/.huan-home` 隔离 + `HUAN_ADMIN_TOKEN=e2e-fixed-token` 固定 token + 端口表）
- 修 bug 一律 TDD：先写失败测试（红）→ 修（绿）→ 跑全量 `go test ./...`
- **不得改动已通过的 E2E 断言口径**，除非该口径注释里明确标了「已知偏差收窄」——修复后把收窄断言扩回完整断言（如 bld-004 从「仅单页不落盘」扩回「聚合也不含 draft」）
- FIXTURES.md 的「已知状态偏差」条目修复后必须更新（标注修复日期 + 提交），不能删除历史记录（原文保留划线或注明「已于 <date> 修复」）
- 提交信息英文 conventional commits，尾行 `Co-Authored-By: Claude <noreply@anthropic.com>`

## 探针实测事实（2026-08-15 复核，修复依据）

| 事实 | 出处/复现 |
|---|---|
| **Bug1**：draft 页进了 `siteCtx.Pages`/`RegularPages` → sitemap.xml、RSS、首页/列表聚合都含 `/posts/draft-post/`；仅「单页落盘」（pipeline_render.go shouldRender）与「公开 JSON API」（pipeline_feeds.go GenerateContentAPI 强制 false）两道防线有效 | PopulateSitePages（template/context.go:565）无过滤地灌入 site.Pages；internal/build/context.go:33 `ctx.Pages = siteCtx.Pages` |
| **Bug2**：multilang 下 `GET /admin/api/content` 的 en 条目 `relPath=posts/hello.md`（语言后缀被 content.LoadDir 归一剥离，load.go:184-192）、`filePath=posts/hello.en.md`（正确）；`DELETE /admin/api/content/posts/hello.md` → 500 `no such file`；用真实文件名 `posts/hello.en.md` 删 → 200。前端 ContentList 用 `relPath` 构造删除/选择 key（ContentList.tsx:404 `DELETE /admin/api/content/${encodeURIComponent(path)}`，path 来自 selected=relPath 集合） | 实测复现 2026-08-15 |
| **Bug3**：Settings.tsx `hasChanges = useRef(false)`（62 行），`update`/`updateParam` 只改 `.current`（125/136 行）不触发 re-render → 保存按钮 `disabled={!hasChanges.current}`（337 行）与「未保存更改」提示（322 行）不刷新 | web/admin/src/pages/Settings.tsx |
| **Bug4a**：dev 模式 `GET /admin/api/plugins` → **200** `{"status":"plugin manager unavailable"}`（api.go:385 用 StatusOK）；load/unload/reload 三处 nil 分支已是 503（api.go:398/434/464）。dev.go:100 已建 `reg`（newPluginRegistry）但 admin.NewHandler 没传 PluginManager/ThemeManager（dev.go:196-203） | 实测 + 源码 |
| **Bug4b**：daemon `NewBuilder`（daemon.go:138-152）未传 `BuilderOptions.PluginRegistry`（build.go:71 有该字段）→ daemon 的构建管线（含 serve 页 JIT）拿不到编译期插件 → PostBuildHook（seo 注入等）在 daemon 产物上静默失效。注意 daemon 的 `OnAfterBuild`/pluginManager 在 build 之后初始化（7.5 节在 7 节后），需要调整初始化顺序或延迟注入 | 源码 |
| **Bug4c 降级**：复现证明「毒化」不存在——坏 home .so（mach-o 损坏）或版本失配 .so 失败后，ScanAndLoad `continue`（loader.go:194-196）正确放行项目同名 .so；Task 3 当时看到的「零注入」是 worktree 二进制与主仓编译 .so 的 **buildid 跨树失配**（Go plugin 固有），且该场景下 InitPlugin 失败仅 stderr warning、构建退出码 0 | 2026-08-15 三场景对照实测 |
| FIXTURES.md 偏差编号：minimal 偏差 1（draft 泄露）、multilang 偏差（languages 端点）、with-plugins 偏差 5（daemon 无注入）——修复后逐一更新 | tests/e2e/fixtures/FIXTURES.md |
| 受影响 E2E 断言：bld-004（draft 收窄口径）、pub-006（公开 API draft 排除）、sr-001（首页 3 条链接显式断言 draft 泄露）、bc-002..004（绕开 en 删除）、plug-001..007（daemon@13295）、bs-002（绕开 UI 断言） | tests/e2e/api|browser/*.yaml |

---

### Task 1: Bug1 — draft 过滤进聚合上下文（sitemap/RSS/列表）

**Files:**
- Modify: `internal/template/context.go:563-576`（PopulateSitePages）
- Modify: `internal/build/pipeline_setup.go:126`（调用点传过滤参数）
- Test: `internal/build/context_test.go`（追加）

**Interfaces:**
- Consumes: `tmpl.PopulateSitePages(siteCtx *SiteContext, site *content.Site, lookup map[*content.Page]*Context)`（现签名）
- Produces: `PopulateSitePages(siteCtx *SiteContext, site *content.Site, lookup map[*content.Page]*Context, includeDrafts bool)`（新签名，追加 bool；true 时行为与旧完全一致）。仅 pipeline_setup.go:126 一个调用点。

设计决策：过滤放在 **PopulateSitePages**（siteCtx.Pages/RegularPages 的唯一灌入口）而不是模板层或 sitemap 单点——一次修复同时覆盖 sitemap/RSS/首页/section 列表/tag 聚合所有消费方，且 `-D` 时传 true 保持 dev 预览语义。Hugo 同义行为：buildDrafts=false 时 draft 根本不进 `.Site.Pages`。

- [ ] **Step 1: 写失败测试**

在 `internal/build/context_test.go` 追加：

```go
// TestPopulateSitePages_ExcludesDrafts verifies that draft pages do not
// leak into siteCtx.Pages/RegularPages (and thus sitemap/RSS/list
// aggregations) when includeDrafts is false — the E2E-found content-safety
// bug where only single-page rendering and the public JSON API had draft
// defenses.
func TestPopulateSitePages_ExcludesDrafts(t *testing.T) {
	site := &content.Site{Title: "t"}
	draft := &content.Page{Title: "draft post", RelPath: "posts/draft.md", Draft: true, Kind: "page"}
	pub := &content.Page{Title: "pub post", RelPath: "posts/pub.md", Kind: "page"}
	site.Pages = []*content.Page{draft, pub}
	site.RegularPages = []*content.Page{draft, pub}

	siteCtx := &tmpl.SiteContext{}
	lookup := map[*content.Page]*tmpl.Context{
		draft: {Title: draft.Title},
		pub:   {Title: pub.Title},
	}
	tmpl.PopulateSitePages(siteCtx, site, lookup, false)

	for _, c := range siteCtx.Pages {
		if c.Title == "draft post" {
			t.Error("draft leaked into siteCtx.Pages")
		}
	}
	for _, c := range siteCtx.RegularPages {
		if c.Title == "draft post" {
			t.Error("draft leaked into siteCtx.RegularPages")
		}
	}
	if len(siteCtx.Pages) != 1 || len(siteCtx.RegularPages) != 1 {
		t.Fatalf("pages=%d regulars=%d, want 1/1", len(siteCtx.Pages), len(siteCtx.RegularPages))
	}
}

// TestPopulateSitePages_IncludeDraftsKeepsAll verifies -D semantics are
// unchanged: includeDrafts=true keeps drafts in both slices.
func TestPopulateSitePages_IncludeDraftsKeepsAll(t *testing.T) {
	site := &content.Site{Title: "t"}
	draft := &content.Page{Title: "draft post", RelPath: "posts/draft.md", Draft: true, Kind: "page"}
	pub := &content.Page{Title: "pub post", RelPath: "posts/pub.md", Kind: "page"}
	site.Pages = []*content.Page{draft, pub}
	site.RegularPages = []*content.Page{draft, pub}

	siteCtx := &tmpl.SiteContext{}
	lookup := map[*content.Page]*tmpl.Context{
		draft: {Title: draft.Title},
		pub:   {Title: pub.Title},
	}
	tmpl.PopulateSitePages(siteCtx, site, lookup, true)

	if len(siteCtx.Pages) != 2 || len(siteCtx.RegularPages) != 2 {
		t.Fatalf("pages=%d regulars=%d, want 2/2 (-D keeps drafts)", len(siteCtx.Pages), len(siteCtx.RegularPages))
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/build/ -run TestPopulateSitePages -v`
Expected: 编译失败 `not enough arguments in call to tmpl.PopulateSitePages`（旧签名无 bool）——先按新签名写测试会编译错，这就是红。

- [ ] **Step 3: 实现**

`internal/template/context.go` PopulateSitePages 改为：

```go
// PopulateSitePages fills siteCtx.Pages/RegularPages from the page→context
// lookup. When includeDrafts is false, draft pages are excluded so they
// cannot leak into aggregations (sitemap/RSS/home/section/tag lists) —
// matching Hugo, where unbuilt drafts never enter .Site.Pages. The
// per-page render loop (shouldRender) and the public JSON API already
// filter drafts independently; this closes the aggregation gap.
func PopulateSitePages(siteCtx *SiteContext, site *content.Site, lookup map[*content.Page]*Context, includeDrafts bool) {
	for _, p := range site.RegularPages {
		if !includeDrafts && p.Draft {
			continue
		}
		if c, ok := lookup[p]; ok {
			siteCtx.RegularPages = append(siteCtx.RegularPages, c)
		}
	}
	for _, p := range site.Pages {
		if !includeDrafts && p.Draft {
			continue
		}
		if c, ok := lookup[p]; ok {
			siteCtx.Pages = append(siteCtx.Pages, c)
		}
	}
}
```

`internal/build/pipeline_setup.go:126` 改：

```go
	tmpl.PopulateSitePages(siteCtx, p.site, lookup, p.opts.IncludeDrafts)
```

- [ ] **Step 4: 跑测试确认通过 + 全量回归**

Run: `go test ./internal/build/ ./internal/template/ -run TestPopulateSitePages -v && go test ./...`
Expected: 新测试 2 个 PASS；全量无新失败（**注意**：internal/build 已有测试可能构造 draft 页并断言聚合含它——如有失败，逐个检查该断言语义，属旧行为快照的更新断言为不含 draft，并在 commit message 注明）。

- [ ] **Step 5: 实跑 E2E 验证（bld-004/sr-001 的行为翻转）**

```bash
go build -o /tmp/huan-t1 ./cmd/huan
rm -rf /tmp/fx1 && cp -r tests/e2e/fixtures/minimal /tmp/fx1 && mkdir -p /tmp/fx1/.huan-home
HUAN_HOME=/tmp/fx1/.huan-home /tmp/huan-t1 build -s /tmp/fx1
grep -c "draft-post" /tmp/fx1/public/sitemap.xml /tmp/fx1/public/index.html /tmp/fx1/public/index.xml || echo "聚合零泄露（期望）"
grep -c "draft-post" /tmp/fx1/public/api/posts.json || echo "JSON API 零泄露（原有防线）"
# -D 语义不变：
HUAN_HOME=/tmp/fx1/.huan-home /tmp/huan-t1 build -s /tmp/fx1 -D
grep -c "draft-post" /tmp/fx1/public/sitemap.xml && echo "-D 时聚合含 draft（期望）"
rm -rf /tmp/fx1 /tmp/huan-t1
```

- [ ] **Step 6: 更新受影响 E2E 断言（从收窄扩回完整）**

- `tests/e2e/api/build-trigger.yaml` bld-004：注释里的「按 FIXTURES 偏差 1 收窄」口径删除，断言扩为：draft 单页不落盘 **且** `sitemap.xml` 不含 draft URL（新增 fs.assert——注意 dev 产物在私有临时目录，sitemap 断言走 HTTP：`GET /sitemap.xml` body 不含 `draft-post`）
- `tests/e2e/browser/site-rendering.yaml` sr-001：首页链接从「3 条（含草稿——偏差 1 显式断言）」改为「2 条（你好 huan、关于），不含草稿」
- `tests/e2e/fixtures/FIXTURES.md` minimal 偏差 1：标注「已于 2026-08-15 修复（本 plan Task 1，commit 见下）」，原文保留；已知状态表补一行「build 聚合产物不含 draft（sitemap/RSS/首页/列表）」

- [ ] **Step 7: 提交**

```bash
git add internal/template/context.go internal/build/pipeline_setup.go internal/build/context_test.go tests/e2e/api/build-trigger.yaml tests/e2e/browser/site-rendering.yaml tests/e2e/fixtures/FIXTURES.md
git commit -m "fix(build): exclude drafts from site aggregation contexts (sitemap/RSS/lists)

Draft pages leaked into sitemap.xml, RSS feeds, and home/section list
renderings; only single-page output and the public JSON API filtered them.
PopulateSitePages now honors includeDrafts so -D semantics are unchanged.

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: Bug2 — admin 内容 API 的 en 文章真实路径暴露与删除修复

**Files:**
- Modify: `internal/admin/content.go:33-50`（listAll 的 ContentItem 构造）与 `:85-95`（readOne 同构问题）
- Modify: `web/admin/src/pages/ContentList.tsx:225,251,372-404`（选择/删除 key 从 relPath 换 filePath）
- Test: `internal/admin/api_test.go`（追加）

**Interfaces:**
- Consumes: `ContentItem{RelPath, FilePath string; ...}`（types.go:6-20，FilePath 字段已存在且已含语言后缀——实测 `posts/hello.en.md` 正确）
- Produces: API 契约不变（字段不动）；前端行为变化——批量选择与删除的 key 从 `relPath` 改为 `filePath`（真实文件名），en 文章浏览器删除不再 500。`GET /admin/api/content/{path}` 的 path 语义不变：既接受语言中性 relPath 也接受真实文件名（现有 readContent 直接拼路径，真实文件名天然可用）。

设计决策：不改后端 RelPath 语义（语言中性是 content.LoadDir 的 URL/配对核心设计，siblings/languages 端点依赖它），只让**前端操作链**用已存在且正确的 FilePath。同时后端加一层防御：`deleteContent`/`updateContent` 收到语言中性 relPath 且物理文件不存在、但存在唯一语言后缀变体时，拒绝并在错误信息中提示真实文件名（不自动猜测删除——内容安全优先，宁可 500 也别删错文件；但错误信息让前端能自愈）。

- [ ] **Step 1: 写失败测试**

`internal/admin/api_test.go` 追加（沿用文件既有 newTestAPIHandler 模式）：

```go
// TestDeleteContent_LanguageNeutralRelPath_ReportsRealFile verifies the
// defensive branch: deleting via the language-neutral relPath of a
// sidecar-only page (e.g. posts/hello.md when only hello.en.md exists on
// disk) fails with a helpful error naming the real files, instead of a
// bare "no such file" — the E2E-found bug where the browser's delete
// button 500'd with no recovery path.
func TestDeleteContent_LanguageNeutralRelPath_ReportsRealFile(t *testing.T) {
	h, src := newTestAPIHandler(t)
	os.MkdirAll(filepath.Join(src, "content", "posts"), 0o755)
	os.WriteFile(filepath.Join(src, "content", "posts", "hello.en.md"),
		[]byte("---\ntitle: Hello EN\n---\nbody\n"), 0o644)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/content/posts/hello.md", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (refuse to guess-delete)", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "hello.en.md") {
		t.Errorf("error should name the real file, got: %q", body["error"])
	}
	// The real file must survive.
	if _, err := os.Stat(filepath.Join(src, "content", "posts", "hello.en.md")); err != nil {
		t.Error("real file was deleted")
	}
}
```

（`testToken` 等常量按 api_test.go 现有命名取；若无则用文件内已有的鉴权辅助。）

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/admin/ -run TestDeleteContent_LanguageNeutral -v`
Expected: FAIL——现有错误是 `remove .../hello.md: no such file or directory`，不含 `hello.en.md`。

- [ ] **Step 3: 实现后端防御分支**

`internal/admin/content.go` 的 `remove` 方法（或 api.go 的 deleteContent handler，选 contentOps 所在层）加：

```go
// remove deletes a content file. When relPath is language-neutral (sidecar
// naming strips the suffix) and no exact file exists, list same-base
// language variants in the error so callers can self-heal — we never
// guess-delete a translation variant.
func (co *contentOps) remove(relPath string) error {
	fullPath := filepath.Join(co.contentDir, relPath)
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			if variants := co.languageVariants(relPath); len(variants) > 0 {
				return fmt.Errorf("no such file: %s (language variants exist: %s) — delete via the real filename",
					relPath, strings.Join(variants, ", "))
			}
		}
		return err
	}
	return nil
}

// languageVariants lists same-base language-suffixed files for a
// language-neutral relPath, e.g. posts/hello.md → [posts/hello.en.md].
func (co *contentOps) languageVariants(relPath string) []string {
	ext := filepath.Ext(relPath)
	base := relPath[:len(relPath)-len(ext)]
	dir := filepath.Dir(relPath)
	entries, err := os.ReadDir(filepath.Join(co.contentDir, dir))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ext) {
			continue
		}
		n := strings.TrimSuffix(name, ext) // hello.en
		if i := strings.LastIndex(n, "."); i > 0 && base == filepath.Join(dir, n[:i]) {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}
```

（按 content.go 现有 remove 的真实结构适配——先读现实现再嵌入，保持英文注释风格。）

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/admin/ -v`
Expected: 新测试 PASS + 既有测试全绿（真实文件名删除路径不变）。

- [ ] **Step 5: 前端改用 filePath 作为操作 key**

`web/admin/src/pages/ContentList.tsx`：
- 225 行注释 `// --- batch selection state (keyed by relPath) ---` → `keyed by filePath`
- `selected` Set 的写入/读取（375/381/385/388 行）：`it.relPath` → `it.filePath`（toggleOne 参数名 relPath → filePath）
- 404 行删除请求 path 用 selected 里的 filePath（已是真实文件名，无需改 URL 构造）
- 251/276 行的过滤/搜索仍可用 relPath（展示语义不变）
改完构建 SPA：`cd web/admin && npm run build`（产物进 internal/admin/dist，本地不提交——与主仓现状一致）。

- [ ] **Step 6: 实跑 E2E 验证**

```bash
go build -o /tmp/huan-t2 ./cmd/huan && cd web/admin && npm run build && cd ../..
rm -rf /tmp/fx2 && cp -r tests/e2e/fixtures/multilang /tmp/fx2 && mkdir -p /tmp/fx2/.huan-home
(HUAN_ADMIN_TOKEN=e2e-fixed-token HUAN_HOME=/tmp/fx2/.huan-home /tmp/huan-t2 dev -s /tmp/fx2 --port 13220 &>/tmp/fx2.log &); sleep 3
# en 条目的 filePath 删除（后端语义）：
curl -s -X DELETE -H "Authorization: Bearer e2e-fixed-token" -w "\n%{http_code}\n" \
  http://127.0.0.1:13220/admin/api/content/posts/hello.en.md   # 期望 200
# 语言中性误删的防御错误：
curl -s -X DELETE -H "Authorization: Bearer e2e-fixed-token" \
  http://127.0.0.1:13220/admin/api/content/posts/zh-only.md | grep -o "variants exist.*" # 期望提示 zh-only.zh-cn.md
pkill -f huan-t2; rm -rf /tmp/fx2 /tmp/huan-t2 /tmp/fx2.log
```

- [ ] **Step 7: 更新 E2E 断言与文档**

- `tests/e2e/browser/admin-content-crud.yaml`：bc-002 的「新建显式选 ZH-CN 绕开」注释改为「en 旅程可用（Task 2 修复后 filePath 删除 200）」；新增旅程 bc-007（en 文章新建→列表出现→删除→api-probe 404，P1）——可加在 tests 数组末尾，沿用既有旅程格式
- `tests/e2e/fixtures/FIXTURES.md` multilang 偏差节：标注 en relPath/filePath 行为已修复（日期 + 本 Task）
- 基线报告 bug 清单第 5 行状态改「已修」（`docs/reports/e2e/2026-08-15-system-baseline.md`）

- [ ] **Step 8: 提交**

```bash
git add internal/admin/content.go internal/admin/api_test.go web/admin/src/pages/ContentList.tsx tests/e2e/browser/admin-content-crud.yaml tests/e2e/fixtures/FIXTURES.md docs/reports/e2e/2026-08-15-system-baseline.md
git commit -m "fix(admin): expose real filename for delete + defensive variant hint on language-neutral paths

Browser deletes of EN sidecar articles 500'd with a bare no-such-file
because list items carry language-neutral relPath. Frontend now keys
operations on filePath (real name); backend refuses guess-deletes and
names the language variants in the error.

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: Bug3 — Settings 页 hasChanges 改 useState 触发重渲染

**Files:**
- Modify: `web/admin/src/pages/Settings.tsx:62,72,125,136,150,322,337`
- Test: 手动验证（SPA 无测试基建，TDD 以实走替代——见 Step 3）

**Interfaces:**
- Consumes: 无新接口；`hasChanges` 仅组件内使用
- Produces: `const [hasChanges, setHasChanges] = useState(false)`；7 处引用同步换 setter。bs-002 E2E 旅程的「保存按钮仍灰」绕开注释将过时。

- [ ] **Step 1: 改造 hasChanges**

`web/admin/src/pages/Settings.tsx`：
- 62 行 `const hasChanges = useRef(false)` → `const [hasChanges, setHasChanges] = useState(false)`
- 72 行 `hasChanges.current = false` → `setHasChanges(false)`（fetchSettings 回调内）
- 125/136 行 `hasChanges.current = true` → `setHasChanges(true)`（update/updateParam）
- 150 行 `hasChanges.current = false` → `setHasChanges(false)`（save 成功分支）
- 322 行 `{hasChanges.current && (` → `{hasChanges && (`
- 337 行 `disabled={saveState === 'saving' || !hasChanges.current}` → `disabled={saveState === 'saving' || !hasChanges}`

- [ ] **Step 2: 构建 SPA**

Run: `cd web/admin && npm run build`
Expected: 构建成功（TS 编译会抓漏改的 `.current`）。

- [ ] **Step 3: 实走验证（TDD 等价：先红后绿的「红」是当前实况，已在 Task 10 记录）**

```bash
go build -o /tmp/huan-t3 ./cmd/huan
rm -rf /tmp/fx3 && cp -r tests/e2e/fixtures/minimal /tmp/fx3 && mkdir -p /tmp/fx3/.huan-home
(HUAN_ADMIN_TOKEN=e2e-fixed-token HUAN_HOME=/tmp/fx3/.huan-home /tmp/huan-t3 dev -s /tmp/fx3 --port 13320 &>/tmp/fx3.log &); sleep 3
# agent-browser 实走 bs-002：改 title → 保存按钮立即亮起（修复前：仍灰）
pkill -f huan-t3; rm -rf /tmp/fx3 /tmp/huan-t3 /tmp/fx3.log
```

- [ ] **Step 4: 更新 E2E 断言与文档**

- `tests/e2e/browser/admin-settings.yaml` bs-002：删除「成功断言走 api-probe 绕开 UI（保存按钮仍灰）」注释，verify 加回「修改任一输入后保存按钮从禁用变为可点击」（用户可见状态断言）；保留 api-probe 回读断言（更强）
- `tests/e2e/fixtures/FIXTURES.md`（如该 bug 记在偏差节）+ 基线报告 bug 清单第 6 行状态改「已修」

- [ ] **Step 5: 提交**

```bash
git add web/admin/src/pages/Settings.tsx tests/e2e/browser/admin-settings.yaml tests/e2e/fixtures/FIXTURES.md docs/reports/e2e/2026-08-15-system-baseline.md
git commit -m "fix(admin-ui): Settings hasChanges as state so dirty UI re-renders

A useRef never triggers re-render, leaving the save button disabled and
hiding the unsaved-changes hint after edits. Switch to useState.

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 4: Bug4a — dev 传 PluginManager + listPlugins nil 分支改 503

**Files:**
- Modify: `cmd/huan/dev.go:100-108,196-203`（复用已建 reg，构造 LifecycleManager 传给 admin）
- Modify: `internal/admin/api.go:384-386`（listPlugins nil 分支 StatusOK → StatusServiceUnavailable）
- Test: `internal/admin/api_test.go`（追加 503 断言）

**Interfaces:**
- Consumes: `plugin.NewLifecycleManager(registry *plugin.Registry, loader *plugin.Loader, bus *eventbus.Bus)`（internal/daemon/daemon.go:171 的用法参照——注意 bus 参数类型，dev 场景可传 nil 若签名允许，先读签名）；dev.go:100 已有 `reg`
- Produces: dev 模式 `/admin/api/plugins` 返回真实插件列表（与 daemon 一致）；无 pluginManager 的宿主（若仍有）得到 503 而非 200——`tests/e2e/api/plugins-admin.yaml` 套件头「runtime=daemon 因 dev 恒 unavailable」注记将过时，改为 dev 也可跑。

- [ ] **Step 1: 写失败测试**

`internal/admin/api_test.go` 追加：

```go
// TestListPlugins_NoManager_Returns503 verifies the nil-manager branch now
// signals unavailability with 503 instead of 200 — a 200 body saying
// "unavailable" is indistinguishable from success for naive callers.
func TestListPlugins_NoManager_Returns503(t *testing.T) {
	h, _ := newTestAPIHandler(t) // constructed without a plugin manager
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/admin/ -run TestListPlugins_NoManager -v`
Expected: FAIL（现返回 200）。

- [ ] **Step 3: 实现两处修改**

`internal/admin/api.go:384-386`：

```go
func (h *apiHandler) listPlugins(w http.ResponseWriter, r *http.Request) {
	if h.pluginManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, APIError{Error: "plugin manager unavailable"})
		return
	}
```

`cmd/huan/dev.go`：admin.NewHandler 调用处（196 行前）加：

```go
	// Plugin lifecycle manager for the admin API (list/load/unload/reload),
	// sharing the build registry and watching the same plugin dir.
	pluginLoader := plugin.NewLoader(pluginDirFromSource(sourceDir))
	if pluginsDir != "" {
		pluginLoader = plugin.NewLoader(pluginsDir)
	}
	adminPluginMgr := plugin.NewLifecycleManager(reg, pluginLoader, nil)
```

（`reg` 是 dev.go:100 已建变量；NewLifecycleManager 第三参 bus 类型按 internal/plugin 实际签名适配——daemon.go:171 传 d.bus，若签名是具体类型 *eventbus.Bus 且不接受 nil，则传 eventbus.New() 并注明；HandlerOptions 加 `PluginManager: adminPluginMgr`。）

- [ ] **Step 4: 跑测试 + dev 实跑验证**

```bash
go test ./internal/admin/ ./cmd/huan/ -v
go build -o /tmp/huan-t4 ./cmd/huan
rm -rf /tmp/fx4 && cp -r tests/e2e/fixtures/with-plugins /tmp/fx4 && mkdir -p /tmp/fx4/.huan-home /tmp/fx4/plugins
(cd plugins/seo-injector && go build -buildmode=plugin -o /tmp/fx4/plugins/seo-injector.so .)
(HUAN_ADMIN_TOKEN=e2e-fixed-token HUAN_HOME=/tmp/fx4/.huan-home /tmp/huan-t4 dev -s /tmp/fx4 --port 13290 &>/tmp/fx4.log &); sleep 3
curl -s -H "Authorization: Bearer e2e-fixed-token" http://127.0.0.1:13290/admin/api/plugins | head -c 200
# 期望：{"plugins":[...],"total":2} 而非 {"status":"plugin manager unavailable"}
pkill -f huan-t4; rm -rf /tmp/fx4 /tmp/huan-t4 /tmp/fx4.log
```

- [ ] **Step 5: 更新 E2E 断言与文档**

- `tests/e2e/api/plugins-admin.yaml` 套件头注记更新：runtime=daemon@13295 的理由从「dev 恒 unavailable」改为「历史原因保留 daemon 主跑；dev 现也可跑（本 Task 修复）」；plug-001 加旁路断言（dev@13290 同样 total≥1，P2）
- `tests/e2e/runbooks/RUNBOOK.md` §4.3 挂载差异表 `/admin/api/plugins` 行：dev 列从「返回 unavailable」改为「有（Task 4 修复后）」
- 基线报告 bug 清单第 7 行拆分：dev 无 pluginManager 项标「已修」

- [ ] **Step 6: 提交**

```bash
git add cmd/huan/dev.go internal/admin/api.go internal/admin/api_test.go tests/e2e/api/plugins-admin.yaml tests/e2e/runbooks/RUNBOOK.md docs/reports/e2e/2026-08-15-system-baseline.md
git commit -m "fix(admin): dev wires plugin manager; nil-manager list returns 503

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 5: Bug4b — daemon build 接入 PluginRegistry（serve 产物恢复插件钩子）

**Files:**
- Modify: `internal/daemon/daemon.go:138-180`（初始化顺序重排：pluginManager 先于 NewBuilder，BuilderOptions 传 PluginRegistry）
- Test: `internal/daemon/daemon_test.go`（追加）

**Interfaces:**
- Consumes: `build.BuilderOptions.PluginRegistry *plugin.Registry`（build.go:67-71 已有字段，dev.go:101 `buildOpts.PluginRegistry = reg` 是参照用法）；`plugin.NewLifecycleManager`（daemon.go:171 现有）
- Produces: daemon 的初始全量构建与增量构建恢复 PostBuildHook（seo-injector 等的产物注入）——serve 页面产物与 `huan build` 一致。依赖 Task 4 无（独立），但同一提交序内 Task 4 已动 dev 侧。

设计决策：把「7.5 Init Plugin Lifecycle Manager」整块**移到「7. Initialize Builder」之前**（不复制代码），让 `d.pluginManager` 的 registry 可传给 NewBuilder。注意 ThemeManager 块（6.75）已在 builder 前、用的是 `opts.PluginRegistry`（cmd 层编译期 registry）——NewBuilder 传的应是**同一个编译期 registry**（`opts.PluginRegistry`，与 build 语义一致：编译期插件参与构建钩子；.so 动态插件不参与构建，维持现状）。

- [ ] **Step 1: 写失败测试**

`internal/daemon/daemon_test.go` 追加（复用既有 daemon 测试的构造模式；若无直接构造 Builder 的 harness 则测 BuilderOptions 传递的最小单元——读现有测试找模式）：

```go
// TestDaemonBuilderGetsPluginRegistry verifies the daemon's builder wires
// the compiled-plugin registry into the build pipeline so PostBuildHooks
// (SEO injection etc.) run on daemon-served output, matching `huan build`.
// Regression for the E2E-found gap where daemon output had zero injection.
func TestDaemonBuilderGetsPluginRegistry(t *testing.T) {
	src := t.TempDir()
	writeIncFile(t, src) // existing helper per daemon_test.go, or minimal site below
	// minimal site if no helper fits:
	// huan.yaml + content/posts/a.md + layouts/_default/{single,list}.html

	reg := plugin.NewRegistry()
	d, err := Run(Options{SourceDir: src, Port: "0", Bind: "127.0.0.1", PluginRegistry: reg})
	if d != nil && d.builder != nil {
		if d.builder.opts.PluginRegistry != reg {
			t.Fatal("daemon builder missing PluginRegistry")
		}
	}
	_ = err // Run may block; if the harness can't run to completion, assert via builder construction refactor instead
}
```

若 `Run` 阻塞导致不可测（daemon.Run 是阻塞服务）：改测法——把 registry→builder 的接线提取为小函数或在 Run 内构造后立即暴露（读现有 daemon_test.go 怎么测 serving/Builder 再定；**务实底线**：至少手工冒烟 Step 4 验证行为翻转，单测覆盖 NewBuilder 参数透传）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/daemon/ -run TestDaemonBuilderGetsPluginRegistry -v`
Expected: FAIL（builder.opts.PluginRegistry == nil）。

- [ ] **Step 3: 实现（初始化顺序重排）**

`internal/daemon/daemon.go`：把「7.5 Init Plugin Lifecycle Manager」整块（含 pluginDir 解析、NewLoader、NewLifecycleManager）剪切到「7. Initialize Builder」之前；NewBuilder 调用加一行：

```go
	d.builder = NewBuilder(BuilderOptions{
		SourceDir:     opts.SourceDir,
		OutputDir:     tmpDir,
		PluginRegistry: opts.PluginRegistry, // compiled plugins join build hooks (match `huan build`)
		Bus:           d.bus,
		// ...其余字段不动
```

（保持英文注释；确认移动后 watcher 启动等依赖顺序不被破坏——pluginManager 的 Start 在其后原有位置不动，只移构造。）

- [ ] **Step 4: 跑测试 + 行为冒烟**

```bash
go test ./internal/daemon/ -v && go build ./...
go build -o /tmp/huan-t5 ./cmd/huan
rm -rf /tmp/fx5 && cp -r tests/e2e/fixtures/with-plugins /tmp/fx5 && mkdir -p /tmp/fx5/.huan-home /tmp/fx5/plugins
(cd plugins/seo-injector && go build -buildmode=plugin -o /tmp/fx5/plugins/seo-injector.so .)
(cd plugins/sitemap-enhancer && go build -buildmode=plugin -o /tmp/fx5/plugins/sitemap-enhancer.so .)
(HUAN_ADMIN_TOKEN=e2e-fixed-token HUAN_HOME=/tmp/fx5/.huan-home /tmp/huan-t5 daemon -s /tmp/fx5 --port 13295 --bind 127.0.0.1 &>/tmp/fx5.log &); sleep 5
curl -s http://127.0.0.1:13295/posts/seo-post/ | grep -c "og:" && echo "daemon serve 产物有注入（修复生效）"
pkill -f huan-t5; rm -rf /tmp/fx5 /tmp/huan-t5 /tmp/fx5.log
```

- [ ] **Step 5: 更新文档与 E2E**

- `tests/e2e/fixtures/FIXTURES.md` with-plugins 偏差 5（daemon 无注入）：标注已修复 + daemon 注入断言可用
- `tests/e2e/api/plugins-admin.yaml` 或 build-trigger：可加 daemon serve 页 og: 注入断言（P1，daemon@13295 前置编译 .so——plug 套件已有该前置）
- 基线报告 bug 清单第 7 行的「daemon build 不传 PluginRegistry」项标「已修」

- [ ] **Step 6: 提交**

```bash
git add internal/daemon/daemon.go internal/daemon/daemon_test.go tests/e2e/fixtures/FIXTURES.md docs/reports/e2e/2026-08-15-system-baseline.md
git commit -m "fix(daemon): pass compiled plugin registry to builder so serve output runs build hooks

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 6: 回归 + 文档收尾（Bug4c 定性更正 + 全量验证）

**Files:**
- Modify: `tests/e2e/fixtures/FIXTURES.md`（with-plugins 偏差 1 的「毒化」表述更正）
- Modify: `docs/reports/e2e/2026-08-15-system-baseline.md`（bug 清单终态刷新）
- Modify: `docs/progress/CURRENT_STATE.md`、`memory/MEMORY.md`、`memory/daily/2026-08-15.md`（追加）

**Interfaces:**
- Consumes: Task 1-5 全部产物。
- Produces: 修复后回归记录；BUG 清单终态：#1/#2/#3 已修（前次），#4 draft 已修（Task 1）、#5 en 路径已修（Task 2）、#6 hasChanges 已修（Task 3）、#7 拆分（dev 503+manager 已修 Task 4 / daemon registry 已修 Task 5 / HUAN_HOME 跨树失配定性为 Go plugin 固有限制非 bug）。

- [ ] **Step 1: Bug4c 表述更正（FIXTURES.md with-plugins 偏差）**

把「毒化/连带拒绝」表述替换为（原文保留在下方缩进或引块）：

```markdown
**已于 2026-08-15 复现更正**：不存在「毒化连带拒绝」——坏 .so 或版本失配 .so 加载失败后，ScanAndLoad 的 continue 会正确放行项目同名 .so。Task 3 当时观察到的「静默零注入」是 worktree 二进制与主仓编译 .so 的 **buildid 跨树失配**（Go plugin 固有：plugin 必须与宿主同源码树构建），该场景 InitPlugin 失败仅 stderr warning、构建退出码 0。对策仍是 RUNBOOK §2.1 的 HUAN_HOME 隔离 + patterns.md §7 的同树现场编译，属环境契约而非引擎 bug。
```

- [ ] **Step 2: 全量回归（按 RUNBOOK）**

1. `go test ./...` 全绿
2. E2E 抽样回归（受修复影响的用例全跑，其余按基线抽样口径）：
   - api：bld-004（扩回完整断言）、pub-006、plug-001（dev 旁路新断言）、plug-004、crud 抽 2、set-002
   - browser：sr-001（2 条链接新断言）、bc-002..004 + bc-007（新 en 旅程）、bs-002（UI 断言回归）
   - daemon@13295 serve 页 og: 注入断言（Task 5 新增）
3. 结果追加进基线报告「回归记录」节（日期、commit、通过率）

- [ ] **Step 3: 文档三处收尾**

- CURRENT_STATE.md：E2E 小节追加「2026-08-15 修复批次：4 项遗留 bug 清零（1 项定性更正）」
- MEMORY.md：项目上下文 E2E 节追加修复批次一行 + 「Go plugin 跨树 buildid 失配是环境契约非 bug」经验教训一条
- daily/2026-08-15.md：追加本 plan 执行记录

- [ ] **Step 4: 提交**

```bash
git add tests/e2e/fixtures/FIXTURES.md docs/reports/e2e/2026-08-15-system-baseline.md docs/progress/CURRENT_STATE.md memory/
git commit -m "docs: E2E bug-fix regression record and 4c reclassification

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Self-Review 结果

1. **Spec 覆盖**：用户点名 4 个 bug——①Task 1（draft 聚合）②Task 2（en relPath/filePath）③Task 3（hasChanges）④拆为 Task 4（dev manager+503）/Task 5（daemon registry）+ Task 6 对 4c 做定性更正（复现实测证明非 bug，写明理由与既有对策）。修复后同步 FIXTURES/E2E/基线：每 Task 内嵌对应 Step，Task 6 统一回归。
2. **占位符**：Task 5 Step 1 的测试对 Run 阻塞性有条件分支（写了务实底线），其余步骤均含完整代码/命令。Task 2 Step 3 注明「按 content.go 现有 remove 真实结构适配」——remove 的现实现位置经 grep 确认存在（api.go deleteContent → ops.remove），嵌入点明确。
3. **类型一致性**：PopulateSitePages 新签名在 Task 1 定义并同步唯一调用点；ContentItem.FilePath 字段已存在（types.go:9）无需新增；NewLifecycleManager 三参用法与 daemon.go:171 现有调用一致并注明 bus 参数适配要求。
4. **风险点**：Task 1 可能翻转 internal/build 既有测试的 draft 快照断言（Step 4 已写处置）；Task 5 移动初始化块需确认 watcher/Start 顺序（Step 3 已注明只移构造不移 Start）。
