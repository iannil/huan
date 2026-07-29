# diagram-renderer 插件设计（Mermaid 等可视化图形的构建期渲染）

- 日期：2026-07-29
- 状态：已确认，待实现
- 关联：[html-injector 设计](2026-07-24-html-injector-design.md)、[seo-injector 设计](2026-07-24-seo-injector-design.md)、[hook 契约拆分](2026-07-28-hook-contract-split-design.md)、[ADR 0014 .so build-hook 契约]

## 1. 背景与目标

huan 是纯 Go 的静态站点引擎，markdown 由 goldmark 渲染，围栏代码块由
`internal/markdown/renderer.go` 中的自定义 `chromaCodeBlockRenderer`（优先级 200）
统一接管做语法高亮。当前 ` ```mermaid ` 会落入 chroma 的 `guessSyntax` 分支被
误当作某种语言高亮，无法得到图形。

参照站点 `../zhurongshuo`（Hugo）当前**未**使用 Mermaid，因此本特性是净新增、
面向未来的能力，不属于「等价性」门禁要求。

**目标**：让 markdown 中的图表围栏（Mermaid、PlantUML、GraphViz、D2 …）在
**构建期**渲染成静态内联 SVG 写入 HTML，从而对「肉眼 / SEO / AI 三维度」都直接可见、
无需浏览器 JS、无布局抖动。

## 2. 已锁定的决策

| 维度 | 决策 | 理由 |
|------|------|------|
| 渲染阶段 | **构建期渲染成内联 SVG** | 爬虫 / AI 直接可读，无 JS 依赖、无布局抖动，最契合三维度目标 |
| 渲染后端 | **自托管 Kroki（Docker）** | 一个服务覆盖 Mermaid/PlantUML/GraphViz/D2… 契合「Mermaid 等」，且符合 Docker 约定，Go 侧零 JS 依赖 |
| 失败降级 | **降级客户端渲染 + 警告** | Kroki 不可达时保留占位并注入 mermaid.js，构建不中断，鲁棒性最好 |
| 打包形态 | **可配置插件**，允许列表在插件配置里改 | 契合仓库主流的后置插件范式，配置单一来源 |
| 集成架构 | **纯后置插件（方案 A）**，不改 `internal/markdown` | 完全「插件化」，零核心耦合，直接套用 `OnOutputWritten` 范式 |

### 架构取舍：方案 A vs 方案 B（已选 A）

- **方案 A（已选）**：纯 `PostBuildHook` 插件。goldmark/chroma 照常输出高亮块，
  插件在构建产物 HTML 里识别图表块、无损取回源码、调 Kroki、内联 SVG。
  零核心改动，配置集中。唯一风险是耦合 chroma 输出形状——用黄金输出单测锁定。
- **方案 B（未选）**：在 `internal/markdown` 编译进拦截器，对允许语言跳过 chroma、
  产出干净占位符（base64 源码），插件只做渲染。源码交接更干净，但改动核心、
  允许列表分散在两处，违背「配置单一来源」。

## 3. 组件与目录

沿用现有插件结构（对照 `plugins/seo-injector/`）：

```
plugins/diagram-renderer/
  plugin_main.go      # InitPlugin(cfg map[string]any) 导出符号 + func main(){}
  plugin.go           # DiagramRenderer：New / Name / PluginMetadata / ConfigSchema / OnOutputWritten
  config.go           # Config + ParseConfig + DefaultConfig + ConfigSchema
  extract.go          # 从 chroma <div class="highlight"> 块无损取回源码
  kroki.go            # Kroki HTTP 客户端（POST 源码 → SVG）
  cache.go            # 内容哈希 SVG 缓存读写
  fallback.go         # 降级：保留 <pre class="mermaid"> + 每页一次注入 mermaid.js
  plugin_test.go
  extract_test.go
  kroki_test.go
  cache_test.go
