# ADR 0012: 主题插件系统

- **状态**: Accepted
- **日期**: 2026-07-27
- **决策者**: 用户 + Claude
- **被引用**: [ADR 0003](0003-unified-plugin-system.md)（统一插件系统）、[主题插件系统设计](../../docs/superpowers/specs/2026-07-27-theme-plugin-system-design.md)

## 背景

huan 已完成插件化架构（ADR 0003），现有三种能力类型：Deployer、Translator、ImageProcessor。下一步引入第四种能力：主题插件。

## 决策

### 1. 形态：ThemePlugin 能力接口

- `internal/theme/` 包定义 `ThemePlugin` 接口，嵌入 `plugin.Plugin`，添加主题专属方法
- 主题作为 .so 插件加载，与现有 plugin 系统一致
- 第一个能力接口（Deployer/Translator/ImageProcessor 之外）新增为第四种能力

### 2. 模板存储：嵌入 .so

- 使用 `//go:embed` 将模板文件和静态资源编译进 .so
- 部署只需一个 .so 文件，无外部目录依赖

### 3. 激活模式：全局唯一

- 任何时候最多一个激活主题
- 切换主题自动清除模板缓存
- 停用主题回退到 `layouts/` 默认模板

### 4. 构建 Hook：可选接口

- `ThemeHooks` 为可选接口，非强制实现
- `BeforeRender` 失败中止构建（fail-fast）
- `AfterRender` 失败记录日志（collection-not-interruption）

### 5. FuncMap 优先级

- 主题自定义模板函数覆盖 huan 内置函数
- 同名时记录警告日志

### 6. 配置

```yaml
theme: "zhurongshuo"    # 顶层 key，激活主题
plugins:
  zhurongshuo: {}       # 主题插件配置（预留）
```

## 架构

```
internal/theme/
├── types.go       # ThemePlugin 接口 + ThemeInfo + TemplateEntry + ThemeHooks
├── types_test.go
├── manager.go     # Manager（激活/停用/列表/状态）
└── manager_test.go

plugins/zhurongshuo/    # 首个官方主题插件
├── plugin_main.go      # InitPlugin 入口
├── plugin.go           # ThemePlugin 实现
├── plugin/plugin.go    # 自包含类型副本
├── funcs.go            # 自定义模板函数
├── templates/          # 嵌入模板
├── assets/             # 嵌入资源
└── go.mod              # 独立模块，零外部依赖
```

## 影响

### 新增

- `internal/theme/` 包（接口 + 管理器）
- `cmd/huan/theme_cmd.go`（CLI 子命令）
- `plugins/zhurongshuo/`（主题插件）
- 主题管理 Admin API 端点

### 改造

- `internal/template/loader.go` — 优先加载主题模板，合并 FuncMap
- `internal/build/build.go` — 插入 BeforeRender/AfterRender Hook
- `internal/config/config.go` — 新增 `Theme` 字段
- `internal/admin/handler.go` + `api.go` — 主题管理 API
- `cmd/huan/daemon.go` + `internal/daemon/` — 集成主题管理器

### 未来扩展路径

- 插件市场安装：`huan theme install <name>`
- 主题预览：`huan theme preview <name> --port=1313`
- 主题配置界面：Admin UI 主题设置页