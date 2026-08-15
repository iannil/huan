# E2E Runbook（agent 执行手册）

本文件是所有 E2E YAML 用例的执行基础设施：变量契约、端口分配、启动/捕获/探活/判定/报告规则都定义在此。后续 Task 的用例文件（`tests/e2e/api/`、`tests/e2e/browser/`、`tests/e2e/cli/`）与执行 agent **只依赖这里定义的契约**，不另行发明命令。

断言复用命令见同目录 [patterns.md](patterns.md)；fixture 已知状态常量见 [../fixtures/FIXTURES.md](../fixtures/FIXTURES.md)——用例断言数字一律引用该表，不凭记忆写。

## 1. 变量契约（保留变量表）

以下 shell 变量是全局保留名，所有套件/用例/命令原样引用，执行 agent 不得重定义或挪作他用：

| 变量 | 定义 | 示例值 | 说明 |
|---|---|---|---|
| `${bin}` | huan 二进制绝对路径 | `/tmp/huan-e2e/huan` | 每次 run 现场构建（见 §2），禁止复用上次的二进制 |
| `${run_id}` | `date +%Y%m%d-%H%M%S` | `20260815-142530` | 一次执行（可含多个套件）的唯一标识，串起 site/artifacts/报告 |
| `${site}` | fixture 的可写拷贝 | `/tmp/huan-e2e/sites/$run_id-minimal` | 用例的 `fs.*` 动作全部作用于此目录；**绝不直接改仓库内 fixture** |
| `${artifacts}` | 证据目录 | `/tmp/huan-e2e/artifacts/$run_id` | 日志、截图、断言输出、事件流的落点；报告引用这里的相对路径 |
| `${port}` | 当前套件固定端口 | `13201` | 查 §3 端口表；一个 run 内多套件并行不撞车 |
| `${base}` | `http://127.0.0.1:${port}` | `http://127.0.0.1:13201` | 所有 http step 的 path 前缀（YAML 中 path 只写 `/...`） |
| `${token}` | admin API token | `e2e-fixed-token` | **默认恒等于启动命令里的 `HUAN_ADMIN_TOKEN` 固定值**（见 §4） |

YAML 用例里写 `${base}/admin/api/status`，执行时按本表展开。fixture 名、severity 语义（P0/P1/P2）见各 `_schema.md`，判定规则见 §7。

## 2. 环境准备（每 run 必做）

```bash
cd /Users/rong.zhu/Code/zhurong/huan          # 仓库根（worktree 即 worktree 根）
go build -o /tmp/huan-e2e/huan ./cmd/huan     # ${bin}：本 run 唯一二进制

run_id=$(date +%Y%m%d-%H%M%S)
export bin=/tmp/huan-e2e/huan
mkdir -p /tmp/huan-e2e/sites /tmp/huan-e2e/artifacts/$run_id
export artifacts=/tmp/huan-e2e/artifacts/$run_id
```

拷贝 fixture（按套件的 `fixture:` 字段）：

```bash
export site=/tmp/huan-e2e/sites/$run_id-minimal     # 末段 = fixture 名
cp -r tests/e2e/fixtures/minimal $site
```

### 2.1 HUAN_HOME 隔离（强制，防插件毒化）

Task 3 实测：宿主 `~/.huan/plugins` 的旧 .so 与新二进制版本失配时，`plugin.Open` 失败会**连带毒化项目本地同名插件的加载**（build 静默零注入、退出码仍 0）。**所有启动命令一律带 `HUAN_HOME=$site/.huan-home`**（空目录，预建）：

```bash
mkdir -p $site/.huan-home
```

即使当前套件不用插件也带上——约定统一比按需判断更不易漏。

### 2.2 admin token（固定值优先）

dev 与 daemon 都支持 `HUAN_ADMIN_TOKEN` 环境变量（daemon 经 Task 1 修复后 env 优先）。**E2E 统一用固定 token**，`${token}` 恒等于它：

```bash
export HUAN_ADMIN_TOKEN=e2e-fixed-token    # 所有 dev/daemon 启动命令都带它
export token=e2e-fixed-token
```

stderr 捕获法（`grep -oE '[0-9a-f]{32}' | head -1`）可行但有噪音风险（日志中其他 32-hex 串可能抢先），**降级为附录 A 的备用方案**，主流程不用。

## 3. 端口分配表

从 **13200** 起，**每套件 +10**，段内偏移给同套件多实例/并行变体。测试一律显式 `--port` + `--bind 127.0.0.1`（dev 默认 1313、daemon 默认 8080，均不用默认值）。