```

单一职责边界：
- `extract` —— 输入一段高亮块 HTML，输出原始源码字符串；不涉网络、不涉配置。
- `kroki` —— 输入 `(lang, source)`，输出 SVG 或 error；只管网络。
- `cache` —— 输入内容哈希，读/写磁盘 SVG；只管落盘。
- `fallback` —— 输入命中块与配置，输出降级后 HTML；只管降级。
- `plugin.go` —— 编排以上，实现 `PostBuildHook`。

## 4. 配置（huan.yaml，全部集中在插件里）

```yaml
plugins:
  diagram_renderer:
    enabled: true
    kroki_url: "http://localhost:8000"            # 自托管 Kroki 端点
    languages: [mermaid, plantuml, graphviz, d2]  # 允许列表，改这里即扩展
    cache_dir: ".huan/cache/diagrams"
    timeout_ms: 5000
    fallback:
      mode: client                                # client | codeblock | fail
      mermaid_js: "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js"
    figure_class: "diagram"                        # 输出 <figure> 的基础 class
    include_kinds: []                              # 复用 html-injector 的 kind 过滤语义
    exclude_kinds: []
```

`Config` 结构体字段与上一一对应；`ConfigSchema()` 为每个字段声明
（`string` / `bool` / `int` / `string_slice`），接入现有插件校验链路。
`DefaultConfig()`：`enabled=false`、`languages=[mermaid,plantuml,graphviz,d2]`、
`kroki_url=http://localhost:8000`、`cache_dir=.huan/cache/diagrams`、
`timeout_ms=5000`、`fallback.mode=client`、`figure_class=diagram`。

## 5. 核心流程 `OnOutputWritten(ctx, outputDir)`

1. 若 `!enabled` 或 `languages` 为空 → 早退（对齐 html-injector）。
2. `collectHTMLFiles(outputDir)` —— 复用 html-injector 里已修好的递归 `WalkDir`
   （Go 的 `Glob("**")` 非递归，此处必须递归）。
3. 逐 HTML 文件：正则匹配 `<div class="highlight">…<code … data-lang="X">…</code>…</div>`，
   其中 `X ∈ languages`。
4. 逐命中块：
   1. `extract` 无损取回源码。
   2. `key = sha256(lang + "\n" + source)`；查 `cache_dir/<key>.svg`。
   3. 命中 → 用缓存 SVG；未命中 → `kroki.Render(ctx, lang, source)`，成功则写缓存。
   4. 成功 → 用 `<figure class="diagram diagram-<lang>" role="img">…SVG…</figure>`
      替换**整个** `<div class="highlight">` 块。
   5. 失败 → 记入本页「需降级」列表。
5. 若本页有降级项 → 按 `fallback.mode` 处理；循环中检查 `ctx.Done()` 安全退出。
6. 仅当内容变化才 `os.WriteFile(path, …, 0644)`（幂等，对齐现有插件）。

**并发**：首版按现有插件的串行 `for` 实现，优先保证正确与幂等；内容哈希缓存已
消除重复图表的网络开销。文件级并发列入后续优化（YAGNI）。

## 6. 源码无损取回（`extract.go`）

chroma 对未知 lexer（如 mermaid）不增删字符，只把字符包进
`<span class="line"><span class="cl">…</span></span>`。取回步骤：

1. 截取 `<code …>` 与 `</code>` 之间内容。
2. 删除所有 `<span…>` / `</span>` 标签。
3. `html.UnescapeString`，还原 `&lt; &gt; &amp; &#34; &#39;` 等
   （覆盖 mermaid 的 `-->`、引号、`&` 等）。

**黄金输出单测**（`extract_test.go`）：固定 mermaid 源码经真实 goldmark+chroma 渲染后，
断言 `extract` **逐字节**还原原始源码。chroma 升级若改变输出结构，此测试立刻变红。
用例覆盖：含 `-->`、双引号、单引号、`&`、中文、多行缩进的图源码。

## 7. Kroki 客户端（`kroki.go`）

- 请求：`POST {kroki_url}/{lang}/svg`，body 为原始源码，
  `Content-Type: text/plain`，`Accept: image/svg+xml`
  （Kroki 简单 POST 接口，免去 deflate+base64 URL 编码）。
