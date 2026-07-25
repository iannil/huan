# 插件元数据系统 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为插件系统添加 MetadataProvider 可选接口，增强 PluginInfo 元数据，在前端 Admin UI 和 CLI 中展示

**Architecture:** 在 `internal/plugin/` 新增 `PluginMeta` 类型和 `MetadataProvider` 可选接口；`LifecycleManager.List()` 检测接口并填充 `PluginInfo` 额外字段；三个 compiled-in 插件实现接口；前端表格增加版本/作者/Tags/官方徽章列

**Tech Stack:** Go + React + TypeScript

## Global Constraints

- MetadataProvider 是可选接口（不强制所有 plugin 实现）
- PluginMeta 字段：Version, Author, RepoURL, License, Tags, IsOfficial
- LifecycleManager.List() 对每个插件检测 MetadataProvider，填充 PluginInfo 额外字段
- 三个 compiled-in 插件（seo_injector, sitemap_enhancer, html_injector）必须实现 MetadataProvider
- Admin UI 表格新增列：Version、Author、Tags、官方徽章
- 所有现有测试必须继续通过

---

### Task 1: 后端 MetadataProvider 接口 + PluginInfo 增强

**Files:**
- Modify: `internal/plugin/plugin.go` — 添加 PluginMeta 和 MetadataProvider
- Modify: `internal/plugin/lifecycle.go` — 扩展 PluginInfo，修改 List() 填充元数据
- Test: `internal/plugin/plugin_test.go` — 添加 MetadataProvider 测试

**Interfaces:**
- Produces: `plugin.PluginMeta`, `plugin.MetadataProvider` interface, `plugin.PluginInfo` 新增字段

- [ ] **Step 1: 在 `internal/plugin/plugin.go` 末尾添加 PluginMeta 和 MetadataProvider**

在 `plugin.go` 末尾添加：

```go
// PluginMeta carries human-readable metadata for a plugin.
type PluginMeta struct {
	Version    string   `json:"version"`
	Author     string   `json:"author"`
	RepoURL    string   `json:"repoURL"`
	License    string   `json:"license"`
	Tags       []string `json:"tags"`
	IsOfficial bool     `json:"isOfficial"`
}

// MetadataProvider is an optional interface plugins can implement to declare
// their metadata. Used by the LifecycleManager.List() and CLI/Admin UI.
type MetadataProvider interface {
	PluginMetadata() PluginMeta
}
```

- [ ] **Step 2: 扩展 `internal/plugin/lifecycle.go` 中的 PluginInfo**

修改 `PluginInfo` struct，添加新字段：

```go
type PluginInfo struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Source     string   `json:"source"`
	Capability string   `json:"capability,omitempty"`
	Status     string   `json:"status"`
	LoadedAt   string   `json:"loadedAt,omitempty"`
	Error      string   `json:"error,omitempty"`
	// 新增元数据字段
	Author     string   `json:"author,omitempty"`
	RepoURL    string   `json:"repoURL,omitempty"`
	License    string   `json:"license,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}
```

- [ ] **Step 3: 修改 `LifecycleManager.List()` 检测 MetadataProvider**

在 `List()` 方法的 `for name, lp := range m.loaded` 循环中，在 capability 检测之后添加 metadata 检测：

```go
// Detect metadata via optional MetadataProvider interface
if mp, ok := lp.plugin.(MetadataProvider); ok {
	meta := mp.PluginMetadata()
	info.Version = meta.Version
	info.Author = meta.Author
	info.RepoURL = meta.RepoURL
	info.License = meta.License
	info.Tags = meta.Tags
}
```

同时，在 `List()` 方法的 for 循环中，当 `info.Version` 仍为空时，尝试检测 MetadataProvider：

```go
// If version is empty (legacy path), try MetadataProvider
if info.Version == "" {
	if mp, ok := lp.plugin.(MetadataProvider); ok {
		meta := mp.PluginMetadata()
		info.Version = meta.Version
		info.Author = meta.Author
		info.RepoURL = meta.RepoURL
		info.License = meta.License
		info.Tags = meta.Tags
	}
}
```

注意：需要在 `List()` 方法中找到插入点。当前 `List()` 在 capability 检测后构建 `info`。在 `out = append(out, info)` 之前添加 metadata 填充。

- [ ] **Step 4: 添加测试**

在 `internal/plugin/plugin_test.go` 末尾添加：

```go
type testMetaPlugin struct {
	stubPlugin
	meta PluginMeta
}

