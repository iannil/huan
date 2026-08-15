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

1. ~~**draft 泄露进列表/聚合渲染**~~ **已修复（2026-08-15，fix-e2e-found-bugs Task 1）**：原缺陷——静态 build 后 `public/index.html`、`posts/index.html`、RSS、`sitemap.xml` 均含 `/posts/draft-post/` 条目，daemon 运行时 GET `/posts/draft-post/` 返回 200；仅「draft 单页不落盘」与「公开 JSON API 排除」两条防线有效。修复：`PopulateSitePages` 加 `includeDrafts` 过滤，聚合上下文（sitemap/RSS/首页/列表/tag）不再含 draft（`-D` 时含，语义保留）。原文保留如下：spec 已将「draft 不进公开 API」列为回归断言（bld-004、历史安全 bug）。修复后断言口径：bld-004 扩回完整（sitemap 聚合零泄露）、sr-001 首页 2 条链接不含 draft。
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

## multilang

zh-cn（默认）+ en 双语言站，覆盖 languages 端点、多语言 build、公开 API。无 draft。

### 文件清单

| 文件 | 说明 |
|---|---|
| `huan.yaml` | baseURL=http://127.0.0.1:1313/，title=huan E2E 多语言站，languageCode=zh-cn，defaultContentLanguage=zh-cn，publishDir=public，languages: zh-cn（weight 1）+ en（weight 2，baseURL /en/） |
| `content/posts/hello.zh-cn.md` | title=你好（中文版），date=2026-08-12，tags=[i18n] |
| `content/posts/hello.en.md` | title=Hello (EN)，date=2026-08-12，tags=[i18n]（hello 的 en sidecar） |
| `content/posts/zh-only.zh-cn.md` | title=仅中文，date=2026-08-12，tags=[i18n]（无 en 版） |
| `layouts/index.html`、`layouts/_default/single.html`、`layouts/_default/list.html` | 与 minimal 逐字节相同（复制） |

### GET /admin/api/content/{relPath}/languages 期望（dev 实测）

- `posts/hello.zh-cn.md` → 200 `{"current":"zh-cn","siblings":[{"language":"en","relPath":"posts/hello.en.md","title":"Hello (EN)","draft":false}]}`
- `posts/hello.en.md` → 200 `{"current":"en","siblings":[{"language":"zh-cn","relPath":"posts/hello.zh-cn.md","title":"你好（中文版）","draft":false}]}`
- `posts/zh-only.zh-cn.md` → 200 `{"current":"zh-cn","siblings":[]}`（无翻译伙伴）

### GET /admin/api/status 期望（dev 实测）

- `total=3, published=3, drafts=0`（3 个 sidecar 文件各算 1 条；hello 双语是**两个条目**）
- `sections=1`，`sectionBreakdown={"posts":3}`
- `languages=["en","zh-cn"]`（排序实测为字母序，非 weight 序）
- `mediaCount=0`（无 static/）
- `recentContent` 3 条全为 `section:"posts"`、`draft:false`、`tags:["i18n"]`；每条含 `filePath`（如 `posts/hello.en.md`，带语言后缀）与 `language` 字段；同 date 下实测顺序 Hello (EN) → 你好（中文版） → 仅中文（同日期排序非确定契约，断言用集合而非顺序）

### 公开 API（daemon 专用；dev 模式 `/api/v1/*` 404 同 minimal）

- `GET /api/v1/pages` → 200 `{"total":3,"page":1,"limit":10}`；`data` 3 条**全部扁平混出**：Hello (EN) 与 你好（中文版） 的 `url` 均为 `/posts/hello/`（**en 条目的 url 无 `/en/` 前缀，且无任何 language 字段**——已知状态偏差见下）；仅中文 `/posts/zh-only/`
- **无 `lang` 查询参数**（contentindex handler 仅支持 `section`/`tag`/`q`/`sort`/`page`/`limit`）——spec 中「公开 API 语言过滤」用例按实际改为：断言 3 条混出 + 无 lang 过滤能力
- `GET /health` → 200 `{"status":"ok","uptime":"…","version":"0.6.0"}`

### build 产物锚点（`huan build`，多语言走 BuildMultiSite）

