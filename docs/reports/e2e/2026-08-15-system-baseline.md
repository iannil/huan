# E2E 体系首次全量基线报告（2026-08-15）

> run_id: 20260815-122309 · 二进制: worktree `e2e-test-system` commit `215569d`（版本串 `huan 0.7.1 (e8fdad2-dirty)`）· 环境: macOS 26.5.2 (darwin/arm64), go1.26.2

本报告是 huan E2E 测试体系（`tests/e2e/`，Task 0-13 建成）的**首次全量实跑基线**，作为后续回归的对照锚点。执行严格按 [tests/e2e/runbooks/RUNBOOK.md](../../../tests/e2e/runbooks/RUNBOOK.md)：现场构建二进制、fixture 拷贝到 `/tmp/huan-e2e/sites/`、`HUAN_HOME=$site/.huan-home` 隔离、固定 token `e2e-fixed-token`、端口按 §3 端口表。

## 1. 执行范围

- **API 套件 10 个**（78 例）：每套件 P0 全跑 + 其余按简报抽 2 例
- **CLI 套件 4 个**（15 例）：全部复跑（Task 12 已全跑过 15 例，本次复跑确认）
- **browser 套件 5 个**（22 旅程）：P0 旅程 lg-001、bc-001、bc-002、sr-001、sr-002 之外加走 lg-002、lg-003、bc-003、bc-004、sr-005（agent-browser 0.31.1 实走）
- **YAML 语法**：`python3` 递归 `safe_load` 全部 19 个 YAML（10 api + 5 browser + 4 cli）通过

## 2. 每套件结果表

API 套件（✓ = 抽样范围内全部通过；实测与 YAML 断言无一处不符，本次实跑**零 YAML 修正**）：

| 套件 | runtime | 端口 | 跑了哪些 | 结果 |
|---|---|---|---|---|
| auth | dev | 13200 | P0 全部（auth-001..004、006）+ 抽样 auth-005 | ✓ 6/6（401 双错误体+WWW-Authenticate、X-Huan-Admin-Token 等价、loopback 自动 token、非 loopback 拒启 exit 1） |
| status | dev | 13210 | P0 全部（stat-001/002）+ 抽样 stat-003/004 | ✓ 4/4（初始全字段、创建后 total+1、recentContent 降序、mediaCount+1） |
| content-crud | dev | 13220 | P0 全部（crud-001/002/004..007）+ 抽样 crud-003/013 | ✓ 8/8（创建/回读/PUT 双断言/DELETE/重复 500 不覆盖/非法 JSON 400/并发 PUT 最后写赢+审计 5 行） |
| media | dev | 13230 | P0 全部（med-001..003）+ 抽样 med-007/008 | ✓ 5/5（SVG 上传、列表分组、删除、PNG 魔数、dir 子目录） |
| settings | dev | 13240 | P0 全部（set-001..006、008）+ 抽样 set-009 | ✓ 8/8（SiteSettings 子集、结构化/YAML 双通道、非法 YAML/JSON 拒绝、异步 rebuild 首页 title 生效、审计 settings.update） |
| build-trigger | dev | 13250 | P0 全部（bld-001/002/004）+ 抽样 bld-003/007 | ✓ 5/5（202 异步、新页可见、draft 单页 404 双验证、增量内容可见性、sitemap 含新页） |
| public-api | daemon | 13260 | 全部 12 例（pub-001..012 全 P0） | ✓ 12/12（分页结构、limit clamp、越界空页、MaxInt64 DoS 回归 clamp 100000、section/tag/q 过滤、rune 计数 201/200 边界、单页详情、404/405） |
| sse-events | daemon | 13270 | P0 全部（sse-001/003/004）+ 抽样 sse-002 | ✓ 4/4（text/event-stream 三头、20s 心跳 `:heartbeat`、content_changed{trigger:admin}、直改文件增量链 build_completed{incremental:true,pages:3}） |
| livereload | dev | 13280 | 全部 4 例（全 P0） | ✓ 4/4（101 握手+固定 Accept、hello 帧 official-7、reload 广播、livereload.js v4.0.2） |
| plugins-admin | daemon | 13295 | P0 全部（plug-001..007）+ 抽样 plug-008/009 | ✓ 9/9（list total=3、load 400/500/200、unload 回读、reload 三态、409 冲突、compiled 卸载拒绝、theme 空态） |

