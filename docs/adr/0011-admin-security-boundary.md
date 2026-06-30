# ADR 0011：Admin Security Boundary

- **状态**：Accepted
- **日期**：2026-06-30
- **决策者**：用户（owner）+ Claude（grill-me 二轮收敛）
- **依赖**：[ADR 0010](0010-v1-0-scope-and-positioning-split.md)（v1.0 scope，本 ADR 实现 gate 4）/ Stage 4 Admin Panel（[`docs/reports/completed/2026-06-26-admin-content-redesign.md`](../reports/completed/2026-06-26-admin-content-redesign.md)）

## 背景

`internal/admin/handler.go` 当前**完全无 auth**：

1. `huan serve` 默认 bind `127.0.0.1:1313`——当前唯一保护
2. `huan serve --bind 0.0.0.0` 是 [`docs/INDEX.md`](../INDEX.md) 第 64 行 advertise 的合法用法（`./huan serve -s . --port 8080 --bind 0.0.0.0 -D`）——一旦 bind，整个 LAN 都能调用 admin API
3. Admin API（`POST /admin/api/content/{path}`）直接写 content/ 目录任意 .md 文件——受限于 content/ 但可写 `.env`、可写 `..`-style 相对路径
4. 即使 bind 127.0.0.1，浏览器 CSRF 也能从任意网站 POST 到 127.0.0.1:1313——没有 token、没有 CORS 限制、没有 SameSite cookie
5. CLAUDE.md 写"全链路可观测性"，但 admin API 写操作无 audit log——文件被改无痕迹

2026-06-26 Stage 4 Admin Panel 决策记录："认证初期无（localhost only）"——这是当时的 YAGNI 决策。2026-06-28 审计 + 2026-06-30 grill-me 收敛：local-first **不等于**无边界，必须显式定义安全边界（CSRF + 误 bind 是 local-first 真实威胁）。

## 决策

三层防御 + 审计日志：

### L1：Bind 检查（fail-fast）

`huan serve --bind` 非 loopback（非 `127.0.0.1` / `::1` / `localhost`）时，admin 路由**拒绝启动**，除非显式设置 `HUAN_ADMIN_TOKEN` env。

```go
// internal/admin/auth.go (NEW)
func CheckBindSafety(bindAddr, token string) error {
    if isLoopback(bindAddr) {
        return nil  // localhost 安全，无 token 也允许
    }
    if token == "" {
        return fmt.Errorf(
            "huan: admin panel requires HUAN_ADMIN_TOKEN env when binding to non-loopback address (%s);\n"+
            "set HUAN_ADMIN_TOKEN to a random string (e.g., `openssl rand -hex 32`), or bind to 127.0.0.1",
            bindAddr)
    }
    return nil
}
```

`internal/serve/server.go` 在启动 admin handler 前调用 `CheckBindSafety`。

### L2：Token 认证（缺失自动生成）

`internal/admin/handler.go` 加中间件：

- 若 bind loopback 且 `HUAN_ADMIN_TOKEN` 未设置：**自动生成一次性 token**（`crypto/rand` 16 字节 hex），打印到 stderr 一次：
  ```
  huan: admin panel token (save this; will not be shown again):
      3f4a1b2c...
  ```
- 若 bind 非 loopback：必须 `HUAN_ADMIN_TOKEN` 已设（L1 保证），中间件验证
- Token 验证逻辑：
  - Header `Authorization: Bearer <token>` 或 `X-Huan-Admin-Token: <token>` 都接受
  - 验证用 `subtle.ConstantTimeCompare`（防时序攻击）
  - 失败返回 401 + `WWW-Authenticate: Bearer realm="huan-admin"`

前端 SPA 配合：
- 首次访问 `/admin` 弹窗输入 token，存 `sessionStorage`（不存 localStorage，关 tab 即失效）
- 所有 fetch 请求自动加 `Authorization: Bearer <token>` header
- 401 响应清 sessionStorage + 重弹输入

### L4：Audit Log（写操作）

admin API 写操作（POST/PUT/DELETE）成功后，append 一行到 `memory/daily/{YYYY-MM-DD}.md`：

```markdown
### admin audit ({HH:MM:SS})

- **action**: content.create | content.update | content.delete | settings.update
- **path**: posts/2026/foo.md
- **sha256**: <before-sha256> → <after-sha256>  (delete 时 only before)
- **trace_id**: <generated UUID>
```

**对接 CLAUDE.md 双层记忆系统**——audit log 自动进入 daily 流层，受 git 跟踪，事后可追溯。

### 不实施 L3（CSRF SameSite cookie）

否决理由：localhost single-user 场景下 CSRF 收益小；token header 已阻挡 CSRF（攻击者无法读 sessionStorage）。L3 推到 v1.x（多用户场景需要时再加）。

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| L1 模式 | warn-and-continue（打印警告但启动） | 用户容易忽略警告；fail-fast 是 forcing function 让用户必须显式配 token 才能暴露 |
| L2 token 缺失策略 | 拒绝启动（强制要求 env） | loopback 单用户场景体验差；自动生成 + stderr 打印兼顾安全 + 易用 |
| L2 token 来源 | 用户必须自己生成并 export | 同上，体验差；自动生成是 sensible default |
| L2 token 验证 | `==` 比较 | 时序攻击风险；`subtle.ConstantTimeCompare` 是标准做法 |
| L2 前端存储 | localStorage | 跨 tab 持久 + XSS 持久窃取风险；sessionStorage 关 tab 即失效更安全 |
| 防护层级 | L1 + L2 + L3 + L4 全做 | L3 CSRF cookie 在 localhost 收益小，~0.5 天工程量不划算；推 v1.x |
| 审计落盘 | 写独立 audit.log 文件 | 与 CLAUDE.md 双层记忆系统重复；daily 流层已存在 + git 跟踪 |