- stdout 末行汇总：`built 2 languages: zh-cn=4 pages en=3 pages`；两段各 1 次 `Build complete.`，各 2 条 WARN（terms/searchindex 模板缺失，同 minimal）
- zh-cn 段（根）：`public/posts/hello/index.html`（h1=你好（中文版））、`public/posts/zh-only/index.html`（h1=仅中文）、`public/index.html`、`public/posts/index.html`（2 条 post-link：仅中文→`/posts/zh-only/`、你好（中文版）→`/posts/hello/`）
- en 段（前缀）：**`public/en/posts/hello/index.html`**（h1=Hello (EN)，title=`Hello (EN) - huan E2E 多语言站`）；`public/en/index.html`（1 条 post-link href 含 `/en/posts/hello/`）；`public/en/` 下**无 zh-only 任何产物**（en 过滤正确）
- `public/api/posts.json` 2 条（zh-cn：你好（中文版）+仅中文，url 无前缀）；`public/en/api/posts.json` 1 条（Hello (EN)，url=`http://127.0.0.1:1313/en/posts/hello/` 带前缀绝对 URL——与 daemon /api/v1 口径又不同，见偏差 3）
- sitemap hreflang：`public/sitemap.xml` 的 hello `<url>` 含两条 `xhtml:link rel="alternate"`（zh-cn→`http://127.0.0.1:1313/posts/hello/`、en→`http://127.0.0.1:1313/en/posts/hello/`）；zh-only 的 url 只有 zh-cn alternate；`public/en/sitemap.xml` 镜像同样两条 alternate
- **`public/posts/hello/index.html` 单页 head 无 hreflang link**（alternate 只进 sitemap，不进页面 head）

### 已知状态偏差（实测发现，写用例时必须绕开或显式断言）

1. **daemon 公开 API 的多语言口径是「源文件维度」而非「站点产出维度」**：`/api/v1/pages` 把 hello.en 与 hello.zh-cn 当两条独立记录，`url` 均为 `/posts/hello/`（en 条目丢 `/en/` 前缀、无 language 字段）——无法从公开 API 区分语言，也无法拼出 en 页面 URL。断言时只验条数/标题/去重 URL，勿断言语言归属。
2. **sitemap/列表的 url 与公开 API 的 url 口径不同**：build 产物内为绝对 URL（baseURL+可选 /en/ 前缀），daemon /api/v1 为相对路径。两套断言分开写。
3. dev serve 的 en 页面可正常访问（`/en/`、`/en/posts/hello/` 返回 200 且内容正确，livereload 注入），但 **`huan dev` 无 `--plugins` 场景下 build 走 BuildMultiSite 时每语言各自渲染**——多语言与 dev 断言互不干扰。
4. **admin 内容 API 的 en 条目 relPath/filePath 行为已修复（2026-08-15，fix-e2e-found-bugs Task 2）**：en 条目的 `relPath` 语言中性（如 `posts/hello.md`），`filePath` 为真实文件名（`posts/hello.en.md`）。此前浏览器用 `relPath` 做删除/更新 key 会命中物理上不存在的 `hello.md` 而 500（无自愈路径）。修复后前端操作 key 改用 `filePath`；后端对语言中性 `relPath` 的误删请求会 500 并报出真实语言变体文件名（绝不猜删）。断言用 `filePath` 做写操作 key。

### 实测记录（2026-08-15，worktree e2e-test-system，commit 801d00f 构建）

```bash
go build -o /tmp/huan-t3 ./cmd/huan
rm -rf /tmp/t3-multilang && cp -r tests/e2e/fixtures/multilang /tmp/t3-multilang
/tmp/huan-t3 build -s /tmp/t3-multilang 2>&1 | tail -3
# Build complete.
# built 2 languages: zh-cn=4 pages en=3 pages (6ms)
ls /tmp/t3-multilang/public/en/posts/hello/index.html   # 存在

/tmp/huan-t3 dev -s /tmp/t3-multilang --port 13212 &    # token 取自启动 stderr（32-hex）
curl -s -H "Authorization: Bearer $TOK" http://127.0.0.1:13212/admin/api/content/posts/hello.zh-cn.md/languages
# {"current":"zh-cn","siblings":[{"language":"en","relPath":"posts/hello.en.md","title":"Hello (EN)","draft":false}]}

HUAN_ADMIN_TOKEN=<fixed> /tmp/huan-t3 daemon -s /tmp/t3-mld --port 13222 --bind 127.0.0.1
curl -s http://127.0.0.1:13222/api/v1/pages
# {"data":[{Hello (EN) url=/posts/hello/},{你好（中文版） url=/posts/hello/},{仅中文 url=/posts/zh-only/}],"total":3,...}
```

## with-plugins

单语 zh-cn 插件站：huan.yaml 声明 `seo_injector` + `sitemap_enhancer`（均为 `category: static`，build 期加载）。覆盖 plugins-admin 端点与 build 插件钩子产物断言。

### 文件清单