- 带 `context.Context` 与 `timeout_ms`；非 2xx 或超时 → 返回 error（触发降级，绝不 panic）。
- SVG 最小清洗：确保根 `<svg>` 带 `class="kroki"`；剥掉 `<?xml …?>` / `<!DOCTYPE …>`
  前缀使其可安全内联进 HTML。

## 8. 降级（`fallback.go`）

- `mode: client`（默认）：命中块替换为 `<pre class="mermaid">…源码…</pre>`；
  若本页存在 mermaid 降级项，则**每页一次**在 `</body>` 前注入
  `<script src="{mermaid_js}"></script>` + `<script>mermaid.initialize({startOnLoad:true})</script>`。
  日志 `WARN`。构建不中断。
  - 约束：仅 `mermaid` 能客户端降级（mermaid.js 只认 mermaid）；
    `plantuml/graphviz/d2` 在 client 模式下自动退化为 `codeblock`（保留高亮代码块）并单独告警。
- `mode: codeblock`：保留原 chroma 高亮块，不注入任何 JS。
- `mode: fail`：打印醒目错误并返回 error。注意 `PostBuildHook` 语义为
  「收集告警、不中断构建」，故首版 fail 模式仍不 abort 整个构建；
  「硬失败退出码」列为后续可选增强。

## 9. 基础设施（Docker，符合独立网络约定）

提供独立网络的 compose 片段（置于 `deploy/` 或本文件同级文档）：

```yaml
services:
  kroki:
    image: yuzutech/kroki
    ports: ["8000:8000"]
    networks: [huan_net]
    depends_on: [kroki-mermaid]
  kroki-mermaid:
    image: yuzutech/kroki-mermaid
    expose: ["8002"]
    networks: [huan_net]
networks:
  huan_net:
    name: huan_net
```

文档需说明：未启动 Kroki 时，构建自动走客户端降级（`fallback.mode=client`），
不阻塞开发；生产发布前应确保 Kroki 就绪，使产物为静态 SVG。

## 10. 测试与可观测性

- `extract_test.go`：黄金输出、含 `-->` / 引号 / `&` / 中文 / 多行缩进用例。
- `kroki_test.go`：`httptest` 假 Kroki，覆盖 2xx、超时、5xx。
- `cache_test.go`：命中 / 未命中、哈希稳定性、缓存文件读写。
- `plugin_test.go`：端到端——假 Kroki + 临时 outputDir，断言 `<figure>` 替换、
  降级注入、`include/exclude_kinds` 过滤、**幂等**（二次运行产物无变化）。
- 结构化日志（对齐 CLAUDE.md 可观测性约定）：每图输出 JSON 日志，
  `event_type ∈ {diagram_render_start, diagram_render_end, diagram_fallback, diagram_cache_hit}`，
  含 `trace_id`、`lang`、`hash`、`duration`；日志代码与业务逻辑解耦。

## 11. 明确不做（YAGNI）

- 不做客户端 / 构建期「二选一」总开关（已定构建期为主、失败才降级）。
- 不做 markdown 层拦截（方案 A 的核心取舍）。
- 首版不做文件级并发渲染、不做 SVG 暗色 / 主题适配、不做图内文字 i18n、
  不做 fail 模式硬退出码。以上均列为后续可选增强。

## 12. 验收标准

1. `plugins.diagram_renderer.enabled=true` 且 Kroki 就绪时，含 ` ```mermaid ` 的页面
   产物中出现内联 `<figure class="diagram diagram-mermaid"><svg …>`，且无残留
   `<div class="highlight">` 高亮块。
2. 相同图源码二次构建命中缓存，无 Kroki 网络请求。
3. Kroki 不可达时，构建成功完成，页面保留 `<pre class="mermaid">` 且注入一次 mermaid.js，
   日志含 `WARN`。
4. `languages` 增删条目即可开关对应图表语言，无需改代码。
5. `include_kinds` / `exclude_kinds` 过滤生效；插件重复运行产物幂等。
6. `go test ./plugins/diagram-renderer/...` 全绿，含黄金输出取回测试。
