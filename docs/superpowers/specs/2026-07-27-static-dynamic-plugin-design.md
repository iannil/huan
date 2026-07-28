# 静态/动态插件分类设计

- **日期**: 2026-07-27
- **状态**: Draft
- **设计者**: 用户 + Claude

## 一、背景

当前 huan 的插件系统有两类加载方式：
1. **compiled-in**: `internal/seo/` 下的 3 个 SEO 插件，hardcoded 在 `cmd/huan/plugins.go` 的 switch 中
2. **.so 插件**: `plugins/` 目录下的独立 module，仅在 daemon 时通过 LifecycleManager 加载

问题：
- 3 个 SEO 插件与 huan 主二进制耦合，无法独立更新
- zhurongshuo 主题插件是 .so，但 `huan build` 无法加载 .so 插件，导致静态构建时主题不生效
- 没有明确的分类机制区分「构建时需要」和「运行时需要」的插件

## 二、插件分类

每个插件在 `huan.yaml` 中声明 `category` 字段：

```yaml
plugins:
  seo_injector:
    category: static
    descriptionMaxLength: 160
  zhurongshuo:
    category: static
  cloudflare:
    category: dynamic
    accountId: ${CLOUDFLARE_ACCOUNT_ID}
  qwen3_translate:
    category: dynamic
    model: qwen3-next:80b
  image_pipeline:
    category: dynamic
```

| category | build 时 | daemon 时 | 用途 |
|----------|----------|-----------|------|
| `static` | ✅ 加载 | ❌ 不加载 | 主题插件、SEO 注入、构建 Hook |
| `dynamic` | ❌ 不加载 | ✅ 加载 | 部署插件、翻译插件、API 服务 |
| `mixed` | ✅ 加载 | ✅ 加载 | 既有构建 Hook 又有运行时能力的插件 |
| 未指定 | ❌ 不加载 | ✅ 加载 | 向后兼容（默认 dynamic） |

## 三、架构变更

### 3.1 3 个 SEO 插件迁移到 `plugins/` 目录

```
internal/seo/injector/       →  plugins/seo-injector/
internal/seo/htmlinjector/   →  plugins/html-injector/
internal/seo/sitemap/        →  plugins/sitemap-enhancer/
```

每个插件：
- 独立 go.mod（零外部依赖，或仅依赖 `golang.org/x/net`）
- 自包含类型副本（`plugin/plugin.go`），参考 cloudflare/zhurongshuo 模式
- `InitPlugin(cfg map[string]any) (plugin.Plugin, error)` 导出函数
- 迁移后删除 `internal/seo/` 目录

### 3.2 config 扩展

`internal/config/config.go` 修改：

```go
type Config struct {
    // ...
    Plugins map[string]PluginConfig `yaml:"plugins"`
}

type PluginConfig struct {
    Category string         `yaml:"category"`   // "static" | "dynamic" | "mixed"
    Config   map[string]any `yaml:",inline"`     // 插件特有配置
}
```

### 3.3 Loader 扩展

`internal/plugin/loader.go` 新增：

```go
// Category 定义插件分类常量。
const (
    CategoryStatic  = "static"
    CategoryDynamic = "dynamic"
    CategoryMixed   = "mixed"
)

// ScanAndLoadByCategory 扫描 pluginDir 中所有 .so 文件并加载，
// 仅返回符合 category 条件的插件（根据 yaml 配置判断）。
func (l *Loader) ScanAndLoadByCategory(category string, pluginConfigs map[string]PluginConfig) ([]ScanAndLoadResult, error)
```

### 3.4 构建管线加载静态插件

`cmd/huan/plugins.go` 重构：

```go
func newPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
    r := plugin.NewRegistry()
    loader := plugin.NewLoader(filepath.Join(sourceDir, "plugins"))
    
    // 加载 category: static 和 category: mixed 的插件
    results, err := loader.ScanAndLoadByCategory(cfg, "static")
    if err != nil {
        return nil, err
    }
    for _, result := range results {
        r.Register(result.Plugin)
    }
    
    // 验证配置
    // ...
    return r, nil
}
```

### 3.5 daemon 加载动态插件

`internal/daemon/daemon.go` 中 LifecycleManager 启动时：

```go
// 创建 LifecycleManager 时，传入 pluginConfigs 用于 category 判断
// LifecycleManager.Start 只加载 category: dynamic 和 category: mixed 的插件
```

## 四、迁移步骤

### Step 1: 迁移 SEO 插件到 plugins/
- 将 `internal/seo/injector/` 复制到 `plugins/seo-injector/`
- 创建独立 go.mod，自包含类型副本
- 构建验证

### Step 2: 迁移 html-injector 和 sitemap-enhancer
- 同上

### Step 3: 修改 config 支持 category
- PluginConfig 类型
- 兼容旧配置（无 category 时默认 dynamic）

### Step 4: 扩展 Loader
- ScanAndLoadByCategory 方法
- 单元测试

### Step 5: 重构 plugins.go
- 移除 hardcoded case
- 改为通用 .so 加载

### Step 6: 集成到 build/dev/daemon 流程
- build/main.go: 加载 static 插件
- dev.go: 加载 static 插件
- daemon.go: 加载 static 插件 + LifecycleManager 加载 dynamic 插件

### Step 7: 删除 internal/seo/
- 清理旧代码
- 更新 capabilityLabels

### Step 8: 端到端验证
- huan build 验证
- huan daemon 验证
- 全量测试

## 五、风险

| 风险 | 缓解 |
|------|------|
| .so 文件在 build 时尚未编译 | 构建前先 `make plugins` 或 `huan plugin build` |
| 插件加载失败导致 build 失败 | 失败时给出明确错误信息，指明是哪个插件 |
| 向后兼容性 | 未指定 category 的插件默认 dynamic，现有行为不变 |
| SEO 插件迁移后功能不一致 | 迁移后与 Hugo 输出逐文件对比验证 |