| 文件 | 说明 |
|---|---|
| `huan.yaml` | baseURL=http://127.0.0.1:1313/，title=huan E2E 插件站，languageCode=zh-cn，publishDir=public，`plugins:` **map 形式**：`seo_injector: {category: static}` + `sitemap_enhancer: {category: static}`（键名=插件 `Name()`，**下划线**；category 仅接受 static/dynamic/mixed） |
| `content/posts/seo-post.md` | title=SEO 验证文章，date=2026-08-12，tags=[seo]，description=用于验证 seo-injector 插件注入效果的文章 |
| `static/logo.svg` | 同 minimal（mediaCount=1） |
| `layouts/index.html`、`layouts/_default/single.html`、`layouts/_default/list.html` | 与 minimal 逐字节相同（复制） |

**注意**：fixture 目录**不含** `plugins/*.so`（`.so` 被 .gitignore 排除且与构建环境强耦合）。E2E 套件运行时需先执行：

```bash
( cd plugins/seo-injector && go build -buildmode=plugin -o <site>/plugins/seo-injector.so . )
( cd plugins/sitemap-enhancer && go build -buildmode=plugin -o <site>/plugins/sitemap-enhancer.so . )
```

### build 产物锚点（`huan build`，插件加载成功时）

- **注入标记**：`public/posts/seo-post/index.html` 的 `</head>` 前有 `<!-- huan seo-injector -->` 注释块，含 8 个 meta：`name="description"`（正文抽取截断）、`og:title`（=`SEO 验证文章 - huan E2E 插件站`，取 `<title>` 全文）、`og:description`、`og:url`（=`/posts/seo-post/index.html`，相对输出路径）、`og:type`（=`website`，见偏差 2）、`twitter:card`（=`summary_large_image`）、`twitter:title`、`twitter:description`。frontmatter 的 description 只进 JSON API，**不进**注入的 meta description（注入器从 body 文本抽取）
- 全部 5 个 HTML（index/posts 列表/seo-post/tags/seo/page/1）都被注入（注入器扫描 outputDir 全部 .html）
- `public/sitemap.xml` 与无插件构建**逐字节相同**（见偏差 3：sitemap-enhancer 恒 no-op）
- 其余产物同 minimal 形态：`public/api/posts.json` 1 条（含 description 字段）、`public/logo.svg`、`public/tags/seo/index.html`
- stdout：`Rendered: 3 pages`、`Output: 13 files`、`Build complete.` + 2 条 WARN（terms/searchindex）

### GET /admin/api/plugins 期望（daemon 实测，插件加载成功时）

- 编译双 .so（seo-injector + sitemap-enhancer）后：200 `total=2`，`plugins` 数组含 `seo_injector` 与 `sitemap_enhancer`（均 `source:"compiled"`，version=0.1.0）
- 再放入 simple-plugin.so（`internal/plugin/testdata/simple_plugin` 现场编译）并重启/启动期 ScanAndLoad 后：`total=3`——第三条 `simple-test`（`source:"loaded"`），它由启动期自动加载（Name 与 huan.yaml 声明的两插件不同名，无冲突）
- dev 模式该端点返回 `{"status":"plugin manager unavailable"}`（dev 无 pluginManager；plugins-admin 用例必须跑 daemon）
- **口径更新（2026-08-15，来源 Task 8 复测 + Task 13 全量实跑复验）**：下文偏差 4 的原始描述（`total:1`）是 Task 3 首测口径，已被两次独立复测推翻，保留原文仅作历史记录

### GET /admin/api/status 期望（dev 与 daemon 一致）

- `total=1, published=1, drafts=0`，`sections=1`，`sectionBreakdown={"posts":1}`，`languages=[]`，`mediaCount=1`

### 公开 API（daemon 专用）

- `GET /api/v1/pages` → 200 `{"total":1,...}`；唯一条目：SEO 验证文章，`url:"/posts/seo-post/"`、`section:"posts"`、tags=[seo]、有 description
- daemon serve 的 `/posts/seo-post/` **无注入标记**（daemon 构建不传 PluginRegistry，见偏差 5）；注入断言只对 `huan build` 产物做

### 已知状态偏差（实测发现，写用例时必须绕开或显式断言）

