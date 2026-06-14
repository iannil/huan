# ADR 0007：i18n 多语言构建系统

- **状态**：Proposed（待 PR2 落地后转 Accepted）
- **日期**：2026-06-14
- **决策者**：用户（owner）+ Claude（grill-me 收敛）
- **依赖**：[ADR 0003](0003-unified-plugin-system.md)（统一插件系统）/ [ADR 0008](0008-translator-capability-qwen3-plugin.md)（Translator capability）
- **被引用**：[ADR 0008](0008-translator-capability-qwen3-plugin.md)（translate plugin 产出 `.en.md` 供本文档消费）

## 背景

zhurongshuo 当前是单语言（zh-cn）静态站点。用户需求：

1. 默认语言仍是中文（zhurongshuo 的身份与受众决定）
2. deploy 时把中文内容翻译为英文（避免 runtime 翻译的延迟与成本）
3. 访问者打开站点时，按浏览器 `Accept-Language` 自动显示中文版或英文版

这是一套"预生成双语静态树 + 边缘 Worker 路由"的经典 i18n 模式，但实施层面需要回答：

- 翻译产物（英文 markdown）由谁生成、如何缓存、如何增量（由 [ADR 0008](0008-translator-capability-qwen3-plugin.md) 的 translate plugin 解决）
- **双语站点如何被 huan build pipeline 消费、并行 site tree 如何构建、hreflang 如何注入、内链如何重写、sitemap 如何组织**（本文档解决）
- 浏览器语言如何检测、cookie 如何记忆、URL 如何路由（本文档 §10 解决，由 zhurongshuo 仓库的 Cloudflare Worker 实施）

**关键架构问题**：i18n 多语言构建是 plugin concern 还是 core feature？

**结论**：i18n build 是 **core feature**（与 RSS / sitemap / taxonomy 同级），translate 是 plugin。原因：

- plugin 不能复制 site tree 构建逻辑（content discovery / taxonomy / output path 计算都依赖 build pipeline 内部状态）
- 模板层、content discovery、output 路径计算都需要语言感知——这些是 huan core 的职责
- 与 Hugo 模型对齐（languages 是 core，翻译服务才是 plugin）

## 决策

### 1. 架构分层：i18n build = core，translate = plugin

| 关注点 | 归属 | 包路径 |
|---|---|---|
| 多语言 site tree 构建 / 内容发现 / 输出路径 | **huan core** | `internal/build/i18n.go` / `internal/build/multisite.go` |
| Languages 配置解析 | **huan core** | `internal/config/languages.go` |
| 模板 i18n helper（i18n / translations / hreflang） | **huan core** | `internal/template/funcs.go`（扩展）|
| 内链 URL rewriting | **huan core** | `internal/build/i18n_rewrite.go` |
| 统一 sitemap + hreflang annotation | **huan core** | `internal/build/sitemap.go`（扩展）|
| **翻译生成**（调 LLM 产 `.en.md`） | **plugin** | `internal/translate/qwen3/`（见 ADR 0008）|
| Runtime 语言路由 | **zhurongshuo 仓库** | `worker/i18n-router/`（Cloudflare Worker）|

**契约**：translate plugin 的唯一产出是 `.en.md` sidecar 文件（标准 markdown + frontmatter）。huan core 在 content discovery 阶段识别 `.en.md` 后缀并纳入 build pipeline。**`.en.md` 文件也可以由人工创建**（不强制经过 plugin）。

### 2. huan.yaml 配置扩展

新增 `languages:` 块（Hugo 风格，最小可用配置）：

```yaml
defaultContentLanguage: zh-cn
languages:
  zh-cn:
    weight: 1
    languageName: 中文
    baseURL: ""                  # 根路径
    contentDir: content/         # 默认（隐式）
    title: "祝融说。"             # 可选，覆盖顶级 title
  en:
    weight: 2
    languageName: English
    baseURL: /en                 # 子路径前缀
    title: "Zhurong Says"
```

**规则**：
- `defaultContentLanguage` 是 fallback；浏览器语言无法匹配时回到默认
- `weight` 决定 sitemap.xml 中语言列出顺序（低 weight 在前）
- `baseURL` 决定该语言输出路径前缀；空字符串 = 根路径
- 不在 `languages:` 块重复 `params`（站点级 metadata 翻译由 translate plugin 缓存，见 §6）

