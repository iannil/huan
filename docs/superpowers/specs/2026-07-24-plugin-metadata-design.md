# 插件元数据系统设计

- **日期**：2026-07-24
- **状态**：Draft
- **关联**：[ADR 0003](../adr/0003-unified-plugin-system.md)、`internal/plugin/lifecycle.go`、`web/admin/src/pages/Plugins.tsx`

## 背景

当前插件元数据仅有 `PluginInfo` 的 `Name`、`Version`、`Source`、`Capability`、`Status`、`LoadedAt`、`Error` 七个字段。`Version` 字段虽存在但从未被填充（始终为空字符串），且前端列表完全没有渲染它。

随着已有 3 个 compiled-in 插件和未来更多插件的增加，用户需要一个更丰富的信息展示来了解每个插件是什么、谁开发的、版本号、是否有官方支持。这是插件市场功能的基础骨架。

## 设计

### 1. MetadataProvider 可选接口

在 `internal/plugin/` 中新增一个可选接口，类似 `SchemaProvider`：

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

### 2. 集成到 LifecycleManager

在 `LifecycleManager.List()` 中，对每个插件检测 `MetadataProvider` 接口，将返回的 `PluginMeta` 合并到 `PluginInfo` 中。

`PluginInfo` 新增字段：

```go
type PluginInfo struct {
    Name       string `json:"name"`
    Version    string `json:"version"`
    Source     string `json:"source"`
    Capability string `json:"capability,omitempty"`
    Status     string `json:"status"`
    LoadedAt   string `json:"loadedAt,omitempty"`
    Error      string `json:"error,omitempty"`
    // 新增
    Author     string   `json:"author,omitempty"`
    RepoURL    string   `json:"repoURL,omitempty"`
    License    string   `json:"license,omitempty"`
    Tags       []string `json:"tags,omitempty"`
}
```

### 3. 现有插件的元数据

```go
// seo_injector
func (p *SEOInjector) PluginMetadata() plugin.PluginMeta {
    return plugin.PluginMeta{
        Version:    "0.1.0",
        Author:     "huan team",
        Tags:       []string{"seo", "og", "twitter"},
        IsOfficial: true,
    }
}

// sitemap_enhancer
func (p *SitemapEnhancer) PluginMetadata() plugin.PluginMeta {
    return plugin.PluginMeta{
        Version:    "0.1.0",
        Author:     "huan team",
        Tags:       []string{"seo", "sitemap"},
        IsOfficial: true,
    }
}

// html_injector
func (p *HTMLInjector) PluginMetadata() plugin.PluginMeta {
    return plugin.PluginMeta{
        Version:    "0.1.0",
        Author:     "huan team",
        Tags:       []string{"html", "script", "css"},
        IsOfficial: true,
    }
}
```

### 4. Admin UI 增强

在现有插件列表页面中：
- 在名称列旁边显示 `IsOfficial` 徽章（"官方"）
- 增加"版本"列
- 增加"作者"列（目前应全是 "huan team"）
- Tags 显示为标签徽章

### 5. CLI 增强

在 `huan plugin list` 命令输出中增加 `Version`、`Author`、`Tags` 列。

### 6. 不在此范围

- 插件评分/下载量统计
- 在线插件仓库浏览
- 插件自动更新检查

## 数据流

```
插件实现 MetadataProvider
  → LifecycleManager.List() 检测 MetadataProvider
    → PluginInfo.Author/RepoURL/License/Tags 被填充
      → Admin API 返回完整 PluginInfo
        → Admin UI Plugins.tsx 渲染为表格列
```