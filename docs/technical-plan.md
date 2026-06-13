# huan 技术方案

> 阶段一：替代 Hugo，与 zhurongshuo.com 站点输出达成三维度等价（肉眼/SEO/AI，[ADR 0001](adr/0001-redefine-equivalence.md)）
>
> **⚠️ 历史标注**：本文档 §4.x 详细设计撰写于 stage 1 期间，其中 **§4.4 redact shortcode 与 §4.5 encrypt 系统、§4.1 huan.yaml 的 `encryptGroups` 示例、参数对照表里的加密相关行，均已于 v0.2.0 移除**（zhurongshuo 实际未启用，详见 [ADR 0005](adr/0005-remove-encrypt-and-v02-feature-batch.md)）。这些段落作为当时设计参考保留，不再反映当前代码状态。当前实际结构以 §3 与 [`docs/INDEX.md`](INDEX.md) 为准。

## 1. 项目定位

huan 是一个用 Go 编写的静态站点生成器。阶段一目标是将 zhurongshuo.com 从 Hugo 迁移到 huan，**生成的站点输出与 Hugo 在肉眼 / SEO / AI 三维度无差异（甚至更好）**——不是逐字节 100% 一致，而是三维度门禁通过（[ADR 0001](adr/0001-redefine-equivalence.md)）。阶段二/三通过统一插件系统（[ADR 0003](adr/0003-unified-plugin-system.md)）增量扩展 deploy / 未来付费 / 多语言等能力。

## 2. 架构决策

| 决策项 | 结论 |
|--------|------|
| 语言 | Go |
| 模板引擎 | 阶段一 `html/template`，阶段二可插件替换 |
| Markdown | goldmark（与 Hugo 同源库）+ chroma 语法高亮（stage 3 port） |
| Shortcode | Go 重新实现，输出一致即可（audio/img；redact v0.2.0 移除） |
| 数据模型 | 保留 Site/Page 对象传递方式 |
| 项目定位 | 独立项目，一次性迁移，非 drop-in |
| 配置格式 | `huan.yaml`（YAML） + `${VAR}` strict 插值 |
| 验证方式 | `./scripts/diff-build.sh` 四模式对比（byte 雷达 + normalized/seo/ai 三维度门禁） |
| 插件架构 | 统一插件系统（[ADR 0003](adr/0003-unified-plugin-system.md)）；首个实例为 Cloudflare deploy 插件（[ADR 0002](adr/0002-cloudflare-deploy-plugin.md)） |
| 发布 | `huan release` 本地打包（[ADR 0004](adr/0004-release-command.md)）+ GitHub Actions 自动建 Release（[ADR 0005](adr/0005-remove-encrypt-and-v02-feature-batch.md)） |

## 3. 项目结构

> **当前实际结构**（v0.2.2）。`internal/encrypt/` 与 `pkg/` 已不存在：前者 v0.2.0 移除（[ADR 0005](adr/0005-remove-encrypt-and-v02-feature-batch.md)），后者从未建（YAGNI）。

