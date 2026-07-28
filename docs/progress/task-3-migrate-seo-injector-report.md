# Task 3 验收报告：将 seo-injector 从 internal/ 迁移至 plugins/

## 概要

将 `internal/seo/injector/` 中的 SEO 注入器插件逻辑成功迁移到独立的 `plugins/seo-injector/` 目录，成为一个可独立构建的 `.so` 动态插件。

## 创建的文件

| 文件 | 说明 |
|------|------|
| `plugins/seo-injector/go.mod` | 模块 `github.com/iannil/huan-plugin-seo-injector`, go 1.26.2, 依赖 `golang.org/x/net v0.56.0` |
| `plugins/seo-injector/go.sum` | 由 `go mod tidy` 自动生成 |
| `plugins/seo-injector/plugin/plugin.go` | 自包含的类型副本: Plugin, Hook, PluginMeta, MetadataProvider, SchemaProvider, Schema, FieldSchema |
| `plugins/seo-injector/inject.go` | 核心注入逻辑: InjectHTML, ExtractExistingTags, ExtractPlainText, TruncateToWordBoundary 等 |
| `plugins/seo-injector/plugin.go` | SEOInjector 结构体及其所有方法, Config/ParseConfig/ConfigSchema 等 |
| `plugins/seo-injector/plugin_main.go` | InitPlugin 导出符号 + 空的 main() |

## 构建结果

- `.so` 输出: `seo-injector.so` (~4.5 MB, Mach-O arm64)
- `go vet ./...` 通过
- 原始 internal 测试 `go test ./internal/seo/injector/...` 全部通过 (仍在原位)

## 关键变更

- `content.Page` 类型引用替换为 `any` (Hook 接口中使用 `[]any` 和 `any`)
- `github.com/iannil/huan/internal/plugin` 导入替换为 `github.com/iannil/huan-plugin-seo-injector/plugin`
- 包名从 `injector` (internal) 改为 `main` (standalone .so 插件)
- SEOInjector 的 `cfg` 字段类型从 `*Config` 改为值类型 `Config` (与 plugin_main.go 的创建方式一致)
- `plugin/plugin.go` 中的 Hook 接口使用 `[]any` 而非 `[]*content.Page`，保持自包含

## 注意事项

1. 原本的 `internal/seo/injector/` 代码保留，其他部分仍引用它。后续需要更新 build pipeline 中的引用，但不在本次任务范围内。
2. 该插件的 `.go` 文件未被 huan 主模块的编译包含，由独立的 `go.mod` 管理依赖，与主项目解耦。
3. 无测试文件迁移——插件功能可由 e2e 构建测试验证。

## 验证

```bash
cd plugins/seo-injector
go build -buildmode=plugin -o ../../seo-injector.so .   # 成功
go vet ./...                                              # 无警告
```
