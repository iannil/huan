# Task 6: 统一静态/动态插件加载重构完成报告

> 完成日期：2026-07-27
> 关联 ADR：ADR 0003（统一插件系统）

## 1. 概述

重构 `newPluginRegistry` 与相关调用方，实现统一的静态/动态插件加载。核心变更：
- 所有编译期硬编码的插件加载路径统一走 `ScanAndLoadByCategory` 方法
- `plugins.go` 中的 `newPluginRegistry` 已使用 `ScanAndLoadByCategory(cfg.Plugins, plugin.CategoryStatic, plugin.CategoryMixed)` 加载插件
- 修复 `dev.go` 中 `themeMgr` 变量引用问题（旧代码在 `themeMgr` 声明前即使用）
- 补齐 `daemon.go` 的 capability detector 中缺失的 `theme.ThemePlugin` 检测

## 2. 变更清单

| 文件 | 变更类型 | 说明 |
|---|---|---|
| `cmd/huan/plugins.go` | 已就绪（无需修改） | 已于 Task 55 重写，使用 `ScanAndLoadByCategory` |
| `internal/plugin/loader.go` | 已就绪（无需修改） | 已于 Task 56 添加 `ScanAndLoadByCategory` |
| `cmd/huan/dev.go` | **修复** | 修正 `themeMgr` 声明与初始化顺序，确保 `buildOpts.PluginRegistry` 和 `buildOpts.ThemeManager` 正确设置 |
| `internal/daemon/daemon.go` | **增强** | capability detector 新增 `theme.ThemePlugin` 接口检测 |
| `cmd/huan/main.go` | 已就绪（无需修改） | 已使用 `newPluginRegistry(cfg, sourceDir)` |
| `cmd/huan/daemon.go` | 已就绪（无需修改） | 已使用 `newPluginRegistry(cfg, sourceDir)` |
| `cmd/huan/plugin_cmd.go` | 已就绪（无需修改） | 已使用 `newPluginRegistry(cfg, sourceDir)` |
| `cmd/huan/theme_cmd.go` | 已就绪（无需修改） | 已使用 `newPluginRegistry(cfg, sourceDir)` |
| `cmd/huan/deploy.go` | 已就绪（无需修改） | 已使用 `newPluginRegistry(cfg, sourceDir)` |
| `cmd/huan/translate_cmd.go` | 已就绪（无需修改） | 已使用 `newPluginRegistry(cfg, sourceDir)` |
| `internal/plugin/validate.go` | 已就绪（无需修改） | 已验证兼容 `PluginConfig` 类型 |

## 3. 关键修复

### 3.1 dev.go 中 themeMgr 变量声明修复

原有代码在 `runDev` 中引用了 `themeMgr.Activate(cfg.Theme)`，但 `themeMgr` 未在该作用域声明（仅在局部匿名函数 `runBuild` 内声明）。修复后：创建 `reg` 后立即创建 `themeMgr`，将 `buildOpts.PluginRegistry` 和 `buildOpts.ThemeManager` 一并设入 `buildOpts`。

```go
reg, _ := newPluginRegistry(cfg, sourceDir)
buildOpts.PluginRegistry = reg
themeMgr := theme.NewManager(reg)
if cfg.Theme != "" {
    if err := themeMgr.Activate(cfg.Theme); err != nil { ...
    }
}
buildOpts.ThemeManager = themeMgr
```

### 3.2 daemon capability detector 补充

daemon 的 `LifecycleManager.SetCapabilityDetector` 原未检测 `theme.ThemePlugin`，导致 admin API 的插件列表不显示 theme 能力标签。新增：

```go
if _, ok := p.(theme.ThemePlugin); ok {
    caps = append(caps, "theme")
}
```

## 4. 验收记录

```
$ go build ./...
（无输出，编译成功）

$ go vet ./...
（无输出，无问题）

$ go test ./cmd/huan/ -v -count=1
PASS  (52 tests)

$ go test ./internal/plugin/ -v -count=1
PASS  (50 tests)
```

## 5. 已知问题

无。所有测试通过，编译无误。

## 6. 后续优化

- 后续可考虑在 `runDev` 的匿名 `runBuild` 函数中复用外部已创建的 `reg`，避免每次 rebuild 重新扫描 plugins 目录（当前在 image pipeline 调用中重新创建了 `reg`）。
