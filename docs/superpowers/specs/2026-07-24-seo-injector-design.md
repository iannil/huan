# SEO 注入器插件设计

- **日期**：2026-07-24
- **状态**：Draft
- **关联**：[ADR 0003](../adr/0003-unified-plugin-system.md)、[build.Hook 接口](../../internal/build/hook.go)

## 背景

`build.Hook` 接口定义了三个 hook 点（`OnContentLoaded`、`OnPageRendered`、`OnOutputWritten`），但自接口创建以来零个实现。需要一个真实的插件来验证整个 Hook 架构通路。

SEO 注入器作为第一个 Hook 实现，在 `OnOutputWritten` 阶段扫描输出目录中的所有 HTML 文件，自动补全缺失的 SEO meta 标签。

## 设计

### 1. 包结构

```
internal/seo/injector/
├── plugin.go       — 插件入口，实现 build.Hook + SchemaProvider
├── inject.go       — 核心注入逻辑（HTML 解析 + 标签生成 + 写入）
└── plugin_test.go  — 测试
```

插件名称：`"seo_injector"`，注册在 `plugins:` 命名空间下。

### 2. 配置

```go
type Config struct {
    DescriptionMaxLength int    `yaml:"descriptionMaxLength"` // 默认 160
    DefaultOGImage       string `yaml:"defaultOGImage"`       // 默认 ""
    InjectOG             bool   `yaml:"injectOG"`             // 默认 true
    InjectTwitter        bool   `yaml:"injectTwitter"`        // 默认 true
}
```

### 3. 核心逻辑

在 `OnOutputWritten(ctx, outputDir)` 中：

```
扫描 outputDir 下所有 *.html 文件（递归）
  对每个文件：
    1. 读取 HTML
    2. 解析 <head> 区域
    3. 检查已有标签：
       - description → 有则跳过，无则从 page.Plain 前 N 字提取
       - og:title → 有则跳过，无则取 <title>
       - og:description → 有则跳过，无则同 description 逻辑
       - og:url → 有则跳过，无则用文件路径 + baseURL 拼接
       - og:type → 有则跳过，无则 article/website 判定
       - og:image → 有则跳过，无则用 defaultOGImage
       - twitter:card → 有则跳过，无则 summary_large_image
       - twitter:title / twitter:description → 同 OG
    4. 注入缺失标签到 </head> 之前
    5. 写回文件
```

**关键约束：**
- 不覆盖已有标签（frontmatter 优先）
- 只在 `OnOutputWritten` 阶段工作，其他 Hook 方法返回 nil
- 集合不中断语义：失败只 log warning，不中止构建

### 4. 标签注入顺序

注入的标签按以下顺序插入到 `</head>` 之前：

```
<!-- 由 huan seo-injector 注入 -->
<meta name="description" content="...">
<meta property="og:title" content="...">
<meta property="og:description" content="...">
<meta property="og:url" content="...">
<meta property="og:type" content="article">
<meta property="og:image" content="...">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="...">
<meta name="twitter:description" content="...">
```

### 5. Description 提取逻辑

当页面缺少 `description` 时，从 `page.Plain`（纯文本，无 HTML 标签）提取：

1. 取 `page.Plain` 的前 `descriptionMaxLength` 个字符
2. 在最后一个完整单词处截断（避免中间截断单词）
3. 如果 `page.Plain` 为空，跳过

注意：此逻辑在 `OnContentLoaded` 或 `OnPageRendered` 阶段更好（因为此时 `page.Plain` 已可用），但为了最小化对管线的影响，选择在 `OnOutputWritten` 阶段对 HTML 做后处理。这意味着 description 的来源是渲染后的 HTML 文本，而非 `page.Plain`。

修正：在 `OnOutputWritten` 阶段扫描 HTML 时，提取 `<body>` 中的纯文本内容作为 description 来源。

### 6. SchemaProvider

实现 `plugin.SchemaProvider`，声明配置 schema：

```go
func (p *SEOInjector) ConfigSchema() plugin.Schema {
    return plugin.Schema{Fields: []plugin.FieldSchema{
        {Key: "descriptionMaxLength", Type: "int", Required: false, Default: 160, Description: "meta description 最大长度"},
        {Key: "defaultOGImage", Type: "string", Required: false, Description: "默认 OG image 路径"},
        {Key: "injectOG", Type: "bool", Required: false, Default: true, Description: "是否注入 OG 标签"},
        {Key: "injectTwitter", Type: "bool", Required: false, Default: true, Description: "是否注入 Twitter Card 标签"},
    }}
}
```

### 7. 注册

在 `cmd/huan/plugins.go` 的 switch 中添加：

```go
case "seo_injector":
    cfg, err := injector.ParseConfig(raw)
    if err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
    if err := r.Register(injector.New(cfg)); err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
```

### 8. 测试策略

- `inject.go` 的注入逻辑：单元测试，mock HTML 输入，验证输出
- 已有标签不覆盖测试
- 缺失标签注入测试
- Description 提取测试（含截断逻辑）
- 配置解析测试

### 不在此范围

- `injectStructuredData`（JSON-LD）— 留待后续
- 多语言 hreflang 增强 — 留待后续
- Sitemap 增强 — 下一个插件
- 自定义输出转换器 — 下下个插件