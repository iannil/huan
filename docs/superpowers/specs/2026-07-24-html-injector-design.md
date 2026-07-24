# HTML 注入器插件设计

- **日期**：2026-07-24
- **状态**：Draft
- **关联**：[ADR 0003](../adr/0003-unified-plugin-system.md)、`internal/build/hook.go`

## 背景

当前 `build.Hook` 的三个 Hook 点中，SEO 注入器和 Sitemap 增强器都用了 `OnOutputWritten`，`OnPageRendered` 还是零使用。需要一个真正的 `OnPageRendered` 插件来验证这条管线通路。

HTML 注入器在 `OnPageRendered` 阶段对每个已渲染的页面注入自定义 HTML 片段（脚本、样式等），用户通过 `huan.yaml` 配置无需修改主题模板。

## 设计

### 1. 包结构

```
internal/seo/htmlinjector/
├── plugin.go       — 插件入口，实现 build.Hook + SchemaProvider
├── inject.go       — 核心注入逻辑
└── plugin_test.go  — 测试
```

插件名称：`"html_injector"`，注册在 `plugins:` 命名空间下。

### 2. 配置

```go
type Config struct {
    Head     []string          `yaml:"head"`     // 注入到 </head> 之前
    BodyEnd  []string          `yaml:"bodyEnd"`  // 注入到 </body> 之前
    IncludeKinds []string      `yaml:"includeKinds"` // 空=全部
    ExcludeKinds []string      `yaml:"excludeKinds"` // 空=不排除
}
```

### 3. 核心逻辑

在 `OnPageRendered(ctx, page *content.Page)` 中：

```
如果 page.Kind 在 ExcludeKinds 中 → 跳过
如果 page.IncludeKinds 非空且 page.Kind 不在其中 → 跳过
如果 Head 非空 → 在 page.Content 的 </head> 前插入
如果 BodyEnd 非空 → 在 page.Content 的 </body> 前插入
```

**关键约束：**
- 只在 `OnPageRendered` 阶段工作，其他 Hook 方法返回 nil
- 集合不中断语义：失败只 log warning，不中止构建
- `IncludeKinds` 和 `ExcludeKinds` 互斥使用（`IncludeKinds` 优先）

### 4. SchemaProvider

```go
{Key: "head", Type: "string_slice", Required: false},
{Key: "bodyEnd", Type: "string_slice", Required: false},
{Key: "includeKinds", Type: "string_slice", Required: false},
{Key: "excludeKinds", Type: "string_slice", Required: false},
```

### 5. 注册

在 `cmd/huan/plugins.go` 的 switch 中添加 `case "html_injector"`.

### 6. 测试策略

- 注入逻辑测试：Head/BodyEnd 注入，不存在目标标签时跳过
- 条件注入测试：IncludeKinds/ExcludeKinds 过滤
- 配置解析测试

### 不在此范围

- 条件表达式（只支持按 page.Kind 过滤，不支持 URL 模式、正则等）
- 动态值替换（只支持静态字符串）
- 模板变量插值