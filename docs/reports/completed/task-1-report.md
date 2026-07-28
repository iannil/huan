# Task 1: 创建 `pkg/plugin/` 共享类型包

> 完成日期：2026-07-27
> 关联 ADR：ADR 0003（统一插件系统）

## 1. 概述

创建 `pkg/plugin/` 共享类型包，提供 huan 内部代码和 .so 插件均可导入的规范类型定义。解决 Go 跨模块类型断言问题——.so 插件使用自包含的类型副本，通过 `pkg/plugin/` 统一后，内部代码和插件都导入同一包，类型断言可直接工作。

## 2. 变更清单

| 文件 | 说明 |
|---|---|
| `pkg/plugin/plugin.go` | 核心类型：Plugin 接口、Registry、Finder、PluginMeta、MetadataProvider、SchemaProvider、Schema、FieldSchema |
| `pkg/plugin/theme.go` | 主题插件类型：ThemePlugin 接口、ThemeInfo、TemplateEntry、ThemeHooks、ShortcodeHandler、ShortcodeContext、ShortcodeProvider |
| `pkg/plugin/hook.go` | 构建钩子类型：Hook 接口（OnContentLoaded / OnPageRendered / OnOutputWritten），使用 `interface{}` 避免导入 content 包 |

## 3. 类型设计说明

### 3.1 pkg/plugin/plugin.go

- `Plugin` 接口：仅含 `Name() string`，与 `internal/plugin/` 中定义一致
- `Registry` 结构体：`NewRegistry` / `Register` / `Get` / `All` / `Unregister` / `Names` / `SortedNames` 完整方法集
- `Find[T any](r *Registry) []T` 泛型函数
- `PluginMeta` 结构体：Version, Author, RepoURL, License, Tags, IsOfficial（均含 json tag）
- `MetadataProvider` / `SchemaProvider` 接口
- `Schema` / `FieldSchema` 结构体
- **未包含** `EventSubscriber` 接口（其引用了 `internal/daemon/eventbus`，不允许从 `pkg/plugin/` 导入）

### 3.2 pkg/plugin/theme.go

- `ThemePlugin` 接口：嵌入 Plugin + Info / Templates / FuncMap / Assets 方法
- `ThemeInfo`、`TemplateEntry`、`ThemeHooks` 结构体/接口
- `ShortcodeHandler` 类型、`ShortcodeContext` 结构体、`ShortcodeProvider` 接口
- 导入 `context`、`html/template`、`io/fs`
- 与 `plugins/zhurongshuo/plugin/plugin.go` 中的自包含副本一致

### 3.3 pkg/plugin/hook.go

- `Hook` 接口：嵌入 Plugin + OnContentLoaded / OnPageRendered / OnOutputWritten
- 使用 `[]interface{}` 和 `interface{}` 替代 `[]*content.Page` 和 `*content.Page`，避免导入 internal/content 包
- 与 `plugins/html-injector/plugin/plugin.go`、`plugins/seo-injector/plugin/plugin.go`、`plugins/sitemap-enhancer/plugin/plugin.go` 中的 Hook 定义一致

## 4. 验收记录

```
$ go build ./pkg/plugin/
（无输出，编译成功）

$ go vet ./pkg/plugin/
（无输出，无问题）
```

## 5. 已知问题

无。`pkg/plugin/` 包独立编译通过，无外部依赖（仅依赖标准库）。

## 6. 后续行动

- 后续任务将更新 `internal/plugin/` 和 `internal/theme/` 中的类型定义改为从 `pkg/plugin/` 引用
- 更新 .so 插件（plugins/*）中的自包含副本改为导入 `pkg/plugin/`
- 更新 `internal/build/hook.go` 中的 Hook 接口改为使用 `[]interface{}` 或从 `pkg/plugin/` 引用
