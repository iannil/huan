# ADR 0010：v1.0 Scope and Positioning Split

- **状态**：Accepted
- **日期**：2026-06-30
- **决策者**：用户（owner）+ Claude（grill-me 二轮收敛）
- **依赖**：[ADR 0001](0001-redefine-equivalence.md)（三维度等价）/ 全项目审计 [`memory/daily/2026-06-28.md`](../../memory/daily/2026-06-28.md) / grill-me 二轮 [`memory/daily/2026-06-30.md`](../../memory/daily/2026-06-30.md)

## 背景

2026-06-26 [positioning-redefine 报告](../reports/completed/2026-06-26-positioning-redefine.md) 把 huan 定位从 "zhurongshuo 专用 Hugo 替代" 升级为 "**一体化内容引擎，替代所有 CMS**"。该决策在 `CLAUDE.md` / `docs/INDEX.md` / `README.md` / `README.zh-CN.md` / `docs/technical-plan.md` §1 / `memory/MEMORY.md` 中一致宣称。

2026-06-28 全项目审计（[`memory/daily/2026-06-28.md`](../../memory/daily/2026-06-28.md)）发现该定位与现实存在显著差距：

1. **缺图片管线**（image processing / responsive srcset / resize）—— WordPress/Ghost/Strapi 的核心能力
2. **缺资源管道**（asset pipeline / SCSS / bundle / hash）—— Hugo 的核心卖点
3. **Admin 无认证**——`huan serve --bind 0.0.0.0` 是文档 advertise 的合法用法，bind 后整个 LAN 能通过 `POST /admin/api/content/{path}` 任意写 content/ 下文件
4. **无多用户、无角色权限、无工作流**
5. **14 个 Hugo 模板函数静默 no-op**——registry 在撒谎，未来模板作者查文档会误以为可用

2026-06-30 grill-me 二轮（7 题全选 A，详见 [`memory/daily/2026-06-30.md`](../../memory/daily/2026-06-30.md)）收敛：保留"local-first single-user"为 v0.x 实际范围，"path toward all-CMS replacement"列为 v1.x+ roadmap；为 v1.0 立 6 条 hard gate。

## 决策

### 1. 定位拆段

**v0.x（当前实际范围）**：*"local-first single-user content engine with built-in admin"*（本地优先、单用户、内置 admin 的内容引擎）

**v1.x+（roadmap）**：*"path toward all-CMS replacement"*（通往替代所有 CMS 的路径）

**含义**：
- 图片管线、资源管道、多用户、角色权限、工作流 → **v1.x+ roadmap，非 v1.0 范围**
- admin 无认证 → **v0.x 仍需补安全边界（gate 4），因为 "local-first" 不等于 "无边界"**（CSRF + 误 bind 是真实威胁）
- 14 个 no-op 模板函数 → **v1.0 必修（诚实度问题，gate 2）**

### 2. v1.0 6 Hard Gate

| # | 标准 | 验证 |
|---|------|------|
| **1** | **文档与实际定位一致**——INDEX / MEMORY / README / README.zh-CN / technical-plan / CURRENT_STATE 全部移除"替代所有 CMS"过度宣称，统一为 "local-first single-user content engine with built-in admin" | grep "替代所有 CMS\|all-in-one.*replaces.*CMS" 在 docs/ 下应只在 ADR 0010 + 本历史报告/ADR 引用中出现 |
| **2** | **无静默错误**——所有注册的模板函数要么真实现，要么注册时 `panic("huan: template func X not implemented; if needed open issue")`；守护测试 `TestNoSilentNoOpFuncs` 扫描 FuncMap 所有函数至少 1 个调用点测试覆盖 | `go test ./internal/template/...` PASS |
| **3** | **I/O 包必须有测试**——`internal/admin/` + `internal/output/` + `internal/i18n/i18n.go` 至少有 happy path 测试 | `go test ./internal/admin/... ./internal/output/... ./internal/i18n/...` PASS + `go test -race ./...` PASS |
| **4** | **Admin 安全边界明确**——`huan serve --bind 0.0.0.0` 时 admin 路由必须 fail-fast（拒绝启动）或要求 `HUAN_ADMIN_TOKEN` env | 详见 [ADR 0011](0011-admin-security-boundary.md) |
| **5** | **BuildSite 可读性**——`BuildSite()` 拆成显式 stage 函数（Load → Render → PostProcess → Write），每段 < 80 行 | `wc -l internal/build/build.go` ≤ 80（orchestrator 主体）；新增 `pipeline_*.go` 各文件每函数 < 80 行 |
| **6** | **zhurongshuo 生产稳定 90 天 + 自己满意** | zhurongshuo 自 2026-06-13 上线 huan 起 90 天（**目标日期：2026-09-11**）CI/CD 无回归 + 无生产 incident + owner 主观满意 |

**Gate 6 语义说明**：local-first single-user 定位下，硬性要求"找第二用户证明"与定位矛盾——单用户产品用"自己满意"作为最终门槛是自洽的。

### 3. 执行顺序（task #1-9）

