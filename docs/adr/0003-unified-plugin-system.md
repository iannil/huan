# ADR 0003：统一插件系统

- **状态**：Accepted
- **日期**：2026-06-13
- **决策者**：用户（owner）+ Claude（grill-me 收敛）
- **替代方案**：见下方各分叉点的备选项
- **被引用**：[ADR 0002](0002-cloudflare-deploy-plugin.md)（首个 deploy 插件实例）

## 背景

[ADR 0002](0002-cloudflare-deploy-plugin.md) 初版把 Cloudflare 发布做成 huan 的「内置 deploy 插件」，但只覆盖 deploy 一种能力：

1. `internal/deploy/types.go` 同时拥有 `Deployer` 接口与 `Registry` —— 命名空间窄，未来加非 deploy 类型插件（付费 / 多语言 / 内容扩展 / 会员）无处归类。
2. yaml 顶层 `deploy.cloudflare.*` —— per-domain 顶层 key，每加一类插件都污染 huan.yaml 顶层。
3. CLI 没有 plugin 管理命令（list / info）—— 用户无法 discover 已编译进二进制的插件清单与 effective config。

用户预期未来会出现的非 deploy 类型插件：

| 类型 | 例子 | 生命周期阶段 |
|---|---|---|
| 付费 | stripe / 微信 / 支付宝 | runtime（HTTP 回调） |
| 多语言 | 自动翻译 | build-time 或 runtime |
| 内容扩展 | WordPress 风格自定义 post type | build-time |
| 会员 | 等级、鉴权 | runtime |

这些插件横跨 build / publish / runtime 三个生命周期阶段。若沿用 ADR 0002 的 deploy-only Registry 模型，每加一类插件都要新增一个顶层 Registry（`internal/payment/registry.go` / `internal/i18n/registry.go` / ...），代码碎片化、配置形状不一、CLI 各自一套语法。

需要把插件做成 huan 的**一等扩展机制**，让 deploy / payment / i18n / membership 等**所有未来插件**共享同一套：基接口 + Registry + 配置命名空间 + CLI 管理命令 + 凭证机制。

## 决策

新建顶层 `internal/plugin/` 包，定义统一插件宿主。Q1-Q7 决策固化如下。

### 1. 形态：顶层 plugin host + capability 接口

- `internal/plugin/` 拥有 `Plugin` 基接口、`Registry`、capability 查询 helper。
- 每个插件**领域包**（如 `internal/deploy/`、未来的 `internal/payment/`）自己拥有它的 capability 接口（`Deployer` / `PaymentProvider` 等）。
- 插件实现位于领域子包（如 `internal/deploy/cloudflare/`），编译期内置。

**YAGNI 边界**：首期只定义 `Deployer` capability，**不**预定义 `PaymentProvider` / `MultiLanguageProvider` / `MembershipProvider` 等接口。这些接口等各自首个实现真要落地时再画 —— 提前画大概率因「真正实施时发现需求不一样」而返工。

### 2. Plugin 基接口：极简 + 构造器模式

```go
// internal/plugin/plugin.go
package plugin

type Plugin interface {
    Name() string  // e.g. "cloudflare" / "stripe"
}

type Registry struct {
    plugins map[string]Plugin
    order   []string  // 注册顺序，稳定迭代
}

func NewRegistry() *Registry
func (r *Registry) Register(p Plugin) error          // 重名报错
func (r *Registry) Get(name string) (Plugin, bool)
func (r *Registry) All() []Plugin                     // 按 order 返回

// 通用 capability 查询（Go 1.18+ 泛型）
func Find[T any](r *Registry) []T {
    var out []T
    for _, p := range r.All() {
        if t, ok := p.(T); ok { out = append(out, t) }
    }
    return out
}
```

**关键约束**：
- 不在 Plugin 上加 `Init(cfg)` / `Start()` / `Stop()` —— 配置由每个插件构造器吃进去（`cloudflare.New(cfg)`），生命周期方法下沉到具体 capability 接口（如未来的 `Server` capability 自带 `Start/Stop`）。
- 构造器失败直接 `return nil, err`，没有「半初始化对象」中间态。
- 测试零摩擦：直接 `cloudflare.New(testCfg)`，不需要两段式 New + Init dance。

### 3. Capability 接口：领域包自治

每个领域包定义自己的 capability 接口，依赖 `plugin.Plugin` 作为基础：

```go
// internal/deploy/types.go
package deploy

import (
    "context"
    "github.com/iannil/huan/internal/plugin"
)

type Deployer interface {
    plugin.Plugin  // 嵌入 Name()
    Deploy(ctx context.Context, opts Options) (*Report, error)
}

type Options struct { ... }
type Report struct { ... }
```

**为什么 capability 接口分散在领域包而不是集中在 `internal/plugin/types.go`**：
- 避免 `internal/plugin/` 沦为「什么接口都往里塞」的大杂烩。
- 领域知识留在领域包（payment 接口不懂 deploy 的事，反之亦然）。
- `internal/plugin/` 保持纯净，只管 Plugin 基接口 + Registry，所有领域包都能依赖它而不会成环。

