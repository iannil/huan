# Task 4: 更新所有 .so 插件使用 pkg/plugin

**时间:** 2026-07-27

## 完成情况

| 插件 | 状态 | 备注 |
|------|------|------|
| plugins/cloudflare | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，修改 `deploy/types.go` 中的 import |
| plugins/html-injector | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，修改 `plugin.go` 中的 import |
| plugins/image-pipeline | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，移除 `plugin_main.go` 中多余的 import |
| plugins/qwen3 | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，修改 `translate/types.go` 中的 import，移除 `plugin_main.go` 中多余的 import |
| plugins/seo-injector | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，修改 `plugin.go` 中的 import |
| plugins/sitemap-enhancer | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，修改 `plugin.go` 中的 import |
| plugins/zhurongshuo | OK | go.mod 增加 `require` + `replace`，删除 `plugin/plugin.go`，修改 `plugin.go` 中的 import |

## 变更摘要

### 所有插件均执行：
1. **go.mod**: 添加 `require github.com/iannil/huan v0.0.0` 和 `replace github.com/iannil/huan => ../../`
2. **删除 `plugin/plugin.go`**: 删除所有自包含的类型拷贝目录
3. **更新 import**: 将 `github.com/iannil/huan-plugin-XXX/plugin` 替换为 `github.com/iannil/huan/pkg/plugin`
4. **构建验证**: `go build -buildmode=plugin -o /dev/null .` 全部通过

### 额外处理：
- **cloudflare**: `deploy/types.go` 有单独的 import，也一并更新
- **image-pipeline**: `plugin_main.go` 不再需要 import，移除
- **qwen3**: `translate/types.go` 有单独的 import，也一并更新；`plugin_main.go` 不再需要 import，移除
- **cloudflare**: `plugin_main.go` 不再需要 import，移除

### 验证结果：
- 全部 7 个 `.so` 插件构建成功
- 主项目 `go build ./...` 正常
- 全部测试通过
