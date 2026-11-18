# 修复 huan.example.yaml 的 zhurongshuo 插件配置警告

- 时间：2026-11-19 00:0x
- 类型：小修复（示例配置 + 回归测试）
- 状态：已完成

## 背景

对 `huan.example.yaml` 运行任意 `huan` 命令（如 `huan plugin list`）时始终输出：

```
huan: plugin config warning: plugin "zhurongshuo": declared in yaml but no .so found in $HUAN_HOME or plugins/
```

## 根因

`zhurongshuo` 是祝融说站点专属的主题插件，其 `.so` 随 zhurongshuo 站点仓库分发，不属于 huan 自带的 7 个插件。2026-07-29 提交 19ba967 为文档化全部配置字段将其原样声明进示例 yaml，导致任何对示例文件运行 huan 的场景都必然触发 `internal/plugin/validate.go:122` 的配置警告。警告语义本身正确（真实站点需要它发现漏装插件），问题仅在示例文件声明了不可解析的插件。

## 修改内容

1. `huan.example.yaml`：将 `zhurongshuo` 插件条目（原 188–202 行）改为注释块，保留 `publicationRevisionNotice` 配置示例的文档价值，并注明归属站点仓库与启用方式。
2. `cmd/huan/example_yaml_plugins_test.go`（新增）：回归测试 `TestExampleYAMLDeclaresOnlyBundledPlugins`，固化约束「示例 yaml 声明的每个插件都必须是 huan 自带插件」。测试动态扫描 `plugins/*/` 的 `Name()`（与 `scripts/build-plugins.sh` 同规则），为自带插件放置 dummy `.so`，隔离 `$HUAN_HOME` 后构建注册表，断言零配置警告。

## 验证

- TDD 红→绿：修复前测试精确因 `plugin "zhurongshuo"` 警告失败；修复后通过。
- 端到端：`huan plugin list`（示例 yaml 拷贝为 huan.yaml 的临时目录）零警告，exit 0。
- 回归：`go test ./...` 全量 1208 个测试、39 个包全部通过。

## 影响面

仅示例文件与新增测试，无运行时行为变化；`zhurongshuo` 站点仓库自身的 huan.yaml 不受影响。
