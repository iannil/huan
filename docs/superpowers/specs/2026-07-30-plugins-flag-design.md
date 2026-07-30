# 设计文档：huan build/dev --plugins 标志

## 概述

为 `huan build` 和 `huan dev` 命令添加 `--plugins` 标志，允许用户指定自定义插件目录，替换默认的 `sourceDir/plugins/` 路径。

## 动机

在开发、测试或使用不同插件集时，用户需要一种灵活的方式切换插件目录，而不必修改项目目录结构或 huan.yaml 配置。

## 设计

### 行为

- `huan build --plugins=path/to/plugins` 将项目级插件目录替换为 `path/to/plugins`
- `huan dev --plugins=path/to/plugins` 同上，开发模式也支持
- 未指定时保持现有行为不变（默认 `sourceDir/plugins/`）
- `$HUAN_HOME/plugins/` 全局搜索路径的优先级仍然高于 `--plugins` 指定的目录（与现有规则一致）
- `huan plugin list/info` 等子命令不受影响，继续使用默认目录

### 变更点

#### 1. `cmd/huan/main.go`

- `buildCmd` 添加 `--plugins` 标志
- `runBuild` 函数获取该标志值，传递给 `newPluginRegistry`

#### 2. `cmd/huan/dev.go`

- `devCmd` 添加 `--plugins` 标志
- `runDev` 函数获取该标志值，传递给 `newPluginRegistry`

#### 3. `cmd/huan/plugins.go`

- `newPluginRegistry` 签名增加可选参数 `pluginDirOverride string`
- 非空时，替换 `pluginDirFromSource(sourceDir)` 的返回值

### 使用示例

```bash
# 默认行为（不变）
huan build

# 指定自定义插件目录
huan build --plugins=./my-plugins

# 开发模式
huan dev --plugins=./test-plugins
```