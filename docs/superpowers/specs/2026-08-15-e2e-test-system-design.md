# huan E2E 测试用例体系设计

- 日期：2026-08-15
- 状态：已批准（brainstorming 产出）
- 前置：无

## 1. 背景与目标

huan 目前有 ~20 个 Go 单元/集成测试文件（httptest + t.TempDir 模式），但**没有任何 E2E 测试体系**：无测试站点 fixture、无跨子系统（daemon + admin SPA + 公开 API + CLI）的验收流程、无面向后续 agent-browser 自动化验收的用例资产。

本设计建立一套**面向 agent 驱动执行**的 E2E 测试用例体系：

- 用例是**结构化 YAML 验收规范**，由 Claude 会话 / agent-browser / curl 直接解析执行，不写传统测试代码
- 覆盖 huan 全功能面（admin API、公开 API、SSE/LiveReload、admin SPA、CLI），按风险分层
- 达到生产级验收密度：所有端点 + 状态转换 + 异常场景 + 边界条件 + 鉴权 + 并发 + 数据一致性

### 原方案的映射调整

原通用方案面向电商系统（登录/session、商品/订单、数据库状态校验）。huan 的实际形态是**文件驱动的静态内容引擎**，映射如下：

| 原方案概念 | huan 映射 |
|---|---|
| 登录 → 操作 → 验证 | 输入 Bearer Token → 进 admin 面板 → 操作 → 验证 |
| 数据库状态校验 | **文件系统即数据库**：直接断言临时站点目录的落盘状态 + 重新 GET 回读 |
| 种子数据 | 3 个固定 fixture 站点 + 用例级 setup 覆盖（不依赖种子，setup/teardown 自包含） |
| 商品/订单等业务对象 | content（Markdown 文件）、media、settings（huan.yaml）、plugins、themes |

## 2. 总体架构（方案 B：分层双 schema）

两类用例、两种 schema，贴合各自执行者：

- **`tests/e2e/api/` + `tests/e2e/cli/`**：严格结构化 schema——steps 是精确的 request/response/filesystem 断言，curl 可复现
- **`tests/e2e/browser/`**：旅程式 schema——goal + 里程碑断言，操作步骤是「意图 + 选择器提示」而非逐次点击脚本，把「怎么点」交给 agent-browser 临场决定，避免选择器漂移导致用例报废

两类用例共享 `fixtures/`（3 个测试站点）和 `runbooks/`（agent 执行手册）。

```
tests/e2e/
├── README.md                    # 入口：体系说明 + 执行指引
├── fixtures/
│   ├── minimal/                 # 最小单语站点
│   ├── multilang/               # zh-cn + en 双语言站点
│   ├── with-plugins/            # 插件站点（编译期插件 + 测试 .so）
│   └── FIXTURES.md              # 每个 fixture 的已知状态表（见 §6）
├── runbooks/
│   ├── RUNBOOK.md               # 主执行手册（环境/启动/判定/报告）
│   └── patterns.md              # 断言模式库（SSE/WS/并发/mtime 等公共套路）
├── api/                         # API 集成用例（严格 schema）
│   ├── _schema.md               # schema 字段字典
│   ├── auth.yaml
│   ├── content-crud.yaml
│   ├── status.yaml
│   ├── media.yaml
│   ├── settings.yaml
│   ├── build-trigger.yaml
│   ├── public-api.yaml
│   ├── sse-events.yaml
│   ├── plugins-admin.yaml
│   └── livereload.yaml
├── browser/                     # 浏览器 E2E 用例（旅程式 schema）
│   ├── _schema.md
│   ├── admin-login.yaml
│   ├── admin-content-crud.yaml
│   ├── admin-settings.yaml
│   ├── admin-plugins.yaml
│   └── site-rendering.yaml
└── cli/                         # CLI 集成用例（复用 api schema 的 cli/fs 动作）
    ├── _schema.md               # 简述：复用 api/_schema.md，仅列差异
    ├── build.yaml
    ├── new.yaml
    ├── config.yaml
    ├── version.yaml
    └── _external-deps.md        # 排除清单（translate/deploy/sync/release 及原因）
```

关键决策：