### 4. 配置：统一 `plugins:` 命名空间

```yaml
plugins:
  cloudflare:               # plugin name = yaml key
    accountId: ${CLOUDFLARE_ACCOUNT_ID}
    apiToken: ${CLOUDFLARE_API_TOKEN}
    pages: { project: zhurongshuo, branch: main }
    # ...

  # 未来：
  # stripe:
  #   apiKey: ${STRIPE_API_KEY}
  # wechat_pay:
  #   mchId: ${WECHAT_MCH_ID}
  # auto_translate:
  #   provider: deepmd
```

**规则**：
- 每个 plugin 在 `plugins:` 下有一个 key，**plugin 身份 = yaml key**。
- `cfg.Plugins` 是 `map[string]map[string]any`，每个 value 是该 plugin 的 raw 配置。
- 每个 plugin 的 `ParseConfig(raw map[string]any) (Config, error)` 自己负责反序列化与校验。
- 不在 yaml 里写 `type: deploy` 字段 —— capability 由代码里的 interface 断言决定，写两遍只会两边对不上。

### 5. 凭证：`${VAR}` 插值（strict mode）

- **整个 `huan.yaml`**（不只 `plugins:`）支持 `${VAR_NAME}` 引用环境变量。
- 配置加载时统一插值，插件代码看到的 `raw map[string]any` 已经是解析后的字符串。
- **strict mode**：任何 `${VAR}` 引用了未设置的 env var，加载时立即报错。避免 `apiToken: ""` 静默失败。
- 不支持 `${VAR:-default}` 默认值语法（v1；需要默认直接写字面量）。
- 不做「知名 env var 自动 fallback」—— yaml 是单一真相源，env var 通过 `${VAR}` 显式引用，没有第二条路径。

```go
// internal/config/interpolate.go
var envPattern = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// Interpolate walks raw yaml tree, replaces ${VAR} with env values.
// Returns error if any ${VAR} references unset env var (strict mode).
func Interpolate(raw map[string]any) (map[string]any, error)
```

调用顺序：
1. `config.Load(path)` → `yaml.Unmarshal` → `raw`
2. `Interpolate(raw)` → resolved（失败 fail-fast）
3. `raw.Plugins["cloudflare"]` 传给 `cloudflare.ParseConfig`

### 6. CLI：per-capability verb + 管理命令

**动作命令**用 per-capability verb（deploy / payment / i18n / ...）：

```
huan deploy cloudflare [pages|r2|worker]
huan deploy cloudflare --build              # build + deploy
huan deploy cloudflare --dry-run            # 计算但不传

# 未来：
# huan payment stripe create-order ...
# huan i18n auto-translate build
```

**管理命令**统一在 `huan plugin`：

```
huan plugin list                            # 列出所有编译期内置 plugin + capability + 配置状态
huan plugin info <name>                     # 元数据 + effective config + health + last action
huan plugin info <name> --show-secrets      # 显示敏感字段（默认 mask 为 ***）
```

不加新 capability 不需要动管理命令 —— `huan plugin list` 通过 Registry 自动反映所有已注册插件。

**为什么不用 `huan plugin run <name> <action>`**：
- 「plugin」是工程概念，用户关心的是「部署」「付款」这种业务动作。
- 每次多 2 个词（`huan plugin run cloudflare deploy`）违反「single binary, simple CLI」定位。
- per-capability verb 让 CI yaml / Makefile / 文档自然好读。

### 7. 注册：编译期 hardcoded（composition root）

```go
// cmd/huan/plugins.go
package main

func newPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
    r := plugin.NewRegistry()
    for name, raw := range cfg.Plugins {
        switch name {
        case "cloudflare":
            cfCfg, err := cloudflare.ParseConfig(raw)
            if err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
            if err := r.Register(cloudflare.New(cfCfg)); err != nil { return nil, fmt.Errorf("plugin %s: %w", name, err) }
        default:
            return nil, fmt.Errorf("plugin %s: unknown (not compiled in)", name)
        }
    }
    return r, nil
}
```

**关键约束**：
- 接线在 `cmd/huan/plugins.go`（composition root），**不**在 `internal/plugin/`。
  - 原因：避免循环 import。`internal/deploy/cloudflare` 要 import `internal/plugin`（实现接口）；如果 `internal/plugin/builtin.go` 又 import `internal/deploy/cloudflare`，成环。把接线放到 binary main 包就断开。
