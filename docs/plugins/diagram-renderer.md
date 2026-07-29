# diagram-renderer 插件

在**构建时**把内容里的图表围栏代码块（Mermaid / PlantUML / GraphViz / D2）渲染为内联 SVG，
写回到输出 HTML。渲染由自托管的 [Kroki](https://kroki.io/) 完成，huan 只做提取、请求、SVG 清洗与替换。

## 它做什么

- 扫描已生成的输出 HTML，识别 chroma 高亮后的图表代码块（语言在 `languages` 允许列表内）。
- 取出原始图表源码，按内容哈希查本地缓存；未命中则 POST 给 Kroki 渲染成 SVG。
- 清洗 SVG 后包裹成 `<figure class="diagram diagram-<lang>"><svg …></figure>` 写回页面。
- 命中缓存的图表不再请求 Kroki；对同一页面重复运行是幂等的。
- **降级安全**：Kroki 不可用时按 `fallback.mode` 处理，构建**永不中止**（见下）。

## 打包形态：根模块插件（root-module plugin）

diagram-renderer 与其它插件不同：它**没有独立的 `go.mod`**，作为 huan 主模块的一部分存在，
以便其黄金测试可以 `import internal/*`。它由**同一个** `scripts/build-plugins.sh` 构建，
构建方式与自带 `go.mod` 的插件完全一致（`cd dir && go build -buildmode=plugin`），无需任何额外步骤。

## 配置（huan.yaml）

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

字段说明：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 开关插件 |
| `kroki_url` | `http://localhost:8000` | 自托管 Kroki 端点 |
| `languages` | `[mermaid, plantuml, graphviz, d2]` | 允许渲染的图表语言 |
| `cache_dir` | `.huan/cache/diagrams` | SVG 缓存目录 |
| `timeout_ms` | `5000` | 单次 Kroki 请求超时（毫秒） |
| `fallback.mode` | `client` | 渲染失败时的降级策略：`client` / `codeblock` / `fail` |
| `fallback.mermaid_js` | jsDelivr mermaid@11 | `client` 模式注入的 mermaid.js URL |
| `figure_class` | `diagram` | 输出 `<figure>` 的基础 class |
| `include_kinds` | `[]` | 仅对这些 page kind 渲染（复用 html-injector 过滤语义） |
| `exclude_kinds` | `[]` | 跳过这些 page kind |

## 启动 Kroki

本仓库自带一份 Docker 编排（`kroki` + `kroki-mermaid`，独立网络 `huan_net`）：

```bash
docker compose -f deploy/kroki/docker-compose.yml up -d
```

停止：

```bash
docker compose -f deploy/kroki/docker-compose.yml down
```

## 降级行为（构建永不中止）

当 Kroki 不可达、超时或返回错误时，插件按 `fallback.mode` 处理，**不会让 `huan build` 失败**：

- `client`（默认）——把图表块替换为 `<pre class="mermaid">…</pre>`，并**每页一次**注入一段
  `mermaid.js`（`fallback.mermaid_js`）脚本，交给浏览器端渲染。
- `codeblock` —— 保留为普通代码块，不做任何图表渲染。
- `fail` —— 保留原始块（等价于不改动该块）；首版实现下同样**不 abort** 整个构建。

即 Kroki 停机时重跑构建会得到 `<pre class="mermaid">` + 一段 mermaid.js，页面仍能正常产出。

## Enable

1. Start Kroki: `docker compose -f deploy/kroki/docker-compose.yml up -d`
2. Build the plugin: `bash scripts/build-plugins.sh && cp release/plugins/diagram-renderer.so "$HUAN_HOME"`
3. In huan.yaml:

   ```yaml
   plugins:
     diagram_renderer:
       enabled: true
       kroki_url: "http://localhost:8000"
       languages: [mermaid, plantuml, graphviz, d2]
   ```

4. Write a ` ```mermaid ` fenced block in any content file and run `huan build`.

渲染成功时，输出页面里会出现
`<figure class="diagram diagram-mermaid"><svg …></svg></figure>`。

## 可观测性

插件通过自身的 `logf` 输出结构化日志（含语言、内容哈希、耗时等语义字段）。
这些日志能否成为结构化事件，取决于宿主是否通过 `SetLogf` 把插件的 `logf` 接到 JSON 日志器；
`trace_id` 等全链路字段由宿主 plumbing 提供，不由本插件负责。