向后兼容：未声明 `languages:` 块时，huan 行为不变（单语言 zh-cn），不破坏现有项目。

### 3. Content discovery：`.en.md` sidecar 后缀

文件后缀规则（Hugo 既有约定，LLM friendly）：
- `<name>.md` → 默认语言（zh-cn）
- `<name>.<lang>.md` → 指定语言变体（如 `foo.en.md`）
- `<name>.<default-lang>.md` → 显式默认语言（罕见，等价于 `<name>.md`）

扫描逻辑（`internal/content/discover.go`）：

```go
for each file matching *.md:
    if name matches `*.en.md`:
        register as English variant of the paired zh-cn post
    else if name matches `*.zh-cn.md`:
        explicit zh-cn (rare; treated as default)
    else:
        register as default language (zh-cn)
```

**关键不变量**：
- 一篇 post 的 zh-cn 版必须存在（缺失 → build 失败）
- en 版可选（缺失 → 该 post 只有 zh-cn 版；en 站点不出现该 post；sitemap/hreflang 不引用它）
- 这避免了"半生不熟的英文版"问题——只有完整翻译才纳入 en build

实现细节：`internal/content` 新增 `MultiLanguagePage` 结构：

```go
type MultiLanguagePage struct {
    DefaultPath string               // "posts/foo.md"
    Translations map[string]*Page    // "zh-cn" / "en" → Page
}
```

### 4. MultiSite 并行 site tree 构建

新增 `internal/build/multisite.go`：

```go
type MultiSite struct {
    Default   *Site                   // 默认语言 site
    Languages map[string]*Site        // "zh-cn" / "en" → Site
}

type Site struct {
    Language      string
    BaseURL       string              // "" or "/en"
    LanguageName  string
    Weight        int
    RegularPages  []*Page
    Taxonomies    map[string]TermMap
    Menus         Menu
    // ... 现有 Site 字段
}
```

`BuildSite` 改为 `BuildMultiSite`：
1. 收集所有 pages，按 language 分组（基于 content discovery 结果）
2. 为每个 language 构建独立 site tree（taxonomy / RSS / list pages）
3. 每个 site 独立处理路径与输出

**与现有 `BuildSite` 的兼容**：单语言场景下（无 `languages:` 块）`BuildMultiSite` 退化为单 site，行为不变。

### 5. 模板 context 扩展

`tmpl.Context` 新增字段：

```go
type Context struct {
    // ... 现有字段 ...
    Language          string             // 当前页语言 ("zh-cn" / "en")
    IsDefaultLanguage bool               // 当前页是否默认语言
    Translations      []TranslationLink  // 该页所有语言版本
}

type TranslationLink struct {
    Lang         string  // "zh-cn" / "en"
    LanguageName string  // "中文" / "English"
    URL          string  // 完整 URL（含 baseURL prefix）
    IsCurrent    bool
}
```

新增模板函数：

| 函数 | 签名 | 用途 |
|---|---|---|
| `i18n` | `func(key string) string` | 按 `.Language` 取 `i18n/<lang>.yaml` 字符串（复用 `internal/i18n.Bundle`）|
| `translations` | `func(ctx *Context) []TranslationLink` | 返回当前页所有语言版本的 TranslationLink 列表 |
| `hreflang` | `func(ctx *Context) template.HTML | 输出所有 `<link rel="alternate" hreflang="...">` 标签（拼接好的 HTML） |
| `langPrefix` | `func(ctx *Context) string` | 当前语言的 URL 前缀（"" 或 "/en"） |

`i18n/<lang>.yaml` 格式沿用现有 Hugo 风格（key → {other: value}）。`internal/i18n/Bundle` 已支持 LoadDir，无需改造。

### 6. 站点级 metadata 翻译机制

`huan.yaml::params` 是单值（如 `params.subTitle: "法不净空，觉无性也。"`）。双语支持两种方案：

**采纳方案 A：plugin config 缓存翻译**

```yaml
plugins:
  qwen3_translate:
    # ...
    site_translations:
      en:
        subTitle: "The Dharma is not inherently empty; awareness has no nature."
        description: "..."
        keywords: ["Zhurong", "Zhurong Says"]
        footerSlogan: "..."
