# zhurongshuo 主题模板迁移实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 zhurongshuo 站点的 30 个模板和 45 个静态资源文件完整迁移到主题插件中，新增 ShortcodeProvider 接口，集成到构建管线。

**Architecture:** 迁移分四步：(1) 复制文件；(2) 更新路径引用；(3) 扩展 ThemePlugin 接口（ShortcodeProvider + ThemeHooks 实现）；(4) 集成到构建管线并验证。

**Tech Stack:** Go + html/template + embed.FS + cobra

## 全局约束

- 模板文件名、目录结构与 Hugo 保持一致（`_default/`、`partials/`、`book/` 等）
- 资源引用路径从 `/css/xxx.css` → `/theme/zhurongshuo/css/xxx.css`，同理 `/js/`、`/images/`、`/fonts/`
- 不迁移 `robots.txt`（属于站点配置）
- `.DS_Store` 文件不迁移
- 插件名 `zhurongshuo`，模块名 `github.com/iannil/huan-plugin-zhurongshuo`

---

### Task 1: 迁移静态资源文件

**Files:**
- Copy: `../zhurongshuo/static/css/*.css` → `plugins/zhurongshuo/assets/css/`
- Copy: `../zhurongshuo/static/js/*.js` → `plugins/zhurongshuo/assets/js/`
- Copy: `../zhurongshuo/static/images/*` → `plugins/zhurongshuo/assets/images/`
- Copy: `../zhurongshuo/static/fonts/*` → `plugins/zhurongshuo/assets/fonts/`
- Modify: `plugins/zhurongshuo/plugin.go`（更新 `Assets()` 确保新资源被嵌入）

**Steps:**

- [ ] **Step 1: 复制 CSS 文件**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
cp ../zhurongshuo/static/css/zozo.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/normalize.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/highlight.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/animate.min.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/animate-custom.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/remixicon.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/remixicon-custom.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/fancybox.min.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/jquery.fancybox.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/gallery.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/search.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/comments.css plugins/zhurongshuo/assets/css/
cp ../zhurongshuo/static/css/post-share.css plugins/zhurongshuo/assets/css/
```

- [ ] **Step 2: 复制 JS 文件**

```bash
cp ../zhurongshuo/static/js/zozo.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/jquery-3.5.1.min.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/jquery.fancybox.min.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/fuse.min.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/qrcode.min.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/html2canvas.min.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/post-share.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/lang-dropdown.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/search.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/ga-optimizer.js plugins/zhurongshuo/assets/js/
cp ../zhurongshuo/static/js/ga-events.js plugins/zhurongshuo/assets/js/
```

- [ ] **Step 3: 复制图片和字体**

```bash
cp ../zhurongshuo/static/images/favicon.ico plugins/zhurongshuo/assets/images/
cp ../zhurongshuo/static/images/logo.png plugins/zhurongshuo/assets/images/
cp ../zhurongshuo/static/images/bg.png plugins/zhurongshuo/assets/images/
cp ../zhurongshuo/static/fonts/remixicon-custom.woff2 plugins/zhurongshuo/assets/fonts/
```

- [ ] **Step 4: 构建并验证嵌入**

```bash
cd plugins/zhurongshuo && go build -buildmode=plugin -o ../../zhurongshuo.so .
```

- [ ] **Step 5: Commit**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
git add plugins/zhurongshuo/assets/
git commit -m "feat(zhurongshuo): migrate static assets (45 files)"
```

---

### Task 2: 迁移模板文件（第一部分 — 核心布局和 partials）

**Files:**
- Copy: `../zhurongshuo/layouts/index.html` → `plugins/zhurongshuo/templates/index.html`
- Copy: `../zhurongshuo/layouts/404.html` → `plugins/zhurongshuo/templates/404.html`
- Copy: `../zhurongshuo/layouts/_default/*.html` → `plugins/zhurongshuo/templates/_default/`
- Copy: `../zhurongshuo/layouts/partials/*.html` → `plugins/zhurongshuo/templates/partials/`
- Copy: `../zhurongshuo/layouts/_default/*.xml` → `plugins/zhurongshuo/templates/_default/`
- Copy: `../zhurongshuo/layouts/_default/*.json` → `plugins/zhurongshuo/templates/_default/`

同时：**搜索替换资源路径**：
- `/css/` → `/theme/zhurongshuo/css/`
- `/js/` → `/theme/zhurongshuo/js/`
- `/images/` → `/theme/zhurongshuo/images/`
- `/fonts/` → `/theme/zhurongshuo/fonts/`

- [ ] **Step 1: 复制并替换 core 模板**

