### Task 2: 三个 compiled-in 插件实现 MetadataProvider

**Files:**
- Modify: `internal/seo/injector/plugin.go`
- Modify: `internal/seo/sitemap/plugin.go`
- Modify: `internal/seo/htmlinjector/plugin.go`

**Interfaces:**
- Consumes: `plugin.MetadataProvider`, `plugin.PluginMeta`
- Produces: 三个插件实现 `PluginMetadata()` 方法

- [ ] **Step 1: SEO 注入器实现 MetadataProvider**

在 `internal/seo/injector/plugin.go` 中，在 `Name()` 方法后添加：

```go
func (p *SEOInjector) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"seo", "og", "twitter"},
		IsOfficial: true,
	}
}
```

- [ ] **Step 2: Sitemap 增强器实现 MetadataProvider**

在 `internal/seo/sitemap/plugin.go` 中，在 `Name()` 方法后添加：

```go
func (p *SitemapEnhancer) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"seo", "sitemap"},
		IsOfficial: true,
	}
}
```

- [ ] **Step 3: HTML 注入器实现 MetadataProvider**

在 `internal/seo/htmlinjector/plugin.go` 中，在 `Name()` 方法后添加：

```go
func (p *HTMLInjector) PluginMetadata() plugin.PluginMeta {
	return plugin.PluginMeta{
		Version:    "0.1.0",
		Author:     "huan team",
		Tags:       []string{"html", "script", "css"},
		IsOfficial: true,
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/seo/... ./internal/plugin/ -v`
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/seo/injector/plugin.go internal/seo/sitemap/plugin.go internal/seo/htmlinjector/plugin.go
git commit -m "feat(seo): implement MetadataProvider for all compiled-in plugins

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