```
huan/
├── cmd/
│   ├── huan/              # CLI 入口（13 子命令，详见 docs/INDEX.md）
│   └── equiv-check/       # 三维度等价检查独立工具
├── internal/
│   ├── config/            # huan.yaml 解析 + ${VAR} strict 插值
│   ├── content/           # 内容加载、frontmatter、content tree、cascade inheritance
│   ├── markdown/          # goldmark + chroma 语法高亮
│   ├── shortcode/         # shortcode 注册与展开（audio/img）
│   ├── template/          # 模板加载、函数注册、Scratch、SortDefault
│   ├── taxonomy/          # 标签/分类（含 BuildWithOriginalCase）
│   ├── pagination/        # 分页器
│   ├── build/             # 构建管线编排（含原子 swap、summary 截断）
│   ├── output/            # 文件写入、minify、canonify、contentapi、llmstxt
│   ├── i18n/              # i18n bundle + collator（zh-cn 拼音序）
│   ├── serve/             # 开发服务器（HTTP + fsnotify + LiveReload）
│   ├── plugin/            # 统一插件宿主（详见 §4.11 / ADR 0003）
│   ├── deploy/            # Deployer capability + Report + JSON Logger
│   ├── deploy/cloudflare/ # Cloudflare 实现：Pages / R2 / Worker（详见 ADR 0002）
│   ├── observability/     # 跨包 JSON Logger（deploy + release 共用）
│   ├── release/           # 跨平台打包（详见 ADR 0004）
│   ├── version/           # VCS info（git SHA via shell out）
│   └── equiv/             # 三维度等价算法（SEO/AI extractor）
├── scripts/               # diff-build / diff-summary / diff-patterns / allowed-diffs.txt
├── docs/                  # 文档
├── memory/                # 双层记忆（MEMORY.md + daily/）
├── release/               # 发布产物（.gitignore）
├── .github/workflows/     # CI（release.yml：v* tag push 自动建 Release）
├── go.mod / go.sum
├── huan.yaml              # huan 项目自身的示例配置
├── README.md / README.zh-CN.md / LICENSE
└── CLAUDE.md
```


## 4. 核心模块设计

### 4.1 config — 配置系统

解析 `huan.yaml`，构建全局配置对象。

```yaml
# huan.yaml 示例
baseURL: "https://zhurongshuo.com/"
title: "祝融说。"
languageCode: "zh-cn"
publishDir: "docs"
paginate: 10
minify: true

author:
  name: "祝融"

params:
  subTitle: "法不净空，觉无性也。"
  footerSlogan: "法不净空，觉无性也。"
  keywords: ["祝融", "祝融说"]
  description: "祝融说：法不净空，觉无性也。"
  copyrights: "Copyright © 2010-2026 祝融说 zhurongshuo.com All Rights Reserved."
  enableMathJax: true
  enableSummary: true
  mainSections: ["posts", "post"]
  googleAnalytics: "G-KKJ5ZEG1NB"
  cdnURL: "https://r2.zhurongshuo.com"

menu:
  main:
    - name: "首页"
      weight: 10
      identifier: "home"
      url: "/"
    - name: "开始"
      weight: 20
      identifier: "start"
      url: "/start/"
    # ... 其余菜单项

social:
  - name: "book-open"
    url: "/books/"
    weight: 10
  # ... 其余社交链接

markup:
  goldmark:
    renderer:
      unsafe: true
    extensions:
      typographer: false

sitemap:
  changefreq: "weekly"
  filename: "sitemap.xml"
  priority: 0.5

rss:
  limit: 20

outputs:
  home: ["HTML", "RSS", "SearchIndex"]
  page: ["HTML"]
  section: ["HTML", "RSS"]
  taxonomy: ["HTML", "RSS"]
  term: ["HTML", "RSS"]
```

**与 Hugo config.toml 的映射关系：**

| Hugo (TOML) | huan (YAML) | 说明 |
|-------------|-------------|------|
| `[params]` | `params:` | 扁平化参数 |
| `[[menu.main]]` | `menu.main:` | 菜单数组 |
| `[markup.goldmark]` | `markup.goldmark:` | Markdown 渲染配置 |
| `pagerSize` | `paginate:` | 分页大小 |
| `publishDir` | `publishDir:` | 输出目录 |

### 4.2 content — 内容加载

**职责：** 扫描 content 目录，解析每个 .md 文件的 frontmatter + body，构建内容树。

**核心数据结构：**

