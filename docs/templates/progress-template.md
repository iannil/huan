# {{标题}}

> 状态：进行中  ·  起始：YYYY-MM-DD  ·  负责人：（可选）

## 背景

（这一段说清楚为什么要做这件事，对应哪个剩余差异 / 用户需求 / stage 2 目标。链接到 `docs/technical-plan.md` 对应小节或 `docs/progress/CURRENT_STATE.md` 待办编号。）

## 目标

- 显式、可验收的目标，避免"改进"、"优化"这种含糊措辞
- 例：把 RSS items 顺序与 Hugo 完全对齐，`./scripts/diff-build.sh` 中 RSS 类差异归零

## 范围

**做**：
- …

**不做**（本次明确排除）：
- …

## 实施步骤

按可独立提交的粒度拆分；每步对应一个 commit。

- [ ] Step 1：…
- [ ] Step 2：…
- [ ] Step 3：…

## 风险与回滚

- 风险 1：…  ·  缓解：…
- 回滚策略：…（如"保留备份到 `/backup/<date>/`，diff 异常上升时回滚"）

## 验收

- [ ] `go build -o huan ./cmd/huan` 通过
- [ ] `go test ./...` 全通过
- [ ] `./scripts/diff-build.sh` 无新差异（或差异符合预期）
- [ ] 手动验收点（按需补充）

## 进展日志

（按时间倒序，每次会话追加。格式：YYYY-MM-DD HH:MM — 动作 + 结果。）

- 2026-MM-DD HH:MM — …