| 套件 | 端口段 | 默认 `${port}` | runtime（挂载差异见 §4.3） |
|---|---|---|---|
| auth | 13200-13209 | 13200 | dev |
| status | 13210-13219 | 13210 | dev |
| content-crud | 13220-13229 | 13220 | dev |
| media | 13230-13239 | 13230 | dev |
| settings | 13240-13249 | 13240 | dev |
| build-trigger | 13250-13259 | 13250 | dev |
| public-api | 13260-13269 | 13260 | daemon |
| sse-events | 13270-13279 | 13270 | daemon |
| livereload | 13280-13289 | 13280 | dev |
| plugins-admin | 13290-13299 | 13290 | dev（list 端点断言跑 daemon 时用 +5 偏移） |
| browser 套件 | 13300-13349 | 依旅程（lg=13300、bc=13310、bs=13320、bp=13330、sr=13340） | dev |
| daemon 附加实例 | 各段 +5 | — | 同段偏移，用于同套件需要 daemon 旁证时 |

并行执行多个套件时各用各段；同套件内顺序用例共用默认端口。段规则：`13200 + 10×套件序号`，新增套件按此续表。

## 4. 启动 / 探活 / 停止

### 4.1 dev 启动（auth/status/content/media/settings/build-trigger/livereload/plugins-admin/browser 套件）

```bash
export port=13200
HUAN_ADMIN_TOKEN=$token HUAN_HOME=$site/.huan-home \
  $bin dev -s $site --port $port --bind 127.0.0.1 &> $artifacts/dev.log &
echo $! > $artifacts/dev.pid
sleep 3
```

探活（dev **没有** `/health`，打前台首页）：

```bash
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:$port/    # 期待 200
```

### 4.2 daemon 启动（public-api、sse-events 套件及 daemon 旁证断言）

```bash
export port=13260
HUAN_ADMIN_TOKEN=$token HUAN_HOME=$site/.huan-home \
  $bin daemon -s $site --port $port --bind 127.0.0.1 &> $artifacts/daemon.log &
echo $! > $artifacts/daemon.pid
sleep 4    # daemon 启动含 initial full build，稍长
```

探活用 `/health`：

```bash
curl -s -w '\n%{http_code}' http://127.0.0.1:$port/health
# 期待 {"status":"ok",...} + 200
```

token 同 `${token}`（Task 1 修复后 daemon 的 `HUAN_ADMIN_TOKEN` env 生效；若不设 env，stderr 会打印 32-hex 自动 token，捕获法见附录 A）。

### 4.3 dev 与 daemon 挂载差异（选 runtime 的依据）

| 挂载 | dev | daemon |
|---|---|---|
| `/admin/api/*`（鉴权同 `${token}`） | 有 | 有（Task 1 后可用） |
| `/livereload` WS + `/livereload.js` | 有 | 无 |
| 前台静态/JIT 渲染页 | 有（含 livereload 注入脚本） | 有（daemon 构建管线，不经插件钩子） |
| `/health` | **无**（404） | 有 |
| `/api/v1/*` 公开 API | **无**（404） | 有 |
| SSE `/api/v1/events` | **无** | 有（心跳 15s） |
| `/admin/api/plugins` | 有（Task 4 修复后 dev 也初始化 manager） | 有 |

### 4.4 停止单实例

```bash
kill $(cat $artifacts/dev.pid) 2>/dev/null      # 或 daemon.pid
```

## 5. 判定规则（severity 语义）

| severity | 失败时的动作 |
|---|---|
| **P0** | 该套件**立即判负并停止**（后续用例不再执行），报告标记 `P0 FAIL` |
| **P1** | 记录失败，**继续执行**其余用例；报告计入通过率分母 |
| **P2** | **仅记录**（观察项/已知偏差显式断言），不影响通过率判负 |

- P0 失败后仍须执行 teardown（§6），并保留 `$artifacts` 供报告引用。
- **verify 失败必须先截图再判负**：用 agent-browser 的 `screenshot` 落盘 `$artifacts/<case-id>-fail.png`，然后才记失败——没有证据的失败不算失败。
- CLI 用例（退出码断言）：非零退出时先 `2>&1 | tee $artifacts/<case-id>.log` 保留完整输出再判定。

## 6. teardown（每 run 收尾）

```bash
pkill -f "huan-e2e/huan" 2>/dev/null; sleep 1
# 报告引用的截图/日志先拷出（$artifacts 本身在 /tmp，会被整体删除时才需要）：
mkdir -p docs/reports/e2e/assets/$run_id
cp $artifacts/*.png docs/reports/e2e/assets/$run_id/ 2>/dev/null
rm -rf /tmp/huan-e2e/sites/$run_id-* /tmp/huan-e2e/artifacts/$run_id
```