```go
type Page struct {
    // Frontmatter
    Title       string
    Date        time.Time
    Lastmod     time.Time
    Draft       bool
    Hidden      bool
    Type        string
    Slug        string
    Tags        []string
    Keywords    []string
    Description string
    Author      string
    Image       string
    FeaturedImage string

    // 访问控制
    Access       string // public, protected, private
    EncryptGroup string
    EncryptMode  string // full, random
    EncryptRatio int

    // Hugo 兼容字段
    Build   BuildConfig
    Cascade CascadeConfig
    Sitemap SitemapConfig

    // 计算字段
    FilePath    string // 源文件路径
    FileDir     string // 源文件目录
    RelPermalink string // 相对 URL
    Permalink   string // 绝对 URL
    Section     string // 所属 section (posts, books, ...)
    Kind        string // page, section, home, taxonomy, term
    Content     template.HTML // 渲染后的 HTML 内容
    Summary     template.HTML
    Plain       string
    WordCount   int
    ReadingTime int

    // 子页面
    Pages         []*Page
    RegularPages  []*Page
    Parent        *Page
}

type Site struct {
    Title      string
    BaseURL    string
    Language   string
    Params     map[string]interface{}
    Menus      map[string][]MenuItem
    Pages      []*Page
    RegularPages []*Page
    Data       map[string]interface{} // data/*.yaml 加载结果
    Taxonomies map[string]Taxonomy
    Config     *Config
}

type BuildConfig struct {
    List             string // never, always
    Render           string // never, always
    PublishResources bool
}

type CascadeConfig struct {
    Build BuildConfig
}
```

**内容加载流程：**

1. 递归扫描 `content/` 目录下所有 `.md` 文件
2. 解析 frontmatter（YAML）和 body
3. 从文件路径推导 URL：`content/posts/2020/08/2601.md` → `/posts/2020/0826/2601/`
4. 从 `data/books.yaml`、`data/practices.yaml` 加载元数据，关联到对应 Page
5. 处理 `_index.md` 作为 section 页面
6. 构建 section 层级关系
7. 处理 `cascade` 配置继承
8. 处理 `build.list: never` 的隐藏页面（hidden section）

**URL 推导规则（复现 Hugo 行为）：**

| 源路径 | URL |
|--------|-----|
| `content/posts/2020/08/2601.md` | `/posts/2020/0826/2601/` |
| `content/books/volume-1/reality-construction/_index.md` | `/books/volume-1/reality-construction/` |
| `content/books/volume-1/reality-construction/part-01/chapter.md` | `/books/volume-1/reality-construction/part-01/chapter/` |
| `content/gallery/jin-jie-rc-jian-yi.md` | `/gallery/jin-jie-rc-jian-yi/` |
| `content/tags/_index.md` | `/tags/` |

### 4.3 markdown — Markdown 渲染

**依赖：** `github.com/yuin/goldmark`

**配置对齐：**

```go
func NewGoldmarkRenderer(cfg *MarkupConfig) goldmark.Markdown {
    extensions := []goldmark.Extender{}
    if cfg.Goldmark.Extensions.Typographer {
        // 默认关闭，与 Hugo 配置一致
    }

    options := []goldmark.Option{
        goldmark.WithRendererOptions(
            goldmarkhtml.WithUnsafe(), // unsafe: true
        ),
    }

    return goldmark.New(append(options, goldmark.WithExtensions(extensions...))...)
}
```

**渲染管线：**

1. Shortcode 展开（在 Markdown 渲染之前）
2. goldmark 渲染 Markdown → HTML
3. 后处理：提取摘要、计算字数、生成目录

### 4.4 shortcode — 短代码系统

**当前使用的 shortcode：**

| 名称 | 参数 | 用途 |
|------|------|------|
| `redact` | `force`, `show`, `random`, `ratio` | 内容涂黑 |
| `audio` | `src`, `title` | 音频播放器 |
| `img` | `src`, `title` | 图片（fancybox） |

**接口设计：**

```go
type Shortcode interface {
    Name() string
    Render(ctx *ShortcodeContext) (string, error)
}

type ShortcodeContext struct {
    Params map[string]string
    Inner  string // {{< redact >}}...{{< /redact >}} 之间的内容
    Page   *Page
    Site   *Site
}

// 注册机制
func Register(name string, shortcode Shortcode)
```

**redact shortcode 实现要点：**

