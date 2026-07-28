# EN 书籍页修复 + 插件查找/发布约定

- **日期**：2026-07-28
- **状态**：已完成
- **范围**：huan 引擎 + zhurongshuo 站点配置

## 一、背景

延续 zhurongshuo 主题迁移（`docs/superpowers/specs/2026-07-27-zhurongshuo-theme-migration-design.md`）的验收，
用户报告英文页 `/en/practices/season-2/silent-games/` 两个问题；随后追加两条插件系统约定。

## 二、修复清单

### Bug #1：英文书籍页显示中文标题

- **根因**：`plugins/zhurongshuo/templates/practice/list.html` 无条件读取 `.Site.Data.practices`（中文
  `data/practices.yaml`）。huan 的 `.Site.Data` 不按语言作用域，`data/en/practices.yaml` 挂在 `.Site.Data.en.practices`。
- **修复**：在单书模板加入语言作用域覆盖（镜像顶层 `practices/list.html` 已有写法）：
  `{{ if eq $.Site.LanguageCode "en" }}{{ with .Site.Data.en }}{{ with .practices }}{{ $practicesData = . }}{{ end }}{{ end }}{{ end }}`。
  书名/副标题/部分标题随之走英文数据。

### Bug #2：英文书籍页目录为空（只有标题）

- **根因**：zhurongshuo `huan.yaml` 把 `practices` 放进 `catalogSections`。catalog 语义是「仅渲染
  `_index.<lang>` 索引、丢弃全部内容页」（用于未翻译章节的书目占位）。而 silent-games 章节已 100% 翻译
  （12 en / 12 zh），catalog 过滤把已翻译章节全部丢弃 → `RegularPagesRecursive` 为空 → 目录为空。
- **修复**：从 `catalogSections` 移除 `practices`（保留 `books`，其为独立同类问题，未在本次报告范围）。
  移除后走默认过滤（`p.Language == cc`）：已翻译章节发布并进入目录；未翻译章节自动排除。书目列表本身由
  data 文件驱动，占位行为不受影响，无回归。
- **验证**：EN 页显示英文书名 + 完整英文目录（导论 + 9 章 + 结语 = 11 条链接）；ZH 页无变化；EN 渲染页数
  540 → 837。

## 三、插件系统约定（用户追加）

### 3.1 查找顺序：$HUAN_HOME 优先，其次项目目录

- `internal/plugin/loader.go` 新增 `HuanHome()`（读 `$HUAN_HOME`，默认 `~/.huan`）、`searchDirs()`
  （`[$HUAN_HOME, <project>/plugins]` 去空去重）、`Resolve(soFile)`（按优先级找 .so）。
- `ScanAndLoad` / `ScanAndLoadByCategory` 改为遍历 searchDirs，同名插件高优先级目录胜出（去重）。
- 单元测试：`TestHuanHome_*`、`TestLoader_Resolve_*`。

### 3.2 命令代码不硬编码插件名（按能力发现）

- deploy / translate / image 命令不再出现 `cloudflare.so` / `qwen3.so` / `image_pipeline` 字面量。
- 新增 `cmd/huan/plugins.go: loadConfiguredPlugins()`：加载 huan.yaml 声明的所有插件（按约定文件名 + Resolve），
  再用 `plugin.Find[deploy.Deployer / translate.Translator / image.ImageProcessor]` 按能力接口挑选。
- 约定辅助 `internal/plugin.SoFileName(name)`：配置名下划线 → 文件名连字符（`qwen3_translate` → `qwen3-translate.so`）。
- `runImagePipeline` 改为纯能力发现（构建期图片插件应为 static/mixed，与 seo/sitemap 一致），签名去掉 cfg 参数。

### 3.3 编译产物发布到 release/plugins

- 新增 `scripts/build-plugins.sh`：遍历 `plugins/*/`，从各插件 `Name()` 按约定推导 .so 文件名，编译到
  `release/plugins/`（`/release/` 已 gitignore，属可重建构建产物）。
- 将 `qwen3.so` 按约定统一为 `qwen3-translate.so`。
- 端到端验证：`HUAN_HOME=release/plugins huan build` → 主题正常加载（37 模板），两处 Bug 均修复。

## 四、附带修复

- 主题迁移遗留的测试编译错误（commit 111e75a 将 `ThemePlugin.Info()` 改为 `map[string]any`、
  `Templates()` 改为 `[]map[string]string`，但未更新测试 mock）：修复
  `internal/theme/{types,manager}_test.go` 与 `internal/template/loader_test.go` 的 mock，全量 `go test ./...` 恢复通过。

## 五、后续（已完成）

- **`books` 一并修复**：从 `catalogSections` 移除 `books`（catalogSections 现为空，已删键）。
  注意 `book/list.html` **本已有**语言作用域覆盖，故 books 无 Bug #1，只需去 catalog。所有卷章节均全译，
  英文书籍页现显示英文标题 + 完整英文目录（验证 `the-silent-construction`：导论 + 4 部分 + 10 章）。
  EN 渲染 837 → 1022 页。
- **插件安装到 `~/.huan`**：`cp release/plugins/*.so ~/.huan/`，随后 **不设** `HUAN_HOME` 直接 `huan build`
  即走默认 `~/.huan` 查找，主题正常（37 模板），两 section 均正确。

## 六、遗留 / 提示
- 仓库根目录仍有 git 跟踪的 .so 二进制（`cloudflare.so` 等）；发布约定已改为 `release/plugins/`，
  这些根目录二进制可作为后续清理项由用户决定是否移除。
- 处理期间检测到**并发进程**在删除仓库跟踪文件（根 .so、`scripts/diff-*.sh`、`cmd/equiv-check/main.go`），
  非本次改动所致，已提示用户核查。
