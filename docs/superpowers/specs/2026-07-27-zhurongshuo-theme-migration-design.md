# zhurongshuo 主题模板迁移设计

- **日期**：2026-07-27
- **状态**：Draft
- **设计者**：用户 + Claude

## 一、背景

zhurongshuo（祝融说）主题插件 `plugins/zhurongshuo/` 已创建骨架（10 个模板 + 2 个资源文件），
但当前模板仅为占位符，未包含站点的实际布局逻辑。需要从 `../zhurongshuo/layouts/` 和 `../zhurongshuo/static/`
迁移完整内容到主题插件中。

## 二、范围

### 2.1 模板迁移（30 个文件）

从 `../zhurongshuo/layouts/` 复制到 `plugins/zhurongshuo/templates/`：

```
templates/
├── index.html
├── 404.html
├── _default/
│   ├── list.html
│   ├── single.html
│   ├── terms.html
│   ├── rss.xml
│   ├── search.json
│   ├── index.searchindex.json
│   ├── sitemap.xml
│   └── sitemapindex.xml
├── book/list.html
├── books/list.html
├── gallery/list.html
├── gallery/single.html
├── practice/list.html
├── practices/list.html
├── products/list.html
├── products/single.html
├── partials/
│   ├── head.html
│   ├── header.html
│   ├── footer.html
│   ├── nav.html
│   ├── post.html
│   ├── schema.html
│   ├── comments.html
│   ├── search.html
│   ├── js.html
│   └── mathjax.html
```

### 2.2 静态资源迁移（45 个文件）

从 `../zhurongshuo/static/` 复制到 `plugins/zhurongshuo/assets/`：

```
assets/
├── css/
│   ├── zozo.css
│   ├── normalize.css
│   ├── highlight.css
│   ├── animate.min.css
│   ├── animate-custom.css
│   ├── remixicon.css
│   ├── remixicon-custom.css
│   ├── fancybox.min.css
│   ├── jquery.fancybox.css
│   ├── gallery.css
│   ├── search.css
│   ├── comments.css
│   └── post-share.css
├── js/
│   ├── zozo.js
│   ├── jquery-3.5.1.min.js
│   ├── jquery.fancybox.min.js
│   ├── fuse.min.js
│   ├── qrcode.min.js
│   ├── html2canvas.min.js
│   ├── post-share.js
│   ├── lang-dropdown.js
│   ├── search.js
│   ├── ga-optimizer.js
│   └── ga-events.js
├── images/
│   ├── favicon.ico
│   ├── logo.png
│   └── bg.png
└── fonts/
    └── remixicon-custom.woff2
```

注意：`robots.txt` 属于站点配置，不嵌入主题插件。

### 2.3 资源引用路径更新

模板中所有静态资源引用需要从 Hugo 路径（`/css/xxx.css`）改为主题插件路径（`/theme/zhurongshuo/css/xxx.css`）。

### 2.4 模板函数验证

每个模板渲染时需要验证调用的模板函数是否存在。验证范围包括：
- huan 内置 FuncMap（`internal/template/funcs.go`）
- 主题插件 FuncMap（`plugins/zhurongshuo/funcs.go`）

## 三、扩展 ThemePlugin 接口

### 3.1 ShortcodeProvider

主题插件需要能声明它支持哪些 shortcode 或覆盖内置实现。

```go
// internal/theme/types.go — 新增

// ShortcodeProvider 是可选接口，主题可实现它来提供或覆盖 shortcode。
// 构建管线在注册 shortcode 时优先使用主题提供的 handler。
type ShortcodeProvider interface {
    Shortcodes() map[string]shortcode.Handler
}
```

### 3.2 构建管线集成

在 `internal/build/pipeline.go` 或 `internal/shortcode/registry.go` 中增加：
- 构建开始时，检查激活主题是否实现 `ShortcodeProvider`
- 如果是，将主题提供的 shortcode handler 注册到 shortcode Registry（覆盖内置）

## 四、实施步骤

### Step 1: 迁移模板文件
- 逐个复制 ../zhurongshuo/layouts/ 文件到 plugins/zhurongshuo/templates/
- 更新资源引用路径（/css/ → /theme/zhurongshuo/css/，/js/ → /theme/zhurongshuo/js/）

### Step 2: 迁移静态资源
- 复制 ../zhurongshuo/static/ 文件到 plugins/zhurongshuo/assets/

### Step 3: 扩展 ThemePlugin 接口
- 新增 ShortcodeProvider 接口
- 新增 PluginsHandler 接口（可选，用于声明插件中嵌入的静态资源依赖）

### Step 4: 集成 ShortcodeProvider 到构建管线
- shortcode Registry 支持从主题插件注册 handler
- 主题 handler 优先于内置 handler

### Step 5: 验证 FuncMap 完整性
- 构建站点并逐个模板渲染
- 发现缺失函数时补充到 FuncMap

### Step 6: 构建并测试
- `cd plugins/zhurongshuo && go build -buildmode=plugin`

## 五、风险与缓解

| 风险 | 缓解 |
|------|------|
| 模板语法不兼容（Hugo vs huan） | 逐个模板验证，使用 huan 的替换规则（replaceDottedFuncs） |
| 资源路径引用遗漏 | 迁移后全站截图对比 |
| .so 体积过大 | 45 个资源文件约 2-3MB，可接受 |
| 模板函数缺失导致渲染失败 | 提前枚举所有模板调用的函数，统一补齐 |
| favicon/robots.txt 等站点级文件被错误嵌入主题 | 明确不迁移 robots.txt；favicon 随资源文件迁移但保留 static/ 副本作为覆盖 |

## 六、验收标准

1. `go build -buildmode=plugin` 成功
2. 激活 zhurongshuo 主题后，站点渲染使用主题模板
3. 所有模板中的函数调用都能正确执行
4. 静态资源可通过 `/theme/zhurongshuo/` 路径访问
5. 站点头部/尾部/样式与当前生产环境一致
6. 全量测试通过（`go test ./...`）