- 检查 `force`/`show` 参数
- 检查页面 `redact` frontmatter
- 检查全局 `redactFolders` 配置
- 完全涂黑：统计 rune 数量，生成等量 `█`
- 随机涂黑：用 MD5 内容 hash 生成确定性种子，按 `(index * 31 + seed) % 100 < ratio` 决定涂黑
- 输出 `<span class="redacted">...</span>`

### 4.5 encrypt — 加密/涂黑系统（v0.2.0 已移除）

> **本节为历史设计记录**。`internal/encrypt/` 整包 + `internal/shortcode/redact.go` 已于 v0.2.0（commit `5c220e2`）移除——zhurongshuo 实际未启用加密功能。详见 [ADR 0005](adr/0005-remove-encrypt-and-v02-feature-batch.md)。下方内容仅作当时设计参考保留。

**与 shortcode 的区别：** shortcode 处理内联标记，encrypt 处理整页的 `access: protected` 逻辑。

**处理流程：**

```
Page.access == "protected" ?
├── No → 正常输出 .Content
└── Yes
    ├── 确定 encryptGroup（frontmatter → books.yaml/practices.yaml → default）
    ├── 从 data/encrypted/content.json 读取加密数据（按 fileId = MD5(文件路径)）
    ├── encryptMode == "full" → 输出完整涂黑占位 + 加密数据属性
    └── encryptMode == "random" → 输出部分涂黑 + 随机涂黑 JS 数据属性
```

**模板输出结构（必须与 Hugo 输出一致）：**

```html
<!-- full 模式 -->
<div class="encrypted-content"
     data-encrypted="..."
     data-group="default"
     data-mode="full"
     data-ratio="50"
     data-seed="..."
     data-title-selector=".post-title">
  <div class="encrypted-content-body">
    <div class="redacted-content"><span class="redacted">████████</span></div>
  </div>
</div>

<!-- random 模式 -->
<div class="encrypted-content" ...>
  <div class="encrypted-content-body">
    <div class="random-redact-content" data-seed="..." data-ratio="50">
      <!-- 原始 HTML 内容 -->
    </div>
  </div>
</div>
```

**注意：** 加密密文的生成仍由 `scripts/encrypt-content.js` 完成（Node.js），huan 只负责在模板渲染时正确读取和嵌入加密数据。

### 4.6 template — 模板系统

**依赖：** `html/template`

**模板查找顺序：**

```
layouts/{type}/{kind}.html        → layouts/products/single.html
layouts/{section}/{kind}.html     → layouts/books/list.html
layouts/_default/{kind}.html      → layouts/_default/single.html
```

**需要注册的自定义模板函数：**

```go
var funcMap = template.FuncMap{
    // 字符串
    "urlize":      strutil.URLize,
    "absURL":      func(s string) string { return cfg.BaseURL + s },
    "safeHTML":    func(s string) template.HTML { return template.HTML(s) },
    "safeJS":      func(s string) template.JS { return template.JS(s) },
    "safeURL":     func(s string) template.URL { return template.URL(s) },
    "plainify":    htmlutil.StripTags,
    "markdownify": mdRenderer.Render,
    "jsonify":     json.Marshal,
    "printf":      fmt.Sprintf,
    "substr":      func(s string, start, end int) string { return s[start:end] },

    // 集合
    "slice":  func(args ...interface{}) []interface{} { return args },
    "append": appendFunc,
    "first":  func(n int, s []interface{}) []interface{} { return s[:n] },
    "where":  whereFunc,
    "sort":   sortFunc,
    "index":  indexFunc,
    "isset":  issetFunc,
    "in":     inFunc,
    "delimit": delimitFunc,

    // 数学
    "add": func(a, b int) int { return a + b },
    "sub": func(a, b int) int { return a - b },
    "mul": func(a, b int) int { return a * b },
    "div": func(a, b int) int { return a / b },
    "mod": func(a, b int) int { return a % b },

    // 字符串操作
    "strings.RuneCount":  utf8.RuneCountInString,
    "strings.Repeat":     strings.Repeat,
    "strings.Split":      strings.Split,
    "strings.Contains":   strings.Contains,
    "strings.HasPrefix":  strings.HasPrefix,
    "strings.ToUpper":    strings.ToUpper,
    "strings.ToLower":    strings.ToLower,
    "strings.Replace":    strings.Replace,
    "strings.ReplaceRE":  regexp.ReplaceAllString,
    "hasPrefix":          strings.HasPrefix,

    // 加密
    "crypto.MD5": func(s string) string { /* MD5 hash */ },

    // 路径
    "path.Base": filepath.Base,
    "path.Dir":  filepath.Dir,

    // 条件
    "default": defFunc,
    "cond":    condFunc,
    "len":     func(v interface{}) int { /* reflect */ },

    // Scratch
    "newScratch": func() *Scratch { return &Scratch{data: map[string]interface{}{}} },
}
```

