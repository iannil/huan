# API 套件 Schema 参考文档

本文档定义所有 API 套件 YAML 的字段字典与断言语义。执行 agent 写/读 YAML 用例前必须先读本 Schema；后续 10 个 API 套件全部按此 Schema 编写。

**配套文档**：
- 变量契约（`${token}`/`${port}`/`${base}`/`${site}` 等）见 [RUNBOOK.md §1](../runbooks/RUNBOOK.md#1-变量契约保留变量表)
- 断言命令模式见 [patterns.md](../runbooks/patterns.md)
- Fixture 已知状态常量见 [FIXTURES.md](../fixtures/FIXTURES.md)
- 判定规则（P0/P1/P2）见 [RUNBOOK.md §5](../runbooks/RUNBOOK.md#5-判定规则severity-语义)
- 端口分配表见 [RUNBOOK.md §3](../runbooks/RUNBOOK.md#3-端口分配表)
- Runtime 选择（dev/daemon/build-only）见 [RUNBOOK.md §4.3](../runbooks/RUNBOOK.md#43-dev-与-daemon-挂载差异选-runtime-的依据)

---

## 套件骨架 YAML 结构

```yaml
suite: auth                    # 套件名（对应 RUNBOOK §3 端口表）
module: admin-auth             # 模块名（测试覆盖的功能模块）
fixture: minimal               # fixture 名（minimal/multilang/with-plugins，见 FIXTURES.md）
runtime: dev                   # 运行时：dev/daemon/build-only（见 RUNBOOK §4.3）
startup:                       # 套件前置（可选）
  - action: fs.write           # 动作类型：http/fs.assert/fs.write/fs.delete/cli/sse.subscribe/wait/var
    path: $site/content/posts/test.md
    content: |
      ---
      title: Test
      ---
tests:                         # 用例列表
  - id: auth-001               # 用例 ID（套件名-序号）
    name: 正确 token 通过 Bearer 头访问 status    # 用例描述（中文）
    severity: P0              # 严重程度：P0（失败判负停止）/P1（记录继续）/P2（仅记录）
    preconditions:            # 前置条件（可选）
      - auth-002              # 同套件内用例 ID 引用；当前用例依赖这些用例先执行成功
    steps:                    # 步骤列表（必填）
      - action: http
        method: GET
        path: /admin/api/status
        headers: { Authorization: "Bearer ${token}" }
        expect:
          status: 200
          body: { total: 3 }   # 引用 FIXTURES.md 常量
    cleanup:                  # 清理步骤（可选）
      - action: fs.delete
        path: $site/content/posts/test.md
```

### 套件级字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `suite` | string | 是 | 套件名，对应 RUNBOOK §3 端口表（如 auth/status/content-crud） |
| `module` | string | 是 | 模块名，描述测试覆盖的功能模块（如 admin-auth） |
| `fixture` | string | 是 | fixture 名：minimal/multilang/with-plugins（见 FIXTURES.md） |
| `runtime` | string | 是 | 运行时：dev/daemon/build-only（见 RUNBOOK §4.3 挂载差异） |
| `startup` | array | 否 | 套件前置步骤列表（在所有用例前执行一次，常用 fs.write 注入测试数据） |
| `tests` | array | 是 | 用例列表 |

### 用例级字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 用例 ID，格式 `<套件名>-<序号>`（如 auth-001），同套件内唯一 |
| `name` | string | 是 | 用例描述，中文 |
| `severity` | string | 是 | 严重程度：P0/P1/P2（见 RUNBOOK §5 判定规则） |
| `preconditions` | array | 否 | 前置条件，同套件内用例 ID 引用列表（当前用例依赖这些用例先执行成功） |
| `steps` | array | 是 | 步骤列表（核心执行逻辑） |
| `cleanup` | array | 否 | 清理步骤列表（用例执行后清理状态；临时目录整体销毁是默认，此处仅显式回滚跨用例状态） |

---

## Step 动作类型（8 个动词）

### 1. http - HTTP 请求与断言

发起 HTTP 请求并断言响应状态码/响应体/响应头。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `http` |
| `method` | string | 是 | HTTP 方法：GET/POST/PUT/DELETE |
| `path` | string | 是 | 请求路径，如 `/admin/api/status`；不包含 `${base}`（YAML 中只写路径） |
| `headers` | object | 否 | 请求头对象，如 `{ Authorization: "Bearer ${token}" }` |
| `body` | string/object | 否 | 请求体（POST/PUT 用）；JSON 对象或原始字符串 |
| `expect` | object | 是 | 响应断言（见下表） |

**expect 断言字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `status` | number | **是** | HTTP 状态码（**硬规则 1：必填**） |
| `body` | object | **是** | 响应体断言（**硬规则 1：至少一键值或显式 `body_absent: true`**） |
| `body_absent` | boolean | 否 | 显式断言响应体为空（设 `true` 时 `body` 字段可省） |
| `headers` | object | 否 | 响应头断言，如 `{ WWW-Authenticate: 'Bearer realm="huan-admin"' }` |
| `side_effect` | string | 否 | 标记副作用类型（如 `async-rebuild`），用于硬规则 2 例外说明 |

**最小示例**：

```yaml
- action: http
  method: GET
  path: /admin/api/status
  headers: { Authorization: "Bearer ${token}" }
  expect:
    status: 200
    body: { total: 3 }   # 引用 FIXTURES.md minimal 常量
```

**断言语义说明**：

- `expect.body` 使用**点路径**（dot-notation）断言嵌套字段，如 `frontmatter.title`、`sectionBreakdown.posts`
- **数组断言**：用 `bodyContains` 断言数组包含某元素（但当前 E2E 体系不用，改用 JSON 探测命令）
- **正则断言**：用 `matches` 字段断言匹配正则，如 `{ title: { matches: "^huan \\d+\\.\\d+\\.\\d+$" } }`
- **错误响应体断言**：admin API 错误统一为 `{"error":"..."}` 结构，如 `{ error: "missing admin token" }`
- **完整字段断言**：如 `{ total: 3, published: 2, drafts: 1 }` 多键同时断言

### 2. fs.assert - 文件系统断言

断言文件存在/不存在/内容包含某个字符串/修改时间等。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `fs.assert` |
| `path` | string | 是 | 文件路径（绝对路径或相对于 `${site}`），如 `$site/public/posts/hello-huan/index.html` |
| `exists` | boolean | 否 | 断言文件存在（`true`）或不存在（`false`），默认 `true` |
| `contains` | string | 否 | 断言文件内容包含此字符串（与 `exists` 配合使用） |
| `matches` | string | 否 | 断言文件内容匹配此正则表达式 |
| `mtime_changed` | boolean | 否 | 断言文件修改时间变化（用于增量构建验证） |

**最小示例**：

```yaml
- action: fs.assert
  path: $site/public/posts/hello-huan/index.html
  exists: true
  contains: <h1 id="page-title">你好 huan</h1>
```

**硬规则 2：写操作后必跟 fs.assert**

- 每个 POST/PUT/DELETE 用例在 HTTP 响应断言后必须跟至少一个 `fs.assert`
- **例外**：POST /admin/api/build 是异步触发（side-effect: async-rebuild），schema 里需显式标注此例外并在用例中使用 `wait` step（wait 的 for 条件见下文）

### 3. fs.write - 写入文件

向文件系统写入文件（用于 startup 步骤注入测试数据）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `fs.write` |
| `path` | string | 是 | 文件路径，如 `$site/content/posts/test.md` |
| `content` | string | 是 | 文件内容（多行用块标量 `|`） |

**最小示例**：

```yaml
- action: fs.write
  path: $site/content/posts/test.md
  content: |
    ---
    title: Test Post
    date: 2026-08-15
    ---
    测试正文
```

**注意事项**：

- 文件内出现 YAML `---` frontmatter 分隔符时，必须用块标量 `|` 或单引号包裹整个内容，避免 YAML 解析歧义
- 写操作后通常跟 `fs.assert` 断言文件存在（见硬规则 2）

### 4. fs.delete - 删除文件

删除文件系统中的文件（用于 cleanup 步骤）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `fs.delete` |
| `path` | string | 是 | 文件路径，如 `$site/content/posts/test.md` |

**最小示例**：

```yaml
- action: fs.delete
  path: $site/content/posts/test.md
```

### 5. cli - CLI 命令执行

执行 huan CLI 命令并断言退出码/输出（仅 CLI 套件使用）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `cli` |
| `command` | string | 是 | CLI 命令，如 `$bin build -s $site` |
| `expect` | object | 是 | 断言（见下表） |

**expect 断言字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `exit_code` | number | 是 | 期待退出码（0 为成功） |
| `stdout_contains` | string | 否 | stdout 包含此字符串 |
| `stdout_matches` | string | 否 | stdout 匹配此正则 |
| `stderr_contains` | string | 否 | stderr 包含此字符串 |

**最小示例**：

```yaml
- action: cli
  command: $bin build -s $site
  expect:
    exit_code: 0
    stdout_contains: Rendered:
```

### 6. sse.subscribe - SSE 订阅

订阅 SSE 事件流并断言事件类型（仅 daemon 套件使用）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `sse.subscribe` |
| `path` | string | 是 | SSE 端点路径，如 `/api/v1/events` |
| `duration` | number | 是 | 订阅持续时间（秒），如 `5` |
| `expect` | object | 是 | 断言（见下表） |

**expect 断言字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `headers` | object | 否 | 响应头断言，如 `{ Content-Type: "text/event-stream" }` |
| `event_types` | array | 否 | 期待收到的事件类型列表，如 `["build", "content"]` |
| `heartbeat` | boolean | 否 | 是否期待心跳（15s 一次的 `: keepalive` 注释行） |

**最小示例**：

```yaml
- action: sse.subscribe
  path: /api/v1/events
  duration: 5
  expect:
    headers: { Content-Type: "text/event-stream" }
```

### 7. wait - 等待条件

等待某个条件成立（用于异步操作后，如 POST /admin/api/build 后等待构建完成）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `wait` |
| `for` | string | 是 | 等待条件类型：`file-change`/`port`/`seconds`/`build-complete` |
| `path` | string | 否 | 文件路径（`for: file-change` 时必填） |
| `seconds` | number | 否 | 等待秒数（`for: seconds` 时必填） |

**wait 条件类型说明**：

| 条件类型 | 必填字段 | 说明 |
|---|---|---|
| `file-change` | `path` | 等待指定文件的修改时间变化（用于增量构建验证） |
| `port` | - | 等待端口可连接（用于服务启动后探活） |
| `seconds` | `seconds` | 等待固定秒数（通用等待） |
| `build-complete` | - | 等待构建完成（用于 POST /admin/api/build 后，等待构建产物落盘） |

**最小示例**：

```yaml
- action: wait
  for: build-complete
```

### 8. var - 变量设置

设置 shell 变量（用于跨步骤传递值，很少使用）。

**字段字典**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | 固定值 `var` |
| `name` | string | 是 | 变量名 |
| `value` | string | 是 | 变量值 |

**最小示例**：

```yaml
- action: var
  name: post_id
  value: crud-001
```

---

## 变量替换规则

YAML 中的变量按 [RUNBOOK.md §1](../runbooks/RUNBOOK.md#1-变量契约保留变量表) 展开：

| 变量 | 展开值 | 示例 |
|---|---|---|
| `${bin}` | huan 二进制绝对路径 | `/tmp/huan-e2e/huan` |
| `${site}` | fixture 的可写拷贝 | `/tmp/huan-e2e/sites/20260815-142530-minimal` |
| `${port}` | 当前套件固定端口 | `13200` |
| `${base}` | `http://127.0.0.1:${port}` | `http://127.0.0.1:13200` |
| `${token}` | admin API token | `e2e-fixed-token` |
| `${fixture.<name>.<field>}` | fixture 字段引用 | `${fixture.post1.title}` → `你好 huan` |

**fixture 引用语法**：从 FIXTURES.md 引用已知常量，如：

- `${fixture.minimal.total}` → `3`
- `${fixture.minimal.post1.title}` → `你好 huan`
- `${fixture.multilang.zhCnPost.title}` → `你好（中文版）`

---

## 硬规则（机器可检查）

### 硬规则 1：expect 断言完整性

每个 `http` step 的 `expect` 必须：
1. 显式断言 `status` 字段（必填）
2. 显式断言至少一个 `body` 字段，或设置 `body_absent: true`

**反面示例（违反）**：

```yaml
- action: http
  method: GET
  path: /admin/api/status
  expect:
    status: 200
    # 缺少 body 断言，违反硬规则 1
```

**正面示例（符合）**：

```yaml
- action: http
  method: GET
  path: /admin/api/status
  expect:
    status: 200
    body: { total: 3 }   # 至少一键值断言
```

或：

```yaml
- action: http
  method: POST
  path: /admin/api/build
  expect:
    status: 202
    body_absent: true   # 显式标记无响应体
```

### 硬规则 2：写操作后必跟 fs.assert

每个写操作（POST/PUT/DELETE）用例必须在 HTTP 响应断言后跟至少一个 `fs.assert`。

**例外**：POST /admin/api/build 是异步触发（side-effect: async-rebuild），schema 里需在用例中显式写 `side_effect: async-rebuild` 并跟 `wait` step，而非 `fs.assert`。

**正面示例（符合）**：

```yaml
steps:
  - action: http
    method: POST
    path: /admin/api/content
    body: { frontmatter: { title: "Test" }, rawContent: "正文" }
    expect:
      status: 201
      body: { relPath: "posts/test.md" }
  - action: fs.assert              # 硬规则 2：写操作后必跟 fs.assert
    path: $site/content/posts/test.md
    exists: true
```

**例外示例（异步 rebuild）**：

```yaml
steps:
  - action: http
    method: POST
    path: /admin/api/build
    expect:
      status: 202
      body: { status: "rebuild triggered" }
      side_effect: async-rebuild   # 显式标注异步副作用
  - action: wait                    # 用 wait 替代 fs.assert
    for: build-complete
```

---

## severity 语义（P0/P1/P2）

severity 值与判定规则见 [RUNBOOK.md §5](../runbooks/RUNBOOK.md#5-判定规则severity-语义)：

| severity | 失败时的动作 |
|---|---|
| **P0** | 该套件**立即判负并停止**后续用例执行，报告标记 `P0 FAIL` |
| **P1** | 记录失败，**继续执行**其余用例；报告计入通过率分母 |
| **P2** | **仅记录**（观察项/已知偏差显式断言），不影响通过率判负 |

---

## runtime 三值（dev/daemon/build-only）

runtime 值与端口分配表见 [RUNBOOK.md §3](../runbooks/RUNBOOK.md#3-端口分配表) 和 §4.3 挂载差异：

| runtime | 说明 | 典型套件 |
|---|---|---|
| `dev` | huan dev 模式（有 admin API、LiveReload、前台渲染；无 /health、/api/v1/*、SSE） | auth/status/content/media/settings/build-trigger/livereload/plugins-admin/browser |
| `daemon` | huan daemon 模式（有 admin API、/health、/api/v1/*、SSE；无 LiveReload） | public-api/sse-events |
| `build-only` | 仅 CLI build，无 HTTP 服务 | CLI 套件（build/new/config/version） |

**dev 与 daemon 挂载差异**（选 runtime 的依据）：

| 挂载 | dev | daemon |
|---|---|---|
| `/admin/api/*`（鉴权同 `${token}`） | 有 | 有（Task 1 后可用） |
| `/livereload` WS + `/livereload.js` | 有 | 无 |
| 前台静态/JIT 渲染页 | 有（含 livereload 注入脚本） | 有（daemon 构建管线，不经插件钩子） |
| `/health` | **无**（404） | 有 |
| `/api/v1/*` 公开 API | **无**（404） | 有 |
| SSE `/api/v1/events` | **无** | 有（心跳 15s） |
| `/admin/api/plugins` | 返回 `{"status":"plugin manager unavailable"}` | 正常 list |

---

## preconditions 与 cleanup

### preconditions（前置条件）

- 用例 `preconditions` 字段是同套件内用例 ID 引用列表
- 当前用例依赖这些用例先执行成功；任一前置用例失败则当前用例跳过
- 用于表达用例间顺序依赖（如创建后再更新）

**示例**：

```yaml
- id: crud-005
  name: 更新文章
  severity: P0
  preconditions:
    - crud-001    # 依赖 crud-001（创建文章）先执行成功
  steps:
    - action: http
      method: PUT
      path: /admin/api/content/posts/crud-001.md
      # ...
```

### cleanup（清理步骤）

- 用例 `cleanup` 字段是步骤列表，用例执行完毕后（无论成功/失败）执行
- 用于显式回滚跨用例状态（如删除测试创建的文件）
- 临时目录整体销毁是默认行为，不需要 cleanup 重复

**示例**：

```yaml
- id: crud-006
  name: 删除文章
  severity: P0
  steps:
    - action: http
      method: DELETE
      path: /admin/api/content/posts/temp.md
      expect: { status: 200, body: { status: "deleted" } }
  cleanup:
    - action: fs.delete
      path: $site/content/posts/temp.md   # 确保删除成功（若 API 失败）
```

---

## 已知偏差断言口径

写用例前先对照 [patterns.md §9](../runbooks/patterns.md#9-已知偏差断言口径引用速查) 与 [FIXTURES.md 各 fixture 的「已知状态偏差」节](../fixtures/FIXTURES.md)，避免断言与实际不符：

| 偏差项 | 断言口径 |
|---|---|
| draft 泄露 | draft 单页不落盘、公开 JSON API 排除 draft，但 sitemap/RSS/列表聚合渲染**含** draft——聚合断言要么绕开要么显式断言该偏差 |
| build stdout WARN | terms/searchindex 模板缺失的 WARN 是常态，不得断言零 WARN |
| `/api/v1/posts` 端点 | 不存在（404），公开 API 断言一律用 `/api/v1/pages` |
| daemon 插件注入 | daemon serve 页面无插件注入（daemon 构建不传 PluginRegistry）——注入断言仅对 CLI `huan build` 产物 |

---

## 执行 agent 读法

执行 agent 在执行任何 API 套件前必须：
1. 读本 `_schema.md`
2. 读 `RUNBOOK.md`（变量契约与启动命令）
3. 读 `FIXTURES.md`（常量引用）
4. 读目标 YAML 套件文件

**读法顺序**：Schema → Runbook → Fixtures → YAML

---

## 示例：完整用例（crud-001 创建文章）

```yaml
suite: content-crud
module: admin-content-crud
fixture: minimal
runtime: dev
tests:
  - id: crud-001
    name: 创建文章并落盘
    severity: P0
    steps:
      - action: http
        method: POST
        path: /admin/api/content
        headers: { Authorization: "Bearer ${token}" }
        body:
          frontmatter:
            title: E2E 创建测试
            date: 2026-08-15
            tags: [e2e, test]
          rawContent: |
            这是 E2E 创建的测试文章。
            第二段。
        expect:
          status: 201
          body:
            relPath: posts/crud-001.md
            filename: crud-001.md
      - action: fs.assert              # 硬规则 2：写操作后必跟 fs.assert
        path: $site/content/posts/crud-001.md
        exists: true
        contains: title: E2E 创建测试
      - action: http
        method: GET
        path: /admin/api/content/posts/crud-001.md
        headers: { Authorization: "Bearer ${token}" }
        expect:
          status: 200
          body:
            frontmatter:
              title: E2E 创建测试
              tags: [e2e, test]
```

---

**本文档自成一体**：执行 agent 只需读本 `_schema.md` + `RUNBOOK.md` + `FIXTURES.md` 三份文档即可执行任何 API 套件。后续 10 个 API 套件全部按此 Schema 编写，字段名与结构严格一致。
