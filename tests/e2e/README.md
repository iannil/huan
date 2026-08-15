# huan E2E 测试用例体系

面向 agent 驱动执行的端到端验收体系：结构化 YAML 用例 + 可复制命令的执行手册，**不写测试代码、不写 runner**——执行者是 agent（按 runbook 起环境、按 schema 读用例、逐条跑命令、按判定规则出报告）。

## 目录结构

```
tests/e2e/
├── README.md                 # 本文件（体系入口）
├── fixtures/                 # 共享 fixture 站点（只读数据层）
│   ├── FIXTURES.md           #   已知状态表——所有断言常量的唯一出处
│   ├── minimal/              #   单语最小站（多数套件用它）
│   ├── multilang/            #   zh-cn + en 双语站
│   └── with-plugins/         #   插件站（.so 需现场编译，见 patterns.md §7）
├── runbooks/
│   ├── RUNBOOK.md            # 执行手册：变量契约/端口表/启动/探活/判定/报告
│   └── patterns.md           # 断言模式库（SSE/WS/并发/mtime/审计/upload/插件编译）
├── api/                      # API 套件（严格结构化 schema）
│   └── _schema.md            #   先读这个，再读 YAML
├── browser/                  # 浏览器旅程套件（意图 + 里程碑 schema）
│   └── _schema.md
└── cli/                      # CLI 套件（build-only runtime）
    ├── _schema.md
    └── _external-deps.md     #   排除清单（translate/deploy 等需外部依赖）
```

## 三类用例的读法

1. **api/**：每个 step 是一个可复现动作（`http` / `fs.assert` / `cli` / `sse.subscribe` / `wait` …）。先读 `api/_schema.md` 的字段字典，再读 YAML——`${token}`/`${port}`/`${base}` 等变量按 RUNBOOK 展开。
2. **browser/**：旅程式（`goto` / `enter-token` / `interact` / `verify` …）。verify 断言用**用户可见内容**表述，DOM 选择器只进 hints。先读 `browser/_schema.md`。
3. **cli/**：CLI 退出码 + stdout 断言（runtime 固定 build-only）。先读 `cli/_schema.md`（差异文档，动作复用 api schema）。

## 执行入口

按 [runbooks/RUNBOOK.md](runbooks/RUNBOOK.md) 逐步执行：环境准备（含 `HUAN_HOME` 隔离与固定 token）→ 按套件端口表启动 dev/daemon → 逐用例执行（断言命令查 [runbooks/patterns.md](runbooks/patterns.md)）→ 按 severity 判定（P0 停 / P1 记 / P2 观察）→ 报告落 `docs/reports/e2e/`。

## 与 Go 单测的边界

- `go test ./...` 管**单元/集成**（进程内、函数级契约）；
- 本体系管 **E2E 验收**（真实进程 + HTTP/浏览器/文件系统 + 跨层行为），两者互不替代：单测红了先修单测；E2E 红了按报告里的证据定位。
- 断言常量以 [fixtures/FIXTURES.md](fixtures/FIXTURES.md) 为准（含实测发现的已知状态偏差——写断言前必读，绕开或显式断言，不盲写「理想值」）。