**模板迁移说明：**

现有 Hugo 模板需要一次性改写。主要变更：
- `{{ partial "xxx.html" . }}` → `{{ template "partials/xxx.html" . }}` 或保持 partial 机制
- `{{ .Site.Params.xxx }}` → 保持（Site 对象结构相同）
- `{{ range .Paginator.Pages }}` → 保持（Paginator 接口相同）
- `{{ with .Params.tags }}` → 保持
- `{{ $.Scratch.Set "key" "value" }}` → 保持（注册 Scratch 类型）

### 4.7 taxonomy — 标签系统

**当前使用：** 仅 `tags`

```go
type Taxonomy map[string]WeightedPages // tag → []Page
type WeightedPages []*Page

func BuildTaxonomies(pages []*Page) map[string]Taxonomy
```

**生成页面：**
- `/tags/` — 标签列表页（按文章数量排序）
- `/tags/{tag}/` — 标签下的文章列表页

### 4.8 pipeline — 构建管线

**构建流程：**

```
huan build
│
├── 1. 加载配置 (huan.yaml)
├── 2. 加载数据文件 (data/*.yaml, data/encrypted/content.json)
├── 3. 扫描内容 (content/**/*.md)
│   ├── 解析 frontmatter
│   ├── 展开 shortcode
│   └── Markdown → HTML
├── 4. 构建内容树
│   ├── 建立 section 层级
│   ├── 关联 data 元数据
│   ├── 处理 cascade 继承
│   ├── 过滤 draft/hidden/build.list=never
│   └── 构建访问控制信息
├── 5. 构建 taxonomy
├── 6. 生成输出
│   ├── 渲染每个 Page（模板 + 内容）
│   ├── 渲染 section 列表页
│   ├── 渲染 taxonomy 列表页
│   ├── 渲染首页
│   ├── 生成 RSS
│   ├── 生成 sitemap.xml
│   ├── 生成 search.json
│   └── 复制 static/ 资源
├── 7. Minify (可选)
└── 8. 写入 publishDir (docs/)
```

### 4.9 output — 输出处理

**文件类型：**

| 类型 | 路径 | 说明 |
|------|------|------|
| HTML | `docs/**/*.html` | 页面 |
| RSS | `docs/index.xml`, `docs/{section}/index.xml` | 订阅源 |
| JSON | `docs/search.json` | 搜索索引 |
| XML | `docs/sitemap.xml` | 站点地图 |
| 静态 | `docs/css/`, `docs/js/`, `docs/images/` | 原样复制 |

**Minify（依赖 `github.com/tdewolff/minify`）：**

- HTML: keepWhitespace = false
- CSS: precision = 0
- JS: precision = 0
- JSON
- SVG
- XML

### 4.10 search — 搜索索引

**复现 `layouts/_default/index.searchindex.json` 的逻辑：**

```go
type SearchEntry struct {
    Title   string `json:"title"`
    URL     string `json:"url"`
    Content string `json:"content"`
    Tags    []string `json:"tags"`
    Date    string `json:"date"`
}
```