```bash
cd /Users/rong.zhu/Code/zhurong/huan

# index.html
cp ../zhurongshuo/layouts/index.html plugins/zhurongshuo/templates/
cd plugins/zhurongshuo/templates
sed -i '' 's|/css/|/theme/zhurongshuo/css/|g' index.html
sed -i '' 's|/js/|/theme/zhurongshuo/js/|g' index.html
sed -i '' 's|/images/|/theme/zhurongshuo/images/|g' index.html
sed -i '' 's|/fonts/|/theme/zhurongshuo/fonts/|g' index.html

# 404.html
cp ../../../../zhurongshuo/layouts/404.html ./
sed -i '' 's|/css/|/theme/zhurongshuo/css/|g' 404.html
sed -i '' 's|/images/|/theme/zhurongshuo/images/|g' 404.html
```

- [ ] **Step 2: 复制 _default/ 模板**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo/templates
mkdir -p _default

for f in ../../../../zhurongshuo/layouts/_default/*; do
  dest="_default/$(basename "$f")"
  cp "$f" "$dest"
  sed -i '' 's|/css/|/theme/zhurongshuo/css/|g' "$dest"
  sed -i '' 's|/js/|/theme/zhurongshuo/js/|g' "$dest"
  sed -i '' 's|/images/|/theme/zhurongshuo/images/|g' "$dest"
  sed -i '' 's|/fonts/|/theme/zhurongshuo/fonts/|g' "$dest"
done
```

复制范围：`list.html`、`single.html`、`terms.html`、`rss.xml`、`search.json`、`index.searchindex.json`、`sitemap.xml`、`sitemapindex.xml`

- [ ] **Step 3: 复制 partials/ 模板**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo/templates
mkdir -p partials

for f in ../../../../zhurongshuo/layouts/partials/*; do
  dest="partials/$(basename "$f")"
  cp "$f" "$dest"
  sed -i '' 's|/css/|/theme/zhurongshuo/css/|g' "$dest"
  sed -i '' 's|/js/|/theme/zhurongshuo/js/|g' "$dest"
  sed -i '' 's|/images/|/theme/zhurongshuo/images/|g' "$dest"
  sed -i '' 's|/fonts/|/theme/zhurongshuo/fonts/|g' "$dest"
done
```

复制范围：`head.html`、`header.html`、`footer.html`、`nav.html`、`post.html`、`schema.html`、`comments.html`、`search.html`、`js.html`、`mathjax.html`

- [ ] **Step 4: 构建验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo
go build -buildmode=plugin -o ../../zhurongshuo.so .
```

- [ ] **Step 5: Commit**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
git add plugins/zhurongshuo/templates/
git commit -m "feat(zhurongshuo): migrate core layouts and partials"
```

---

### Task 3: 迁移模板文件（第二部分 — section 模板）

**Files:**
- Copy: `../zhurongshuo/layouts/book/` → `plugins/zhurongshuo/templates/book/`
- Copy: `../zhurongshuo/layouts/books/` → `plugins/zhurongshuo/templates/books/`
- Copy: `../zhurongshuo/layouts/gallery/` → `plugins/zhurongshuo/templates/gallery/`
- Copy: `../zhurongshuo/layouts/practice/` → `plugins/zhurongshuo/templates/practice/`
- Copy: `../zhurongshuo/layouts/practices/` → `plugins/zhurongshuo/templates/practices/`
- Copy: `../zhurongshuo/layouts/products/` → `plugins/zhurongshuo/templates/products/`

- [ ] **Step 1: 复制 section 模板文件并替换路径**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo/templates

for section in book books gallery practice practices products; do
  mkdir -p "$section"
  for f in ../../../../zhurongshuo/layouts/$section/*; do
    [ -f "$f" ] || continue
    dest="$section/$(basename "$f")"
    cp "$f" "$dest"
    sed -i '' 's|/css/|/theme/zhurongshuo/css/|g' "$dest"
    sed -i '' 's|/js/|/theme/zhurongshuo/js/|g' "$dest"
    sed -i '' 's|/images/|/theme/zhurongshuo/images/|g' "$dest"
    sed -i '' 's|/fonts/|/theme/zhurongshuo/fonts/|g' "$dest"
  done
done
```

- [ ] **Step 2: 构建验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo
go build -buildmode=plugin -o ../../zhurongshuo.so .
```

- [ ] **Step 3: Commit**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
git add plugins/zhurongshuo/templates/
git commit -m "feat(zhurongshuo): migrate section-specific templates (book/books/gallery/practice/practices/products)"
```

---

### Task 4: 扩展 ThemePlugin 接口 — 新增 ShortcodeProvider

**Files:**
- Modify: `internal/theme/types.go` — 新增 `ShortcodeProvider` 接口
- Test: `internal/theme/types_test.go` — 验证接口

**Interfaces:**
- Consumes: `shortcode.Handler`（来自 `internal/shortcode/shortcode.go`）
- Produces: `ShortcodeProvider` 接口

- [ ] **Step 1: 添加 ShortcodeProvider 接口**

```go
// internal/theme/types.go — 在 ThemeHooks 接口后添加

// ShortcodeProvider 是可选接口，主题可实现它来提供或覆盖 shortcode。
type ShortcodeProvider interface {
    // Shortcodes returns a map of shortcode name to handler.
    // These handlers are registered in the shortcode registry,
    // overriding any built-in handlers with the same name.
    Shortcodes() map[string]shortcode.Handler
}
```

- [ ] **Step 2: 运行测试验证**

Run: `go test ./internal/theme/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/theme/types.go
git commit -m "feat(theme): add ShortcodeProvider optional interface"
```

---

### Task 5: 集成 ShortcodeProvider 到构建管线

**Files:**
- Modify: `internal/build/pipeline_setup.go`（在 `renderMarkdownAndTree` 或 `setupTemplatesAndWriter` 中集成）

- [ ] **Step 1: 修改 pipeline_setup.go 注册主题 shortcode**

在 `renderMarkdownAndTree` 方法中，在创建 `scRegistry` 后，检查激活主题是否实现 `ShortcodeProvider`：

```go
// internal/build/pipeline_setup.go — 在 renderMarkdownAndTree 方法中

func (p *pipeline) renderMarkdownAndTree() error {
    p.scRegistry = shortcode.NewRegistry()
    p.md = markdown.NewRenderer(&p.cfg.Markup)

    // Register theme shortcodes if the active theme provides them
    if p.themeManager != nil {
        if tp := p.themeManager.Active(); tp != nil {
            if sp, ok := tp.(theme.ShortcodeProvider); ok {
                for name, handler := range sp.Shortcodes() {
                    p.scRegistry.Register(name, handler)
                    p.logf("  shortcode: theme registered %q\n", name)
                }
            }
        }
    }

    // 后续代码不变...
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/build/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/build/pipeline_setup.go
git commit -m "feat(build): integrate theme ShortcodeProvider into shortcode registry"
```

---

### Task 6: 实现 ZhurongshuoTheme 的 ThemeHooks 和 ShortcodeProvider

**Files:**
- Modify: `plugins/zhurongshuo/plugin.go` — 实现 `ShortcodeProvider` 和 `ThemeHooks`
- Modify: `plugins/zhurongshuo/plugin/plugin.go` — 添加 `ShortcodeProvider` 和 `ThemeHooks` 类型副本

- [ ] **Step 1: 在 plugin/plugin.go 中添加接口副本**

```go
// plugins/zhurongshuo/plugin/plugin.go — 在 ThemeHooks 后添加

// ShortcodeProvider is an optional interface that themes can implement
// to register custom shortcodes.
type ShortcodeProvider interface {
    Shortcodes() map[string]ShortcodeHandler
}

// ShortcodeHandler is a function that renders a shortcode.
type ShortcodeHandler func(ctx ShortcodeContext) (string, error)

// ShortcodeContext carries the parameters and context for a shortcode invocation.
type ShortcodeContext struct {
    Params map[string]string
    Inner  string
    Page   interface{}
    Site   interface{}
}
```

- [ ] **Step 2: 在 plugin.go 中实现 ThemeHooks 和 ShortcodeProvider**

```go
// plugins/zhurongshuo/plugin.go — 新增方法

// BeforeRender implements theme.ThemeHooks.
func (t *ZhurongshuoTheme) BeforeRender(ctx context.Context) error {
    // 预留：注入模板全局变量等
    return nil
}

// AfterRender implements theme.ThemeHooks.
func (t *ZhurongshuoTheme) AfterRender(ctx context.Context) error {
    // 预留：后处理等
    return nil
}

// Shortcodes implements theme.ShortcodeProvider.
// Returns the shortcode handlers provided by this theme.
func (t *ZhurongshuoTheme) Shortcodes() map[string]func(ctx ShortcodeContext) (string, error) {
    return map[string]func(ctx ShortcodeContext) (string, error){
        // zhurongshuo 主题暂不提供自定义 shortcode
        // 内置的 audio/img shortcode 由 huan 引擎提供
    }
}
```

- [ ] **Step 3: 构建验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo
go build -buildmode=plugin -o ../../zhurongshuo.so .
```

- [ ] **Step 4: Commit**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
git add plugins/zhurongshuo/
git commit -m "feat(zhurongshuo): implement ThemeHooks and ShortcodeProvider"
```

---

### Task 7: 端到端验证

**Files:**
- Run: 构建插件
- Run: 全量测试
- Run: 验证主题加载

- [ ] **Step 1: 构建插件**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/zhurongshuo
go build -buildmode=plugin -o ../../zhurongshuo.so .
ls -lh ../../zhurongshuo.so
```

- [ ] **Step 2: 全量测试**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go test ./...
```

- [ ] **Step 3: 验证主题 CLI**

```bash
go build -o huan ./cmd/huan
./huan theme list
```

- [ ] **Step 4: Commit（如果有额外修复）**

```bash
git add -A
git commit -m "fix: address migration issues"
```
