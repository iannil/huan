# goldmark 数学透传修复

更新时间：2026-10-27 17:42 +0800。状态：v0.12.3 已提交、推送、发布并完成本地 go install；GitHub Release 与 GHCR 发布流水线成功。

### 发布验收结果（2026-10-27 17:42 +0800）

- 源码提交及标签：`473f8e2b6c6458bd0ac5b4af999ba9955a06dc19`，`v0.12.3`。
- [GitHub Release](https://github.com/iannil/huan/releases/tag/v0.12.3)：五个平台归档 + checksums + manifest，共七个附件；已下载至 `release/0.12.3/`，五个归档 SHA256 全部匹配。darwin/arm64 发布二进制实测 `huan 0.12.3 (473f8e2)`。
- [Release workflow](https://github.com/iannil/huan/actions/runs/40482117212)：completed / success。
- GHCR：`ghcr.io/iannil/huan:v0.12.3` 镜像索引已通过 registry 查询验证，digest `sha256:eb1daf6dde47e527c54c5ad45f708673aaa797ff804f524ad24b35e170664748`；运行平台为 linux/amd64。
- 本地安装：先从标签对应的干净源码构建 Admin UI，再执行 `go install ./cmd/huan`；`/Users/rong.zhu/go/bin/huan version` 输出 `huan 0.12.3 (473f8e2)`，Go 构建信息为 v0.12.3、vcs.modified=false。SHA256：`8b684c928f0c9756b196637953d4b0afaf04dc20ecad484c123ce1c7c9c03c23`。
- 已安装二进制在独立 HUAN_HOME 下构建 minimal fixture 成功：4 页、17 文件；fixture 缺 terms/search 模板警告保留。
- 安装限制：直接 `go install github.com/iannil/huan/cmd/huan@v0.12.3` 失败，因为源码模块不含被忽略的 `internal/admin/dist`，而 handler 使用 go:embed。此次按现有 CI 的前端构建步骤解决本地安装，未改变已发布标签或模块打包机制。
- 前端构建通过；npm ci 报告现有依赖 22 个漏洞（5 moderate、16 high、1 critical），未在本次发布中执行依赖升级。
- 本次未重启已有服务、升级下游站点或替换本地插件；已发布镜像不等于下游线上服务已升级。

## v0.12.3 发布记录

- 源版本 v0.12.2 → 目标 v0.12.3，发布内容为本报告的数学透传修复，无新增依赖或插件契约变更。
- 2026-10-27 17:37 +0800：重新运行 `go test ./...`（1136 测试、35 包）及 `git diff --check`，均通过。
- 产物沿用 `.github/workflows/release.yml`：五个平台归档、SHA256 校验及 manifest 位于 `release/0.12.3/`；GHCR 镜像包含同源构建的宿主与七个插件。
- 发布顺序：源码提交 → master/标签推送 → CI 发布 → 本地指定版本安装与验证。消费方为 GitHub 下载用户、Go 安装用户和 GHCR 使用者；本次不修改下游站点锁定版本。
- 恢复方式：保留已有 v0.12.2 标签和本地旧二进制备份；需要恢复时可重新安装旧版本或使用旧镜像标签，不覆盖已发布标签。

## 问题与实现

正文与模板 `markdownify` 的 goldmark 均没有数学解析器，导致 `\(...\)` / `\[...\]` 的反斜杠、TeX 花括号转义、aligned 中的 `\\` 换行被 Markdown 消耗。单独调整浏览器 MathJax 配置无法恢复已经损坏的 HTML 文本。

新增共享 `internal/markdown/math.go` AST 扩展并在两个入口注册，无新第三方依赖。支持 `\(...\)`、`\[...\]`、`$...$`、`$$...$$`。正文使用 HTML 转义而非原始 HTML 注入，浏览器解码后保留 TeX。多行独占分隔符的 display 生成 `<div class="math">`；行内节点不加包装。代码 spans/fences/缩进代码仍由 goldmark 负责。美元边界限制开头紧邻非空白、末尾非空白且后方非数字，避免 `$5 and **only** $10` 等金额被识别为数学。

块起止分隔符应独占行；同一 Markdown 段落的行内数学可跨行，但单美元限一行。跨 Markdown 块的行内数学（如下一行以 `+ ` 形成列表）不属于支持语法，应改为独占起止行的 display。GFM 表格的未转义 `|` 仍是列分隔符，公式中请使用 TeX `\mid` 或符合 GFM 的转义写法。未闭合公式回退到普通 Markdown，不承诺原样保留非法数学语法。

持久缓存无需格式版本提升：`cmd/huan/build_cache.go` 使用可执行文件 SHA-256 作为 renderer namespace，新二进制不会复用旧渲染缓存。

## 验证

- TDD：先运行回归，确认正文与 markdownify 的反斜杠、转义、aligned 双反斜杠、HTML 文本和多行块失败，再实现 AST 扩展。追加未闭合块测试先失败，再收紧 closing-line 查找。
- `go test ./internal/markdown ./internal/template ./internal/build ./cmd/huan -count=1` 通过。
- `go test ./...`：35 包、1136 测试通过。
- `go test -race ./internal/markdown ./internal/template`：274 测试通过。
- `git diff --check` 通过。
- 真实源文件：zhurongshuo `content/general/paper/formalization/index.md` 与 `index.en.md`。复制到独立临时 fixture 后，用候选二进制构建，各语言 **94 行内 + 15 块公式** 均在 HTML 解码后与源 TeX 逐字匹配。aligned/cases 原文同时进入永久单元回归。
- 临时 fixture：`/tmp/huan-math-fixture.Y98yph`。仅配置 single 模板，缺少首页/list/search 模板产生警告；两篇目标页完整输出，公式校验全过。此项为源文件→真实 CLI 构建验收，不是浏览器 MathJax 排版验收。

## 候选与后续构建

候选：`/tmp/huan-math-candidate.9xX8b1/huan`。固定 `/tmp/huan-math-fixed` 输出被自动审批以可能覆盖既有二进制为由拒绝，故改用全新临时目录，未覆盖任何安装文件。

```bash
cd /Users/rong.zhu/Code/zhurong/huan
candidate_dir=$(mktemp -d /tmp/huan-math-candidate.XXXXXX)
go build -o "$candidate_dir/huan" ./cmd/huan
```

站点仓库另行修复 MathJax 配置并补齐2.7.8运行资源。完整双语站点构建通过；原1313中文链接实测109处公式、15处display，错误及资源404均为0，390px视口无横向页面溢出。安装目标 `/Users/rong.zhu/go/bin/huan` 已经用户明确授权；旧二进制备份在 `/tmp/huan-math-preview.KE9CKn/huan-original`。构建及dev加载现有插件未出现兼容性警告，未重编或安装插件。站点侧报告为 `../zhurongshuo/developer/reports/2026-10-27-formalization-math-rendering.md`。