- `internal/plugin/` 只定义契约（Plugin / Registry / capability helper），不导入任何具体插件。
- yaml 声明了未编译进二进制的 plugin（如 `plugins.unknown_thing: {}`）→ 启动时报错（fail-fast，避免 deploy 时才 NPE）。
- **不**用 `init()` 自注册 —— Go 反模式，init() 跨包顺序不可预测，测试难剥离。

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| 形态 | 只统一 publish-time，build-time / runtime 各自独立 | 用户预期的付费 / 多语言 / 内容扩展 / 会员跨 build / runtime，需统一 |
| 实施深度 | 一次性定义所有 capability 接口骨架（PaymentProvider / MultiLanguageProvider / ...） | YAGNI；提前画接口大概率返工 |
| 实施深度 | 把 §4.11 build-time 骨架一并迁入新 plugin 系统 | PR 过大；build-time 与 publish-time 是不同生命周期，分批迁移更稳 |
| Plugin 接口 | `Init(cfg)` + 半类型安全 Config | Java/C# 两段式构造习惯；Go 习惯是构造器吃配置 |
| Plugin 接口 | `Init/Start/Stop` 完整生命周期 | Deployer 是一次性 action 用不上 Start/Stop；需要时按 capability 加 |
| 配置形状 | per-domain 顶层 key（`deploy:` / `payment:` / ...） | host 必须知道每个顶层 key 名；`huan plugin list` 需扫描多个顶层 |
| 配置形状 | `plugins:` + 显式 `type` 字段 | type 与代码里 capability 接口重复，两边对不上 |
| 凭证 | env only（不插值） | 插件代码硬编码 env var 名；用户不看代码不知要设哪些 |
| 凭证 | 插值 + 知名 env fallback | 双轨制；规则不可发现（哪个赢？）|
| 凭证 | 不做 strict mode（unset = 空字符串）| `apiToken: ""` 静默失败，调试半天才知 env 没设 |
| CLI | `huan plugin run <name> <action>` 统一入口 | 脚本里全是 `huan plugin run ...`，可读性差 |
| CLI | 高频 capability 用 verb、低频用 `plugin run` | 规则不可发现（用户怎么知道 stripe 是低频？）|
| 管理 | 只有 `huan plugin list`，无 `info` | debug 失败时无法看 effective config；用户必须直接读 yaml |
| 注册 | `init()` 自注册 | Go 反模式；init() 顺序跨包字母序，依赖全局状态撞坑 |
| 注册 | go:generate 代码生成 | < 20 插件时不值得；多一个构建步骤 |
| 注册 | blank import 触发 init 注册 | 同上 + import 副作用 |

## 影响

### 文档

- 新增本 ADR（0003）。
- [ADR 0002](0002-cloudflare-deploy-plugin.md) 更新：去掉 deploy-only 范围，引用本 ADR；yaml 形状改 `deploy.cloudflare.*` → `plugins.cloudflare.*`；CLI 加 `huan plugin list/info` 一笔。
- `docs/technical-plan.md` §4.11 更新：老的 build-time 骨架与新统一 plugin 系统对齐。

### 代码

**新增**：

```
internal/plugin/
├── plugin.go              # Plugin 接口 + Registry + Find[T] helper
└── plugin_test.go

internal/config/
└── interpolate.go         # ${VAR} strict 插值
```

**改造**（原 ADR 0002 设想的调整）：

- `internal/deploy/types.go` 不再拥有 `Registry`，只拥有 `Deployer` capability 接口 + `Options` + `Report`。
- `internal/deploy/cloudflare/plugin.go` 实现 `plugin.Plugin`（嵌入 Name）+ `deploy.Deployer`。
- `cmd/huan/plugins.go` 新增：cfg → plugin.Registry 接线（composition root）。
- `cmd/huan/plugin_cmd.go` 新增：`huan plugin list` / `huan plugin info` 子命令。

**未来扩展路径**（不在首期实施范围）：

```
internal/payment/
├── types.go               # PaymentProvider capability 接口
└── stripe/
    └── plugin.go          # 实现 plugin.Plugin + payment.PaymentProvider
```

加新插件 = 新建领域包 + 在 `cmd/huan/plugins.go` switch 加一个 case + yaml 加 `plugins.<name>.*`。**不动 `internal/plugin/`**。

### 风险

1. **首期只有 Deployer 一个 capability**：capability 接口模式是否真的能优雅承载付费 / 多语言等场景，要等第二个领域包真落地才验证。**缓解**：plugin 基接口极简（只有 Name），领域包自治，未来加 capability 几乎不可能伤到 host 包。
2. **yaml 失去 domain 分组语义**：所有 plugin 平铺在 `plugins:` 下。**缓解**：靠注释分组（`# === payment ===`）+ plugin 命名（`stripe` / `wechat_pay` / `alipay` 本身够清楚）。
3. **配置加载必须做 strict 插值**：用户首次配置时若漏设 env var，启动即报错。这是 feature 不是 bug —— fail-fast 比 401 debug 体验好。
4. **未知 plugin fail-fast 可能误伤**：用户写了 `plugins.cloudflare` 但还没升级到含该 plugin 的二进制版本，启动报错。**缓解**：错误信息明确指出「unknown plugin 'cloudflare', not compiled in」，用户立刻知道是二进制版本问题。