```

build 时按当前语言选择：zh 用 `params.subTitle`，en 用 `plugins.qwen3_translate.site_translations.en.subTitle`。

**否决方案 B（`huan.yaml::params` 直接双语字段）**：会让 `huan.yaml` schema 复杂化，且 params 是 source-of-truth，加入衍生翻译破坏单一真相源原则。

### 7. 输出路径

| 内容类型 | zh-cn 路径 | en 路径 |
|---|---|---|
| 首页 | `/index.html` | `/en/index.html` |
| Post | `/posts/<slug>/index.html` | `/en/posts/<slug>/index.html` |
| Section list | `/posts/index.html` | `/en/posts/index.html` |
| Term page | `/tags/<zh-term>/index.html` | `/en/tags/<en-term>/index.html`（term 翻译走 `i18n/terms.yaml`）|
| Taxonomy index | `/tags/index.html` | `/en/tags/index.html` |
| RSS | `/rss.xml`（或 `/index.xml`） | `/en/rss.xml`（或 `/en/index.xml`） |
| Sitemap | `/sitemap.xml`（**单一**，含所有语言 + hreflang）| （同上，不单独生成） |
| Robots | `/robots.txt`（共用） | （同上） |
| Manifest / favicon | `/manifest.json` / `/favicon.ico`（共用） | （同上） |

**Sitemap 单一**是 Google 推荐做法（不要分裂成 `sitemap.xml` + `en/sitemap.xml`）。每个 URL 条目附 `<xhtml:link rel="alternate" hreflang="...">` 注解所有语言版本。

### 8. 内链 URL rewriting

作者在 zh post 内写 `[/posts/foo/](/posts/foo/)`。在 en build 时，该链接需重写为 `/en/posts/foo/`。

实现（`internal/build/i18n_rewrite.go`）：

```go
// RewriteInternalLinks post-processes rendered HTML for non-default languages.
// It scans <a href="/..."> and prepends the language baseURL prefix to
// internal links, leaving external URLs / anchors / static resources untouched.
func RewriteInternalLinks(html string, langPrefix string) string
```

**规则**：
- `<a href="/...">` 以 `/` 开头 + 非 `/en/` 开头 → 加 langPrefix 前缀
- 外链（`http://` / `https://` / `mailto:` / `tel:`）不动
- 锚点（`#...`）不动
- 静态资源 URL（`/cdn-images/...` / `/favicon.ico` / `/manifest.json` / `/robots.txt`）不动
- 调用时机：`RenderPage` 之后、写盘之前

### 9. hreflang 注入策略

**显式 + 兜底**：

- 模板在 `<head>` 调用 `{{ hreflang . }}`（推荐方式）
- build pipeline 后处理：若模板输出未含 `<link rel="alternate" hreflang=`，注入 `{{ hreflang . }}` 等价 HTML + 警告日志

**为什么不全自动注入**：模板布局可能复杂（不同 section 不同 head 结构），强制后处理可能破坏模板预期。**显式调用 + 兜底警告**兼顾灵活性与正确性。

### 10. Runtime 路由：Cloudflare Worker 边缘检测（zhurongshuo 仓库实施）

i18n router Worker 由 zhurongshuo 仓库持有（`worker/i18n-router/`），通过 `huan deploy cloudflare worker` 部署。本文档锁定 Worker 行为契约：

| 路径模式 | Worker 行为 |
|---|---|
| `/cdn-images/*`（image-resizer 路径） | 不拦截（已由 image-resizer Worker 处理）|
| `/en/*` | **不拦截**（纯静态，CDN 直接命中）|
| `/`、`/posts/*`、`/tags/*`、`/about/*`、...（所有 zh-cn 路径）| 拦截：无 cookie → 检测 + 设 cookie + 302；有 cookie → 透传 |
| `/sitemap.xml`、`/robots.txt`、`/rss.xml`、`/index.xml` | **不拦截**（SEO 资源必须确定性输出）|

