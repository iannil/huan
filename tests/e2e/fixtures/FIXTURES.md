# Fixture 已知状态表

用例断言直接引用本表常量；fixture 内容变更必须同步本表并重跑相关套件。

所有数字为 2026-08-15 实测值（命令与原文输出见文末「实测记录」）。

## minimal

单语 zh-cn 最小站，覆盖 auth/content/status/media/settings/build/public-api 及多数 browser 用例。

### 文件清单

| 文件 | 说明 |
|---|---|
| `huan.yaml` | baseURL=http://127.0.0.1:1313/，title=huan E2E 最小站，languageCode=zh-cn，publishDir=public，paginate=10，hasCJKLanguage=true |
| `content/posts/hello-huan.md` | 正式文章；title=你好 huan，date=2026-08-10，tags=[go, e2e]，description=第一篇正式文章 |
| `content/posts/draft-post.md` | 草稿；title=草稿文章，date=2026-08-11，draft=true，tags=[go] |
| `content/about.md` | 根级页面；title=关于，date=2026-08-01 |
| `archetypes/default.md` | archetype 模板（`{{ .Name }}`/`{{ .Date }}` 为 huan new 渲染占位，见 cmd/huan/newcmd.go renderArchetype），默认 draft=true |
| `static/logo.svg` | 唯一静态文件（mediaCount 的来源） |
| `layouts/index.html` | 首页：遍历 `.Site.RegularPages` 输出 `a.post-link` |
| `layouts/_default/single.html` | 单页：`h1#page-title` + `article` |
| `layouts/_default/list.html` | 列表页：遍历 `.Pages` 输出 `a.post-link` |

### GET /admin/api/status 期望（dev 与 daemon 实测一致）

- `total=3, published=2, drafts=1`
- `sections=2`，`sectionBreakdown={"_root":1,"posts":2}`（about.md 计入名为 `_root` 的根 section——实测校准，非计划推测的空串/`page`）
- `languages=[]`（未配置多语言）
- `mediaCount=1`（static/logo.svg）
- `recentContent` 按 date 降序：草稿文章(2026-08-11) → 你好 huan(2026-08-10) → 关于(2026-08-01)；草稿条目 `draft:true`、`section:"posts"`；关于条目 `section:"_root"`、`tags:null`
- `title="huan E2E 最小站"`，`baseURL="http://127.0.0.1:1313/"`；`serveURL` 随启动端口变化，断言时用占位符

### 公开 API（daemon 专用；dev 模式 `/api/v1/*` 未挂载，返回 404）

- `GET /api/v1/pages` → 200 `{"total":2,"page":1,"limit":10}`；`data` 2 条且不含 draft：你好 huan（`url:"/posts/hello-huan/"`、`section:"posts"`、tags=[go,e2e]、有 description）+ 关于（`url:"/about/"`、`section:""`、无 tags/description 字段）
- `GET /api/v1/posts` → 404 `{"error":"not found"}`（端点不存在，勿在用例中使用）
- `?draft=1` 不改变结果（无 draft 包含开关）
- `GET /health` → 200 `{"status":"ok","uptime":"…","version":"…"}`（uptime/version 动态）

### build 产物锚点（`huan build`）

- `public/posts/hello-huan/index.html` 存在，含 `h1#page-title` 文本「你好 huan」与 `<strong>huan</strong>`
- `public/about/index.html` 存在，h1=「关于」
- `public/index.html` 存在（首页模板）
- `public/posts/draft-post/` **不存在**（draft 单页不落盘）
- `public/logo.svg` 存在
- `public/posts/index.html`（posts 列表页）、`public/tags/go/index.html`、`public/tags/e2e/index.html`（tag 页，各含 1 条 hello-huan 链接）存在
- `public/api/posts.json` 1 条（仅 hello-huan，draft 已排除）；`public/api/.json` 1 条（about，根 section 聚合文件名）
- `public/sitemap.xml`、`public/index.xml`、`public/posts/index.xml`、`public/llms.txt` 存在
- stdout 汇总：`Rendered: 4 pages`、`Output: 17 files`、`Build complete.`（无 Errors 行）；另有 2 条非致命 WARN：terms 模板缺失（`_default/terms.html`）、searchindex 模板缺失
- 首页被内置模板渲染：含 `<meta name=generator content="Hugo 0.160.1">`（layout 钩子优先级所致，非 fixture 模板，断言首页时注意）

### 已知状态偏差（实测发现，写用例时必须绕开或显式断言）

1. **draft 泄露进列表/聚合渲染**：静态 build 后 `public/index.html`、`public/posts/index.html` 及 RSS（`index.xml`、`posts/index.xml`）、`sitemap.xml` 均包含 `/posts/draft-post/` 条目；daemon 运行时 GET `/posts/draft-post/` 返回 200 完整渲染页。仅「draft 单页不落盘」与「公开 JSON API（/api/v1/pages、public/api/posts.json）排除 draft」两条防线有效。spec 已将「draft 不进公开 API」列为回归断言（bld-004、历史安全 bug），列表泄露如需修复请另立任务并同步本表。
2. **`/api/v1/posts` 端点不存在**（404），公开 API 断言一律用 `/api/v1/pages`。
3. 公开 API 中 about 的 `section` 为空串，而 admin status 的 sectionBreakdown 用 `_root` 键——两套口径不同，勿混用断言。

### 实测记录（2026-08-15，worktree e2e-test-system，commit e0a8f2e 构建）

```bash
go build -o /tmp/huan-t2 ./cmd/huan
rm -rf /tmp/t2 && cp -r tests/e2e/fixtures/minimal /tmp/t2
/tmp/huan-t2 build -s /tmp/t2 2>&1 | tail -4
#   WARN: terms: template not found: _default/terms.html
#   WARN: search: template not found: _default/index.searchindex.json
#   Rendered:     4 pages
#   Output:       17 files, 8.4 KB
# Build complete.

/tmp/huan-t2 dev -s /tmp/t2 --port 13202 &   # token 取自启动 stderr（32-hex）
curl -s -H "Authorization: Bearer $TOK" http://127.0.0.1:13202/admin/api/status
# {"title":"huan E2E 最小站","baseURL":"http://127.0.0.1:1313/","serveURL":"http://127.0.0.1:13202/","total":3,"published":2,"drafts":1,"sections":2,"languages":[],"mediaCount":1,"sectionBreakdown":{"_root":1,"posts":2},"recentContent":[...按 date 降序 3 条...]}

HUAN_ADMIN_TOKEN=<fixed> /tmp/huan-t2 daemon -s /tmp/t2d --port 13203 --bind 127.0.0.1
curl -s http://127.0.0.1:13203/api/v1/pages
# {"data":[{你好 huan...},{关于...}],"total":2,"page":1,"limit":10}   # draft 已排除
```

（后续 Task 3/4 往同一文件追加 multilang / with-plugins 小节。）