CLI 套件（build-only，无端口）：

| 套件 | 跑了哪些 | 结果 |
|---|---|---|
| build | cb-001..008 全部（P0: 001/002/003/007） | ✓ 8/8（4 pages 锚点、draft 排除/-D 落盘、-F/-E 三态矩阵、增量内容可见性、--minify wc -l=0、缺配置 exit 1、multilang 双语言段） |
| new | cn-001..003 全部（P0: 001/002） | ✓ 3/3（archetype 渲染 RFC3339、重名 exit 1 不覆盖、内置默认 `title: "No Archetype"`） |
| config | cc-001..003 全部（P0: 001/002） | ✓ 3/3（合法输出、非法 YAML exit 1、`${VAR:-default}` 插值默认/env 双态） |
| version | cv-001 | ✓ 1/1（`^huan \d+\.\d+\.\d+`） |

browser 旅程（agent-browser 实走；截图存 `docs/reports/e2e/assets/20260815-122309/`）：

| 旅程 | 端口 | 结果 | 证据 |
|---|---|---|---|
| lg-001 首次认证见面板 | 13300 | ✓（双 prompt 应答、统计卡 内容总数3/草稿1、五项导航、api-probe 200） | lg-001-milestone-1.png |
| lg-002 错 token 拒绝 | 13300 | ✓（二次 prompt dismiss、「加载失败」可见、无统计数字） | lg-002-milestone-1.png |
| lg-003 刷新会话保持 | 13300 | ✓（sessionStorage=huan-admin-token、无 prompt、面板直渲染） | lg-003-milestone-1.png |
| bc-001 内容列表 | 13310 | ✓（「全部 · 3 篇」、树 posts2、chips 3/1/2、api-probe） | bc-001-milestone-1.png |
| bc-002 新建→落盘可回读 | 13310 | ✓（ZH-CN 语言下拉、编辑器「开始编写」、api-probe draft:true、列表 4 篇/草稿 2） | bc-002-milestone-1/2.png |
| bc-003 编辑保存双回读 | 13310 | ✓（标题+正文改、保存按钮已保存、api-probe title/rawContent） | bc-003-milestone-1.png |
| bc-004 删除旅程 | 13310 | ✓（勾选→已选1项→确认删除→列表回 3 篇、api-probe 404） | bc-004-milestone-1.png |
| sr-001 首页渲染 | 13340 | ✓（3 条链接含草稿——偏差 1 显式断言） | sr-001-milestone-1.png |
| sr-002 文章页 | 13340 | ✓（title/h1/加粗 huan（strong）/livereload.js 注入） | sr-002-milestone-1.png |
| sr-005 LiveReload 双页 | 13340 | ✓（双 tab 并开、fs 改文、两页 15s 内自动出新标记文本、URL 原地） | sr-005-milestone-1/2.png |

### 汇总

- **通过率：API 抽样 65/65，CLI 15/15，browser 10/10（P0 旅程 5/5 全过）——90/90 零失败**
- **P0 状态：全过**（API 套件 P0 52 例全跑全过；CLI P0 9 例全过；browser P0 旅程 lg-001/bc-001/bc-002/sr-001/sr-002 全过）
- **YAML 修正清单：无**——本次全量实跑未发现任何「断言与实测不符」（Task 5-12 写套件时的两次预校准已消化全部偏差，含 FIXTURES 偏差 4 的 total 口径）
- teardown：`pgrep -f huan-e2e/huan` 无残留；`/tmp/huan-e2e/sites|artifacts/$run_id` 已清理（截图已拷出）

## 3. Bug 清单（实跑过程发现/复验的引擎缺陷终态）