- **fixture 是共享资产**：三类用例都从 `fixtures/` 取站点，setup 一律「复制 fixture → 临时目录 → 起服务」，不依赖种子数据也不各自拼站点
- **`_schema.md` 就近放置**：agent 读模块用例前先读同目录 schema 说明，不跳出目录
- **runbook 单独成层**：环境启动/判定规则/报告格式是跨用例公共知识，不重复写进用例

## 3. API 用例 Schema（严格结构化）

每个 YAML 文件是一个**模块级套件**：

```yaml
suite: content-crud          # 套件标识（= 文件名）
module: admin-content        # 业务模块（报告归类）
fixture: minimal             # 使用的 fixture 名
runtime: daemon              # daemon | dev | build-only（套件级默认，用例可覆盖）
startup:                     # 可选：起服务前的额外准备
  - action: fs.write
    path: content/posts/extra.md
    content: |
      ---
      title: 额外文章
      ---
tests:
  - id: crud-001
    name: 创建文章并验证落盘
    severity: P0             # P0=发布阻断 / P1=核心 / P2=边界
    preconditions: []        # 前置用例 id
    steps: [...]             # 动作序列
    cleanup: []              # 临时目录整体销毁；仅跨用例状态需显式回滚
```

### 七种 step 动作类型

| 类型 | 关键字段 | 用途 |
|---|---|---|
| `http` | method/path/headers/body/expect(status/body/files) | API 请求 + 响应断言 |
| `fs.assert` | path/exists、contains、matches(正则) | 文件系统落盘断言（文件系统即数据库） |
| `fs.write` / `fs.delete` | path/content | 模拟外部变更（如直接改文件触发 watcher） |
| `cli` | args/expect(exitCode/stdout) | 跑 huan 子命令 |
| `sse.subscribe` | events/maxWait | SSE 流监听 + 事件断言 |
| `wait` | for(file-change\|port\|seconds) | 等待异步（构建完成、端口就绪） |
| `var` | set/get | 用例内变量（如 `${resp.body.slug}` 供后续 step 引用） |

### 两条硬规则（保证 LLM 可靠执行）

1. 每个 `http` step 的 `expect` 必须显式声明 status 和关键 body 字段——不允许「200 即过」，防止静默失效（huan 历史教训：插件钩子静默失效、draft 泄露到公开 API 都是「成功但错」）
2. 每个**写操作**用例必须在响应断言后跟一个 `fs.assert`——API 返回成功 ≠ 落盘成功

### 鉴权注入

`${token}` 是 runbook 层保留变量：daemon/dev 在 loopbind 下启动时 token 打印到 stderr，runbook 规定从启动日志捕获，用例只引用不负责获取。

### 示例（粒度基准）

```yaml
steps:
  - action: http
    method: POST
    path: /admin/api/content
    headers: { Authorization: "Bearer ${token}" }
    body: { section: posts, filename: crud-001.md, title: "CRUD测试文章" }
    expect:
      status: 201
      body: { title: "CRUD测试文章" }
  - action: fs.assert
    path: content/posts/crud-001.md
    contains: "title: CRUD测试文章"
  - action: http
    method: GET
    path: /admin/api/content/posts/crud-001.md
    headers: { Authorization: "Bearer ${token}" }
    expect: { status: 200, body: { frontmatter: { title: "CRUD测试文章" } } }
```

## 4. API 套件清单与覆盖矩阵（10 套件，~78 用例）

端点全集已对照 `internal/admin/api.go`、`internal/daemon/contentindex/handler.go`、`internal/daemon/serving.go` 核实。