## 影响

### 文档

- 新增本 ADR（0011）
- 更新 `docs/INDEX.md`：admin 安全使用说明（loopback only 或配 HUAN_ADMIN_TOKEN）
- 更新 `README.md` / `README.zh-CN.md`：admin 段加安全说明
- 更新 `docs/technical-plan.md` §4（如有 admin 段）：加安全边界

### 代码

**新增**：
- `internal/admin/auth.go`：`CheckBindSafety` + `TokenMiddleware` + token 自动生成
- `internal/admin/audit.go`：`LogAudit(action, path, beforeSHA, afterSHA)` 写入 daily note

**改造**：
- `internal/admin/handler.go`：注册中间件 + 调用 audit
- `internal/admin/{content,settings,media}.go`：每个写操作调 `LogAudit`
- `internal/serve/server.go`：启动前调 `CheckBindSafety`
- `web/admin/src/`：SPA 加 token 输入弹窗 + Authorization header

### 测试（gate 3 admin 测试覆盖）

- `TestCheckBindSafety_Loopback_NoToken_Allows`
- `TestCheckBindSafety_NonLoopback_NoToken_FailsFast`
- `TestCheckBindSafety_NonLoopback_WithToken_Allows`
- `TestTokenMiddleware_ValidBearer_Allows`
- `TestTokenMiddleware_MissingToken_Returns401`
- `TestTokenMiddleware_WrongToken_Returns401`（`subtle.ConstantTimeCompare`）
- `TestTokenMiddleware_NoLengthLeak`（时序测试，多次失败响应时间方差 < 阈值）
- `TestAuditLog_Write_AppendsToDaily`（temp dir 验证 daily note 增量）
- `TestAuditLog_Delete_RecordsOnlyBeforeSHA`

### 风险

1. **Token 打印到 stderr 被滥用**：若 stderr 重定向到日志聚合系统，token 会泄露。缓解：打印时用醒目 separator + 提示"do not log this output"；文档建议用 `HUAN_ADMIN_TOKEN` env 显式设置而非依赖自动生成
2. **Audit log 文件并发写**：多个 admin 请求并发写 daily note 可能 race。缓解：写入用 `os.OpenFile(..., O_APPEND|O_WRONLY)` + 文件锁；CI race gate 验证
3. **Token 暴露在前端 sessionStorage**：XSS 攻击可读取。缓解：SPA 不引第三方 JS（已 self-host，[ADR 0009](0009-self-contained-downstream-deploys.md)）；CSP header 限制（v1.x）
4. **Fail-fast 破坏现有用户工作流**：若有人已经习惯 `--bind 0.0.0.0` 无 token，升级后会启动失败。缓解：错误信息明确指引（设置 token 或 bind loopback）；CHANGELOG 显式 breaking change 标注

## 验证

### L1 验证

```bash
# case 1: loopback 无 token，应正常启动
unset HUAN_ADMIN_TOKEN
./huan serve -s . --bind 127.0.0.1 --port 1313 &
# 期望：正常启动

# case 2: 非 loopback 无 token，应 fail-fast
unset HUAN_ADMIN_TOKEN
./huan serve -s . --bind 0.0.0.0 --port 1313
# 期望：stderr 输出错误 + 拒绝启动 + exit 1

# case 3: 非 loopback + token，应正常启动
export HUAN_ADMIN_TOKEN=$(openssl rand -hex 32)
./huan serve -s . --bind 0.0.0.0 --port 1313 &
# 期望：正常启动
```

### L2 验证

```bash
# 自动生成 token（loopback 无 env）
unset HUAN_ADMIN_TOKEN
./huan serve -s . --port 1313 2>&1 | grep "admin panel token"
# 期望：打印一次性 token 到 stderr

# token 验证
curl -X POST http://127.0.0.1:1313/admin/api/content/test.md \
  -H "Authorization: Bearer <token>" \
  -d '...'
# 期望：200 OK

curl -X POST http://127.0.0.1:1313/admin/api/content/test.md
# 期望：401 Unauthorized
```

### L4 验证

```bash
# 写一篇文章
curl -X POST .../admin/api/content/posts/test.md -H "Authorization: ..." -d '...'

# 检查 daily note
tail -10 memory/daily/$(date +%Y-%m-%d).md
# 期望：含 admin audit 段，记录 action=path=sha256
```

### CI Gate

`go test -race ./internal/admin/...` 必 PASS。

## 不在范围（YAGNI）

- L3 CSRF SameSite cookie—— v1.x 多用户场景
- OAuth / OIDC / SSO —— v1.x+
- IP allowlist（除 loopback 判断）—— 单用户场景无需求
- Token rotation / refresh token —— 单用户场景 token 长期有效即可
- Admin 操作回滚（undo）—— git revert 已覆盖
- 多用户权限矩阵（RBAC）—— v1.x+
