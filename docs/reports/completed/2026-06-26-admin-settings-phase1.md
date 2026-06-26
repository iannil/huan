# 实现计划：Admin 站点配置后台 — Phase 1

## 决策汇总

| 决策点 | 选择 | 详情 |
|--------|------|------|
| **方向** | B — 站点配置后台 | 通过 admin UI 修改 `huan.yaml` |
| **生效方式** | C — 混合模式 | 热更新 + 重建触发；需重启的配置 UI 标记 ⚠️ |
| **UI 方案** | 混合（表单 + YAML 编辑器） | 常用字段表单，高级用户可切到 YAML 原文件编辑 |
| **导航位置** | Layout 新增「设置」导航项 | `/admin/settings` |
| **YAML 写入** | 双轨制 | 表单用 `yaml.Node` 保格式修改；YAML 编辑器直接文本替换 |
| **Phase 1 范围** | 组 A（文本字段）+ 组 B（开关/数值） | 组 C（菜单/社交/关键词）延后到 Phase 1.5 |

## Phase 1 字段清单

### 组 A — 输入框

| 字段 | YAML 路径 | 类型 | UI 控件 |
|------|-----------|------|---------|
| 站点标题 | `title` | string | Input |
| 副标题 | `params.subTitle` | string | Input |
| 页脚标语 | `params.footerSlogan` | string | Input |
| 站点描述 | `params.description` | string | Textarea |
| 版权信息 | `params.copyrights` | string | Input |
| Google Analytics | `params.googleAnalytics` | string | Input |
| CDN URL | `params.cdnURL` | string | Input |

### 组 B — 开关 / 数字输入

| 字段 | YAML 路径 | 类型 | UI 控件 |
|------|-----------|------|---------|
| 启用摘要 | `params.enableSummary` | bool | Switch |
| 启用 MathJax | `params.enableMathJax` | bool | Switch |
| 启用 Emoji | `enableEmoji` | bool | Switch |
| 压缩 HTML | `minify` | bool | Switch |
| 页面分页数 | `paginate` | int | Input (number) |
| 摘要长度 | `summaryLength` | int | Input (number) |

## 文件变更清单

### 新增文件

| 文件 | 内容 |
|------|------|
| `internal/admin/settings.go` | `SiteSettings` 结构体 + `readSettings()` + `updateSettings()` + YAML raw 端点 |
| `web/admin/src/pages/Settings.tsx` | 设置页面组件（表单 + YAML 编辑器切换） |
| `web/admin/src/pages/SettingsYamlEditor.tsx` | YAML 原始文件编辑器 |

### 修改文件

| 文件 | 修改内容 |
|------|---------|
| `internal/admin/api.go` | 新增 `settings` 和 `settings/yaml` case；`apiHandler` 新增 `sourceDir` 字段 |
| `internal/admin/handler.go` | 传递 `sourceDir` 给 `newAPIHandler()` |
| `web/admin/src/App.tsx` | 添加 `/admin/settings` 路由 |
| `web/admin/src/components/Layout.tsx` | 侧栏添加「设置」导航项 |

## 后端架构

### apiHandler 新增字段

```go
type apiHandler struct {
    ops       *contentOps
    media     *mediaOps
    rebuild   func()
    sourceDir string  // ← 新增，用于读取 huan.yaml
    siteTitle string
    baseURL   string
    serveURL  string
    staticDir string
}
```

### 新增路由

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/admin/api/settings` | 返回 JSON 格式的 SiteSettings |
| PUT | `/admin/api/settings` | 接收 JSON → yaml.Node 保格式写入 → 触发重建 |
| GET | `/admin/api/settings/yaml` | 返回 huan.yaml 原始文本 |
| PUT | `/admin/api/settings/yaml` | 直接替换 huan.yaml 文本 → 触发重建 |

### yaml.Node 修改策略

1. 读取 huan.yaml 原始 bytes → `yaml.Unmarshal` 到 `yaml.Node`
2. 在 `yaml.Node` 树中定位目标 key 的 scalar node
3. 修改 `Value` 字段（保留注释/格式/行号）
4. `yaml.Encoder` 写回文件（SetIndent=2）

## 前端架构

### Settings 页面布局

```
┌─────────────────────────────────────────┐
│  站点设置                          YAML │  ← Tab 切换
├─────────────────────────────────────────┤
│  站点信息                               │
│  ┌─────────────────────────────────┐   │
│  │ 站点标题     [________________] │   │
│  │ 副标题       [________________] │   │
│  │ 站点描述     [________________] │   │
│  │ ...                             │   │
│  └─────────────────────────────────┘   │
│                                         │
│  功能开关                               │
│  ┌─────────────────────────────────┐   │
│  │ 启用摘要        [Switch]        │   │
│  │ 启用 MathJax    [Switch]        │   │
│  │ ...                             │   │
│  └─────────────────────────────────┘   │
│                                         │
│  [保存设置]        [触发重建]           │
└─────────────────────────────────────────┘
```

### 数据流

```
表单输入 → Settings 组件 state
    → 用户点击「保存」
    → PUT /admin/api/settings (JSON body)
    → Go: yaml.Node 修改 huan.yaml
    → 触发 rebuild
    → 响应 { status: "saved" }
    → 表单显示绿色「已保存」toast
```

## 实施步骤

1. 后端：新增 `settings.go` — `SiteSettings` 结构体 + yaml.Node 读写
2. 后端：修改 `api.go` — 新增 settings case + sourceDir 字段
3. 后端：修改 `handler.go` — 传递 sourceDir
4. 前端：新增 `Settings.tsx` — 表单页面
5. 前端：修改 `App.tsx` — 添加路由
6. 前端：修改 `Layout.tsx` — 添加导航项
7. 构建验证：`go build` + `npm run build`