| 套件 | 端点/对象 | 用例数 | 覆盖维度 |
|---|---|---|---|
| `auth.yaml` | TokenMiddleware | 6 | 正确 token；缺失/错误→401；`X-Huan-Admin-Token` 头等价；loopback 自动 token；非 loopback bind 无环境变量→拒绝启动 |
| `content-crud.yaml` | content 全 CRUD + languages | 14 | 全 CRUD + 落盘；缺 title/filename→400；非法 JSON→400；不存在路径→404；重复创建；draft 流转；languages 查询（multilang）；**审计断言**（写后 `memory/daily/*.md` 有记录含 SHA）；并发×2（同文件并发 PUT 最后写赢 + 审计两条；创建/删除竞争终态一致） |
| `status.yaml` | GET status | 4 | 初始统计=fixture 已知值；创建后增量；sectionBreakdown/recentContent 排序；mediaCount |
| `media.yaml` | media 3 端点 | 8 | 上传→static/ 落盘 + 列表出现；删除→消失；删不存在→404；空文件；二进制安全（PNG 魔数） |
| `settings.yaml` | settings 4 端点 | 9 | 结构化读写回读；原始 YAML 读写回读；非法 YAML→400；写后 huan.yaml 落盘 diff；settings 与 settings/yaml 交叉一致性 |
| `build-trigger.yaml` | POST build + JIT | 7 | 触发→200；publishDir 出现新页 HTML；增量：改 1 篇只该页变更（mtime 对比）；draft 默认不进公开 API（历史安全 bug 回归）；模板变更→全量回退 |
| `public-api.yaml` | /api/v1/* | 12 | 分页（默认 10/上限 50/超界 clamp）；section/tag/q 过滤；q 超 200 **rune**→400（中文回归）；page=MaxInt64 溢出回归（历史 DoS）；不存在 URL→404；非 GET→405 |
| `sse-events.yaml` | /api/v1/events | 5 | 订阅收初始/心跳；build→build 事件；内容写→content 事件；插件 load→plugin 事件；Content-Type: text/event-stream |
| `plugins-admin.yaml` | plugins 4 + theme 3 端点 | 9 | list（with-plugins）；load .so；unload→消失；reload；load 不存在路径→错误；theme list/activate/deactivate |
| `livereload.yaml` | /livereload（dev） | 4 | WS 握手 + hello；文件变更→reload 广播；构建错误→alert；/livereload.js 可取 |

**异常与边界原则**：每个端点至少 1 正常 + 1 异常（4xx/5xx）+ 1 边界用例；所有错误响应体断言 `{"error": "..."}` 结构（`internal/admin/types.go` APIError 契约）。并发不设单独套件，嵌入相关套件（见 content-crud）。

## 5. 浏览器 E2E 用例（旅程式 Schema）

### Schema 差异

browser 用例锁定**意图和里程碑**，不锁点击路径：

```yaml
suite: admin-content-crud
module: admin-spa
fixture: minimal
runtime: daemon
journey:
  persona: 内容编辑
  goal: 在后台完成一篇文章的新建、编辑、删除
browser_defaults:
  viewport: 1280x800
  url: http://127.0.0.1:${port}/admin
tests:
  - id: bc-001
    name: 登录后台并进入内容列表
    severity: P0
    steps:
      - action: goto
        target: /admin
      - action: enter-token        # patterns 定义：从启动日志取 ${token} 填入
      - action: verify
        milestone: 内容列表可见
        expect:
          - "页面出现文章标题「${fixture.post1.title}」"
          - "出现内容树，含 posts 节点"
        hints:                      # 选择器只是提示，不是契约
          - "列表项通常为 .content-item 或 table tr"
      - action: screenshot
        name: bc-001-milestone-1
```

### 七种 browser 动作

`goto` / `enter-token` / `interact`（意图式：「点击新建按钮」「在标题输入框填 ${var}」）/ `verify`（里程碑断言）/ `screenshot` / `wait`（reload、URL 变化、元素出现）/ `api-probe`（旅程中穿插 API 调用做旁证，复用 API schema 全部断言语法——浏览器为壳、API 为锚）。

### 可靠性规则

1. `verify` 的 expect 用**用户可见内容**表述（文本、状态），DOM 选择器只作 `hints`——SPA 改版时 hints 失效不判负
2. 每个 `verify` 失败必须先截图再判负（便于人审）
3. `api-probe` 让浏览器旅程获得与 API 用例同等的断言精度

### 用例清单（5 文件，~22 旅程）

| 文件 | 旅程 | 数量 |
|---|---|---|
| `admin-login.yaml` | 首次进入→输 token→进面板；错误 token→提示+停留；刷新保持会话（sessionStorage）；401 过期→自动弹 token 框 | 4 |
| `admin-content-crud.yaml` | 列表浏览（树+统计）；新建→列表出现→落盘；编辑 frontmatter+正文→保存→回读；删除→列表消失→文件消失；draft 开关；多语言 siblings | 6 |
| `admin-settings.yaml` | 读设置页字段回显；改标题→保存→API 回读一致；原始 YAML 编辑→保存→huan.yaml diff；非法值→前端报错 | 4 |
| `admin-plugins.yaml` | 插件列表加载；load .so→状态变化；unload/reload | 3 |
| `site-rendering.yaml` | 前台站点（dev）：首页渲染；文章页（标题/正文/日期）；404 页；tag 页；LiveReload：开两页→编辑文件→两页均刷新（截图前后对比） | 5 |

## 6. CLI 集成用例（tests/e2e/cli/）

CLI 复用 API schema 的 `cli` + `fs.assert` 动作，不引入新 schema。范围收敛到**无外部依赖**的子命令：

| 文件 | 覆盖 | 用例数 |
|---|---|---|
| `build.yaml` | `huan build`：正常构建→publishDir 产物断言（HTML/index/sitemap/api JSON）；`-D` 含 draft/默认不含；`-F`/`-E`；增量 vs 全量；minify 开关；缺 huan.yaml→非零退出码；多语言构建（multilang） | 8 |
| `new.yaml` | `huan new`：创建→落盘+frontmatter 结构；非法 section；重名 | 3 |
| `config.yaml` | `huan config`：合法通过；非法 YAML 报错；`${VAR:-default}` 插值 | 3 |
| `version.yaml` | `huan version`：格式断言 | 1 |
| `_external-deps.md` | **排除清单**：translate（需 AI 插件）、deploy（需 Cloudflare 凭据）、sync、release——显式记录原因，避免后人误当遗漏 | — |

## 7. Fixture 设计

| fixture | 内容 | 服务的模块 |
|---|---|---|
| `minimal` | huan.yaml（单语 zh-cn、paginate 10）+ content/posts/ 2 篇（1 正式 1 draft，带 tags）+ content/about.md + static/logo.png + 最小内嵌主题 | auth/content/status/media/settings/build/public-api/多数 browser |
| `multilang` | languages: zh-cn + en，每语言 2 篇 + 1 篇只有中文（en 侧 catalogSections 占位） | languages 端点、多语言 build、公开 API 语言过滤 |
| `with-plugins` | 声明 seo-injector + sitemap-enhancer（编译期，无外部依赖）+ plugins/ 含 1 个测试 .so（复用 `internal/plugin/testdata/simple_plugin`） | plugins-admin、build 插件钩子产物断言（meta 注入）、browser 插件页 |

每个 fixture 在 `FIXTURES.md` 维护一张**已知状态表**：文件清单、status 统计精确值（total/drafts/sections/languages）、公开 API 期望条数——用例断言直接引用这些常量，fixture 变更只改一处。

## 8. Runbook（agent 执行手册）

`runbooks/RUNBOOK.md` 规定：

1. **环境准备**：`go build -o /tmp/huan-e2e/huan ./cmd/huan`（固定二进制路径）；复制 fixture 到 `/tmp/huan-e2e/sites/<run-id>/`，run-id 含时间戳保证隔离
2. **服务启动与 token 捕获**：`huan daemon --source <tmp> --port <空闲端口>`；从 stderr 解析 token 存 `${token}`；`/health` 探测端口就绪后才开始用例。dev 模式同理
3. **判定规则**：P0 失败→套件判负并停止该套件；P1 失败→记录继续；截图归档 `/tmp/huan-e2e/artifacts/<run-id>/`
4. **报告产出**：写入 `docs/reports/` 验收报告（对接项目文档规范）：每用例一行（id/结果/耗时/证据链接）+ 汇总（通过率、P0 状态、bug 列表）
5. **patterns.md**：SSE 监听（curl -N + timeout）、WS 握手、并发 PUT 循环、mtime 对比法、审计日志断言位置（测试站点在 /tmp 下时审计路径解析）

## 9. 使用时机与验收流程

主要时机：**每轮开发完成后的验收 + 回归**（不接 CI——YAML 用例由 agent 驱动，CI 化需额外 runner，当前阶段成本不值）。

流程：开发完成 → 执行受影响模块的套件（如改了 admin API 就跑 api/*.yaml 相关套件）→ 结果记入 docs/reports/ → 发现 bug 记入报告 bug 列表 → 全部 P0 通过视为验收通过。

## 10. 明确不做（YAGNI）

- 不写 Go/Playwright 测试代码或 YAML runner——用例由 agent 会话执行
- 不接 CI 自动化
- 不测有外部依赖的子命令（translate/deploy/sync/release）
- 不做性能/压测
- 不做浏览器视觉回归比对（LiveReload 截图对比仅限「内容出现/消失」级别的粗对比）