**检测逻辑**：
- 解析 `Accept-Language` header q-value 列表
- 取最高 q-value，前缀匹配：`en*` → 倾向 en；`zh*` 或其他 → 默认 zh-cn
- 无 cookie + en 倾向 → 302 redirect 到 `/en/<原路径>` + `Set-Cookie: lang=en; ...`
- 有 cookie → cookie 主导，不再检测

**Cookie 设计**：
- name: `lang`
- value: `zh` / `en`（二元，不带 region）
- domain: `.zhurongshuo.com`（带前导 dot，子域共享）
- path: `/`
- expiration: `Max-Age=31536000`（1 年）
- SameSite: `Lax`（外链点入也带 cookie）
- Secure: `true`
- HttpOnly: `true`（Worker 服务端读，JS 不需要）

**不做 geo fallback**（cf-IPcountry）：v1 不引入。Accept-Language 在所有现代浏览器都正确设置；地理 fallback 反而误判（海外华人 / 国内英语用户反例多）。

**切换按钮**：每页右上角 / 页脚放"中文 / English"link，点击 → `<对端 URL>`（不预置 cookie，让用户偶尔"瞄一眼"对端成为轻量操作）。

### 11. Tag 翻译机制（与 build 集成）

zhurongshuo 的 tags 是文化负载短词（如 "专注" / "觉察" / "祝融"）。Tag 翻译必须跨文章一致——所有 50 篇带 "专注" tag 的英文文章都必须用 "focus"。

**机制**：
- `i18n/terms.yaml`：手工维护的中英术语字典
  ```yaml
  专注: focus
  觉察: awareness
  祝融: Zhurong
  法: Dharma
  道: the Way
  ```
- 新 tag 出现 → 字典查不到 → **fail-fast 阻断翻译**，提示用户运行 `huan translate terms --propose`（详见 ADR 0008）
- Tag URL：zh-cn 用原始中文（`/tags/专注/`），en 用字典译文（`/en/tags/focus/`）
- Term page title：zh-cn 显示原始中文 tag；en 显示字典译文

**为什么手工字典而非 LLM 自由翻译**：tag 是站点架构的一部分（影响 URL / taxonomy / RSS / cross-reference），不能由 LLM 单次翻译随机决定。手工字典是"术语表"，是这类内容站点的标配。

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| 架构形态 | i18n build 作为 plugin | plugin 无法复制 site tree 构建逻辑；模板/output/discovery 都需语言感知，这些是 core 职责 |
| URL 形状 | 子域名 `en.zhurongshuo.com` | DNS 新增记录 + Worker Host 路由 + 跨子域 cookie + SEO 权重分散；工程量翻倍 |
| URL 形状 | 参数 `/?lang=en` | 搜索引擎索引混乱；Cloudflare CDN 默认按 path 缓存，参数绕过缓存 |
| Runtime 路由 | 客户端 JS 跳转 | FOUC + SEO 不友好 + 首屏闪烁；GH Pages + Cloudflare 镜像架构下 Worker 走另一套，体验割裂 |
| Runtime 路由 | 不做自动跳转，靠 hreflang + 手动 | 不满足用户"自动显示"需求 |
| 检测机制 | 301 永久重定向 | 浏览器永久缓存，用户切换体验差；测试环境出错难恢复 |
| 检测机制 | Worker 内部 rewrite（无重定向）| URL 与内容语言不一致 = Google cloaking 风险（整站降权）|
| 检测机制 | 不记忆（每次都检测） | 用户主动切换"看英文版"后刷新又跳回中文；切换按钮形同虚设 |
| 检测机制 | cf-IPcountry 地理 fallback | 海外华人 / 国内英语用户反例多；Accept-Language 已够用 |
| 翻译产物存放 | 并行目录树 `content/en/...` | 目录结构调整需双向 sync；与 Hugo `.en.md` 约定不一致 |
| 翻译产物存放 | 外部缓存 `.translate-cache/`（不入 git）| CI 无 GPU，路线不通 |
| 翻译产物存放 | 不入 git，CI 内置 hash 缓存 | 同上 |
| 翻译产物 frontmatter | 无 source_hash（每次重译）| $22 × N deploy 成本累积；增量无意义 |
| 站点级 metadata 翻译 | `huan.yaml::params` 直接双语字段 | 破坏 params 单一真相源；schema 复杂化 |
| Tag 翻译 | LLM 自动决定 | 跨文章术语不一致；URL 漂移；不可预测 |
| Sitemap | 分裂 `/sitemap.xml` + `/en/sitemap.xml` | Google 推荐统一 sitemap + hreflang；分裂后双索引风险 |
| hreflang 注入 | 全自动后处理 | 复杂模板布局下破坏预期；显式 + 兜底更稳 |
| 内链 rewriting | 模板层手动处理 | 模板负担重；遗漏率高；后处理自动化更可靠 |