| # | 缺陷 | 严重度 | 状态 | 来源 |
|---|---|---|---|---|
| 1 | daemon admin token 鉴权失效（token 硬编码为空，`HUAN_ADMIN_TOKEN` env 不生效） | P0 | **已修** `e0a8f2e`（Task 1） | 探针期发现；本基线全部 daemon 用例依赖此修复 |
| 2 | simple_plugin `InitPlugin` 签名错（返回 `plugin.Plugin` 跨模块接口身份校验失败，load 恒 500 "InitPlugin has wrong signature"） | P0（插件加载面） | **已修** `9e4c675`（Task 8） | plug-004 前置探针发现；本基线 plug-004/006/007 通过即复验 |
| 3 | loader_test 过时断言（`$HUAN_HOME/plugins` 子目录布局） | 测试 | **已修** `975991f`（Task 0，基线外） | Task 0 |
| 4 | draft 泄露进 sitemap/RSS/列表聚合渲染（仅「单页不落盘」「公开 JSON API 排除」两条防线有效） | P1（内容安全） | **未修**（FIXTURES.md minimal 偏差 1 记录在案；bld-004/pub-006/sr-001 按「防线有效+偏差显式断言」口径覆盖） | Task 2 发现；本基线 sr-001 复现（首页含草稿链接） |
| 5 | en 文章 `relPath` 丢语言后缀 + `filePath` 双拼后缀（`.en.en.md`）——admin 列表 API 的语言路径归一 bug，浏览器删 en 文章必 500 | P1 | **未修**（bc 套件绕开：新建/删除走 zh-CN；bc-002 以 P2 观察项记录） | Task 10 发现；本基线 bc-002..004 按绕开口径全过 |
| 6 | Settings 页 `hasChanges` ref 不重渲染（表单改脏状态 UI 不刷新） | P2 | **未修**（bs 套件绕开） | Task 10 发现 |
| 7 | dev 模式无 pluginManager（`/admin/api/plugins` 恒 `{"status":"plugin manager unavailable"}`，200 而非 503）；daemon build 不传 PluginRegistry（serve 页面无插件注入）；`$HUAN_HOME` 陈旧 .so 毒化连带拒绝项目本地同名插件 | 观察 | **未修**（RUNBOOK §4.3/§9 与 FIXTURES 偏差记录在案；plugins-admin 定 daemon、注入断言只对 CLI build） | Task 3/8 观察；本基线 plug 套件与 RUNBOOK 隔离约定即其对策 |

未修项 4-7 均已在 FIXTURES.md / 套件头注释 / RUNBOOK 显式记录，不是回归缺口而是「已知状态」；修复请另立任务并同步 FIXTURES.md。

## 4. 与 FIXTURES.md 的引用关系

- 所有断言常量引用 [tests/e2e/fixtures/FIXTURES.md](../../../tests/e2e/fixtures/FIXTURES.md)（minimal/multilang/with-plugins 三张已知状态表）。
- 本基线实跑顺带完成 FIXTURES.md **with-plugins 偏差 4 的回改**：Task 3 首测的 plugins list `total=1` 已过时，按 Task 8 + Task 13 两次复测更新为「双 .so 编译后 total=2；simple-plugin.so 加载后 total=3」（更新日期 2026-08-15，来源 task 8/13）。
- 其余偏差（minimal 偏差 1/2/3、multilang 偏差 1/2/3、with-plugins 偏差 1/2/3/5/6）本基线未发现与实测相悖，维持原记录。

## 5. 环境与复现

```bash
# 二进制：worktree e2e-test-system @ 215569d 现场 go build（版本串 huan 0.7.1 (e8fdad2-dirty)）
go build -o /tmp/huan-e2e/huan ./cmd/huan
# 逐套件按 RUNBOOK §3 端口表起 dev/daemon（HUAN_HOME 隔离 + HUAN_ADMIN_TOKEN=e2e-fixed-token）
# 复跑入口：tests/e2e/README.md「执行入口」节
```

- go1.26.2 darwin/arm64，macOS 26.5.2；agent-browser 0.31.1（CDP Chrome）
- 基线性质声明：本报告的数字是**抽样口径**（P0 全跑 + 其余抽 2）而非 78 例逐例全跑；P0 全量无失败即判定体系绿。后续回归优先重跑 P0 集与上表抽样集，与本报告对照。