```
1. ADR 0010+0011 + 文档定位同步（0.5 天）
2. Gate 4 admin 安全边界（1-1.5 天）
3. Gate 3 admin 测试（0.5 天，依赖 2）
4. Gate 2 no-op funcs 修复（5-7h，可与 3 并行）
5. Gate 3 output + i18n 测试（1 天，可与 2-4 并行）
6. Gate 5 BuildSite 重构（1-2 天，依赖 5）
7. v0.4.0 发版（0.5 天，依赖 1-6）
8. Gate 6 等待期（自动，至 2026-09-11）
9. v1.0.0 发版（0.5 天）
```

总计：~5-6 工作日 + 90 天等待 = **v1.0 在 2026-09-11 后发版**。

## 备选方案（已否决）

| 分叉点 | 备选 | 否决理由 |
|---|---|---|
| 定位拆段 | 保持"替代所有 CMS"为 v1.0 必达 | 图片管线 + 认证 + 多用户必须进 critical path，工程量翻倍；zhurongshuo 唯一用户不需要 |
| 定位拆段 | 砍掉 admin，回归纯 SSG | zhurongshuo 不需要 admin 也能跑，但 admin 已实施（1162 LOC），砍掉是 sunk cost 浪费；保留为 v0.x 实际范围 |
| v1.0 门槛 | 更严（加图片管线 / 2 外部用户） | 与 local-first single-user 定位矛盾 |
| v1.0 门槛 | 更松（只保留 gate 1+2+4） | gate 3 测试是 gate 5 重构前提，跳过 3 无法做 5；gate 5 是 LLM Friendly mandate |
| v1.0 门槛 | 不要 hard gate，主观判断 | 版本号注水风险；无法对外说"v1.0 = 什么标准" |
| Gate 6 | 找第二用户而非"自己满意" | local-first single-user 定位下硬性"外部证明"自相矛盾 |

## 影响

### 文档

- 新增本 ADR（0010）
- 新增 [ADR 0011](0011-admin-security-boundary.md)（admin 安全边界）
- 更新 `memory/MEMORY.md`：项目上下文段落修订 + 4 条新关键决策
- 更新 `memory/daily/2026-06-30.md`：本次 grill-me 全部决策的不可变日志（已写）
- 更新 `docs/INDEX.md`：一句话定位 + 加 v1.0 criteria 链接
- 更新 `README.md` + `README.zh-CN.md`：tagline 同步
- 更新 `docs/technical-plan.md` §1：定位段重写
- 更新 `docs/progress/CURRENT_STATE.md`：加 v1.0 tracking 段落

### 代码（v0.4.0 范围）

- `internal/template/funcs.go`：7 个 implement + 4 个 panic-on-call + 守护测试（gate 2）
- `internal/admin/{handler,auth,audit}.go`：L1+L2+L4 安全边界（gate 4）
- `internal/build/{build,pipeline_*}.go`：6 文件纯抽取重构（gate 5）
- `internal/{admin,output,i18n}/*_test.go`：补齐测试（gate 3）

### 风险

1. **v1.0 延期**：90 天稳定性 + 5-6 工作日实施，若期间 zhurongshuo 出现 incident 或 gate 4-5 实施受阻，v1.0 推迟。缓解：90 天是等待期不阻塞实施，可并行
2. **图片管线延后引发用户预期落差**：定位撤回"all-CMS"后若有用户期待图片管线，会失望。缓解：README Roadmap 明示"v1.x+ target"，不为 v1.0 承诺
3. **no-op panic 破坏现有模板**：panic-on-call 比静默更安全，但若 zhurongshuo 模板未来用到这些 funcs，会运行时挂。缓解：zhurongshuo 当前零调用；守护测试 + diff-build.sh 双重保险
4. **BuildSite 重构期 regression**：640 行拆 6 文件过程中可能引入 byte 偏移。缓解：每 stage 一个 commit + diff-build.sh 4 模式 0 regression gate

## 验证

### v1.0 发版前清单

- [ ] gate 1：`grep -r "替代所有 CMS" docs/ MEMORY.md` 应只在历史报告 + 本 ADR 引用中出现
- [ ] gate 2：`go test ./internal/template/... -run TestNoSilentNoOpFuncs` PASS
- [ ] gate 3：`go test -race ./internal/admin/... ./internal/output/... ./internal/i18n/...` PASS
- [ ] gate 4：见 [ADR 0011](0011-admin-security-boundary.md) 验证段
- [ ] gate 5：`wc -l internal/build/build.go` ≤ 80；`go test ./...` PASS；`./scripts/diff-build.sh` 4 模式 0 regression
- [ ] gate 6：zhurongshuo 自 2026-06-13 起 90 天无 incident；`gh run list --repo iannil/zhurongshuo --status success` 90 天连续绿
- [ ] owner 主观签字

## 不在范围（YAGNI）

- 图片管线（resize / srcset / format conversion）—— v1.x+
- 资源管道（SCSS / bundle / hash）—— v1.x+
- 多用户 / 角色权限 —— v1.x+
- Admin 多用户协作（git-based PR flow 除外）—— v1.x+
- 工作流 / 审核 / 定时发布 —— v1.x+
- 自定义 permalink 模式 —— 待用户实际需求
- `.Site.Data` CSV 支持 —— 待用户实际需求
- Archetypes / content hooks —— 待用户实际需求