**处理规则：**
- 过滤 `access: protected` 的页面
- 移除 `<span class="redacted">` 内容
- 清理 HTML 标签
- 输出 JSON 数组

### 4.11 plugin — 插件架构（统一插件系统）

**详细决策见 [ADR 0003](adr/0003-unified-plugin-system.md)。** 本节为总图速览。

stage 2 起把插件做成 huan 的一等扩展机制，覆盖 deploy / payment / i18n / membership 等所有未来扩展。统一插件宿主位于 `internal/plugin/`，**不**预定义所有 capability 接口（YAGNI），按需在领域包中新增。

```go
// internal/plugin/plugin.go — 统一插件基接口 + Registry
type Plugin interface {
    Name() string  // "cloudflare" / "stripe" / ...
}

type Registry struct { /* name → Plugin */ }

func NewRegistry() *Registry
func (r *Registry) Register(p Plugin) error          // 重名报错
func (r *Registry) Get(name string) (Plugin, bool)
func (r *Registry) All() []Plugin
func Find[T any](r *Registry) []T                    // 按 capability 查询
```

```go
// internal/deploy/types.go — Deployer 是首个 capability 接口
type Deployer interface {
    plugin.Plugin
    Deploy(ctx context.Context, opts Options) (*Report, error)
}
```

**核心约束**（详见 [ADR 0003](adr/0003-unified-plugin-system.md)）：
- Plugin 基接口极简（只有 `Name()`）；配置由构造器吃进去，**不**加 `Init/Start/Stop`。
- Capability 接口分散在领域包（`internal/deploy/`、未来的 `internal/payment/` 等），**不**集中在 `internal/plugin/types.go`。
- 配置统一在 yaml 顶层 `plugins:<name>.*` 命名空间；凭证通过 `${VAR}` strict 插值注入。
- CLI：per-capability verb（`huan deploy cloudflare [...]`）+ 统一管理命令（`huan plugin list/info`）。
- 注册：编译期 hardcoded 在 `cmd/huan/plugins.go`（composition root），避免循环 import；**不**用 `init()` 自注册。

**首期实施**：plugin 系统 + Cloudflare deploy 插件（Pages only）。详见 [ADR 0002](adr/0002-cloudflare-deploy-plugin.md)。

**阶段二及以后扩展方向**（按需新增领域包与 capability 接口）：
- `PaymentProvider` —— 付费（stripe / 微信 / 支付宝），runtime HTTP 回调
- `MultiLanguageProvider` —— 自动多语言翻译，build 或 runtime
- `MembershipProvider` —— 会员等级与鉴权，runtime
- `ContentProcessor` / `TemplateFunctionProvider` / `OutputProcessor` —— build-time 内容/模板/输出扩展（原 §4.11 build-time 骨架，待 build-time 插件真要落地时再画）
- `DynamicRenderPlugin` —— HTTP server 动态渲染

加新插件 = 新建领域包 + 在 `cmd/huan/plugins.go` switch 加 case + yaml 加 `plugins.<name>.*`。**不动 `internal/plugin/`**。

## 5. 模板迁移清单

现有模板需要从 Hugo 语法迁移到 huan 语法。虽然底层都用 `html/template`，但需要调整以下内容：

### 需要迁移的模板文件