## 影响

### 文档

- 新增本 ADR（0007）
- 新增 [ADR 0008](0008-translator-capability-qwen3-plugin.md)（Translator capability + Qwen3 plugin，本文档的 producer side）
- 更新 `docs/INDEX.md`：加 i18n 命令索引
- 更新 `docs/technical-plan.md`：加 i18n build 阶段
- 更新 `README.md` / `README.zh-CN.md`：双语能力介绍

### 代码（huan core）

**新增**：
- `internal/build/i18n.go`：i18n build 协调逻辑
- `internal/build/multisite.go`：MultiSite 结构与构建
- `internal/build/i18n_rewrite.go`：内链 URL rewriting
- `internal/config/languages.go`：languages 块解析

**改造**：
- `internal/content/discover.go`：识别 `.en.md` sidecar
- `internal/content/types.go`：新增 `MultiLanguagePage`
- `internal/build/build.go`：`BuildSite` → `BuildMultiSite`（保持单语言兼容）
- `internal/build/sitemap.go`：统一 sitemap + hreflang annotation
- `internal/template/funcs.go`：加 `i18n` / `translations` / `hreflang` / `langPrefix` 函数
- `internal/template/context.go`：Context 加 Language / IsDefaultLanguage / Translations 字段
- `internal/output/`：per-lang 路径计算

### 代码（zhurongshuo 仓库）

- 新增 `worker/i18n-router/`：Cloudflare Worker（302 + Cookie）
- 新增 `i18n/terms.yaml`：手工术语字典
- 新增 `i18n/translate-prompt-zh-en.md`：translate plugin 的 system prompt（ADR 0008 引用）
- 改造 `layouts/_default/baseof.html`：加 `{{ hreflang . }}` + 语言切换按钮
- 改造 `layouts/partials/header.html`：加语言切换 UI
- 改造 `layouts/partials/post_meta.html`：日期格式按语言切换
- 替换 ~30 处硬编码中文 UI 串为 `{{ i18n "key" }}`
- 改造 `.github/workflows/deploy.yml`：build 时设 `HUAN_STRICT_I18N=true`（CI stale 检测严格模式）
- 改造 `deploy.sh`：加 `huan deploy cloudflare worker` 部署 i18n router

### 风险

1. **模板改造工作量**：~30 处硬编码中文替换为 `{{ i18n "key" }}`，~1-2 天工作量在 zhurongshuo 仓库
2. **内链 rewriting 边缘 case**：外链 / 锚点 / 静态资源 / 跨语言 deep link 需明确分类，遗漏率高时表现差异大。缓解：单元测试覆盖所有 URL 模式
3. **hreflang 配置错误**：导致 SEO 双索引惩罚。缓解：单一 sitemap + 严格测试 + Google Search Console 验收
4. **MultiSite 内存翻倍**：1500 文章 × 2 语言 = 3000 Page 对象，huan build 内存占用增长。缓解：评估实际占用，必要时 stream 处理
5. **stale 翻译产物检测**：CI strict mode 阻断 deploy，但用户本地非 strict mode 可能漏检。缓解：本地 pre-commit hook 提示
6. **i18n Worker 部署失败**：必须不阻断主 deploy。缓解：HTML/Worker/翻译产物三个独立回滚单元（见 ADR 0008 §10.3）
7. **多语言 build 时间增长**：build 时间预计增长 30-50%（双 site tree + 双输出）。缓解：内容 hash 缓存 + 增量 build（未来工作）