func (p *testMetaPlugin) PluginMetadata() PluginMeta { return p.meta }

var _ MetadataProvider = (*testMetaPlugin)(nil)

func TestMetadataProvider_OptionalInterface(t *testing.T) {
	r := NewRegistry()
	// Plugin without MetadataProvider
	_ = r.Register(&stubPlugin{name: "plain"})
	// Plugin with MetadataProvider
	_ = r.Register(&testMetaPlugin{
		stubPlugin: stubPlugin{name: "metap"},
		meta: PluginMeta{
			Version:    "1.0.0",
			Author:     "test author",
			Tags:       []string{"tag1", "tag2"},
			IsOfficial: true,
		},
	})

	// Test Find[MetadataProvider]
	providers := Find[MetadataProvider](r)
	if len(providers) != 1 {
		t.Fatalf("Find[MetadataProvider] len = %d, want 1", len(providers))
	}
	meta := providers[0].PluginMetadata()
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", meta.Version)
	}
	if meta.Author != "test author" {
		t.Errorf("Author = %q", meta.Author)
	}
	if len(meta.Tags) != 2 || meta.Tags[0] != "tag1" {
		t.Errorf("Tags = %v", meta.Tags)
	}
	if !meta.IsOfficial {
		t.Error("IsOfficial should be true")
	}
}

func TestMetadataProvider_NotRequired(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&stubPlugin{name: "plain"})
	providers := Find[MetadataProvider](r)
	if len(providers) != 0 {
		t.Errorf("Find[MetadataProvider] len = %d, want 0", len(providers))
	}
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/plugin/ -v`
Expected: ALL PASS

- [ ] **Step 6: 提交**

```bash
git add internal/plugin/plugin.go internal/plugin/lifecycle.go internal/plugin/plugin_test.go
git commit -m "feat(plugin): add MetadataProvider interface and enhance PluginInfo

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

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

### Task 3: Admin UI 插件列表页面增强

**Files:**
- Modify: `web/admin/src/pages/Plugins.tsx`

- [ ] **Step 1: 更新 PluginInfo 接口，添加新字段**

```tsx
interface PluginInfo {
  name: string
  version: string
  source: string
  capability: string
  status: string
  loadedAt: string
  error: string
  author: string
  repoURL: string
  license: string
  tags: string[]
}
```

- [ ] **Step 2: 修改表格列**

当前表格使用 grid 布局：`grid-cols-[1fr_80px_100px_80px_1fr_120px]`

改为：`grid-cols-[1fr_80px_80px_100px_80px_1fr_120px]`

在"名称"和"来源"之间插入"版本"列。

表格 header 在 `"名称"` 后增加 `"版本"`。

表格 body 在名称 span 后增加版本 span：

```tsx
<span className="text-muted-foreground truncate">{p.version || '-'}</span>
```

- [ ] **Step 3: 添加官方徽章和 Tags**

在名称列中，如果 `p.tags` 非空且 `p.author` 为 "huan team"，在名称后显示 "官方" badge：

```tsx
<span className="text-foreground font-medium truncate">{p.name}</span>
{p.tags && p.tags.length > 0 && (
  <div className="flex items-center gap-1">
    {p.tags.slice(0, 3).map((tag) => (
      <Badge key={tag} variant="secondary" className="text-[9px] leading-none px-1 py-0">
        {tag}
      </Badge>
    ))}
  </div>
)}
```

- [ ] **Step 4: 构建验证**

```bash
cd web/admin && npm run build
```
Expected: Build succeeds

- [ ] **Step 5: 提交**

```bash
git add web/admin/src/pages/Plugins.tsx
git commit -m "feat(admin): enhance plugin list with version, tags, official badge

Co-Authored-By: Claude <noreply@anthropic.com>"
```