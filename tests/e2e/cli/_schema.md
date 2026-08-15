# CLI 套件 Schema 参考文档（差异文档）

本文档**只写 CLI 套件与 API 套件的差异**。字段字典、断言语义、硬规则、severity、变量替换全部复用 [api/_schema.md](../api/_schema.md)——执行 agent 写/读 CLI 用例前**先读 api/_schema.md 再读本文**。

**配套文档**：RUNBOOK.md（变量契约/判定/报告）、patterns.md（§4 mtime、§9 偏差口径）、FIXTURES.md（build 产物锚点）、[_external-deps.md](_external-deps.md)（未覆盖命令清单）。

---

## 差异 1：runtime 固定 `build-only`

CLI 套件 `runtime` 恒为 `build-only`：每个用例直接执行 `$bin <cmd> -s $site ...`，**无 HTTP 服务、无进程探活**。

- **套件端口表不适用**（RUNBOOK §3 无 CLI 段）——`${port}`/`${base}`/`${token}` 变量在本套件 YAML 中不出现。
- 启动/停止（RUNBOOK §4）整体跳过；每 run 仍按 §2 准备（`${bin}` 现场构建、fixture 拷贝、`HUAN_HOME=$site/.huan-home` 隔离——CLI build 同样会扫 `$HUAN_HOME/plugins`，隔离不可省）。

## 差异 2：动作动词收窄为 4 个

复用 api/_schema.md 的动词定义，CLI 套件**只用**：`cli`（§5）、`fs.assert`（§2）、`fs.write`（§3）、`fs.delete`（§4）。`http`/`sse.subscribe`/`wait`/`var` 不使用（无服务、无异步）。

## 差异 3：cli expect 的硬规则（适配硬规则 1）

api 硬规则 1 针对 http step；CLI 套件的对应规则——每个 `cli` step 的 `expect` 必须：

1. 显式断言 `exit_code`（必填）
2. 显式断言 `stdout_contains`/`stdout_matches`/`stderr_contains` **至少一项**

（api/_schema.md §5 字段名为 `exit_code`/`stdout_contains`/`stdout_matches`/`stderr_contains`，本文沿用，不再列出。）

## 差异 4：硬规则 2 的适配

「写操作后必跟 fs.assert」在 CLI 套件的口径：**凡产生落盘副作用的 `cli` step（`huan new`、`huan build`）之后，必须跟至少一个 `fs.assert`** 验证产物。纯读命令（`version`/`config`）无此要求。

## 差异 5：YAML 注释承载执行细节

CLI 变体繁多（`-D/-F/-E/--minify/-b/-d`），不便逐个建 fixture 的场景（future/expired 内容、缺 huan.yaml、无 archetype）用 **startup 的 `fs.write`/`fs.delete` 注入**或独立临时目录，并在用例注释里写清执行指引——不发明新字段。

## 套件与用例 ID

| 文件 | suite | 用例前缀 | 数量 |
|---|---|---|---|
| build.yaml | build | `cb-` | 8 |
| new.yaml | new | `cn-` | 3 |
| config.yaml | config | `cc-` | 3 |
| version.yaml | version | `cv-` | 1 |