| Hugo 路径 | huan 路径 | 主要变更 |
|-----------|-----------|----------|
| `layouts/_default/single.html` | `layouts/_default/single.html` | minimal |
| `layouts/_default/index.searchindex.json` | `layouts/_default/searchindex.json` | Scratch → Go 变量 |
| `layouts/books/list.html` | `layouts/books/list.html` | 数据访问方式 |
| `layouts/book/list.html` | `layouts/book/list.html` | 数据访问方式 |
| `layouts/practices/list.html` | `layouts/practices/list.html` | 数据访问方式 |
| `layouts/practice/list.html` | `layouts/practice/list.html` | 数据访问方式 |
| `layouts/gallery/list.html` | `layouts/gallery/list.html` | minimal |
| `layouts/gallery/single.html` | `layouts/gallery/single.html` | minimal |
| `layouts/products/list.html` | `layouts/products/list.html` | minimal |
| `layouts/products/single.html` | `layouts/products/single.html` | minimal |
| `layouts/shortcodes/redact.html` | 内置到 shortcode 模块 | 完全重写为 Go |
| `layouts/partials/content-redact.html` | `layouts/partials/content-redact.html` | 重写涂黑逻辑 |
| `layouts/partials/js.html` | `layouts/partials/js.html` | minimal |
| `layouts/partials/nav.html` | `layouts/partials/nav.html` | minimal |
| 主题模板 (`themes/zozo/layouts/`) | 直接迁移到 `layouts/` | 合并主题 |

### Shortcode 模板（从主题）

| Shortcode | 来源 | huan 处理 |
|-----------|------|-----------|
| `audio` | themes/zozo | Go 实现 |
| `img` | themes/zozo | Go 实现 |

## 6. 依赖库

```
github.com/yuin/goldmark               # Markdown 渲染
github.com/tdewolff/minify              # 输出压缩
github.com/golang/crypto                # MD5 等（标准库扩展）
gopkg.in/yaml.v3                        # YAML 解析
github.com/spf13/cobra                  # CLI 框架
html/template                           # Go 标准库模板
encoding/json                           # JSON 处理
```

## 7. 实施步骤

### 里程碑 1：项目骨架 + 配置系统
- CLI 入口（cobra）：`huan build`、`huan serve`
- `huan.yaml` 解析
- 配置结构体定义
- 目录结构创建

### 里程碑 2：内容加载 + Markdown 渲染
- content 目录扫描
- frontmatter 解析
- goldmark 集成
- URL 推导
- 内容树构建

### 里程碑 3：模板系统
- 模板加载（layouts/ + partials）
- 自定义函数注册
- Site/Page 对象注入
- 模板渲染执行

### 里程碑 4：Shortcode + 加密系统
- shortcode 注册框架
- `redact` shortcode 实现
- `audio`/`img` shortcode 实现
- 整页加密/涂黑逻辑
- content-redact partial 实现

### 里程碑 5：列表页 + Taxonomy + 分页
- Section 列表页渲染
- books/practices 数据驱动列表
- Tags taxonomy
- 分页器

### 里程碑 6：辅助输出
- sitemap.xml 生成
- RSS feed 生成
- search.json 生成
- static/ 资源复制

### 里程碑 7：Minify + 输出优化
- HTML/CSS/JS minify
- 输出写入
- 404 页面

### 里程碑 8：验证 + 修正
- diff 测试管线搭建
- 逐页面对比修正
- URL 一致性验证
- SEO meta 一致性验证

### 里程碑 9：开发服务器
- `huan serve` 命令
- 文件监听 + 自动重建
- LiveReload

## 8. 验证方案

### 8.1 Diff 测试管线

```bash
# 完整 diff（含 Hugo/huan 重建）
./scripts/diff-build.sh

# 仅生成结构化报告
./scripts/diff-summary.sh
```

### 8.2 当前状态（里程碑 8 进行中）

| 指标 | 数值 |
|------|------|
| Hugo 总文件数 | 2029 |
| huan 总文件数 | 2036 |
| 共有文件数 | 2028 |
| 仅 Hugo（缺失）| 0 |
| 仅 huan（多余）| 8 |
| 完全一致 | 905 |
| 内容差异 | 1124 |

### 8.3 已修复的核心问题