`$artifacts` 里的证据在删除前须已写入报告（报告只引用已拷出或仍存在的路径；删了就写「已清理」）。确认无残留进程：`pgrep -f "huan-e2e/huan"` 应无输出。

## 7. 报告

结果报告写入 **`docs/reports/e2e/<date>-<module>.md`**（首次执行前 `mkdir -p docs/reports/e2e`），中文，风格对接 `docs/templates/report-template.md`：

```markdown
# E2E <模块> 执行报告（<YYYY-MM-DD>）

> run_id: <run_id> · 二进制: <commit hash> · 环境: macOS/<ver>, go<ver>

## 结果表

| 用例 id | 结果 | 耗时 | 证据 |
|---|---|---|---|
| auth-001 | PASS | 0.4s | artifacts/auth-001.txt |
| auth-002 | FAIL(P1) | 0.3s | artifacts/auth-002.txt, auth-002-fail.png |

## 汇总

- 通过率: X/Y（P0 全过: 是/否）
- bug 列表: <id + 一句话 + 证据路径；无则写「无新增」>
- 已知偏差命中: <引用 FIXTURES.md 偏差编号>

## 异常详情
<每个 FAIL 的断言期望 vs 实测原文>
```

硬性要求：每用例一行（id/结果/耗时/证据路径）+ 汇总（通过率/P0 状态/bug 列表）。证据路径写 `$artifacts` 内的文件名（报告成文时若已 teardown，标注「已清理，摘录如下」并贴关键输出）。

## 8. api-probe 通则

所有 curl 探测统一形态——`-s -w '\n%{http_code}'` 同时拿 body 与 status：

```bash
curl -s -w '\n%{http_code}' -H "Authorization: Bearer $token" ${base}/admin/api/status
```

JSON 字段断言给两种写法（按环境可用性选，优先 python3——macOS 自带；jq 需 brew）：

```bash
# python3（默认）
curl -s -H "Authorization: Bearer $token" ${base}/admin/api/status \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["total"]==3, d; print("ok total=3")'

# jq（环境有 jq 时）
curl -s -H "Authorization: Bearer $token" ${base}/admin/api/status \
  | jq -e '.total == 3' >/dev/null && echo "ok total=3"
```

注意：
- body + status 一起断言时（`-w '\n%{http_code}'`），先 `resp=$(curl ...)` 再分别取末行与剩余行；纯 JSON 解析则不加 `-w`。
- **build stdout 断言不得要求零 WARN**——terms/searchindex 模板缺失的 WARN 是 fixture 常态（见 FIXTURES.md），只断言 `Rendered:`/`Output:`/`Build complete.` 与 `Errors` 行。
- 断言公开产物（sitemap/RSS/列表聚合）时注意 draft 泄露已知偏差（FIXTURES.md minimal 偏差 1）：draft 单页不落盘、公开 JSON API 排除 draft，但聚合渲染含 draft——断言要么绕开聚合、要么显式断言该偏差。

## 9. 常见坑速查

| 症状 | 原因 | 处置 |
|---|---|---|
| with-plugins build 零注入、退出码 0 | `~/.huan` 旧 .so 毒化（FIXTURES.md with-plugins 偏差 1） | 启动/构建命令带 `HUAN_HOME=$site/.huan-home` |
| daemon admin API 401 | 用了 stderr 自动 token 而 env 已设固定值（或反之） | 统一走 §2.2 固定 token |
| dev 打 `/health` 404 | dev 无此挂载（§4.3） | dev 探活打 `/` |
| SSE 命令卡死 | macOS 无 `timeout` 命令 | curl 一律加 `--max-time N`（patterns.md 模式 1） |
| 端口被占 | 上一 run 未 teardown | 先 `pkill -f "huan-e2e/huan"` 再起 |
| dev /admin/api/plugins 空返回 | dev 的 plugins 目录无 .so 或未声明 | 与 daemon 一样需先现场编译 .so（patterns §7）+ huan.yaml 声明（Task 4 后 dev 已有 manager） |

---

## 附录 A：token 的 stderr 捕获法（备用）

不设 `HUAN_ADMIN_TOKEN` 时，dev 与 daemon（Task 1 后）都会在 stderr 打印自动生成的 32-hex token。捕获：

```bash
token=$(grep -oE '[0-9a-f]{32}' $artifacts/dev.log | head -1)
[ -n "$token" ] || { echo "token capture failed"; exit 1; }
```

风险：stderr 里若先出现其他 32-hex 串（sha256 摘要、buildid 等）会被误捕。主流程用 §2.2 固定 token；本附录仅在需要测试「自动生成 token」这一行为本身（auth-005 类用例）时使用。