1. **`$HUAN_HOME`（默认 `~/.huan/plugins`）的陈旧 .so 会毒化同名插件加载**：loader 先扫 `$HUAN_HOME` 再扫项目 `plugins/`；`~/.huan` 里的 .so 若与当前 host 二进制非同一次构建（Go plugin 按各包 buildid+绝对源码路径校验），`plugin.Open` 失败后**同名包的第二次 Open 也连带失败**（报 `different version of package …`，连本地全新 .so 也被拒），build 静默继续（collection-not-interruption）→ 无注入、无报错退出码。**E2E 启动 daemon/build 前必须 `export HUAN_HOME=<空目录>`** 隔离全局插件目录；主机相对路径（worktree vs 主仓库）不同即触发。
2. **seo-injector 的 `og:type` 对文章页误报 `website`**：`guessKind` 把所有 `*/index.html` 一律判为 section（分页 `/page/` 除外），`posts/seo-post/index.html` 也是该形态 → og:type=website（文章本应 article）。断言只能取 `website`。
3. **sitemap-enhancer 对 huan 自产 sitemap 恒 no-op**：内置 sitemap 模板给每个 url 预填 `priority=0.5/changefreq=weekly`（config 默认），而该插件只「填空不覆盖」（`Priority==0 || Changefreq==""` 才写入）→ 无任何可观察效果（显式配 `defaultPriority` 也无效，因非空不触发）。sitemap 断言不能作为 sitemap-enhancer 生效的证据；它的「生效」只能通过 daemon plugins list 或未来 fixture 造空 priority sitemap 验证。
4. **（已过时，见上方口径更新）daemon `/admin/api/plugins` list 只显示 `source:"compiled"` 的 1 条**：~~实测 list 恒 `total:1`~~。Task 3 首测（commit 801d00f，13223 端口）记 total=1；Task 8（9e4c675 前后）与 Task 13 全量实跑（本表更新日）两次独立复测均为 **total=2（双 .so 编译）→ total=3（simple-plugin.so 加载后）**——首测的 registry 只进了单插件，复测两插件均以 compiled 身份注册。断言以复测口径为准；`ScanAndLoad` 的同名冲突跳过机制仅在与 compiled 同名时触发（simple-test 不同名故正常加载）。
5. ~~**daemon 的 JIT/预渲染页面不经插件钩子**~~ **已修复（2026-08-15，fix-e2e-found-bugs Task 5）**：原缺陷——`internal/daemon/builder.go` 的 `build.Options` 未传 `PluginRegistry`，`OnOutputWritten` 不执行 → daemon serve 的 HTML 无注入。修复：`BuilderOptions` 新增 `PluginRegistry`，`build.Options` 传给全量/增量/JIT 三路径；daemon 初始化 7.5 块移到 7（builder）之前。原文保留如上。修复后 daemon serve `/posts/seo-post/` 有注入（og: ×4 实测）。注入断言现对 daemon 与 `huan build` 均成立。
6. **dev 模式 build 会注入**（dev.go 传 registry；输出在临时目录），但 HTTP 响应页实测无注入标记（serve 路径与 build 输出非同源），且 `--plugins` 缺省时依赖 `<source>/plugins/` 存在——dev 模式不做插件断言，统一走 CLI build + daemon。

### 实测记录（2026-08-15，worktree e2e-test-system，commit 801d00f 构建）

```bash
go build -o /tmp/huan-t3 ./cmd/huan
rm -rf /tmp/t3-with-plugins && cp -r tests/e2e/fixtures/with-plugins /tmp/t3-with-plugins
( cd plugins/seo-injector && go build -buildmode=plugin -o /tmp/t3-with-plugins/plugins/seo-injector.so . )
( cd plugins/sitemap-enhancer && go build -buildmode=plugin -o /tmp/t3-with-plugins/plugins/sitemap-enhancer.so . )

# 不隔离 HUAN_HOME（~/.huan 有 7-29 旧 .so）→ 4 条 plugin load warning、0 注入
/tmp/huan-t3 build -s /tmp/t3-with-plugins 2>&1 | grep -c "huan seo-injector"   # 0

# 隔离后 → 注入成功
rm -rf /tmp/t3-empty-home && mkdir -p /tmp/t3-empty-home
HUAN_HOME=/tmp/t3-empty-home /tmp/huan-t3 build -s /tmp/t3-with-plugins
grep -c "huan seo-injector" /tmp/t3-with-plugins/public/posts/seo-post/index.html   # 1
# <!-- huan seo-injector -->
# <meta name="description" content="SEO 验证文章 这是 SEO 注入验证 的正文段落。">
# <meta property="og:title" content="SEO 验证文章 - huan E2E 插件站"> ...

HUAN_ADMIN_TOKEN=<fixed> HUAN_HOME=/tmp/t3-empty-home /tmp/huan-t3 daemon -s /tmp/t3-wpd --port 13215 --bind 127.0.0.1
curl -s http://127.0.0.1:13215/api/v1/pages   # {"data":[{SEO 验证文章...}],"total":1,...}
curl -s -H "Authorization: Bearer <fixed>" http://127.0.0.1:13223/admin/api/plugins
# {"plugins":[{"name":"seo_injector","version":"0.1.0","source":"compiled",...}],"total":1}
# ↑ Task 3 首测口径（单插件 registry，已过时）。复测口径（Task 8 + Task 13，双 .so + simple-plugin.so）：
#   {"plugins":[{seo_injector...compiled},{sitemap_enhancer...compiled},{simple-test...loaded}],"total":3}
#   断言以复测口径为准（见「GET /admin/api/plugins 期望」节的口径更新说明）。
```
