# Sitemap 增强器插件设计

- **日期**：2026-07-24
- **状态**：Draft
- **关联**：[ADR 0003](../adr/0003-unified-plugin-system.md)、[SEO 注入器](./2026-07-24-seo-injector-design.md)、`internal/build/hook.go`

## 背景

现有的 sitemap.xml 模板已支持 `<lastmod>`、`<changefreq>`、`<priority>`、`<xhtml:link rel="alternate" hreflang>` 标签，但 `<priority>` 和 `<changefreq>` 需要每个页面前端（frontmatter）手动配置。大多数站点要么完全缺失这两个字段，要么全部是默认值（0.5 / weekly）。

Sitemap 增强器作为第二个 `build.Hook` 实现，在 `OnOutputWritten` 阶段读取生成的 `sitemap.xml`，根据页面种类自动推算 `<priority>` 和 `<changefreq>`，然后写回。

## 设计

### 1. 包结构

```
internal/seo/sitemap/
├── plugin.go       — 插件入口，实现 build.Hook + SchemaProvider
├── enhance.go      — 核心增强逻辑（XML 解析 + 标签补全 + 写入）
└── plugin_test.go  — 测试
```

插件名称：`"sitemap_enhancer"`，注册在 `plugins:` 命名空间下。

### 2. 配置

```go
type Config struct {
    DefaultPriority   map[string]float64 `yaml:"defaultPriority"`   // key: page kind
    DefaultChangefreq map[string]string  `yaml:"defaultChangefreq"` // key: page kind
}
```

默认值：hardcoded 在 `DefaultConfig()` 中。

### 3. 核心逻辑

在 `OnOutputWritten(ctx, outputDir)` 中：

```
读取 outputDir/sitemap.xml
如果文件不存在或解析失败 → log warning，返回 nil

解析 XML → 遍历每个 <url>
  对每个 <url>:
    1. 从 <loc> 推断页面种类：guessKind(loc)
    2. 如果 <priority> 不存在 → 按种类设置默认 priority
    3. 如果 <changefreq> 不存在 → 按种类设置默认 changefreq
    4. 如果 <lastmod> 存在但为空 → 跳过（保留站点的 lastmod）
    5. 不覆盖已有标签（frontmatter 优先）

写回 sitemap.xml
```

**关键约束：**
- 不覆盖已有 `<priority>`/`<changefreq>`（frontmatter 优先）
- 只在 `OnOutputWritten` 阶段工作，其他 Hook 方法返回 nil
- 集合不中断语义：失败只 log warning，不中止构建

### 4. guessKind 策略

从 `sitemap.xml` 的 `<loc>` 值推断页面种类，复用 SEO 注入器的逻辑（但独立实现以避免跨包依赖）：

```
/index.html / / → home
/section-name/ → section (1 级路径)
/post-slug/ → page (2 级路径)
/tags/ → taxonomy
/tags/tag-name/ → term
/page/N/ → page (paginated)
/404.html → page
```

注意：此逻辑比 SEO 注入器的更复杂，因为 sitemap 中每个 `<loc>` 是完整 URL（如 `https://example.com/posts/my-post/`），需要先抽取 URL 路径部分，再按路径段数推断。

### 5. 默认优先级表

| 页面种类 | priority | changefreq |
|---------|----------|------------|
| home | 1.0 | daily |
| page | 0.8 | weekly |
| section | 0.6 | weekly |
| taxonomy | 0.5 | weekly |
| term | 0.4 | monthly |

### 6. SchemaProvider

实现 `plugin.SchemaProvider`，声明配置 schema：

```go
func (c *Config) ConfigSchema() plugin.Schema {
    return plugin.Schema{Fields: []plugin.FieldSchema{
        {Key: "defaultPriority", Type: "map", Required: false},
        {Key: "defaultChangefreq", Type: "map", Required: false},
    }}
}
```

### 7. 注册

在 `cmd/huan/plugins.go` 的 switch 中添加：

```go
case "sitemap_enhancer":
    raw := cfg.Plugins[name]
    pluginCfg, err := sitemap.ParseConfig(raw)
    if err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
    if err := r.Register(sitemap.New(pluginCfg)); err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
```

### 8. 测试策略

- 增强逻辑测试：输入含/不含 priority/changefreq 的 XML，验证输出
- 已有标签不覆盖测试
- guessKind 测试：验证不同 URL 路径的种类推断
- 配置解析测试

### 不在此范围

- `<lastmod>` 增强（已有模板处理）
- hreflang validation（已有模板处理）
- URL 排除过滤（模板 `where` 足够）