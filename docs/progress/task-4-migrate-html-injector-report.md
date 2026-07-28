# Task 4 验收报告：将 html-injector 从 internal/ 迁移至 plugins/

## 概要

将 `internal/seo/htmlinjector/` 中的 HTML 注入器插件逻辑成功迁移到独立的 `plugins/html-injector/` 目录，成为一个可独立构建的 `.so` 动态插件。与 Task 3（seo-injector 迁移）采用相同模式。

## 创建的文件

| 文件 | 说明 |
|------|------|
| `plugins/html-injector/go.mod` | 模块 `github.com/iannil/huan-plugin-html-injector`, go 1.26.2, 无外部依赖 |
| `plugins/html-injector/plugin/plugin.go` | 自包含的类型副本: Plugin, Hook, PluginMeta, MetadataProvider, SchemaProvider, Schema, FieldSchema |
| `plugins/html-injector/plugin.go` | HTMLInjector 结构体及其所有方法, Config/ParseConfig/ConfigSchema 等 |
| `plugins/html-injector/plugin_main.go` | InitPlugin 导出符号 + 空的 main() |

## 构建结果

- `.so` 输出: `html-injector.so` (~4.1 MB, Mach-O arm64)
- `go build -buildmode=plugin` 通过
- `go build`（非 plugin 模式）通过

## 关键变更

1. **包结构迁移**: 与 seo-injector 不同，原 htmlinjector 的核心逻辑 `InjectHTML` 和 `contains` 未拆分为独立 `inject.go` 文件，而是直接合并到 `plugin.go` 中作为包内私有函数（`injectHTML`），简化了文件数量。这是与原 seo-injector 迁移的一个小差异——原 seo-injector 有独立的 `inject.go`，而 htmlinjector 的文件少且小，合并后更简洁。

2. **`content.Page` 类型引用替换为 `any`**: 原 `OnContentLoaded` 和 `OnPageRendered` 使用了 `[]*content.Page` 和 `*content.Page`，迁移后使用 `[]any` 和 `any`（与自包含的 Hook 接口一致）。

3. **`github.com/iannil/huan/internal/plugin` 导入替换**: 改为 `github.com/iannil/huan-plugin-html-injector/plugin`。

4. **包名从 `htmlinjector` (internal) 改为 `main`** (standalone .so 插件)。

5. **`htmlinjector.HTMLInjector` 的 `cfg` 字段类型从 `*Config` 改为值类型 `Config`**: 与 plugin_main.go 的创建方式一致，并且与 seo-injector 迁移模式对齐。

6. **与原逻辑的差异**: 
   - 原 `internal/seo/htmlinjector/plugin.go` 中 `OnPageRendered` 实际上执行了注入（调用 `InjectHTML` 并修改 `page.Content`），而迁移版本中 `OnPageRendered` 改为 no-op，将注入逻辑移到 `OnOutputWritten`（基于文件扫描模式）。这是与 seo-injector 迁移保持一致的架构变更——`.so` 插件在构建后扫描输出目录并修改文件，而非在渲染阶段修改页面对象。
   - 新增 `guessKind` 方法用于从文件路径推断 page kind（与 seo-injector 一致）。

7. **无外部依赖**: 与 seo-injector 不同（依赖 `golang.org/x/net`），htmlinjector 仅有标准库依赖，go.mod 更简洁。

## 注意事项

1. 原本的 `internal/seo/htmlinjector/` 代码保留，其他部分仍引用它。后续需要更新 build pipeline 中的引用，但不在本次任务范围内。
2. 该插件的 `.go` 文件未被 huan 主模块的编译包含，由独立的 `go.mod` 管理依赖，与主项目解耦。
3. 无测试文件迁移——插件功能可由 e2e 构建测试验证。

## 验证

```bash
cd plugins/html-injector
go build -buildmode=plugin -o ../../html-injector.so .   # 成功
go build -o /dev/null .                                    # 非 plugin 模式也成功
```
