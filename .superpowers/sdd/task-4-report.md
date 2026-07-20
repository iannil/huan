# Task 4 Report: CLI 重命名（serve → dev）+ daemon 命令骨架

## Status: DONE

## Commits
- (待提交，见下方 commit 内容)

## 变更摘要

### 文件变更
- **创建**: `cmd/huan/daemon.go` — daemon 命令骨架（stub），via `init()` 注册到 `rootCmd`
- **创建**: `cmd/huan/dev.go` — 从旧 `serve.go` 复制完整逻辑，import 改为 `internal/dev`，命令名 "dev"
- **修改**: `cmd/huan/main.go` — `rootCmd` 提升为包级变量并在 `init()` 中初始化，解决 init order 依赖；`serveCmd` 改为 deprecated 别名（`Hidden: true` + `Deprecated: "use 'huan dev' instead"`），回退为 `runDev` 代理
- **修改**: `cmd/huan/serve.go` — 重写为 deprecated stub，`runServe` 打印警告后调用 `runDev`
- **重命名**: `internal/serve/` → `internal/dev/` — 包内所有文件 `package serve` → `package dev`
- **删除**: `internal/serve/` 目录

### 关键设计决策
1. `rootCmd` 定义为包级变量（`var rootCmd = &cobra.Command{...}`），在 `init()` 中设置 flags，确保 `dev.go`/`daemon.go` 的 `init()` 中 `rootCmd.AddCommand()` 时 `rootCmd` 已初始化
2. `devCmd` 和 `daemonCmd` 各在自身文件的 `init()` 中注册到 `rootCmd`，`main.go` 不再显式 `AddCommand` dev/daemon
3. `serveCmd` 保留在 `main.go` 中，标记为 deprecated，转发给 `runDev`

### 编译验证
- `go build -o huan ./cmd/huan` — 编译成功无错误
- `./huan --help` — 正确显示 `dev` 和 `daemon` 命令
- `./huan serve --help` — 正确显示 deprecated 提示
- `./huan daemon --help` — 正确显示 daemon 子命令 flags

## 测试结果
- `go test ./internal/dev/... -v -count=1` — 16/16 PASS
- `go test ./...` — 全部包测试通过（无 `internal/serve` 残留引用）

## 自检要点
- [x] `internal/serve/` 已删除，`internal/dev/` 包全部替换为 `package dev`
- [x] 代码库中无残留引用 `github.com/iannil/huan/internal/serve`
- [x] `serve` 命令在 help 中隐藏（`Hidden: true`），但可通过 `./huan serve` 执行
- [x] `dev` 命令 flags 与原 `serve` 完全一致
- [x] `daemon` 命令骨架已建立，含 TLS/systemd 等 flags 预留

## 关注点
- `rootCmd` 必须为包级变量才能被 `dev.go`/`daemon.go` 的 `init()` 引用，这改变了原有 `main.go` 中 `rootCmd` 的局部变量结构
- `serve` 命令的 flags 不再独立定义（原 `serveCmd.Flags().String(...)` 已移除），改用 `serveCmd` 的 deprecated 实现直接转发到 `runDev`，`serve` 子命令不会再显示 flags 帮助（只有 `-h` 和全局 flags）
