# huan 技术方案

> 阶段一：替代 Hugo，100% 复现 zhurongshuo.com 站点输出

## 1. 项目定位

huan 是一个用 Go 编写的静态站点生成器。阶段一目标是将 zhurongshuo.com 从 Hugo 迁移到 huan，生成的站点输出与 Hugo 完全一致。阶段二通过插件架构增量扩展出版平台能力。

## 2. 架构决策

| 决策项 | 结论 |
|--------|------|
| 语言 | Go |
| 模板引擎 | 阶段一 `html/template`，阶段二可插件替换 |
| Markdown | goldmark（与 Hugo 同源库） |
| Shortcode | Go 重新实现，输出一致即可 |
| 数据模型 | 保留 Site/Page 对象传递方式 |
| 项目定位 | 独立项目，一次性迁移，非 drop-in |
| 配置格式 | `huan.yaml` |
| 验证方式 | diff 管线，与 Hugo 输出逐字节对比 |
| 插件架构 | 阶段一预留骨架，阶段二增量扩展 |

## 3. 项目结构

```
huan/
├── cmd/
│   └── huan/              # CLI 入口
│       └── main.go
├── internal/
│   ├── config/            # huan.yaml 解析
│   ├── content/           # 内容加载、frontmatter 解析
│   ├── markdown/          # goldmark 渲染管线
│   ├── shortcode/         # shortcode 注册与展开
│   ├── encrypt/           # 加密/涂黑系统
│   ├── template/          # 模板加载、函数注册、渲染
│   ├── taxonomy/          # 标签/分类系统
│   ├── pipeline/          # 构建管线编排
│   ├── output/            # 文件写入、minify、sitemap/RSS
│   ├── search/            # 搜索索引生成
│   └── plugin/            # 插件接口定义（骨架）
├── pkg/                   # 可导出的公共库（阶段二用）
├── docs/                  # 文档
├── go.mod
├── go.sum
├── huan.yaml              # huan 项目自身的示例配置
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

  encryptGroups:
    default:
      hint: "受保护内容"
      mode: "full"
    teach:
      hint: "私人课内容"
      mode: "full"
    kachuai:
      hint: "卡揣内容"
      mode: "random"
      ratio: 50

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
| `[params.encryptGroups]` | `params.encryptGroups:` | 加密组配置 |
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

### 4.5 encrypt — 加密/涂黑系统

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

### 4.11 plugin — 插件架构（骨架）

**阶段一只定义接口，不实现功能。**

```go
// 插件接口
type Plugin interface {
    Name() string
    Init(config map[string]interface{}) error
}

// 内容处理器插件（阶段二用）
type ContentProcessor interface {
    Plugin
    Process(content *ContentContext) error
}

// 模板函数插件（阶段二用）
type TemplateFunctionProvider interface {
    Plugin
    Functions() template.FuncMap
}

// 输出处理器插件（阶段二用，如动态渲染、API 网关）
type OutputProcessor interface {
    Plugin
    Process(output *OutputContext) error
}

// 插件注册表
type Registry struct {
    plugins map[string]Plugin
}

func (r *Registry) Register(p Plugin)
func (r *Registry) LoadDir(dir string) error  // 从目录加载插件
```

**阶段二扩展方向：**
- `AuthPlugin` — JWT 鉴权，控制加密内容访问
- `PaymentPlugin` — 付费内容校验
- `MemberPlugin` — 会员等级管理
- `DynamicRenderPlugin` — HTTP server，动态渲染
- `TemplatePlugin` — 替换模板引擎

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
# 1. 用 Hugo 构建 baseline
cd ../zhurongshuo && hugo && cp -r docs/ /tmp/hugo-baseline/

# 2. 用 huan 构建
cd ../huan && ./huan build --source ../zhurongshuo

# 3. 递归 diff
diff -r /tmp/hugo-baseline/ ../zhurongshuo/docs/ \
  --exclude=".git" \
  -u
```

### 8.2 允许的差异

| 项目 | 原因 |
|------|------|
| `<meta name="generator" content="Hugo ...">` | huan 不输出此标签 |
| 构建时间戳 | 时间不同是正常的 |
| CSS/JS 内的空白差异 | minify 行为可能微小不同 |

### 8.3 验证检查清单

- [ ] 所有 URL 路径一致
- [ ] 所有页面 HTML 结构一致
- [ ] Open Graph / Twitter Card meta 一致
- [ ] 加密/涂黑输出一致
- [ ] RSS feed 内容一致
- [ ] sitemap.xml URL 列表一致
- [ ] search.json 内容一致
- [ ] 分页导航一致
- [ ] 标签页一致
- [ ] 静态资源完整复制

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