- ✅ 主题静态资源复制（CSS、fonts）
- ✅ 404.html、about/、general/、start/、posts/ 等 section 页面生成
- ✅ Taxonomy term 页面（/tags/、/tags/{tag}/）
- ✅ Taxonomy RSS（/tags/index.xml、/tags/{tag}/index.xml）
- ✅ 分页导航页面（/page/N/）—— /page/1/ 是 redirect，/page/N/ N≥2 是分页 home
- ✅ Leaf bundle 行为（index.md 等价于目录 URL，但不创建 section）
- ✅ Partial 函数返回 template.HTML 避免 HTML escape
- ✅ YAML date 字段（time.Time 类型）正确解析
- ✅ `canonifyURLs` 后处理（root-relative URLs 转为绝对 URL）
- ✅ JSON-LD minify（保留字段顺序，压成单行）
- ✅ Heading ID 生成（CJK 保留，对齐 Hugo 行为）
- ✅ Site.Params 同时支持 lowercase/PascalCase 访问
- ✅ OutputFormats 按 page kind 决定
- ✅ sort 模板函数按字段排序
- ✅ Heading ID 处理 CJK + 中文标点 + HTML 实体
- ✅ 模板查找按 type 优先（type:book 用 book/list.html）
- ✅ Scratch.Set/Add 返回值兼容 Go template action 调用
- ✅ Page Summary 计算（HTML 形式，支持 <!--more-->，按 word count 截断）
- ✅ 子页面 RegularPagesRecursive 正确递归（part-XX 没 _index.md 时归入最近祖先 section）
- ✅ Home title 包含 site.Params.subtitle
- ✅ Site.Copyright 来自 config.copyright（非 params.copyrights）
- ✅ Hugo generator meta（home page + /page/N/ 分页都注入）
- ✅ Home keywords 列表 tag 大小写（lowercase）+ 无 trailing comma
- ✅ urlize 函数输出大写 percent-encoding
- ✅ len 函数支持 TaxonomyContext 类型
- ✅ Type 默认为 Section（Hugo behavior）
- ✅ i18n bundle 加载（zh-cn.yaml 等）
- ✅ Paginate 缓存（/page/N/ 渲染时 partial 重用已设 paginator）
- ✅ categories 空 taxonomy 生成
- ✅ page/1 redirect 到 /
- ✅ 404 page 完整渲染（HTMLOnlyOutputFormats，title "404 Page not found"，无 RSS link）
- ✅ RSS lastBuildDate 用 section RegularPages 而非 site-wide
- ✅ WordCount 从渲染后的 HTML plainify 计算

### 8.4 剩余差异

| 类别 | 说明 | 影响 |
|------|------|------|
| 字数统计精度 | Hugo 用 specialized CJK word segmenter（基于 dictionary），与 huan 简单字符计数有 ~25% 差距 | 列表页 |
| RSS items 顺序 | Hugo 内部多字段排序（date desc → LinkTitle asc → path asc），date 相同时顺序不稳定 | RSS 文件 |
| RSS item description | Hugo summary 在 word 边界截断，huan 在 `</p>` 边界截断 | RSS 文件 |
| products page description | summary 中 block-level 换行（`</h2>\n<p>`）Hugo 转为空格，huan 保留换行 | products 页面 |
| general page summary | summary 截断位置略不同 | general 页面 |

### 8.5 验证检查清单

- [x] 所有 URL 路径一致
- [x] 所有页面 HTML 结构基本一致
- [x] Open Graph / Twitter Card meta 一致
- [x] 加密/涂黑输出一致
- [x] sitemap.xml URL 列表一致
- [x] search.json 内容一致
- [x] 分页导航一致
- [x] 标签页一致
- [x] 静态资源完整复制

## 9. 阶段二预留

阶段二通过插件架构扩展，以下是预期方向（不在阶段一范围内）：

| 插件 | 功能 |
|------|------|
| AuthPlugin | JWT 鉴权，前端身份校验 |
| PaymentPlugin | 内容付费校验 |
| MemberPlugin | 会员等级、权益管理 |
| DynamicRenderPlugin | HTTP server，动态渲染受保护内容 |
| SearchPlugin | 服务端全文搜索 |
| ContentRelationPlugin | 内容关系图、交叉引用 |
| CustomTemplatePlugin | 替换模板引擎 |
