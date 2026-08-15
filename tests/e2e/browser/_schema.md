# Browser 套件 Schema 参考文档

本文档定义所有 Browser 套件 YAML 的字段字典与断言语义。执行 agent 写/读 YAML 用例前必须先读本 Schema；后续 5 个 Browser 套件全部按此 Schema 编写。

**配套文档**：
- 变量契约（`${token}`/`${port}`/`${base}`/`${site}` 等）见 [RUNBOOK.md §1](../runbooks/RUNBOOK.md#1-变量契约保留变量表)
- 判定规则（P0/P1/P2）见 [RUNBOOK.md §5](../runbooks/RUNBOOK.md#5-判定规则severity-语义)
- 端口分配表见 [RUNBOOK.md §3](../runbooks/RUNBOOK.md#3-端口分配表)
- Fixture 已知状态常量见 [FIXTURES.md](../fixtures/FIXTURES.md)
- api-probe 的 expect 语法见 [api/_schema.md §1](../api/_schema.md#1-http---http-请求与断言)

**核心理念**：Browser 用例是**旅程式（Journey-based）**——锁定「意图 + 里程碑」而非点击路径，把「怎么点」交给 agent-browser 临场决定，避免选择器漂移导致用例报废。

---

## 套件骨架 YAML 结构

```yaml
suite: admin-login                      # 套件名（对应 RUNBOOK §3 端口表）
module: admin-spa                       # 模块名（测试覆盖的功能模块）
fixture: minimal                        # fixture 名（minimal/multilang/with-plugins，见 FIXTURES.md）
runtime: dev                            # 运行时：dev/daemon（见 RUNBOOK §4.3 挂载差异）
journey:                                # 旅程定义（必填）
  persona: 内容编辑                     # 角色：谁在执行此旅程
  goal: 在后台完成一篇文章的新建、编辑、删除   # 目标：要达成什么
browser_defaults:                       # 浏览器默认配置（必填）
  viewport: 1280x800                    # 视口大小（固定值，模拟桌面浏览器）
  url: http://127.0.0.1:${port}/admin  # 起始 URL
tests:                                  # 用例列表
  - id: bc-001                         # 用例 ID（格式：<套件前缀>-序号）
    name: 登录后台并进入内容列表         # 用例描述（中文）
    severity: P0                       # 严重程度：P0/P1/P2（见 RUNBOOK §5 判定规则）
    steps:                             # 步骤列表（必填）
      - action: goto
        target: /admin
      - action: enter-token
      - action: verify
        milestone: 内容列表可见
        expect:
          - "页面出现文章标题「${fixture.minimal.post1.title}」"
          - "出现内容树，含 posts 节点"
        hints:
          - "列表项通常为 .content-item 或 table tr"
      - action: screenshot
        name: bc-001-milestone-1
      - action: interact
        intent: 点击新建按钮
        hints:
          - "按钮可能在右上角或工具栏"
      - action: wait
        for: element-visible
        hints:
          - "编辑表单或模态框出现"
      - action: api-probe
        method: GET
        path: /admin/api/status
        expect:
          status: 200
          body: { total: 3 }
```

### 套件级字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `suite` | string | 是 | 套件名，对应 RUNBOOK §3 端口表 browser 段（如 admin-login/admin-content-crud/admin-settings/admin-plugins/site-rendering） |
| `module` | string | 是 | 模块名，描述测试覆盖的功能模块（如 admin-spa） |
| `fixture` | string | 是 | fixture 名：minimal/multilang/with-plugins（见 FIXTURES.md） |
| `runtime` | string | 是 | 运行时：dev/daemon（见 RUNBOOK §4.3 挂载差异） |
| `journey` | object | 是 | 旅程定义（见下表） |
| `browser_defaults` | object | 是 | 浏览器默认配置（见下表） |
| `tests` | array | 是 | 用例列表 |

### journey 字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `persona` | string | 是 | 角色：谁在执行此旅程（如「内容编辑」、「站点访客」、「管理员」） |
| `goal` | string | 是 | 目标：要达成什么（如「在后台完成一篇文章的新建、编辑、删除」） |

### browser_defaults 字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `viewport` | string | 是 | 视口大小，固定值 `1280x800`（模拟桌面浏览器） |
| `url` | string | 是 | 起始 URL，如 `http://127.0.0.1:${port}/admin` |

### 用例级字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 用例 ID，格式 `<套件前缀>-<序号>`（如 bc-001），同套件内唯一 |
| `name` | string | 是 | 用例描述，中文 |
| `severity` | string | 是 | 严重程度：P0/P1/P2（见 RUNBOOK §5 判定规则） |
| `steps` | array | 是 | 步骤列表（核心执行逻辑） |

---

## Step 动作类型（7 个动词）

### 1. goto - 导航到 URL

导航到指定 URL（相对路径或完整 URL）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `goto` |
| `target` | string | 是 | 目标 URL（相对路径如 `/admin` 或完整 URL） |

**最小示例**：

```yaml
- action: goto
  target: /admin
```

**注意事项**：

- 相对路径基于 `browser_defaults.url`
- 完整 URL 可用于跨站导航（如测试前台站点）

---

### 2. enter-token - 输入认证 Token

在后台的 Token 输入框填入 `${token}`（对应 RUNBOOK §2.2 固定 token）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `enter-token` |

**最小示例**：

```yaml
- action: enter-token
```

**执行语义**：

- 从 `${token}` 变量获取 token 值（固定为 `e2e-fixed-token`）
- 定位页面上的 Token 输入框（通常为 `/admin` 首页）
- 填入 token 并提交/确认
- 等待后续页面加载完成

**注意事项**：

- 无需显式指定输入框位置——agent-browser 临场决定
- token 值见 [RUNBOOK.md §2.2](../runbooks/RUNBOOK.md#22-admin-token固定值优先)

---

### 3. interact - 交互（意图式）

执行用户交互操作（点击、输入、选择等），用**自然语言描述意图**而非精确点击路径。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `interact` |
| `intent` | string | 是 | 意图描述（自然语言），如「点击新建按钮」、「在标题输入框填 ${var}」 |
| `hints` | array | 否 | DOM 选择器提示（数组，可选），用于辅助定位 |

**最小示例**：

```yaml
- action: interact
  intent: 点击新建按钮
  hints:
    - "按钮可能在右上角或工具栏"
    - "可能含 '新建'、'New'、'Create' 文本"
```

**意图类型示例**：

| 意图类型 | 示例 |
|---|---|
| 点击按钮 | 「点击新建按钮」、「点击保存按钮」、「点击删除按钮」 |
| 输入文本 | 「在标题输入框填 测试文章」、「在正文框填 多行内容」 |
| 选择选项 | 「从下拉菜单选择 '草稿' 状态」、「勾选 '发布' 选项」 |
| 切换开关 | 「打开多语言开关」、「切换到编辑模式」 |

**注意事项**：

- `intent` 是**用户意图的高层描述**，不包含具体 DOM 结构
- `hints` 是**提示非契约**——SPA 改版时 hints 失效不判负，用自然语言重新定位
- 变量替换：`intent` 中可用 `${var}` 引用变量（如 `${fixture.minimal.post1.title}`）

---

### 4. verify - 验证里程碑

验证页面状态是否达到预期里程碑——用**用户可见内容**表述（文本/状态），DOM 选择器只作 hints。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `verify` |
| `milestone` | string | 是 | 里程碑名称（自然语言），如「内容列表可见」、「文章详情页加载完成」 |
| `expect` | array | 是 | 期待状态列表（自然语言描述），每项是用户可见的内容/状态 |
| `hints` | array | 否 | DOM 选择器提示（数组，可选），用于辅助定位 |

**最小示例**：

```yaml
- action: verify
  milestone: 内容列表可见
  expect:
    - "页面出现文章标题「${fixture.minimal.post1.title}」"
    - "出现内容树，含 posts 节点"
    - "统计信息显示总数 3"
  hints:
    - "列表项通常为 .content-item 或 table tr"
    - "内容树可能在左侧边栏"
```

**expect 断言语义**：

| 断言类型 | 示例 |
|---|---|
| 文本存在 | 「页面出现文章标题『你好 huan』」、「出现 '保存成功' 提示」 |
| 元素可见 | 「出现内容树，含 posts 节点」、「编辑表单可见」 |
| 状态匹配 | 「统计信息显示总数 3」、「URL 变为 /admin/content/posts」 |
| 数值匹配 | 「文章列表显示 3 篇」、「标签数量为 2」 |

**可靠性规则**：

1. **用户可见内容优先**：`expect` 用自然语言描述用户可见的内容/状态（文本、UI 状态、URL 变化），不依赖 DOM 结构
2. **hints 是提示非契约**：`hints` 中的 DOM 选择器只是辅助定位的提示，SPA 改版时 hints 失效不判负，用自然语言重新定位
3. **verify 失败先截图**：每个 `verify` 失败必须先截图（screenshot 动作）再判负（见 RUNBOOK §5 判定规则）

**注意事项**：

- `expect` 是**用户视角的可见状态**，不是 DOM 结构断言
- 变量替换：`expect` 中可用 `${var}` 引用变量（如 `${fixture.minimal.post1.title}`）
- 里程碑是旅程的关键节点，命名要清晰反映达成的状态

---

### 5. screenshot - 截图

在里程碑处或失败时截图，便于人工审查。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `screenshot` |
| `name` | string | 是 | 截图文件名（不含扩展名，自动加 `.png`） |

**命名规范**：

| 场景 | 命名格式 | 示例 |
|---|---|---|
| 里程碑截图 | `<case-id>-milestone-<n>` | `bc-001-milestone-1`、`bc-001-milestone-2` |
| 失败截图 | `<case-id>-fail` | `bc-001-fail` |

**最小示例**：

```yaml
- action: screenshot
  name: bc-001-milestone-1
```

**注意事项**：

- 截图自动保存到 `${artifacts}` 目录（见 RUNBOOK §1）
- 失败截图由执行 agent 自动触发（verify 失败时）
- 里程碑截图由用例显式声明（关键节点必截）

---

### 6. wait - 等待条件

等待某个条件成立（用于异步操作后，如页面导航、元素出现、URL 变化）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `wait` |
| `for` | string | 是 | 等待条件类型：`reload`/`url-change`/`element-visible` |
| `hints` | array | 否 | DOM 选择器提示（`for: element-visible` 时必填） |

**wait 条件类型说明**：

| 条件类型 | 必填字段 | 说明 |
|---|---|---|
| `reload` | - | 等待页面重新加载完成（如点击链接后） |
| `url-change` | - | 等待 URL 变化（如导航后） |
| `element-visible` | `hints` | 等待指定元素可见（如异步加载的内容） |

**最小示例**：

```yaml
# 等待页面重新加载
- action: wait
  for: reload

# 等待 URL 变化
- action: wait
  for: url-change

# 等待元素可见
- action: wait
  for: element-visible
  hints:
    - "编辑表单或模态框"
    - "可能含 .editor 或 .modal 类"
```

**注意事项**：

- `for: element-visible` 时必须提供 `hints` 辅助定位
- 超时时间由执行 agent 定义（建议 10s）
- 用于异步操作后的同步（如 AJAX 加载、页面导航）

---

### 7. api-probe - API 调用旁证

在浏览器旅程中穿插 API 调用做旁证——浏览器为壳、API 为锚，复用 api/_schema.md 的 expect 语法。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `api-probe` |
| `method` | string | 是 | HTTP 方法：GET/POST/PUT/DELETE |
| `path` | string | 是 | 请求路径，如 `/admin/api/status` |
| `headers` | object | 否 | 请求头对象，如 `{ Authorization: "Bearer ${token}" }` |
| `body` | string/object | 否 | 请求体（POST/PUT 用） |
| `expect` | object | 是 | 响应断言（**语法见 [api/_schema.md §1](../api/_schema.md#1-http---http-请求与断言)**） |

**最小示例**：

```yaml
- action: api-probe
  method: GET
  path: /admin/api/status
  headers: { Authorization: "Bearer ${token}" }
  expect:
    status: 200
    body: { total: 3 }   # 引用 FIXTURES.md minimal 常量
```

**内联最小示例（免跳转）**：

```yaml
# api-probe 的 expect 语法见 api/_schema.md §1
# 内联最小示例：
- action: api-probe
  method: GET
  path: /admin/api/status
  expect:
    status: 200
    body: { total: 3 }
```

**api-probe 与 api/_schema.md 的关系**：

- **复用语法**：`expect` 字段完全复用 [api/_schema.md §1](../api/_schema.md#1-http---http-请求与断言) 的断言语法
- **body 断言**：点路径（`frontmatter.title`）、数组断言（`bodyContains`）、正则断言（`matches`）全部可用
- **差异**：`api-probe` 用在浏览器旅程中穿插调用，`http` 用在纯 API 套件中

**典型用途**：

| 用途 | 示例 |
|---|---|
| 旁证落盘 | 浏览器操作后调用 GET /admin/api/content 验证文件落盘 |
| 状态验证 | 浏览器操作后调用 GET /admin/api/status 验证统计变化 |
| 数据一致性 | 浏览器操作后调用 API 回读，验证前后台数据一致 |

**注意事项**：

- `expect` 的完整字段见 [api/_schema.md §1](../api/_schema.md#1-http---http-请求与断言)
- 变量替换：可用 `${token}`、`${base}` 等全局变量（见 RUNBOOK §1）
- 固定 token：`Authorization` 头统一用 `${token}`（见 RUNBOOK §2.2）

---

## 变量替换规则

YAML 中的变量按 [RUNBOOK.md §1](../runbooks/RUNBOOK.md#1-变量契约保留变量表) 展开：

| 变量 | 展开值 | 示例 |
|---|---|---|
| `${token}` | admin API token（固定值） | `e2e-fixed-token` |
| `${port}` | 当前套件固定端口 | `13300` |
| `${base}` | `http://127.0.0.1:${port}` | `http://127.0.0.1:13300` |
| `${site}` | fixture 的可写拷贝 | `/tmp/huan-e2e/sites/20260815-142530-minimal` |
| `${fixture.<name>.<field>}` | fixture 字段引用 | `${fixture.minimal.post1.title}` → `你好 huan` |

**fixture 引用语法**：从 FIXTURES.md 引用已知常量，如：

- `${fixture.minimal.total}` → `3`
- `${fixture.minimal.post1.title}` → `你好 huan`
- `${fixture.multilang.zhCnPost.title}` → `你好（中文版）`

---

## severity 语义（P0/P1/P2）

severity 值与判定规则见 [RUNBOOK.md §5](../runbooks/RUNBOOK.md#5-判定规则severity-语义)：

| severity | 失败时的动作 |
|---|---|
| **P0** | 该套件**立即判负并停止**后续用例执行，报告标记 `P0 FAIL` |
| **P1** | 记录失败，**继续执行**其余用例；报告计入通过率分母 |
| **P2** | **仅记录**（观察项/已知偏差显式断言），不影响通过率判负 |

---

## runtime 两值（dev/daemon）

runtime 值与端口分配表见 [RUNBOOK.md §3](../runbooks/RUNBOOK.md#3-端口分配表) 和 §4.3 挂载差异：

| runtime | 说明 | 典型套件 |
|---|---|---|
| `dev` | huan dev 模式（有 admin API、LiveReload、前台渲染；无 /health、/api/v1/*、SSE） | 大多数 browser 套件 |
| `daemon` | huan daemon 模式（有 admin API、/health、/api/v1/*、SSE；无 LiveReload） | browser 套件中需要公开 API 旁证的用例 |

**dev 与 daemon 挂载差异**（选 runtime 的依据）：

| 挂载 | dev | daemon |
|---|---|---|
| `/admin/api/*`（鉴权同 `${token}`） | 有 | 有 |
| `/livereload` WS + `/livereload.js` | 有 | 无 |
| 前台静态/JIT 渲染页 | 有（含 livereload 注入脚本） | 有 |
| `/health` | **无**（404） | 有 |
| `/api/v1/*` 公开 API | **无**（404） | 有 |
| SSE `/api/v1/events` | **无** | 有 |

---

## 端口分配（browser 段）

Browser 套件从 **13300** 起，按旅程类型分配端口（见 RUNBOOK §3）：

| 旅程类型 | 端口 | 说明 |
|---|---|---|
| `lg`（admin-login） | 13300 | 登录旅程 |
| `bc`（admin-content-crud） | 13310 | 内容 CRUD 旅程 |
| `bs`（admin-settings） | 13320 | 设置旅程 |
| `bp`（admin-plugins） | 13330 | 插件旅程 |
| `sr`（site-rendering） | 13340 | 前台渲染旅程 |

---

## 执行 agent 读法

执行 agent 在执行任何 Browser 套件前必须：
1. 读本 `_schema.md`
2. 读 `RUNBOOK.md`（变量契约与启动命令）
3. 读 `FIXTURES.md`（常量引用）
4. 读 `api/_schema.md`（api-probe 的 expect 语法）
5. 读目标 YAML 套件文件

**读法顺序**：Browser Schema → Runbook → Fixtures → API Schema → YAML

---

## 示例：完整用例（bc-001 登录后台并进入内容列表）

此用例演示 spec §5 的 bc-001 旅程：goto /admin → enter-token → verify 内容列表可见 → screenshot。

```yaml
suite: admin-content-crud
module: admin-spa
fixture: minimal
runtime: dev
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

      - action: enter-token

      - action: verify
        milestone: 内容列表可见
        expect:
          - "页面出现文章标题「${fixture.minimal.post1.title}」"
          - "出现内容树，含 posts 节点"
          - "统计信息显示总数 3"
        hints:
          - "列表项通常为 .content-item 或 table tr"
          - "内容树可能在左侧边栏"

      - action: screenshot
        name: bc-001-milestone-1

      - action: api-probe
        method: GET
        path: /admin/api/status
        headers: { Authorization: "Bearer ${token}" }
        expect:
          status: 200
          body: { total: 3 }
```

**执行流程**：

1. **goto**：导航到 `/admin`，加载登录页
2. **enter-token**：在 Token 输入框填入 `${token}`（`e2e-fixed-token`），提交登录
3. **verify**：验证「内容列表可见」里程碑——页面出现文章标题、内容树、统计信息
4. **screenshot**：截图保存为 `bc-001-milestone-1.png`
5. **api-probe**：调用 GET /admin/api/status 旁证后台状态，`expect` 语法复用 api/_schema.md

---

## 可靠性规则总结

### 1. verify 断言用用户可见内容

- `expect` 用**用户可见内容**表述（文本、状态、URL 变化）
- DOM 选择器只作 `hints`——SPA 改版时 hints 失效不判负
- 用自然语言重新定位，不依赖精确 DOM 结构

### 2. verify 失败先截图再判负

- 每个 `verify` 失败必须先截图（screenshot 动作）再判负
- 截图命名格式：`<case-id>-fail`
- 见 RUNBOOK §5 判定规则

### 3. hints 语义「提示非契约」

- `hints` 是**提示非契约**——辅助定位，非强制要求
- SPA 改版时 hints 失效不判负
- 用自然语言重新定位元素

### 4. interact 意图式

- `interact` 用**自然语言描述意图**（「点击新建按钮」）
- 可带 `hints` 辅助定位
- 把「怎么点」交给 agent-browser 临场决定

### 5. screenshot 命名规范

- 里程碑截图：`<case-id>-milestone-<n>`（如 `bc-001-milestone-1`）
- 失败截图：`<case-id>-fail`（如 `bc-001-fail`）

### 6. wait 三条件

- `reload`：等待页面重新加载完成
- `url-change`：等待 URL 变化
- `element-visible`：等待指定元素可见（必须带 `hints`）

### 7. api-probe 复用 api/_schema.md

- `expect` 语法完全复用 [api/_schema.md §1](../api/_schema.md#1-http---http-请求与断言)
- 浏览器为壳、API 为锚——在旅程中穿插 API 调用做旁证

---

**本文档自成一体**：执行 agent 只需读本 `_schema.md` + `RUNBOOK.md` + `FIXTURES.md` + `api/_schema.md` 四份文档即可执行任何 Browser 套件。后续 5 个 Browser 套件全部按此 Schema 编写，字段名与结构严格一致。
