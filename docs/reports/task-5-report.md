# Task 5: sitemap-enhancer 插件迁移完成报告

> 完成日期：2026-07-27
> 计划与设计原文：Task 5 描述（SDD）

## 1. 概述

将 `internal/seo/sitemap/` 中的 SitemapEnhancer 插件迁移为独立的 `.so` 动态插件，位于 `plugins/sitemap-enhancer/`。迁移后可通过 `plugins.sitemap_enhancer` 配置项在 `huan.yaml` 中启用，实现静态插件到动态插件的解耦。

## 2. 新增依赖

无外部依赖。仅使用 Go 标准库（`encoding/xml`、`os`、`path/filepath`、`strings`）。

## 3. 新增 / 修改的包

| 路径 | 职责 | 关键文件 |
|---|---|---|
| `plugins/sitemap-enhancer/` | sitemap 增强插件（.so 构建） | `go.mod`, `plugin.go`, `plugin_main.go` |
| `plugins/sitemap-enhancer/plugin/` | 自包含的接口类型副本 | `plugin.go` |
| `sitemap-enhancer.so` | 编译产物 | 二进制文件 |

## 4. CLI / API 变更

无变更。插件的配置仍然通过 `huan.yaml` 的 `plugins.sitemap_enhancer.*` 传入，由插件加载器统一管理。

## 5. 关键设计决策

1. **与之前迁移模式保持一致** —— 采用与 robots-enhancer、html-injector、seo-injector 完全相同的三层结构：`plugin/plugin.go`（自包含接口）、`plugin.go`（主逻辑）、`plugin_main.go`（入口）。降低维护成本。

2. **Sitemap 增强逻辑内联到插件内部** —— `enhance.go` 中的 `EnhanceSitemap`、`GuessKindFromURL` 等函数直接复制到 `plugin.go` 末尾，作为包内私有函数。原因是这些函数无需对外暴露，且动态插件不能引用 `internal/` 包。

3. **`cfg` 字段改为值类型而非指针** —— 与 seo-injector 保持一致，`SitemapEnhancer.cfg` 使用 `Config` 而非 `*Config`，避免指针共享带来的意外修改风险。

## 6. 验收记录

```
$ cd plugins/sitemap-enhancer && go build -buildmode=plugin -o ../../sitemap-enhancer.so .
（无输出，编译成功）

$ ls -la ../../sitemap-enhancer.so
-rw-r--r--@ 1 rong.zhu  staff  4510066  7月 27 21:36 ../../sitemap-enhancer.so
```

## 7. 已知限制

- 仍保留 `internal/seo/sitemap/` 中的原始实现，尚未删除。后续需确认 `html/test-plugins/*` 等测试是否依赖原始 internal 包路径后，再删除旧代码。
- 动态插件暂不支持 `SetLogf` 外部注入；生命周期管理器的日志注入逻辑需单独验证。

## 8. 后续优化项

- 在确认无外部依赖后，删除 `internal/seo/sitemap/plugin.go`（保留 `enhance.go` 若其他包仍引用）。
- 将 sitemap-enhancer 加入 `scripts/diff-build.sh` 的基准测试配置中。
